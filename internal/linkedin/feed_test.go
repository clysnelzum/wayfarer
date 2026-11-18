package linkedin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestFeedRead(t *testing.T) {
	c := Client{Reader: readerFunc(func(_ context.Context, pageURL, expression string) (json.RawMessage, error) {
		if pageURL != base+"/feed/foryou/" || !strings.HasSuffix(expression, ")(8)") {
			t.Fatal("expected a bounded feed evaluation")
		}
		return json.RawMessage(`{"items":[{"id":"example1","text":"Alice, hello","content":"hello","author":{"kind":"person","slug":"alice","name":"Alice","url":"https://www.linkedin.com/in/alice/"}}],"sort":"recent","has_more":true,"load_status":"budget_reached"}`), nil
	})}
	v, err := c.Feed(context.Background(), FeedQuery{Limit: 8})
	if err != nil || len(v.Items) != 1 || v.Items[0].Author == nil || v.Items[0].Author.Slug != "alice" ||
		v.Items[0].Content != "hello" || !v.HasMore || v.LoadStatus != "budget_reached" {
		t.Fatalf("unexpected feed result: %+v (%v)", v, err)
	}
}

func TestFeedRejectsInvalidResults(t *testing.T) {
	for _, data := range []string{
		`{"items":[{"id":"example1","text":"hello"}],"load_status":"initial"}`,
		`{"items":[{"id":"example1","text":"hello"},{"id":"example1","text":"hello"}],"load_status":"no_progress"}`,
		`{"items":[{"text":"hello"}],"load_status":"no_progress"}`,
	} {
		c := Client{Reader: readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
			return json.RawMessage(data), nil
		})}
		if _, err := c.Feed(context.Background(), FeedQuery{Limit: 8}); err == nil {
			t.Fatalf("accepted an invalid feed result: %s", data)
		}
	}
}
