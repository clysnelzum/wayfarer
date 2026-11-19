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

var demoSource = linkedin.JobSourceContext{
	Source: "preferences", Origin: "PREFERENCES_LANDING", Keywords: "Example private query", Locations: "10,20", Seed: "100,101",
}

func TestSourceRefRoundTripAndExpiry(t *testing.T) {
	refs := newSourceRefs()
	now := time.Unix(123456, 0)
	ref := refs.seal(&demoSource, now)
	value := refs.open(ref, "preferences", now)
	if ref == "" || value == nil || *value != demoSource {
		t.Fatal("could not resolve newly issued reference")
	}
	replacement := "A"
	if ref[0] == 'A' {
		replacement = "B"
	}
	if refs.open(replacement+ref[1:], "preferences", now) != nil || refs.open(ref, "preferences", now.Add(sourceRefTTL)) != nil ||
		refs.open(ref, "profile_match", now) != nil || newSourceRefs().open(ref, "preferences", now) != nil {
		t.Fatal("accepted a changed, expired, wrong-source or foreign reference")
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
		return linkedin.JobResults{Sections: []linkedin.JobSection{
			{Title: "Example", JobIDs: []string{"100"}, Source: "preferences", SourceContext: &demoSource},
		}}, nil
	}
	return linkedin.JobResults{Search: &linkedin.JobSearchInfo{Source: q.Source, SourceContext: q.SourceContext}}, nil
}

func TestSourceRefHTTPPinsTheIssuedContext(t *testing.T) {
	s := &sourceRefStub{}
	handler := New("local", s)
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer local")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	initial := get("/v1/jobs")
	var recommendations linkedin.JobResults
	if initial.Code != 200 || json.Unmarshal(initial.Body.Bytes(), &recommendations) != nil || len(recommendations.Sections) != 1 ||
		strings.Contains(initial.Body.String(), demoSource.Keywords) || strings.Contains(initial.Body.String(), demoSource.Seed) {
		t.Fatalf("unexpected response: %s", initial.Body)
	}
	ref := recommendations.Sections[0].SourceRef
	page := get("/v1/jobs?source=preferences&page=2&source_ref=" + url.QueryEscape(ref))
	var results linkedin.JobResults
	if ref == "" || page.Code != 200 || json.Unmarshal(page.Body.Bytes(), &results) != nil || results.Search == nil ||
		results.Search.SourceRef != ref || s.q.SourceContext == nil || *s.q.SourceContext != demoSource || s.q.Page != 2 {
		t.Fatalf("context did not carry through the request: HTTP %d", page.Code)
	}
	if gone := get("/v1/jobs?source=profile_match&source_ref=" + url.QueryEscape(ref)); gone.Code != 410 || s.calls != 2 {
		t.Fatalf("wrong-source reference was not rejected: HTTP %d, %d backend calls", gone.Code, s.calls)
	}
}
