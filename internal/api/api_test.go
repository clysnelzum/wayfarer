package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

type stub struct {
	query         linkedin.JobQuery
	feedQuery     linkedin.FeedQuery
	locationQuery string
	companyQuery  string
	err           error
	calls         int
}

func (s *stub) Jobs(_ context.Context, q linkedin.JobQuery) (linkedin.JobResults, error) {
	s.calls++
	s.query = q
	return linkedin.JobResults{Items: []linkedin.JobSummary{{ID: "42", Title: "DevOps"}}}, s.err
}

func (s *stub) Job(_ context.Context, id string) (linkedin.Job, error) {
	s.calls++
	return linkedin.Job{ID: id, Description: "Cloud"}, s.err
}

func (s *stub) JobLocations(_ context.Context, q string) (linkedin.JobLocations, error) {
	s.calls++
	s.locationQuery = q
	return linkedin.JobLocations{Items: []linkedin.JobLocation{{ID: "90009870", Name: "Buenos Aires"}}, Status: "suggestions"}, s.err
}

func (s *stub) JobCompanies(_ context.Context, q string) (linkedin.JobCompanies, error) {
	s.calls++
	s.companyQuery = q
	return linkedin.JobCompanies{Items: []linkedin.JobCompany{{ID: "1035", Name: "Microsoft"}}, Status: "suggestions"}, s.err
}

func (s *stub) Feed(_ context.Context, q linkedin.FeedQuery) (linkedin.FeedResults, error) {
	s.calls++
	s.feedQuery = q
	return linkedin.FeedResults{Items: []linkedin.Post{{ID: "example1", Text: "Post", Content: "Post"}}, LoadStatus: "initial"}, s.err
}

func (s *stub) Profile(_ context.Context, slug string) (linkedin.Profile, error) {
	s.calls++
	return linkedin.Profile{Slug: slug, Name: "Example Person"}, s.err
}

func (s *stub) Company(_ context.Context, slug string) (linkedin.Company, error) {
	s.calls++
	return linkedin.Company{Slug: slug, Name: "Example Company"}, s.err
}

func TestRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, auth, code string
		err                            error
		status, calls                  int
	}{
		{"health", "GET", "/healthz", "Bearer local", "", nil, 200, 0},
		{"recommendations", "GET", "/v1/jobs", "Bearer local", "", nil, 200, 1},
		{"preferences source page two", "GET", "/v1/jobs?source=preferences&page=2", "Bearer local", "", nil, 200, 1},
		{"unavailable source", "GET", "/v1/jobs?source=profile_match", "Bearer local", "recommendation_source_unavailable", linkedin.ErrJobSourceUnavailable, 404, 1},
		{"unsupported source", "GET", "/v1/jobs?source=recent", "Bearer local", "invalid_query", nil, 400, 0},
		{"source with query", "GET", "/v1/jobs?source=profile_match&q=Go", "Bearer local", "invalid_query", nil, 400, 0},
		{"source with false filter", "GET", "/v1/jobs?source=profile_match&verified=false", "Bearer local", "invalid_query", nil, 400, 0},
		{"signed-in locations search", "GET", "/v1/jobs?q=Go&geo_ids=90009872,101220015", "Bearer local", "", nil, 200, 1},
		{"duplicate locations", "GET", "/v1/jobs?q=Go&geo_ids=90009872,90009872", "Bearer local", "invalid_query", nil, 400, 0},
		{"expanded recommendations", "GET", "/v1/jobs?limit=30", "Bearer local", "", nil, 200, 1},
		{"zero recommendation limit", "GET", "/v1/jobs?limit=0", "Bearer local", "invalid_query", nil, 400, 0},
		{"recommendation limit too high", "GET", "/v1/jobs?limit=51", "Bearer local", "invalid_query", nil, 400, 0},
		{"repeated recommendation limit", "GET", "/v1/jobs?limit=1&limit=2", "Bearer local", "invalid_query", nil, 400, 0},
		{"recommendation limit with search", "GET", "/v1/jobs?q=DevOps&limit=2", "Bearer local", "invalid_query", nil, 400, 0},
		{"recommendation limit with filter", "GET", "/v1/jobs?limit=2&verified=false", "Bearer local", "invalid_query", nil, 400, 0},
		{"jobs", "GET", "/v1/jobs?q=DevOps+Go", "Bearer local", "", nil, 200, 1},
		{"jobs suggested facets", "GET", "/v1/jobs?q=DevOps&suggested_facet_ids=230001%3A277008,228001%3A282003", "Bearer local", "", nil, 200, 1},
		{"unsupported suggestion group", "GET", "/v1/jobs?q=DevOps&suggested_facet_ids=225001%3A272001", "Bearer local", "invalid_query", nil, 400, 0},
		{"suggestion without q", "GET", "/v1/jobs?suggested_facet_ids=230001%3A277008", "Bearer local", "invalid_query", nil, 400, 0},
		{"jobs page two", "GET", "/v1/jobs?q=DevOps+Go&page=2", "Bearer local", "", nil, 200, 1},
		{"jobs filtered", "GET", "/v1/jobs?q=Go&page=2&workplace=remote,hybrid&geo_id=90009870&posted_within=week&easy_apply=true", "Bearer local", "", nil, 200, 1},
		{"upstream dropped search filter", "GET", "/v1/jobs?q=Go&workplace=remote", "Bearer local", "upstream_search_changed", linkedin.ErrSearchMismatch, 502, 1},
		{"filters without q", "GET", "/v1/jobs?workplace=remote", "Bearer local", "invalid_query", nil, 400, 0},
		{"unknown workplace", "GET", "/v1/jobs?q=Go&workplace=anywhere", "Bearer local", "invalid_query", nil, 400, 0},
		{"duplicate workplaces", "GET", "/v1/jobs?q=Go&workplace=remote,remote", "Bearer local", "invalid_query", nil, 400, 0},
		{"bad company id", "GET", "/v1/jobs?q=Go&company_id=company-slug", "Bearer local", "invalid_query", nil, 400, 0},
		{"invalid boolean filter", "GET", "/v1/jobs?q=Go&easy_apply=maybe", "Bearer local", "invalid_query", nil, 400, 0},
		{"filter with no terms", "GET", "/v1/jobs?verified=false", "Bearer local", "invalid_query", nil, 400, 0},
		{"jobs multi-company and part-time", "GET", "/v1/jobs?q=Go&company_ids=14226,613034&experience=director,manager&employment_type=part_time&under_10_applicants=true", "Bearer local", "", nil, 200, 1},
		{"both company filters", "GET", "/v1/jobs?q=Go&company_id=14226&company_ids=613034", "Bearer local", "invalid_query", nil, 400, 0},
		{"duplicate company IDs", "GET", "/v1/jobs?q=Go&company_ids=14226,14226", "Bearer local", "invalid_query", nil, 400, 0},
		{"invalid under 10", "GET", "/v1/jobs?q=Go&under_10_applicants=1", "Bearer local", "invalid_query", nil, 400, 0},
		{"empty date", "GET", "/v1/jobs?q=Go&posted_within=", "Bearer local", "invalid_query", nil, 400, 0},
		{"job location autocomplete", "GET", "/v1/jobs/locations?q=Buenos+Aires", "Bearer local", "", nil, 200, 1},
		{"job company autocomplete", "GET", "/v1/jobs/companies?q=Microsoft", "Bearer local", "", nil, 200, 1},
		{"short company query", "GET", "/v1/jobs/companies?q=M", "Bearer local", "invalid_query", nil, 400, 0},
		{"repeated company query", "GET", "/v1/jobs/companies?q=Apple&q=Microsoft", "Bearer local", "invalid_query", nil, 400, 0},
		{"unknown company query param", "GET", "/v1/jobs/companies?q=Microsoft&limit=1", "Bearer local", "invalid_query", nil, 400, 0},
		{"bad location query", "GET", "/v1/jobs/locations?q=A", "Bearer local", "invalid_query", nil, 400, 0},
		{"page without q", "GET", "/v1/jobs?page=2", "Bearer local", "invalid_query", nil, 400, 0},
		{"page zero", "GET", "/v1/jobs?q=Go&page=0", "Bearer local", "invalid_query", nil, 400, 0},
		{"page too high", "GET", "/v1/jobs?q=Go&page=101", "Bearer local", "invalid_query", nil, 400, 0},
		{"repeated page", "GET", "/v1/jobs?q=Go&page=1&page=2", "Bearer local", "invalid_query", nil, 400, 0},
		{"detail", "GET", "/v1/jobs/42", "Bearer local", "", nil, 200, 1},
		{"personal profile", "GET", "/v1/profiles/example-person", "Bearer local", "", nil, 200, 1},
		{"company profile", "GET", "/v1/companies/example-co", "Bearer local", "", nil, 200, 1},
		{"invalid profile path", "GET", "/v1/profiles/example/extra", "Bearer local", "invalid_slug", nil, 400, 0},
		{"invalid company slug", "GET", "/v1/companies/-example", "Bearer local", "invalid_slug", nil, 400, 0},
		{"personal profile unavailable", "GET", "/v1/profiles/example-person", "Bearer local", "profile_not_found", linkedin.ErrProfileNotFound, 404, 1},
		{"company unavailable", "GET", "/v1/companies/example-co", "Bearer local", "company_not_found", linkedin.ErrCompanyNotFound, 404, 1},
		{"feed", "GET", "/v1/feed", "Bearer local", "", nil, 200, 1},
		{"feed with limit", "GET", "/v1/feed?limit=8", "Bearer local", "", nil, 200, 1},
		{"invalid feed limit", "GET", "/v1/feed?limit=31", "Bearer local", "invalid_query", nil, 400, 0},
		{"repeated feed limit", "GET", "/v1/feed?limit=2&limit=4", "Bearer local", "invalid_query", nil, 400, 0},
		{"authentication", "GET", "/v1/feed", "", "unauthorized", nil, 401, 0},
		{"mutations denied", "POST", "/v1/feed", "Bearer local", "method_not_allowed", nil, 405, 0},
		{"bad id", "GET", "/v1/jobs/42/extra", "Bearer local", "invalid_job_id", nil, 400, 0},
		{"repeated q", "GET", "/v1/jobs?q=a&q=b", "Bearer local", "invalid_query", nil, 400, 0},
		{"unknown query", "GET", "/v1/jobs?url=abc", "Bearer local", "invalid_query", nil, 400, 0},
		{"missing job", "GET", "/v1/jobs/42", "Bearer local", "job_not_found", linkedin.ErrNotFound, 404, 1},
		{"expired session", "GET", "/v1/feed", "Bearer local", "upstream_auth_required", chrome.ErrAuth, 503, 1},
		{"timed out", "GET", "/v1/feed", "Bearer local", "upstream_timeout", context.DeadlineExceeded, 504, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &stub{err: tc.err}
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			New("local", s).ServeHTTP(w, r)
			if w.Code != tc.status || s.calls != tc.calls || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("got %d, %d calls, headers %v (%s)", w.Code, s.calls, w.Header(), w.Body)
			}
			if tc.code != "" {
				var body struct{ Error struct{ Code string } }
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Code != tc.code {
					t.Fatalf("expected error %s, got %s", tc.code, w.Body)
				}
			}
			var want linkedin.JobQuery
			switch tc.name {
			case "unavailable source":
				want = linkedin.JobQuery{Source: "profile_match", Page: 1}
			case "preferences source page two":
				want = linkedin.JobQuery{Source: "preferences", Page: 2}
			case "signed-in locations search":
				want = linkedin.JobQuery{Keywords: "Go", Page: 1, Filters: linkedin.JobFilters{LocationIDs: []string{"90009872", "101220015"}}}
			case "expanded recommendations":
				want = linkedin.JobQuery{RecommendationLimit: 30}
			case "jobs":
				want = linkedin.JobQuery{Keywords: "DevOps Go", Page: 1}
			case "jobs suggested facets":
				want = linkedin.JobQuery{Keywords: "DevOps", Page: 1, Filters: linkedin.JobFilters{
					SuggestedFacetIDs: []string{"230001:277008", "228001:282003"},
				}}
			case "upstream dropped search filter":
				want = linkedin.JobQuery{Keywords: "Go", Page: 1, Filters: linkedin.JobFilters{Workplace: []string{"remote"}}}
			case "jobs page two":
				want = linkedin.JobQuery{Keywords: "DevOps Go", Page: 2}
			case "jobs filtered":
				want = linkedin.JobQuery{Keywords: "Go", Page: 2, Filters: linkedin.JobFilters{
					LocationID: "90009870", PostedWithin: "week", Workplace: []string{"remote", "hybrid"}, EasyApply: true,
				}}
			case "jobs multi-company and part-time":
				want = linkedin.JobQuery{Keywords: "Go", Page: 1, Filters: linkedin.JobFilters{
					CompanyIDs: []string{"14226", "613034"}, Experience: []string{"director", "manager"},
					Employment: []string{"part_time"}, Under10: true,
				}}
			case "job company autocomplete":
				var body linkedin.JobCompanies
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || s.companyQuery != "Microsoft" ||
					body.Status != "suggestions" || len(body.Items) != 1 || body.Items[0].ID != "1035" {
					t.Fatalf("unexpected company autocomplete: query %q, body %s", s.companyQuery, w.Body)
				}
			case "job location autocomplete":
				if s.locationQuery != "Buenos Aires" {
					t.Fatalf("got location query %q", s.locationQuery)
				}
			}
			if tc.name == "feed with limit" && s.feedQuery.Limit != 8 {
				t.Fatalf("got feed query %+v", s.feedQuery)
			}
			if !reflect.DeepEqual(s.query, want) {
				t.Fatalf("got query %+v, want %+v", s.query, want)
			}
		})
	}
}
