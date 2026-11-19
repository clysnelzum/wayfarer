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

func TestFeedRead(t *testing.T) {
	provider := &fakeScriptProvider{}
	c := mustNewClient(t, readerFunc(func(_ context.Context, pageURL, expression string) (json.RawMessage, error) {
		if pageURL != defaultBaseURL+"/feed/foryou/" || expression == "" {
			t.Fatal("expected a bounded feed evaluation")
		}
		return json.RawMessage(`{"items":[{"id":"example1","text":"Alice, hello","content":"hello","author":{"kind":"person","slug":"alice","name":"Alice","url":"https://www.linkedin.com/in/alice/"}}],"sort":"recent","has_more":true,"load_status":"budget_reached"}`), nil
	}), WithScripts(provider))
	v, err := c.Feed(context.Background(), 8)
	if err != nil || len(v.Items) != 1 || v.Items[0].Author == nil || v.Items[0].Author.Slug != "alice" ||
		v.Items[0].Content != "hello" || !v.HasMore || v.LoadStatus != "budget_reached" {
		t.Fatalf("unexpected feed result: %+v (%v)", v, err)
	}
	if !reflect.DeepEqual(provider.calls, []scriptCall{{id: scripts.ScriptFeed, args: []any{8}}}) {
		t.Fatalf("unexpected script calls: %+v", provider.calls)
	}
}

func TestFeedInitialBatchAndEmptyItemsSerialization(t *testing.T) {
	c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
		return json.RawMessage(`{"items":[],"load_status":"initial"}`), nil
	}))
	v, err := c.Feed(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Items == nil || len(v.Items) != 0 {
		t.Fatalf("expected non-nil empty slice, got %#v", v.Items)
	}
	encoded, err := json.Marshal(v)
	if err != nil || !strings.Contains(string(encoded), `"items":[]`) || strings.Contains(string(encoded), `"has_more"`) {
		t.Fatalf("unexpected JSON output: %s (%v)", encoded, err)
	}
}

func TestFeedInvalidLimit(t *testing.T) {
	c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
		t.Fatal("unexpected reader call on invalid feed limit")
		return nil, nil
	}))
	for _, limit := range []int{-1, MaxFeedLimit + 1} {
		if _, err := c.Feed(context.Background(), limit); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("limit=%d: expected ErrInvalidInput, got %v", limit, err)
		}
	}
}

func TestFeedRejectsInvalidResults(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		data  string
	}{
		{"missing content", 0, `{"items":[{"id":"example1","text":"hello"}],"load_status":"initial"}`},
		{"initial status with limit", 8, `{"items":[{"id":"example1","text":"hello","content":""}],"load_status":"initial"}`},
		{"limit_reached without limit", 0, `{"items":[{"id":"example1","text":"hello","content":""}],"load_status":"limit_reached"}`},
		{"duplicate id", 8, `{"items":[{"id":"example1","text":"hello","content":""},{"id":"example1","text":"hello","content":""}],"load_status":"no_progress"}`},
		{"missing id", 8, `{"items":[{"text":"hello","content":""}],"load_status":"no_progress"}`},
		{"missing text", 8, `{"items":[{"id":"example1","text":"  ","content":""}],"load_status":"no_progress"}`},
		{"missing items", 8, `{"load_status":"no_more_observed"}`},
		{"too many items", 1, `{"items":[{"id":"a","text":"1","content":""},{"id":"b","text":"2","content":""}],"load_status":"limit_reached"}`},
		{"unrecognized load_status", 8, `{"items":[],"load_status":"unknown"}`},
		{"unrecognized sort", 8, `{"items":[],"sort":"chronological","load_status":"no_more_observed"}`},
		{"invalid url", 8, `{"items":[{"id":"a","text":"1","content":"","url":"/relative"}],"load_status":"no_more_observed"}`},
		{"invalid author kind", 8, `{"items":[{"id":"a","text":"1","content":"","author":{"kind":"school","slug":"uba","name":"UBA","url":"https://www.linkedin.com/school/uba/"}}],"load_status":"no_more_observed"}`},
		{"invalid media type", 8, `{"items":[{"id":"a","text":"1","content":"","media":[{"type":"audio"}]}],"load_status":"no_more_observed"}`},
		{"negative engagement", 8, `{"items":[{"id":"a","text":"1","content":"","engagement":{"reaction_count":-1}}],"load_status":"no_more_observed"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
				return json.RawMessage(tc.data), nil
			}))
			_, err := c.Feed(context.Background(), tc.limit)
			assertContractError(t, err)
		})
	}
}
