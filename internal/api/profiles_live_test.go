package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

// TestLiveProfilesAndCompaniesHTTP sequentially tests /v1/companies/{slug} and
// /v1/profiles/{slug} against a live Chrome session via the production HTTP handler.
func TestLiveProfilesAndCompaniesHTTP(t *testing.T) {
	if os.Getenv("WAYFARER_LIVE_PROFILES") != "1" && os.Getenv("WAYFARER_LIVE_JOBS") != "1" {
		t.Skip("set WAYFARER_LIVE_PROFILES=1 (or WAYFARER_LIVE_JOBS=1) and WAYFARER_CHROME_WS to run the live HTTP check")
	}
	browser, err := chrome.New(os.Getenv("WAYFARER_CHROME_WS"))
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()

	const token = "wayfarer-http-smoke-test-loopback-only"
	server := httptest.NewServer(New(token, linkedin.Client{Reader: browser}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 95 * time.Second

	get := func(path string, authorized bool) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if authorized {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatal("missing JSON or no-store response header")
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, body
	}

	pretty := func(body []byte) string {
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "  "); err != nil {
			return string(body)
		}
		return buf.String()
	}

	if code, _ := get("/v1/companies/microsoft", false); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated company request returned HTTP %d", code)
	}

	for _, path := range []string{
		"/v1/companies/microsoft",
		"/v1/companies/mercadolibre",
	} {
		code, body := get(path, true)
		t.Logf("%s -> HTTP %d\n%s", path, code, pretty(body))
		if code != http.StatusOK {
			t.Fatalf("%s returned HTTP %d: %s", path, code, body)
		}
		var co linkedin.Company
		if err := json.Unmarshal(body, &co); err != nil {
			t.Fatalf("%s invalid JSON: %v", path, err)
		}
		if co.Slug == "" || co.URL == "" || co.Name == "" {
			t.Fatalf("%s missing required company fields: %+v", path, co)
		}
	}

	for _, path := range []string{
		"/v1/profiles/satyanadella",
		"/v1/profiles/gergelyorosz",
	} {
		code, body := get(path, true)
		t.Logf("%s -> HTTP %d\n%s", path, code, pretty(body))
		if code != http.StatusOK {
			t.Fatalf("%s returned HTTP %d: %s", path, code, body)
		}
		var pr linkedin.Profile
		if err := json.Unmarshal(body, &pr); err != nil {
			t.Fatalf("%s invalid JSON: %v", path, err)
		}
		if pr.Slug == "" || pr.URL == "" || pr.Name == "" {
			t.Fatalf("%s missing required profile fields: %+v", path, pr)
		}
	}
}
