package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/clysnelzum/wayfarer/internal/linkedin/scripts"
)

func TestJobCompanyLookup(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		wantItems      int
		wantError      bool
	}{
		{"suggestion", `{"items":[{"id":"1035","name":"Microsoft","industry":"Software Development"}],"status":"suggestions"}`, 1, false},
		{"empty", `{"items":[],"status":"no_suggestions_observed"}`, 0, false},
		{"unknown format", `{"status":"unrecognized"}`, 0, true},
		{"missing items", `{"status":"no_suggestions_observed"}`, 0, true},
		{"empty items with suggestions status", `{"items":[],"status":"suggestions"}`, 0, true},
		{"non-empty items with empty status", `{"items":[{"id":"1035","name":"Microsoft"}],"status":"no_suggestions_observed"}`, 0, true},
		{"bad ID", `{"items":[{"id":"microsoft","name":"Microsoft"}],"status":"suggestions"}`, 0, true},
		{"empty name", `{"items":[{"id":"1035","name":"  "}],"status":"suggestions"}`, 0, true},
		{"duplicate IDs", `{"items":[{"id":"1035","name":"Microsoft"},{"id":"1035","name":"Microsoft"}],"status":"suggestions"}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &fakeScriptProvider{}
			c := mustNewClient(t, readerFunc(func(_ context.Context, raw, expr string) (json.RawMessage, error) {
				if raw != defaultBaseURL+jobTypeaheadPath || expr == "" {
					t.Fatal("expected an expression and the fixed job typeahead page")
				}
				return json.RawMessage(tc.response), nil
			}), WithScripts(provider))
			got, err := c.JobCompanies(context.Background(), `Company "Name"`)
			if tc.wantError {
				assertContractError(t, err)
				return
			}
			if err != nil || len(got.Items) != tc.wantItems || got.Items == nil {
				t.Fatalf("got %+v (%v)", got, err)
			}
			if !reflect.DeepEqual(provider.calls, []scriptCall{{id: scripts.ScriptJobCompanies, args: []any{`Company "Name"`}}}) {
				t.Fatalf("unexpected script calls: %+v", provider.calls)
			}
			if tc.wantItems == 0 {
				encoded, err := json.Marshal(got)
				if err != nil || !strings.Contains(string(encoded), `"items":[]`) {
					t.Fatalf("expected non-nil empty items in JSON: %s (%v)", encoded, err)
				}
			}
		})
	}
}

func TestJobCompaniesInvalidInput(t *testing.T) {
	c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
		t.Fatal("unexpected reader call on invalid company query")
		return nil, nil
	}))
	for _, query := range []string{"", "a", " Microsoft", "Microsoft\n", strings.Repeat("x", 121)} {
		if _, err := c.JobCompanies(context.Background(), query); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("query=%q: expected ErrInvalidInput, got %v", query, err)
		}
	}
}
