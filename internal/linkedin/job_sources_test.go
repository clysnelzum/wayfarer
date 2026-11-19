package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestJobSourceFollowsNativePagingContext(t *testing.T) {
	for _, page := range []int{1, 2} {
		calls := 0
		c := Client{Reader: readerFunc(func(_ context.Context, raw, expr string) (json.RawMessage, error) {
			calls++
			if calls == 1 {
				if raw != base+"/jobs/" || !strings.HasSuffix(expr, `, 0, "preferences")`) {
					t.Fatalf("expected the native preferences link to be resolved first: %s", raw)
				}
				return json.RawMessage(`{"source":"preferences","origin":"JobSearchOrigin_PREFERENCES_LANDING","keywords":"DevOps or Software Engineer","locations":"90009872,101220015","seed":"101,102"}`), nil
			}
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			args := u.Query()
			if u.Path != "/jobs/search-results/" || args.Get("keywords") != "DevOps or Software Engineer" || args.Get("geoId") != "90009872,101220015" {
				t.Fatalf("expected native search controls, got %s", raw)
			}
			if page == 1 && (args.Get("originToLandingJobPostings") != "101,102" || args.Has("start")) ||
				page == 2 && (args.Has("originToLandingJobPostings") || args.Get("start") != "25") {
				t.Fatalf("unexpected pagination: %s", raw)
			}
			args.Set("origin", "PREFERENCES_LANDING") // Upstream normalizes its display name.
			return json.Marshal(map[string]any{"page": page, "items": []JobSummary{}, "match_mode": "no_results", "query_evidence": args})
		})}
		got, err := c.Jobs(context.Background(), JobQuery{Source: "preferences", Page: page})
		if err != nil || calls != 2 || got.Search == nil || got.Search.Source != "preferences" || got.Pagination.Page != page {
			t.Fatalf("unexpected result %+v, %d reads, error %v", got, calls, err)
		}
		wire, err := json.Marshal(got)
		if err != nil || strings.Contains(string(wire), "101,102") || strings.Contains(string(wire), "DevOps") {
			t.Fatalf("unexpected disclosure of native search inputs: %s", wire)
		}
	}
}

func TestPinnedJobSourceUsesOnlyTheSearchNavigation(t *testing.T) {
	link := JobSourceContext{Source: "preferences", Origin: "PREFERENCES_LANDING", Keywords: "Example", Locations: "10,20", Seed: "51,52"}
	calls := 0
	c := Client{Reader: readerFunc(func(_ context.Context, raw, _ string) (json.RawMessage, error) {
		calls++
		u, err := url.Parse(raw)
		if err != nil || u.Path != "/jobs/search-results/" || u.Query().Get("originToLandingJobPostings") != link.Seed {
			t.Fatalf("did not navigate directly with the pinned context: %s (%v)", raw, err)
		}
		return json.Marshal(map[string]any{"page": 1, "items": []JobSummary{}, "match_mode": "no_results", "query_evidence": u.Query()})
	})}
	got, err := c.Jobs(context.Background(), JobQuery{Source: link.Source, SourceContext: &link, Page: 1})
	if err != nil || got.Search == nil || got.Search.SourceContext != &link || calls != 1 {
		t.Fatalf("unexpected pinned search: %+v, %d calls (%v)", got, calls, err)
	}
}

func TestJobSourceNotOffered(t *testing.T) {
	c := Client{Reader: readerFunc(func(_ context.Context, raw, _ string) (json.RawMessage, error) {
		if raw != base+"/jobs/" {
			t.Fatal("must not navigate to a fallback query")
		}
		return json.RawMessage(`{"unavailable":true}`), nil
	})}
	if _, err := c.Jobs(context.Background(), JobQuery{Source: "profile_match", Page: 1}); !errors.Is(err, ErrJobSourceUnavailable) {
		t.Fatalf("expected an unavailable recommendation source, got %v", err)
	}
}

func TestJobSourceRejectsChangedContext(t *testing.T) {
	for _, key := range []string{"origin", "originToLandingJobPostings"} {
		c := Client{Reader: readerFunc(func(_ context.Context, raw, _ string) (json.RawMessage, error) {
			if raw == base+"/jobs/" {
				return json.RawMessage(`{"source":"profile_match","origin":"JobSearchOrigin_QUALIFICATION_LANDING","keywords":"DevOps","locations":"90009872","seed":"101,102"}`), nil
			}
			u, err := url.Parse(raw)
			if err != nil {
				return nil, err
			}
			actual := u.Query()
			actual.Del(key)
			return json.Marshal(map[string]any{"page": 1, "items": []JobSummary{}, "match_mode": "no_results", "query_evidence": actual})
		})}
		_, err := c.Jobs(context.Background(), JobQuery{Source: "profile_match", Page: 1})
		if !errors.Is(err, ErrSearchMismatch) {
			t.Errorf("expected a mismatch for missing %s, got %v", strconv.Quote(key), err)
		}
	}
}
