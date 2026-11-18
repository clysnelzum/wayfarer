package linkedin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestJobCompanyLookup(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		wantItems      int
		wantError      bool
	}{
		{"suggestion", `{"items":[{"id":"1035","name":"Microsoft"}],"status":"suggestions"}`, 1, false},
		{"empty", `{"items":[],"status":"no_suggestions_observed"}`, 0, false},
		{"unknown format", `{"status":"unrecognized"}`, 0, true},
		{"missing items", `{"status":"no_suggestions_observed"}`, 0, true},
		{"bad ID", `{"items":[{"id":"microsoft","name":"Microsoft"}],"status":"suggestions"}`, 0, true},
		{"duplicate IDs", `{"items":[{"id":"1035","name":"Microsoft"},{"id":"1035","name":"Microsoft"}],"status":"suggestions"}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Client{Reader: readerFunc(func(_ context.Context, raw, expr string) (json.RawMessage, error) {
				if raw != base+jobTypeaheadPath || !strings.Contains(expr, `)("Company \"Name\"", (`) {
					t.Fatal("expected an encoded query and the fixed job typeahead page")
				}
				return json.RawMessage(tc.response), nil
			})}
			got, err := c.JobCompanies(context.Background(), `Company "Name"`)
			if (err != nil) != tc.wantError || len(got.Items) != tc.wantItems {
				t.Fatalf("got %+v (%v)", got, err)
			}
		})
	}
}
