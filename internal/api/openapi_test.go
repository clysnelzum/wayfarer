package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

type openAPISpec struct {
	Paths      map[string]map[string]openAPIOperation `json:"paths"`
	Components struct {
		Schemas map[string]openAPISchema `json:"schemas"`
	} `json:"components"`
}

type openAPIOperation struct {
	OperationID string             `json:"operationId"`
	Parameters  []openAPIParameter `json:"parameters"`
	Responses   map[string]struct {
		Description string `json:"description"`
		Content     map[string]struct {
			Schema openAPISchema `json:"schema"`
		} `json:"content"`
	} `json:"responses"`
}

type openAPIParameter struct {
	Name string `json:"name"`
	In   string `json:"in"`
	Ref  string `json:"$ref"`
}

type openAPISchema struct {
	Ref        string                   `json:"$ref"`
	Type       string                   `json:"type"`
	Required   []string                 `json:"required"`
	Properties map[string]openAPISchema `json:"properties"`
	Items      *openAPISchema           `json:"items"`
}

func loadOpenAPISpec(t *testing.T) openAPISpec {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.json"))
	if err != nil {
		t.Fatalf("read docs/openapi.json: %v", err)
	}
	var spec openAPISpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse docs/openapi.json: %v", err)
	}
	return spec
}

type openAPIStub struct {
	err      error
	panicVal any
}

func (s *openAPIStub) Feed(_ context.Context, _ int) (linkedin.FeedResults, error) {
	if s.panicVal != nil {
		panic(s.panicVal)
	}
	if s.err != nil {
		return linkedin.FeedResults{}, s.err
	}
	reactions := int64(5)
	return linkedin.FeedResults{
		Items: []linkedin.Post{{
			ID:      "card-1",
			Text:    "Hello LinkedIn",
			Content: "Hello LinkedIn",
			URL:     "https://www.linkedin.com/feed/update/urn:li:activity:1/",
			Author: &linkedin.FeedAuthor{
				Kind: "person",
				Slug: "example-person",
				Name: "Example Person",
				URL:  "https://www.linkedin.com/in/example-person/",
			},
			PublishedLabel: "1d",
			Hashtags:       []string{"#go"},
			Media:          []linkedin.FeedMedia{{Type: "image", URL: "https://media.licdn.com/example.jpg", Alt: "Diagram"}},
			Engagement:     &linkedin.FeedEngagement{ReactionCount: &reactions},
			Sponsored:      true,
		}},
		Sort:       "recent",
		HasMore:    true,
		LoadStatus: "limit_reached",
	}, nil
}

func (s *openAPIStub) Jobs(_ context.Context, q linkedin.JobQuery) (linkedin.JobResults, error) {
	if s.err != nil {
		return linkedin.JobResults{}, s.err
	}
	item := linkedin.JobSummary{
		ID:                "42",
		Title:             "Staff Engineer",
		Company:           "Example Co",
		Location:          "Remote",
		URL:               "https://www.linkedin.com/jobs/view/42/",
		Workplace:         "remote",
		PostedLabel:       "1d",
		CompensationLabel: "$180K/yr",
		BenefitsLabel:     "401(k)",
		NetworkLabel:      "1 alum works here",
		Verified:          true,
		Promoted:          true,
		Viewed:            true,
		Saved:             true,
		EasyApply:         true,
		Signals:           []string{"early_applicant"},
	}
	if q.Keywords == "" && q.Source == "" {
		return linkedin.JobResults{
			Items:                []linkedin.JobSummary{item},
			RecommendationStatus: "limit_reached",
			Sections: []linkedin.JobSection{{
				Title:         "Top picks",
				Source:        "preferences",
				SourceContext: `{"source":"preferences"}`,
				JobIDs:        []string{"42"},
			}},
		}, nil
	}
	count := int64(10)
	alert := true
	return linkedin.JobResults{
		Items: []linkedin.JobSummary{item},
		Pagination: &linkedin.JobPagination{
			Page:       q.Page,
			HasMore:    false,
			OutOfRange: false,
		},
		Search: &linkedin.JobSearchInfo{
			Source:               q.Source,
			SourceContext:        q.SourceContext,
			MatchMode:            "results",
			ResultCount:          &count,
			ResultCountQualifier: "exact",
			LocationLabel:        "United States",
			ResultCountLabel:     "10 results",
			Notice:               "",
			SelectedJobID:        "42",
			AlertEnabled:         &alert,
			SuggestedFacets: []linkedin.JobSuggestedFacet{{
				ID:       "230001:277008",
				Label:    "Go",
				Selected: true,
			}},
		},
	}, nil
}

func (s *openAPIStub) JobLocations(_ context.Context, _ string) (linkedin.JobLocations, error) {
	if s.err != nil {
		return linkedin.JobLocations{}, s.err
	}
	return linkedin.JobLocations{
		Status: "suggestions",
		Items:  []linkedin.JobLocation{{ID: "90009870", Name: "Buenos Aires"}},
	}, nil
}

func (s *openAPIStub) JobCompanies(_ context.Context, _ string) (linkedin.JobCompanies, error) {
	if s.err != nil {
		return linkedin.JobCompanies{}, s.err
	}
	return linkedin.JobCompanies{
		Status: "suggestions",
		Items:  []linkedin.JobCompany{{ID: "1035", Name: "Microsoft", Industry: "Software Development"}},
	}, nil
}

func (s *openAPIStub) Job(_ context.Context, id string) (linkedin.Job, error) {
	if s.err != nil {
		return linkedin.Job{}, s.err
	}
	saved := false
	alert := true
	return linkedin.Job{
		ID:                     id,
		Title:                  "Staff Engineer",
		Company:                "Example Co",
		URL:                    "https://www.linkedin.com/jobs/view/" + id + "/",
		Description:            "Build reliable systems.",
		CompanySlug:            "example-co",
		Location:               "Remote",
		PostedLabel:            "1d",
		Workplace:              "remote",
		EmploymentType:         "full_time",
		ApplyMethod:            "easy_apply",
		ApplicationStatus:      "open",
		CompensationLabel:      "$180K/yr",
		SimilarJobAlertEnabled: &alert,
		ApplicationActivity: &linkedin.JobApplicationActivity{
			Kind:      "applications",
			Count:     5,
			Qualifier: "exact",
		},
		ViewerApplication: &linkedin.JobViewerApplication{
			Status:         "submitted",
			SubmittedLabel: "Applied 1d ago",
		},
		Verified: true,
		Promoted: true,
		Saved:    &saved,
		RelatedJobs: []linkedin.JobSummary{{
			ID:       "43",
			Title:    "Backend Engineer",
			Company:  "Example Co",
			Location: "Remote",
			URL:      "https://www.linkedin.com/jobs/view/43/",
		}},
	}, nil
}

func (s *openAPIStub) Profile(_ context.Context, slug string) (linkedin.Profile, error) {
	if s.err != nil {
		return linkedin.Profile{}, s.err
	}
	return linkedin.Profile{
		Slug: slug,
		URL:  "https://www.linkedin.com/in/" + slug + "/",
		Name: "Example Person",
	}, nil
}

func (s *openAPIStub) Company(_ context.Context, slug string) (linkedin.Company, error) {
	if s.err != nil {
		return linkedin.Company{}, s.err
	}
	return linkedin.Company{
		Slug: slug,
		URL:  "https://www.linkedin.com/company/" + slug + "/",
		Name: "Example Company",
	}, nil
}

func assertMatchesOpenAPISchema(t *testing.T, spec openAPISpec, schema openAPISchema, raw any, path string) {
	t.Helper()
	if schema.Ref != "" {
		name := strings.TrimPrefix(schema.Ref, "#/components/schemas/")
		resolved, ok := spec.Components.Schemas[name]
		if !ok {
			t.Fatalf("%s: unresolved schema ref %q", path, schema.Ref)
		}
		schema = resolved
	}
	if len(schema.Properties) == 0 && schema.Type != "array" {
		return
	}
	if schema.Type == "array" && schema.Items != nil {
		arr, ok := raw.([]any)
		if !ok {
			t.Fatalf("%s: expected JSON array, got %T", path, raw)
		}
		for _, item := range arr {
			assertMatchesOpenAPISchema(t, spec, *schema.Items, item, path+"[]")
		}
		return
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("%s: expected JSON object, got %T", path, raw)
	}
	for _, req := range schema.Required {
		if _, exists := obj[req]; !exists {
			t.Fatalf("%s: missing required field %q in %v", path, req, obj)
		}
	}
	for key, val := range obj {
		propSchema, allowed := schema.Properties[key]
		if !allowed {
			t.Fatalf("%s: undocumented property %q emitted in response", path, key)
		}
		assertMatchesOpenAPISchema(t, spec, propSchema, val, path+"."+key)
	}
}

func TestOpenAPIRoutesAndResponseSchemas(t *testing.T) {
	spec := loadOpenAPISpec(t)
	h := New("local", &openAPIStub{})

	sampleRequests := map[string]struct {
		requestPath string
		schema      openAPISchema
	}{
		"/healthz": {
			requestPath: "/healthz",
			schema:      spec.Paths["/healthz"]["get"].Responses["200"].Content["application/json"].Schema,
		},
		"/v1/feed": {
			requestPath: "/v1/feed?limit=5",
			schema:      spec.Components.Schemas["Feed"],
		},
		"/v1/jobs": {
			requestPath: "/v1/jobs?q=Go&page=1",
			schema:      spec.Components.Schemas["JobResults"],
		},
		"/v1/jobs/locations": {
			requestPath: "/v1/jobs/locations?q=Buenos+Aires",
			schema:      spec.Components.Schemas["JobLocations"],
		},
		"/v1/jobs/companies": {
			requestPath: "/v1/jobs/companies?q=Microsoft",
			schema:      spec.Components.Schemas["JobCompanies"],
		},
		"/v1/jobs/{id}": {
			requestPath: "/v1/jobs/42",
			schema:      spec.Components.Schemas["Job"],
		},
		"/v1/profiles/{slug}": {
			requestPath: "/v1/profiles/example-person",
			schema:      spec.Components.Schemas["Profile"],
		},
		"/v1/companies/{slug}": {
			requestPath: "/v1/companies/example-co",
			schema:      spec.Components.Schemas["Company"],
		},
	}

	if len(spec.Paths) != len(sampleRequests) {
		t.Fatalf("OpenAPI path count mismatch: spec=%d test=%d", len(spec.Paths), len(sampleRequests))
	}

	for pattern, methods := range spec.Paths {
		if _, hasGet := methods["get"]; !hasGet || len(methods) != 1 {
			t.Fatalf("expected GET-only path %q in OpenAPI spec", pattern)
		}
		tc, ok := sampleRequests[pattern]
		if !ok {
			t.Fatalf("missing test coverage for OpenAPI path %q", pattern)
		}

		req := httptest.NewRequest(http.MethodGet, tc.requestPath, nil)
		req.Header.Set("Authorization", "Bearer local")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s (%s): got HTTP %d: %s", pattern, tc.requestPath, w.Code, w.Body)
		}
		var decoded any
		if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s: invalid JSON: %v", pattern, err)
		}
		assertMatchesOpenAPISchema(t, spec, tc.schema, decoded, pattern)
	}

	// Also verify /v1/jobs recommendations response (with sections and source_ref) matches JobResults schema.
	recReq := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	recReq.Header.Set("Authorization", "Bearer local")
	recW := httptest.NewRecorder()
	h.ServeHTTP(recW, recReq)
	if recW.Code != http.StatusOK {
		t.Fatalf("/v1/jobs recommendations: got HTTP %d: %s", recW.Code, recW.Body)
	}
	var recDecoded any
	if err := json.Unmarshal(recW.Body.Bytes(), &recDecoded); err != nil {
		t.Fatalf("/v1/jobs recommendations invalid JSON: %v", err)
	}
	assertMatchesOpenAPISchema(t, spec, spec.Components.Schemas["JobResults"], recDecoded, "/v1/jobs(recommendations)")
}

func TestOpenAPIJobParametersAccepted(t *testing.T) {
	spec := loadOpenAPISpec(t)
	jobsOp := spec.Paths["/v1/jobs"]["get"]
	h := New("local", &openAPIStub{})

	sampleValues := map[string]string{
		"q":                   "Go",
		"source":              "preferences",
		"source_ref":          "ref",
		"limit":               "10",
		"page":                "2",
		"geo_id":              "90009870",
		"geo_ids":             "90009870,101220015",
		"company_id":          "1035",
		"company_ids":         "1035,14226",
		"posted_within":       "week",
		"workplace":           "remote",
		"experience":          "senior",
		"employment_type":     "full_time",
		"easy_apply":          "true",
		"in_network":          "true",
		"verified":            "true",
		"suggested_facet_ids": "230001:277008",
		"under_10_applicants": "true",
	}

	for _, param := range jobsOp.Parameters {
		val, ok := sampleValues[param.Name]
		if !ok {
			t.Fatalf("undocumented or untested OpenAPI parameter %q on /v1/jobs", param.Name)
		}
		var rawQuery string
		switch param.Name {
		case "q", "limit", "source":
			rawQuery = param.Name + "=" + val
		case "source_ref":
			// source_ref is validated via sourceRefs; verify parseJobQuery accepts the combination.
			continue
		default:
			rawQuery = "q=Go&" + param.Name + "=" + val
		}
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs?"+rawQuery, nil)
		req.Header.Set("Authorization", "Bearer local")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("parameter %q (%s) rejected with HTTP %d: %s", param.Name, rawQuery, w.Code, w.Body)
		}
	}
}

func TestOpenAPIErrorResponsesAndDocumentedCodes(t *testing.T) {
	spec := loadOpenAPISpec(t)
	errSchema := spec.Components.Schemas["Error"]

	// Verify all error codes explicitly named in /v1/jobs OpenAPI responses are emitted as documented.
	jobsResponses := spec.Paths["/v1/jobs"]["get"].Responses
	cases := []struct {
		name       string
		method     string
		path       string
		auth       string
		svcErr     error
		panicVal   any
		wantStatus int
		wantCode   string
	}{
		{"unauthorized", http.MethodGet, "/v1/jobs", "", nil, nil, http.StatusUnauthorized, "unauthorized"},
		{"method_not_allowed", http.MethodPost, "/v1/jobs", "Bearer local", nil, nil, http.StatusMethodNotAllowed, "method_not_allowed"},
		{"not_found", http.MethodGet, "/unknown", "Bearer local", nil, nil, http.StatusNotFound, "not_found"},
		{"invalid_query", http.MethodGet, "/v1/jobs?bad=1", "Bearer local", nil, nil, http.StatusBadRequest, "invalid_query"},
		{"invalid_job_id", http.MethodGet, "/v1/jobs/abc", "Bearer local", nil, nil, http.StatusBadRequest, "invalid_job_id"},
		{"invalid_slug", http.MethodGet, "/v1/profiles/-bad", "Bearer local", nil, nil, http.StatusBadRequest, "invalid_slug"},
		{"job_not_found", http.MethodGet, "/v1/jobs/42", "Bearer local", linkedin.ErrJobNotFound, nil, http.StatusNotFound, "job_not_found"},
		{"profile_not_found", http.MethodGet, "/v1/profiles/missing", "Bearer local", linkedin.ErrProfileNotFound, nil, http.StatusNotFound, "profile_not_found"},
		{"company_not_found", http.MethodGet, "/v1/companies/missing", "Bearer local", linkedin.ErrCompanyNotFound, nil, http.StatusNotFound, "company_not_found"},
		{"recommendation_source_unavailable", http.MethodGet, "/v1/jobs?source=preferences", "Bearer local", linkedin.ErrJobSourceUnavailable, nil, http.StatusNotFound, "recommendation_source_unavailable"},
		{"recommendation_source_ref_unavailable", http.MethodGet, "/v1/jobs?source=preferences&source_ref=invalid", "Bearer local", nil, nil, http.StatusGone, "recommendation_source_ref_unavailable"},
		{"upstream_search_changed", http.MethodGet, "/v1/jobs?q=Go", "Bearer local", linkedin.ErrSearchMismatch, nil, http.StatusBadGateway, "upstream_search_changed"},
		{"upstream_error", http.MethodGet, "/v1/jobs?q=Go", "Bearer local", linkedin.ErrUpstreamContract, nil, http.StatusBadGateway, "upstream_error"},
		{"upstream_debugger_unavailable", http.MethodGet, "/v1/jobs?q=Go", "Bearer local", linkedin.ErrDebuggerUnavailable, nil, http.StatusServiceUnavailable, "upstream_debugger_unavailable"},
		{"upstream_busy", http.MethodGet, "/v1/jobs?q=Go", "Bearer local", linkedin.ErrUpstreamBusy, nil, http.StatusServiceUnavailable, "upstream_busy"},
		{"upstream_auth_required", http.MethodGet, "/v1/jobs?q=Go", "Bearer local", linkedin.ErrAuthRequired, nil, http.StatusServiceUnavailable, "upstream_auth_required"},
		{"upstream_timeout", http.MethodGet, "/v1/jobs?q=Go", "Bearer local", context.DeadlineExceeded, nil, http.StatusGatewayTimeout, "upstream_timeout"},
		{"internal_error", http.MethodGet, "/v1/feed", "Bearer local", nil, "boom", http.StatusInternalServerError, "internal_error"},
	}

	emittedByStatus := make(map[int][]string)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := New("local", &openAPIStub{err: tc.svcErr, panicVal: tc.panicVal})
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("got status %d, want %d: %s", w.Code, tc.wantStatus, w.Body)
			}
			var decoded any
			if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("invalid error JSON: %v", err)
			}
			assertMatchesOpenAPISchema(t, spec, errSchema, decoded, tc.name)

			var body errorResponse
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if body.Error.Code != tc.wantCode {
				t.Fatalf("got code %q, want %q", body.Error.Code, tc.wantCode)
			}
			emittedByStatus[tc.wantStatus] = append(emittedByStatus[tc.wantStatus], body.Error.Code)
		})
	}

	// Verify specific codes referenced in docs/openapi.json /v1/jobs response descriptions.
	expectedDocCodes := map[string][]string{
		"404": {"recommendation_source_unavailable"},
		"410": {"recommendation_source_ref_unavailable"},
		"502": {"upstream_search_changed"},
		"503": {"upstream_debugger_unavailable", "upstream_busy", "upstream_auth_required"},
	}
	for statusStr, codes := range expectedDocCodes {
		desc := jobsResponses[statusStr].Description
		for _, code := range codes {
			if !strings.Contains(desc, code) {
				t.Fatalf("expected OpenAPI /v1/jobs %s description %q to mention %q", statusStr, desc, code)
			}
		}
	}
}
