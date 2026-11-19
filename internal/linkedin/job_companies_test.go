package linkedin

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/clysnelzum/wayfarer/internal/linkedin/scripts"
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
			provider := &fakeScriptProvider{}
			c := Client{
				Scripts: provider,
				Reader: readerFunc(func(_ context.Context, raw, expr string) (json.RawMessage, error) {
					if raw != base+jobTypeaheadPath || expr == "" {
						t.Fatal("expected an expression and the fixed job typeahead page")
					}
					return json.RawMessage(tc.response), nil
				}),
			}
			got, err := c.JobCompanies(context.Background(), `Company "Name"`)
			if (err != nil) != tc.wantError || len(got.Items) != tc.wantItems {
				t.Fatalf("got %+v (%v)", got, err)
			}
			if !reflect.DeepEqual(provider.calls, []scriptCall{{id: scripts.ScriptJobCompanies, args: []any{`Company "Name"`}}}) {
				t.Fatalf("unexpected script calls: %+v", provider.calls)
			}
		})
	}
}
