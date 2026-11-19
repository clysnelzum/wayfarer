package linkedin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin/scripts"
)

// TestLiveProfilesAndCompanies is opt-in: it navigates temporary tabs in an
// authenticated browser to verify personal and company profile extraction.
func TestLiveProfilesAndCompanies(t *testing.T) {
	if os.Getenv("WAYFARER_LIVE_PROFILES") != "1" && os.Getenv("WAYFARER_LIVE_JOBS") != "1" {
		t.Skip("set WAYFARER_LIVE_PROFILES=1 (or WAYFARER_LIVE_JOBS=1) and WAYFARER_CHROME_WS to run the live profiles check")
	}
	browser, err := chrome.New(os.Getenv("WAYFARER_CHROME_WS"), chrome.WithAuthGuard(scripts.DefaultRegistry().AuthGuard()))
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	c, err := NewClient(browser)
	if err != nil {
		t.Fatal(err)
	}

	for _, slug := range []string{"microsoft", "mercadolibre"} {
		t.Run("company/"+slug, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			co, err := c.Company(ctx, slug)
			if err != nil {
				t.Fatalf("Company(%q) failed: %v", slug, err)
			}
			if co.Slug == "" || co.URL == "" || co.Name == "" {
				t.Fatalf("Company(%q) missing required fields: %+v", slug, co)
			}
		})
	}

	for _, slug := range []string{"satyanadella", "gergelyorosz"} {
		t.Run("profile/"+slug, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			pr, err := c.Profile(ctx, slug)
			if err != nil {
				t.Fatalf("Profile(%q) failed: %v", slug, err)
			}
			if pr.Slug == "" || pr.URL == "" || pr.Name == "" {
				t.Fatalf("Profile(%q) missing required fields: %+v", slug, pr)
			}
		})
	}
}
