package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestJobFiltersAndBroadenedResults(t *testing.T) {
	c := Client{Reader: readerFunc(func(_ context.Context, raw, expression string) (json.RawMessage, error) {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		want := url.Values{
			"keywords": {"DevOps & Cloud"}, "start": {"25"}, "geoId": {"90009870"}, "f_C": {"14226"}, "f_TPR": {"r604800"},
			"f_SAL": {"f_SA_id_225001:272001,1621001$f_SA_id_227001:277001$f_SA_id_226001:272015"},
			"f_AL":  {"true"}, "f_VJ": {"true"}, "f_JIYN": {"true"}, "f_EA": {"true"},
		}
		if u.Path != "/jobs/search-results/" || !reflect.DeepEqual(u.Query(), want) || !strings.Contains(expression, "job-card-component-ref-") {
			t.Fatalf("unexpected request: %s", raw)
		}
		return json.Marshal(map[string]any{
			"items": []JobSummary{{ID: "42", Workplace: "hybrid"}}, "page": 2, "has_more": false,
			"match_mode": "broadened", "result_count": 3, "result_count_qualifier": "exact", "location_label": "Greater Buenos Aires",
			"notice": "We couldn't find exact matches", "query_evidence": u.Query(), "selected_job_id": "42",
		})
	})}
	q := JobQuery{Keywords: "DevOps & Cloud", Page: 2, Filters: JobFilters{
		LocationID: "90009870", CompanyID: "14226", PostedWithin: "week", Workplace: []string{"remote", "hybrid"},
		Experience: []string{"senior"}, Employment: []string{"full_time"}, EasyApply: true, InNetwork: true, Verified: true, Under10: true,
	}}
	v, err := c.Jobs(context.Background(), q)
	if err != nil || v.Search == nil || v.Search.MatchMode != "broadened" || v.Search.ResultCount == nil || *v.Search.ResultCount != 3 ||
		len(v.Items) != 1 || v.Items[0].Workplace != "hybrid" || v.Search.Notice == "" || v.Search.SelectedJobID != "42" {
		t.Fatalf("unexpected filtered response: %+v (%v)", v, err)
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
	if err := q.Validate(); err == nil {
		t.Fatal("duplicate company IDs must be rejected")
	}
	q.Filters.CompanyIDs = []string{"613034"}
	q.Filters.CompanyID = "14226"
	if err := q.Validate(); err == nil {
		t.Fatal("single and multiple company filters are mutually exclusive")
	}
}

func TestSuggestedFacetsRoundTrip(t *testing.T) {
	q := JobQuery{Keywords: "Product Designer", Page: 2, Filters: JobFilters{
		Workplace: []string{"remote"}, SuggestedFacetIDs: []string{"230001:275014", "228001:288003", "230001:272014"},
	}}
	c := Client{Reader: readerFunc(func(_ context.Context, raw, _ string) (json.RawMessage, error) {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if u.Query().Get("f_SAL") != "f_SA_id_225001:272001$f_SA_id_230001:275014,272014$f_SA_id_228001:288003" {
			t.Fatalf("unexpected facet encoding: %v", u.Query())
		}
		return json.RawMessage(`{"items":[],"page":2,"match_mode":"no_results","suggested_facets":[{"id":"230001:272014","label":"UI","selected":true}],"query_evidence":{"keywords":["Product Designer"],"start":["25"],"f_SAL":["f_SA_id_228001:288003$f_SA_id_230001:272014,275014$f_SA_id_225001:272001"]}}`), nil
	})}
	got, err := c.Jobs(context.Background(), q)
	if err != nil || got.Search == nil || len(got.Search.SuggestedFacets) != 1 ||
		got.Search.SuggestedFacets[0] != (JobSuggestedFacet{ID: "230001:272014", Label: "UI", Selected: true}) {
		t.Fatalf("unexpected job search response: %+v (%v)", got, err)
	}
	for _, ids := range [][]string{{"230001:275014", "230001:275014"}, {"225001:272001"}, {"230001:00001"}, {"230001:1$f_SA_id_227001:123"}} {
		q.Filters.SuggestedFacetIDs = ids
		if err := q.Validate(); err == nil {
			t.Errorf("expected invalid suggested IDs: %v", ids)
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
			c := Client{Reader: readerFunc(func(_ context.Context, raw, _ string) (json.RawMessage, error) {
				u, err := url.Parse(raw)
				if err != nil {
					return nil, err
				}
				return json.Marshal(map[string]any{
					"items": []JobSummary{}, "page": tc.page, "match_mode": "no_results", "has_more": true,
					"result_count": json.RawMessage(tc.count), "result_count_qualifier": tc.qualifier, "query_evidence": u.Query(),
				})
			})}
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
	c := Client{Reader: readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
		return json.RawMessage(`{"items":[{"id":"42"}],"page":1,"query_evidence":{"keywords":["DevOps"]}}`), nil
	})}
	_, err := c.Jobs(context.Background(), JobQuery{Keywords: "DevOps", Page: 1, Filters: JobFilters{EasyApply: true}})
	if !errors.Is(err, ErrSearchMismatch) {
		t.Fatalf("expected upstream search mismatch, got %v", err)
	}
}

func TestJobLocationLookup(t *testing.T) {
	c := Client{Reader: readerFunc(func(_ context.Context, raw, expression string) (json.RawMessage, error) {
		if raw != base+"/jobs/search-results/?keywords=jobs" || !strings.HasSuffix(expression, `)("Côte d'Azur \"France\"")`) {
			t.Fatal("expected the location query to be encoded as a JS string literal")
		}
		return json.RawMessage(`{"items":[{"id":"90009870","name":"Greater Buenos Aires"}],"status":"suggestions"}`), nil
	})}
	v, err := c.JobLocations(context.Background(), `Côte d'Azur "France"`)
	if err != nil || len(v.Items) != 1 || v.Items[0].ID != "90009870" {
		t.Fatalf("unexpected location lookup: %+v (%v)", v, err)
	}
}

func TestJobDetailKeepsApplicationUnits(t *testing.T) {
	c := Client{Reader: readerFunc(func(_ context.Context, raw, expression string) (json.RawMessage, error) {
		if raw != base+"/jobs/view/42/" || !strings.Contains(expression, jobExpression) {
			t.Fatal("expected the job detail extractor")
		}
		return json.RawMessage(`{"job":{"id":"42","title":"Engineer","company":"ACME","description":"Cloud","apply_method":"external","application_activity":{"kind":"apply_clicks","count":100,"qualifier":"more_than"},"saved":false,"related_jobs":[{"id":"17","title":"DevOps"}]}}`), nil
	})}
	v, err := c.Job(context.Background(), "42")
	if err != nil || v.ApplyMethod != "external" || v.ApplicationActivity == nil || v.ApplicationActivity.Kind != "apply_clicks" ||
		v.ApplicationActivity.Count != 100 || v.ApplicationActivity.Qualifier != "more_than" || v.Saved == nil || *v.Saved || len(v.RelatedJobs) != 1 {
		t.Fatalf("unexpected job detail: %+v (%v)", v, err)
	}
}
