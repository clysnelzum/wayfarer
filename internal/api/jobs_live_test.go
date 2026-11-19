package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin"
	"github.com/clysnelzum/wayfarer/internal/linkedin/scripts"
)

// TestLiveJobsHTTP uses the production handler and reader on a loopback HTTP
// socket. It does not depend on a fixed personalized result or change listings.
func TestLiveJobsHTTP(t *testing.T) {
	if os.Getenv("WAYFARER_LIVE_JOBS") != "1" {
		t.Skip("set WAYFARER_LIVE_JOBS=1 and WAYFARER_CHROME_WS to run the live HTTP check")
	}
	browser, err := chrome.New(os.Getenv("WAYFARER_CHROME_WS"), chrome.WithAuthGuard(scripts.DefaultRegistry().AuthGuard()))
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	svc, err := linkedin.NewClient(browser)
	if err != nil {
		t.Fatal(err)
	}
	const token = "wayfarer-http-smoke-test-loopback-only"
	server := httptest.NewServer(New(token, svc))
	defer server.Close()
	client := server.Client()
	client.Timeout = 29 * time.Second

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
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, body
	}

	if code, _ := get("/v1/jobs?q=DevOps", false); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated search returned HTTP %d", code)
	}
	for _, tc := range []struct {
		path  string
		limit int
	}{
		{"/v1/jobs", 0},
		{"/v1/jobs?limit=30", 30},
	} {
		code, body := get(tc.path, true)
		if code != http.StatusOK {
			t.Fatalf("%s returned HTTP %d: %s", tc.path, code, body)
		}
		var value linkedin.JobResults
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		if value.Items == nil || value.Pagination != nil || value.Search != nil ||
			(tc.limit != 0 && len(value.Items) > tc.limit) || (value.RecommendationStatus != "") != (tc.limit != 0) {
			t.Fatalf("%s returned invalid recommendation metadata", tc.path)
		}
		t.Logf("%s returned %d recommendations in %d sections (%s)", tc.path, len(value.Items), len(value.Sections), value.RecommendationStatus)
	}
	for _, tc := range []struct {
		path string
		page int
	}{
		{"/v1/jobs?q=DevOps&under_10_applicants=true", 1},
		{"/v1/jobs?q=DevOps&page=2", 2},
	} {
		code, body := get(tc.path, true)
		if code != http.StatusOK {
			t.Fatalf("%s returned HTTP %d: %s", tc.path, code, body)
		}
		var value linkedin.JobResults
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		p := value.Pagination
		if p == nil || p.Page != tc.page || value.Search == nil || value.Items == nil || len(value.Items) > 25 {
			t.Fatalf("%s returned invalid search metadata", tc.path)
		}
		if (p.HasMore && p.NextPage != tc.page+1) || (!p.HasMore && p.NextPage != 0) {
			t.Fatalf("%s returned inconsistent next page: %+v", tc.path, p)
		}
		if p.OutOfRange && (len(value.Items) != 0 || p.HasMore || value.Search.MatchMode != "no_results") {
			t.Fatalf("%s returned inconsistent out-of-range state", tc.path)
		}
		if len(value.Items) == 0 && value.Search.MatchMode != "no_results" {
			t.Fatalf("%s returned an unexplained empty page", tc.path)
		}
	}
}
