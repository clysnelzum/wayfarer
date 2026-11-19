package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

const demoSourceContext = `{"source":"preferences","origin":"PREFERENCES_LANDING","keywords":"Example private query","locations":"10,20","seed":"100,101"}`

func TestSourceRefRoundTripAndExpiry(t *testing.T) {
	now := time.Unix(123456, 0)
	refs, err := newSourceRefs(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	ref := refs.seal("preferences", demoSourceContext)
	value, ok := refs.open(ref, "preferences")
	if ref == "" || !ok || value != demoSourceContext {
		t.Fatal("could not resolve newly issued reference")
	}
	replacement := "A"
	if ref[0] == 'A' {
		replacement = "B"
	}
	if _, ok := refs.open(replacement+ref[1:], "preferences"); ok {
		t.Fatal("accepted a modified reference")
	}
	if _, ok := refs.open(ref, "profile_match"); ok {
		t.Fatal("accepted a wrong-source reference")
	}
	other, err := newSourceRefs(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := other.open(ref, "preferences"); ok {
		t.Fatal("accepted a foreign reference")
	}
	now = now.Add(sourceRefTTL)
	if _, ok := refs.open(ref, "preferences"); ok {
		t.Fatal("accepted an expired reference")
	}
}

type sourceRefStub struct {
	stub
	q linkedin.JobQuery
}

func (s *sourceRefStub) Jobs(_ context.Context, q linkedin.JobQuery) (linkedin.JobResults, error) {
	s.calls++
	s.q = q
	if q.Source == "" {
		return linkedin.JobResults{
			Items: []linkedin.JobSummary{{
				ID:       "100",
				Title:    "Example Job",
				Company:  "Example Co",
				Location: "Remote",
				URL:      "https://www.linkedin.com/jobs/view/100/",
			}},
			Sections: []linkedin.JobSection{
				{Title: "Example", JobIDs: []string{"100"}, Source: "preferences", SourceContext: demoSourceContext},
			},
		}, nil
	}
	return linkedin.JobResults{
		Items: []linkedin.JobSummary{},
		Search: &linkedin.JobSearchInfo{
			Source:        q.Source,
			SourceContext: q.SourceContext,
			MatchMode:     "results",
		},
	}, nil
}

func TestSourceRefHTTPPinsTheIssuedContext(t *testing.T) {
	now := time.Unix(123456, 0)
	s := &sourceRefStub{}
	handler, err := NewHandler("local", s, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer local")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	initial := get("/v1/jobs")
	var recommendations jobResultsResponse
	if initial.Code != 200 || json.Unmarshal(initial.Body.Bytes(), &recommendations) != nil || len(recommendations.Sections) != 1 ||
		strings.Contains(initial.Body.String(), "Example private query") || strings.Contains(initial.Body.String(), "100,101") {
		t.Fatalf("unexpected response: %s", initial.Body)
	}
	ref := recommendations.Sections[0].SourceRef
	page := get("/v1/jobs?source=preferences&page=2&source_ref=" + url.QueryEscape(ref))
	var results jobResultsResponse
	if ref == "" || page.Code != 200 || json.Unmarshal(page.Body.Bytes(), &results) != nil || results.Search == nil ||
		results.Search.SourceRef != ref || s.q.SourceContext != demoSourceContext || s.q.Page != 2 {
		t.Fatalf("context did not carry through the request: HTTP %d (%s)", page.Code, page.Body)
	}
	if gone := get("/v1/jobs?source=profile_match&source_ref=" + url.QueryEscape(ref)); gone.Code != 410 || s.calls != 2 {
		t.Fatalf("wrong-source reference was not rejected: HTTP %d, %d backend calls", gone.Code, s.calls)
	}
	now = now.Add(sourceRefTTL)
	if expired := get("/v1/jobs?source=preferences&page=2&source_ref=" + url.QueryEscape(ref)); expired.Code != 410 || s.calls != 2 {
		t.Fatalf("expired reference was not rejected: HTTP %d, %d backend calls", expired.Code, s.calls)
	}
}
