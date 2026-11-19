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

func TestJobLocationLookup(t *testing.T) {
	for _, tc := range []struct {
		name      string
		response  string
		wantItems int
		wantError bool
	}{
		{"suggestion", `{"items":[{"id":"90009870","name":"Greater Buenos Aires"}],"status":"suggestions"}`, 1, false},
		{"empty", `{"items":[],"status":"no_suggestions_observed"}`, 0, false},
		{"unknown status", `{"items":[],"status":"unknown"}`, 0, true},
		{"missing items", `{"status":"no_suggestions_observed"}`, 0, true},
		{"empty items with suggestions status", `{"items":[],"status":"suggestions"}`, 0, true},
		{"non-empty items with empty status", `{"items":[{"id":"90009870","name":"Buenos Aires"}],"status":"no_suggestions_observed"}`, 0, true},
		{"bad location ID", `{"items":[{"id":"0123","name":"Buenos Aires"}],"status":"suggestions"}`, 0, true},
		{"non-numeric ID", `{"items":[{"id":"ba","name":"Buenos Aires"}],"status":"suggestions"}`, 0, true},
		{"empty name", `{"items":[{"id":"90009870","name":" "}],"status":"suggestions"}`, 0, true},
		{"duplicate IDs", `{"items":[{"id":"90009870","name":"Buenos Aires"},{"id":"90009870","name":"Greater Buenos Aires"}],"status":"suggestions"}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &fakeScriptProvider{}
			c := mustNewClient(t, readerFunc(func(_ context.Context, raw, expression string) (json.RawMessage, error) {
				if raw != defaultBaseURL+jobTypeaheadPath || expression == "" {
					t.Fatal("expected a non-empty expression and the fixed job typeahead page")
				}
				return json.RawMessage(tc.response), nil
			}), WithScripts(provider))
			got, err := c.JobLocations(context.Background(), `Côte d'Azur "France"`)
			if tc.wantError {
				assertContractError(t, err)
				return
			}
			if err != nil || len(got.Items) != tc.wantItems || got.Items == nil {
				t.Fatalf("unexpected location lookup: %+v (%v)", got, err)
			}
			if tc.wantItems == 1 && got.Items[0].ID != "90009870" {
				t.Fatalf("unexpected location item: %+v", got.Items[0])
			}
			if tc.wantItems == 0 {
				encoded, err := json.Marshal(got)
				if err != nil || !strings.Contains(string(encoded), `"items":[]`) {
					t.Fatalf("expected non-nil empty items in JSON: %s (%v)", encoded, err)
				}
			}
			if !reflect.DeepEqual(provider.calls, []scriptCall{{id: scripts.ScriptJobLocations, args: []any{`Côte d'Azur "France"`}}}) {
				t.Fatalf("unexpected script calls: %+v", provider.calls)
			}
		})
	}
}

func TestJobLocationsInvalidInput(t *testing.T) {
	c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
		t.Fatal("unexpected reader call on invalid location query")
		return nil, nil
	}))
	for _, query := range []string{"", "a", "Buenos ", "Buenos\tAires", strings.Repeat("x", 121)} {
		if _, err := c.JobLocations(context.Background(), query); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("query=%q: expected ErrInvalidInput, got %v", query, err)
		}
	}
}
