package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestProfileAndCompanyAdapters(t *testing.T) {
	responses := map[string]string{
		base + "/in/example-person/":           `{"profile":{"slug":"example-person","name":"Example Person","headline":"Engineer","url":"ignored"}}`,
		base + "/company/example-co/about/":    `{"company":{"slug":"example-co","name":"Example Company","industry":"Technology","url":"ignored"}}`,
		base + "/in/missing/":                  `{"not_found":true}`,
		base + "/company/missing/about/":       `{"not_found":true}`,
		base + "/company/wrong-content/about/": `{"company":{"slug":"another-company","name":"Another Company"}}`,
	}
	c := Client{Reader: readerFunc(func(_ context.Context, pageURL, expression string) (json.RawMessage, error) {
		body, ok := responses[pageURL]
		if !ok || expression == "" {
			t.Fatalf("unexpected upstream query %q", pageURL)
		}
		return json.RawMessage(body), nil
	})}
	ctx := context.Background()
	p, err := c.Profile(ctx, "example-person")
	if err != nil || p.URL != base+"/in/example-person/" || p.Headline != "Engineer" {
		t.Fatalf("unexpected personal profile: %+v (%v)", p, err)
	}
	co, err := c.Company(ctx, "example-co")
	if err != nil || co.URL != base+"/company/example-co/" || co.Industry != "Technology" {
		t.Fatalf("unexpected company profile: %+v (%v)", co, err)
	}
	if _, err := c.Profile(ctx, "missing"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected missing profile error, got %v", err)
	}
	if _, err := c.Company(ctx, "missing"); !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("expected missing company error, got %v", err)
	}
	if _, err := c.Company(ctx, "wrong-content"); err == nil {
		t.Fatal("expected a slug mismatch error")
	}
	if _, err := c.Profile(ctx, "invalid/slug"); err == nil {
		t.Fatal("expected an invalid slug error without contacting upstream")
	}
}
