package linkedin

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

type readerFunc func(context.Context, string, string) (json.RawMessage, error)

func (f readerFunc) Read(ctx context.Context, pageURL, expression string) (json.RawMessage, error) {
	return f(ctx, pageURL, expression)
}

func TestJobDescriptionBlocks(t *testing.T) {
	for _, blocks := range []string{"", `,"description_blocks":[{"kind":"list","ordered":false,"items":["Go","Kubernetes"]}]`} {
		c := Client{Reader: readerFunc(func(_ context.Context, raw, expr string) (json.RawMessage, error) {
			if raw != "https://www.linkedin.com/jobs/view/42/" || !strings.Contains(expr, "job.description_blocks") {
				t.Fatalf("unexpected job read: %q", raw)
			}
			return json.RawMessage(`{"job":{"id":"42","title":"Engineer","description":"Requirements"` + blocks + `}}`), nil
		})}
		job, err := c.Job(context.Background(), "42")
		if err != nil || job.Description != "Requirements" {
			t.Fatalf("unexpected job: %+v (%v)", job, err)
		}
		out, err := json.Marshal(job)
		if err != nil || strings.Contains(string(out), `"description_blocks"`) != (blocks != "") ||
			(blocks != "" && !strings.Contains(string(out), `"ordered":false,"items":["Go","Kubernetes"]`)) {
			t.Fatalf("unexpected job JSON: %s (%v)", out, err)
		}
	}
}

func TestJobsPagination(t *testing.T) {
	for _, tc := range []struct {
		name   string
		query  JobQuery
		path   string
		offset string
		body   string
		more   bool
		next   int
		bad    bool
	}{
		{"recommendations", JobQuery{}, "/jobs/", "", `{"items":[],"page":0}`, false, 0, false},
		{"first page", JobQuery{Keywords: "Go", Page: 1}, "/jobs/search-results/", "", `{"items":[],"page":1,"query_evidence":{"keywords":["Go"]}}`, false, 0, false},
		{"second page", JobQuery{Keywords: "C++ & Go", Page: 2}, "/jobs/search-results/", "25", `{"items":[{"id":"42"}],"page":2,"has_more":true,"query_evidence":{"keywords":["C++ & Go"],"start":["25"]}}`, true, 3, false},
		{"page cap", JobQuery{Keywords: "Go", Page: 100}, "/jobs/search-results/", "2475", `{"items":[{"id":"42"}],"page":100,"has_more":true,"query_evidence":{"keywords":["Go"],"start":["2475"]}}`, false, 0, false},
		{"unexpected upstream page", JobQuery{Keywords: "Go", Page: 2}, "/jobs/search-results/", "25", `{"items":[{"id":"42"}],"page":1}`, false, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Client{Reader: readerFunc(func(_ context.Context, raw, expr string) (json.RawMessage, error) {
				u, err := url.Parse(raw)
				if err != nil || u.Host != "www.linkedin.com" || u.Path != tc.path ||
					u.Query().Get("keywords") != tc.query.Keywords || u.Query().Get("start") != tc.offset || expr == "" {
					t.Fatalf("unexpected browser request: %q (%v)", raw, err)
				}
				return json.RawMessage(tc.body), nil
			})}
			got, err := c.Jobs(context.Background(), tc.query)
			if (err != nil) != tc.bad {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.bad {
				return
			}
			if got.Items == nil || (got.Pagination == nil) != (tc.query.Page == 0) {
				t.Fatalf("unexpected result: %+v", got)
			}
			if p := got.Pagination; p != nil && (p.Page != tc.query.Page || p.HasMore != tc.more || p.NextPage != tc.next) {
				t.Fatalf("unexpected pagination: %+v", p)
			}
		})
	}
}
