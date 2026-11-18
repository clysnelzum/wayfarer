package linkedin

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/clysnelzum/wayfarer/internal/chrome"
)

// liveJobsAudit independently reads card IDs from the same DOM snapshot as the
// extractor so personalized ordering is not compared across two navigations.
type liveJobsAudit struct {
	reader Reader
	native []string
}

func (a *liveJobsAudit) Read(ctx context.Context, pageURL, expression string) (json.RawMessage, error) {
	probe := `async () => {
		const result = await (` + expression + `)();
		if (result === null) return null;
		const nodes = [...document.querySelectorAll('main [componentkey^="job-card-component-ref-"]')];
		const ids = nodes.filter(n => n.getClientRects().length).map(n => n.getAttribute('componentkey').split('-').at(-1)).filter(id => /^\d+$/.test(id));
		return {result, native: [...new Set(ids)].slice(0, 25)};
	}`
	raw, err := a.reader.Read(ctx, pageURL, probe)
	if err != nil {
		return nil, err
	}
	var observed struct {
		Result json.RawMessage
		Native []string
	}
	if err := json.Unmarshal(raw, &observed); err != nil {
		return nil, err
	}
	a.native = observed.Native
	return observed.Result, nil
}

func TestLiveJobSuggestedFacet(t *testing.T) {
	if os.Getenv("WAYFARER_LIVE_JOBS") != "1" {
		t.Skip("set WAYFARER_LIVE_JOBS=1 and WAYFARER_CHROME_WS to run the live jobs check")
	}
	browser, err := chrome.New(os.Getenv("WAYFARER_CHROME_WS"))
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	c := Client{Reader: browser}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	q := JobQuery{Keywords: "Product Designer", Page: 1}
	initial, err := c.Jobs(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Search == nil || len(initial.Search.SuggestedFacets) == 0 {
		t.Skip("no supported suggestions were shown for this search")
	}
	id := initial.Search.SuggestedFacets[0].ID
	q.Filters.SuggestedFacetIDs = []string{id}
	filtered, err := c.Jobs(ctx, q)
	if err != nil {
		t.Fatalf("could not reuse a discovered suggestion: %v", err)
	}
	if filtered.Search == nil || !slices.ContainsFunc(filtered.Search.SuggestedFacets, func(f JobSuggestedFacet) bool {
		return f.ID == id && f.Selected
	}) {
		t.Fatal("browser did not show the discovered suggestion as selected")
	}
}

func TestLiveJobCompanyLookup(t *testing.T) {
	if os.Getenv("WAYFARER_LIVE_JOBS") != "1" {
		t.Skip("set WAYFARER_LIVE_JOBS=1 and WAYFARER_CHROME_WS to run the live jobs check")
	}
	browser, err := chrome.New(os.Getenv("WAYFARER_CHROME_WS"))
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	c := Client{Reader: browser}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	companies, err := c.JobCompanies(ctx, "Microsoft")
	if err != nil {
		t.Fatal(err)
	}
	var microsoft string
	for _, item := range companies.Items {
		if item.Name == "Microsoft" && item.ID == "1035" {
			microsoft = item.ID
			break
		}
	}
	if microsoft == "" {
		t.Fatal("company picker did not return Microsoft's organization ID")
	}
	_, err = c.Jobs(ctx, JobQuery{Keywords: "DevOps", Page: 1, Filters: JobFilters{CompanyID: microsoft}})
	if err != nil {
		t.Fatalf("search did not retain the ID returned by the company picker: %v", err)
	}
}

// TestLiveJobSearch is deliberately opt-in: its queries open tabs in an
// authenticated browser. It does not save, dismiss, or apply to a listing.
func TestLiveJobSearch(t *testing.T) {
	if os.Getenv("WAYFARER_LIVE_JOBS") != "1" {
		t.Skip("set WAYFARER_LIVE_JOBS=1 and WAYFARER_CHROME_WS to run the live jobs check")
	}
	browser, err := chrome.New(os.Getenv("WAYFARER_CHROME_WS"))
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	for _, tc := range []struct {
		name string
		q    JobQuery
	}{
		{"search", JobQuery{Keywords: "DevOps", Page: 1}},
		{"page two", JobQuery{Keywords: "DevOps", Page: 2}},
		{"filtered", JobQuery{Keywords: "DevOps", Page: 1, Filters: JobFilters{Workplace: []string{"remote", "hybrid"}, PostedWithin: "month", EasyApply: true}}},
		{"under ten applicants", JobQuery{Keywords: "DevOps", Page: 1, Filters: JobFilters{Under10: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			audit := &liveJobsAudit{reader: browser}
			result, err := (Client{Reader: audit}).Jobs(ctx, tc.q)
			if err != nil {
				t.Fatalf("browser search did not satisfy the Wayfarer contract: %v", err)
			}
			if result.Pagination == nil || result.Pagination.Page != tc.q.Page || result.Search == nil {
				t.Fatal("missing search or pagination metadata")
			}
			got := make([]string, 0, len(result.Items))
			for _, item := range result.Items {
				got = append(got, item.ID)
			}
			if !slices.Equal(got, audit.native) {
				t.Fatalf("browser order or membership differs: got %d cards, expected %d", len(got), len(audit.native))
			}
			if len(result.Items) == 0 && result.Search.MatchMode != "no_results" {
				t.Fatal("empty search without a matching upstream signal")
			}
			if result.Search.MatchMode == "broadened" && result.Search.Notice == "" {
				t.Fatal("broadened results without an upstream notice")
			}
		})
	}
}
