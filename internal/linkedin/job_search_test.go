package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/clysnelzum/wayfarer/internal/linkedin/scripts"
)

func testJobSummary(id string) JobSummary {
	return JobSummary{
		ID:       id,
		Title:    "Engineer " + id,
		Company:  "ACME",
		Location: "Remote",
		URL:      "https://www.linkedin.com/jobs/view/" + id + "/",
	}
}

func TestJobFiltersAndBroadenedResults(t *testing.T) {
	provider := &fakeScriptProvider{}
	c := mustNewClient(t, readerFunc(func(_ context.Context, raw, expression string) (json.RawMessage, error) {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		want := url.Values{
			"keywords": {"DevOps & Cloud"}, "start": {"25"}, "geoId": {"90009870"}, "f_C": {"14226"}, "f_TPR": {"r604800"},
			"f_SAL": {"f_SA_id_225001:272001,1621001$f_SA_id_227001:277001$f_SA_id_226001:272015"},
			"f_AL":  {"true"}, "f_VJ": {"true"}, "f_JIYN": {"true"}, "f_EA": {"true"},
		}
		if u.Path != "/jobs/search-results/" || !reflect.DeepEqual(u.Query(), want) || expression == "" {
			t.Fatalf("unexpected request: %s (expr=%q)", raw, expression)
		}
		item := testJobSummary("42")
		item.Workplace = "hybrid"
		item.CompensationLabel = "180K USD/yr - 225K USD/yr"
		item.BenefitsLabel = "1 prestación"
		item.NetworkLabel = "5 antiguos alumnos trabajan aquí"
		return json.Marshal(map[string]any{
			"items": []JobSummary{item},
			"page":  2, "has_more": false,
			"match_mode": "broadened", "result_count": 3, "result_count_qualifier": "exact", "location_label": "Greater Buenos Aires",
			"notice": "We couldn't find exact matches", "query_evidence": u.Query(), "selected_job_id": "42",
		})
	}), WithScripts(provider))
	q := JobQuery{Keywords: "DevOps & Cloud", Page: 2, Filters: JobFilters{
		LocationID: "90009870", CompanyID: "14226", PostedWithin: "week", Workplace: []string{"remote", "hybrid"},
		Experience: []string{"senior"}, Employment: []string{"full_time"}, EasyApply: true, InNetwork: true, Verified: true, Under10: true,
	}}
	v, err := c.Jobs(context.Background(), q)
	if err != nil || v.Search == nil || v.Search.MatchMode != "broadened" || v.Search.ResultCount == nil || *v.Search.ResultCount != 3 ||
		len(v.Items) != 1 || v.Items[0].Workplace != "hybrid" || v.Search.Notice == "" || v.Search.SelectedJobID != "42" {
		t.Fatalf("unexpected filtered response: %+v (%v)", v, err)
	}
	if !reflect.DeepEqual(provider.calls, []scriptCall{{id: scripts.ScriptJobs, args: []any{0, ""}}}) {
		t.Fatalf("unexpected script calls: %+v", provider.calls)
	}
	card := v.Items[0]
	if card.CompensationLabel != "180K USD/yr - 225K USD/yr" || card.BenefitsLabel != "1 prestación" || card.NetworkLabel != "5 antiguos alumnos trabajan aquí" {
		t.Fatalf("lost job card labels: %+v", card)
	}
}

func TestObservedSearch(t *testing.T) {
	q := JobQuery{Keywords: "DevOps", Page: 1, Filters: JobFilters{Workplace: []string{"remote", "hybrid"}, Experience: []string{"senior"}}}
	path, err := q.searchPath()
	if err != nil {
		t.Fatal(err)
	}
	const facets = "f_SA_id_227001:277001$f_SA_id_225001:1621001,272001"
	for _, tc := range []struct {
		name     string
		observed url.Values
		ok       bool
	}{
		{"reordered facets and default geo", url.Values{"keywords": {"DevOps"}, "f_SAL": {facets}, "geoId": {"100446943"}, "start": {"0"}, "f_AL": {"false"}}, true},
		{"dropped workplace", url.Values{"keywords": {"DevOps"}, "f_SAL": {"f_SA_id_227001:277001"}}, false},
		{"rewritten query", url.Values{"keywords": {"Engineer"}, "f_SAL": {facets}}, false},
		{"changed page", url.Values{"keywords": {"DevOps"}, "f_SAL": {facets}, "start": {"25"}}, false},
		{"injected filter", url.Values{"keywords": {"DevOps"}, "f_SAL": {facets}, "f_AL": {"true"}}, false},
		{"ambiguous filter values", url.Values{"keywords": {"DevOps"}, "f_SAL": {facets, facets}}, false},
		{"missing evidence", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateObservedSearch(path, tc.observed)
			if tc.ok && err != nil || !tc.ok && !errors.Is(err, ErrSearchMismatch) {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
	}
}

func TestMultipleCompaniesAndNewFacets(t *testing.T) {
	q := JobQuery{Keywords: "DevOps", Page: 1, Filters: JobFilters{
		CompanyIDs: []string{"14226", "613034"}, Experience: []string{"manager", "director", "executive"},
		Employment: []string{"part_time"}, Under10: true,
	}}
	path, err := q.searchPath()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	chosen := u.Query()
	if chosen.Get("f_C") != "14226,613034" || chosen.Get("f_EA") != "true" ||
		chosen.Get("f_SAL") != "f_SA_id_227001:278001,272003,279001$f_SA_id_226001:273001" {
		t.Fatalf("unexpected filters: %v", chosen)
	}
	chosen.Set("f_C", "613034,14226")
	if err := validateObservedSearch(path, chosen); err != nil {
		t.Fatalf("reordered companies should remain equivalent: %v", err)
	}
	chosen.Set("f_C", "613034")
	if err := validateObservedSearch(path, chosen); !errors.Is(err, ErrSearchMismatch) {
		t.Fatalf("expected a dropped company to be rejected: %v", err)
	}
	q.Filters.CompanyIDs = []string{"14226", "14226"}
	if err := q.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate company IDs must return ErrInvalidInput, got %v", err)
	}
	q.Filters.CompanyIDs = []string{"613034"}
	q.Filters.CompanyID = "14226"
	if err := q.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("single and multiple company filters must return ErrInvalidInput, got %v", err)
	}
}

func TestSuggestedFacetsRoundTrip(t *testing.T) {
	q := JobQuery{Keywords: "Product Designer", Page: 2, Filters: JobFilters{
		Workplace: []string{"remote"}, SuggestedFacetIDs: []string{"230001:275014", "228001:288003", "230001:272014"},
	}}
	c := mustNewClient(t, readerFunc(func(_ context.Context, raw, _ string) (json.RawMessage, error) {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if u.Query().Get("f_SAL") != "f_SA_id_225001:272001$f_SA_id_230001:275014,272014$f_SA_id_228001:288003" {
			t.Fatalf("unexpected facet encoding: %v", u.Query())
		}
		return json.RawMessage(`{"items":[],"page":2,"match_mode":"no_results","suggested_facets":[{"id":"230001:272014","label":"UI","selected":true}],"query_evidence":{"keywords":["Product Designer"],"start":["25"],"f_SAL":["f_SA_id_228001:288003$f_SA_id_230001:272014,275014$f_SA_id_225001:272001"]}}`), nil
	}))
	got, err := c.Jobs(context.Background(), q)
	if err != nil || got.Search == nil || len(got.Search.SuggestedFacets) != 1 ||
		got.Search.SuggestedFacets[0] != (JobSuggestedFacet{ID: "230001:272014", Label: "UI", Selected: true}) {
		t.Fatalf("unexpected job search response: %+v (%v)", got, err)
	}
	for _, ids := range [][]string{{"230001:275014", "230001:275014"}, {"225001:272001"}, {"230001:00001"}, {"230001:1$f_SA_id_227001:123"}} {
		q.Filters.SuggestedFacetIDs = ids
		if err := q.Validate(); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for suggested IDs %v, got %v", ids, err)
		}
	}
}

func TestEmptyJobPageOnlyInfersOutOfRangeFromAnExactTotal(t *testing.T) {
	for _, tc := range []struct {
		name      string
		page      int
		count     string
		qualifier string
		want      bool
	}{
		{"at exact end", 3, "50", "exact", true},
		{"above exact end", 4, "50", "exact", true},
		{"within exact total", 3, "51", "exact", false},
		{"lower bound", 5, "99", "more_than", false},
		{"no count", 100, "null", "", false},
		{"no matches overall", 2, "0", "exact", false},
		{"first page", 1, "0", "exact", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := mustNewClient(t, readerFunc(func(_ context.Context, raw, _ string) (json.RawMessage, error) {
				u, err := url.Parse(raw)
				if err != nil {
					return nil, err
				}
				return json.Marshal(map[string]any{
					"items": []JobSummary{}, "page": tc.page, "match_mode": "no_results", "has_more": true,
					"result_count": json.RawMessage(tc.count), "result_count_qualifier": tc.qualifier, "query_evidence": u.Query(),
				})
			}))
			v, err := c.Jobs(context.Background(), JobQuery{Keywords: "DevOps", Page: tc.page})
			if err != nil || v.Pagination == nil || v.Pagination.OutOfRange != tc.want || v.Pagination.HasMore || v.Pagination.NextPage != 0 {
				t.Fatalf("unexpected pagination: %+v (%v)", v.Pagination, err)
			}
		})
	}
}

func TestDiscardedSuggestedFacetIsMismatch(t *testing.T) {
	q := JobQuery{Keywords: "DevOps", Page: 1, Filters: JobFilters{SuggestedFacetIDs: []string{"230001:277008"}}}
	path, err := q.searchPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateObservedSearch(path, url.Values{"keywords": {"DevOps"}}); !errors.Is(err, ErrSearchMismatch) {
		t.Fatalf("expected dropped suggestion to be a search mismatch, got %v", err)
	}
}

func TestJobSearchRejectsMissingRequestedFilter(t *testing.T) {
	c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
		return json.RawMessage(`{"items":[{"id":"42","title":"Engineer","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/"}],"page":1,"match_mode":"results","query_evidence":{"keywords":["DevOps"]}}`), nil
	}))
	_, err := c.Jobs(context.Background(), JobQuery{Keywords: "DevOps", Page: 1, Filters: JobFilters{EasyApply: true}})
	if !errors.Is(err, ErrSearchMismatch) {
		t.Fatalf("expected upstream search mismatch, got %v", err)
	}
}

func TestJobRecommendationsLimit(t *testing.T) {
	items := make([]JobSummary, 26)
	for i := range items {
		items[i] = testJobSummary(strconv.Itoa(i + 1))
	}
	for _, tc := range []struct {
		limit   int
		status  string
		wantErr bool
	}{
		{26, "limit_reached", false},
		{30, "no_more_observed", false},
		{30, "budget_reached", false},
		{30, "limit_reached", true},
		{30, "", true},
		{25, "limit_reached", true},
	} {
		provider := &fakeScriptProvider{}
		c := mustNewClient(t, readerFunc(func(_ context.Context, raw, expression string) (json.RawMessage, error) {
			if raw != defaultBaseURL+"/jobs/" || expression == "" {
				t.Fatalf("expected the limit only in the browser extractor, not in the navigation URL: %s", raw)
			}
			return json.Marshal(JobResults{Items: items, RecommendationStatus: tc.status})
		}), WithScripts(provider))
		v, err := c.Jobs(context.Background(), JobQuery{RecommendationLimit: tc.limit})
		if tc.wantErr {
			assertContractError(t, err)
		} else if err != nil || v.RecommendationStatus != tc.status {
			t.Fatalf("limit=%d status=%q: got %q (%v)", tc.limit, tc.status, v.RecommendationStatus, err)
		}
		if !reflect.DeepEqual(provider.calls, []scriptCall{{id: scripts.ScriptJobs, args: []any{tc.limit, ""}}}) {
			t.Fatalf("unexpected script calls: %+v", provider.calls)
		}
	}
}

func TestMultipleJobSearchLocations(t *testing.T) {
	q := JobQuery{Keywords: "DevOps", Page: 1, Filters: JobFilters{LocationIDs: []string{"90009872", "101220015"}}}
	path, err := q.searchPath()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(path)
	if err != nil || u.Query().Get("geoId") != "90009872,101220015" {
		t.Fatalf("unexpected location encoding: %s (%v)", path, err)
	}
	observed := url.Values{"keywords": {"DevOps"}, "geoId": {"101220015,90009872"}}
	if err := validateObservedSearch(path, observed); err != nil {
		t.Fatal(err)
	}
	observed.Set("geoId", "90009872")
	if err := validateObservedSearch(path, observed); !errors.Is(err, ErrSearchMismatch) {
		t.Fatalf("expected a dropped location error, got %v", err)
	}
	q.Filters.LocationID = "90009872"
	if err := q.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected geo_id and geo_ids to return ErrInvalidInput, got %v", err)
	}
}

func TestJobDetailKeepsApplicationUnitsAndValidatesInput(t *testing.T) {
	provider := &fakeScriptProvider{}
	c := mustNewClient(t, readerFunc(func(_ context.Context, raw, expression string) (json.RawMessage, error) {
		if raw != defaultBaseURL+"/jobs/view/42/" || expression == "" {
			t.Fatal("expected the job detail extractor")
		}
		return json.RawMessage(`{"job":{"id":"42","title":"Engineer","company":"ACME","description":"Cloud","apply_method":"external","application_activity":{"kind":"apply_clicks","count":100,"qualifier":"more_than"},"saved":false,"related_jobs":[{"id":"17","title":"DevOps","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/17/"}]}}`), nil
	}), WithScripts(provider))
	v, err := c.Job(context.Background(), "42")
	if err != nil || v.ApplyMethod != "external" || v.ApplicationActivity == nil || v.ApplicationActivity.Kind != "apply_clicks" ||
		v.ApplicationActivity.Count != 100 || v.ApplicationActivity.Qualifier != "more_than" || v.Saved == nil || *v.Saved || len(v.RelatedJobs) != 1 {
		t.Fatalf("unexpected job detail: %+v (%v)", v, err)
	}
	if !reflect.DeepEqual(provider.calls, []scriptCall{{id: scripts.ScriptJobDetail}}) {
		t.Fatalf("unexpected script calls: %+v", provider.calls)
	}
	for _, badID := range []string{"", "0", "abc", "../12"} {
		if _, err := c.Job(context.Background(), badID); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("id=%q: expected ErrInvalidInput, got %v", badID, err)
		}
	}
}

func TestJobDetailNotFound(t *testing.T) {
	c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
		return json.RawMessage(`{"not_found":true}`), nil
	}))
	_, err := c.Job(context.Background(), "42")
	if !errors.Is(err, ErrJobNotFound) || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrJobNotFound / ErrNotFound, got %v", err)
	}
}

func TestJobsAndJobContractDrift(t *testing.T) {
	jobCases := []struct {
		name string
		body string
	}{
		{"missing job object", `{}`},
		{"mismatched job id", `{"job":{"id":"99","title":"Eng","company":"ACME","description":"Cloud"}}`},
		{"missing company", `{"job":{"id":"42","title":"Eng","description":"Cloud"}}`},
		{"missing title", `{"job":{"id":"42","company":"ACME","description":"Cloud"}}`},
		{"missing description", `{"job":{"id":"42","title":"Eng","company":"ACME"}}`},
		{"invalid company slug", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","company_slug":"../bad"}}`},
		{"invalid workplace", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","workplace":"anywhere"}}`},
		{"invalid employment_type", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","employment_type":"freelance"}}`},
		{"invalid apply_method", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","apply_method":"email"}}`},
		{"invalid application_status", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","application_status":"paused"}}`},
		{"empty description_blocks", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","description_blocks":[]}}`},
		{"invalid description_block kind", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","description_blocks":[{"kind":"table","text":"x"}]}}`},
		{"paragraph missing text", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","description_blocks":[{"kind":"paragraph"}]}}`},
		{"list missing ordered", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","description_blocks":[{"kind":"list","items":["Go"]}]}}`},
		{"list empty items", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","description_blocks":[{"kind":"list","ordered":false,"items":[]}]}}`},
		{"invalid application_activity kind", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","application_activity":{"kind":"views","count":1,"qualifier":"exact"}}}`},
		{"negative application_activity count", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","application_activity":{"kind":"applications","count":-1,"qualifier":"exact"}}}`},
		{"invalid viewer_application status", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","viewer_application":{"status":"draft"}}}`},
		{"related_job missing url", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","related_jobs":[{"id":"17","title":"DevOps","company":"ACME","location":"Remote"}]}}`},
		{"related_job includes self", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","related_jobs":[{"id":"42","title":"Eng","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/"}]}}`},
		{"duplicate related_job", `{"job":{"id":"42","title":"Eng","company":"ACME","description":"Cloud","related_jobs":[{"id":"17","title":"A","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/17/"},{"id":"17","title":"B","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/17/"}]}}`},
	}
	for _, tc := range jobCases {
		t.Run("job/"+tc.name, func(t *testing.T) {
			c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
				return json.RawMessage(tc.body), nil
			}))
			_, err := c.Job(context.Background(), "42")
			assertContractError(t, err)
		})
	}

	searchCases := []struct {
		name  string
		query JobQuery
		body  string
	}{
		{"missing items", JobQuery{}, `{"page":0}`},
		{"missing summary title", JobQuery{}, `{"items":[{"id":"42","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/"}],"page":0}`},
		{"missing summary company", JobQuery{}, `{"items":[{"id":"42","title":"Eng","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/"}],"page":0}`},
		{"missing summary location", JobQuery{}, `{"items":[{"id":"42","title":"Eng","company":"ACME","url":"https://www.linkedin.com/jobs/view/42/"}],"page":0}`},
		{"invalid summary url", JobQuery{}, `{"items":[{"id":"42","title":"Eng","company":"ACME","location":"Remote","url":"/jobs/view/42/"}],"page":0}`},
		{"invalid summary workplace", JobQuery{}, `{"items":[{"id":"42","title":"Eng","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/","workplace":"mars"}],"page":0}`},
		{"invalid summary signal", JobQuery{}, `{"items":[{"id":"42","title":"Eng","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/","signals":["urgent"]}],"page":0}`},
		{"duplicate summary signal", JobQuery{}, `{"items":[{"id":"42","title":"Eng","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/","signals":["early_applicant","early_applicant"]}],"page":0}`},
		{"duplicate item id", JobQuery{}, `{"items":[{"id":"42","title":"Eng","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/"},{"id":"42","title":"Eng2","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/"}],"page":0}`},
		{"unexpected recommendation_status", JobQuery{}, `{"items":[],"page":0,"recommendation_status":"limit_reached"}`},
		{"section absent job", JobQuery{}, `{"items":[],"page":0,"sections":[{"title":"Picked","job_ids":["42"]}]}`},
		{"section duplicate job_ids", JobQuery{}, `{"items":[{"id":"42","title":"Eng","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/"}],"page":0,"sections":[{"title":"Picked","job_ids":["42","42"]}]}`},
		{"missing match_mode", JobQuery{Keywords: "Go", Page: 1}, `{"items":[],"page":1,"query_evidence":{"keywords":["Go"]}}`},
		{"invalid match_mode", JobQuery{Keywords: "Go", Page: 1}, `{"items":[],"page":1,"match_mode":"fuzzy","query_evidence":{"keywords":["Go"]}}`},
		{"negative result_count", JobQuery{Keywords: "Go", Page: 1}, `{"items":[],"page":1,"match_mode":"no_results","result_count":-1,"result_count_qualifier":"exact","query_evidence":{"keywords":["Go"]}}`},
		{"missing result_count_qualifier", JobQuery{Keywords: "Go", Page: 1}, `{"items":[],"page":1,"match_mode":"no_results","result_count":5,"query_evidence":{"keywords":["Go"]}}`},
		{"invalid selected_job_id", JobQuery{Keywords: "Go", Page: 1}, `{"items":[],"page":1,"match_mode":"no_results","selected_job_id":"abc","query_evidence":{"keywords":["Go"]}}`},
		{"suggested_facet missing selected", JobQuery{Keywords: "Go", Page: 1}, `{"items":[],"page":1,"match_mode":"no_results","suggested_facets":[{"id":"230001:272014","label":"UI"}],"query_evidence":{"keywords":["Go"]}}`},
	}
	for _, tc := range searchCases {
		t.Run("jobs/"+tc.name, func(t *testing.T) {
			c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
				return json.RawMessage(tc.body), nil
			}))
			_, err := c.Jobs(context.Background(), tc.query)
			assertContractError(t, err)
		})
	}
}
