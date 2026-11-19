package scripts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

type failingFS struct {
	err error
}

func (f failingFS) Open(string) (fs.File, error) {
	return nil, f.err
}

func TestDefaultRegistryLoadsAllScriptsAndDigests(t *testing.T) {
	reg := DefaultRegistry()
	digests := reg.Digests()
	if len(digests) != len(AllScriptIDs) {
		t.Fatalf("got %d digests, want %d", len(digests), len(AllScriptIDs))
	}
	for _, id := range AllScriptIDs {
		digest, ok := digests[id]
		if !ok || len(digest) != 64 {
			t.Fatalf("missing or invalid SHA-256 digest for %q: %q", id, digest)
		}
		var inv Invocation
		var err error
		switch id {
		case ScriptFeed:
			inv, err = reg.Feed(5)
		case ScriptJobs:
			inv, err = reg.Jobs(10, "preferences")
		case ScriptJobLocations:
			inv, err = reg.JobLocations("Buenos Aires")
		case ScriptJobCompanies:
			inv, err = reg.JobCompanies("Microsoft")
		default:
			inv, err = reg.Build(id)
		}
		if err != nil {
			t.Fatalf("Build(%q) failed: %v", id, err)
		}
		if inv.ID != id || strings.TrimSpace(inv.Expression) == "" {
			t.Fatalf("unexpected invocation for %q: %+v", id, inv)
		}
	}

	// Digests returns a defensive copy.
	digests[ScriptFeed] = "mutated"
	if DefaultRegistry().Digests()[ScriptFeed] == "mutated" {
		t.Fatal("Digests() leaked internal map")
	}

	if guard := reg.AuthGuard(); !strings.Contains(guard, "https://www.linkedin.com") || !strings.Contains(guard, "login-submit") {
		t.Fatalf("unexpected auth guard expression: %q", guard)
	}
}

func TestOverrideFSOverridesSingleScriptAndFallsBack(t *testing.T) {
	customFeed := "async limit => ({items: [], load_status: 'initial', limit})"
	override := fstest.MapFS{
		"feed.js": {Data: []byte(customFeed)},
	}
	reg, err := NewRegistry(WithOverrideFS(override))
	if err != nil {
		t.Fatalf("NewRegistry with override failed: %v", err)
	}

	inv, err := reg.Feed(12)
	if err != nil {
		t.Fatalf("Feed failed: %v", err)
	}
	wantExpr := "() => (" + customFeed + ")(12)"
	if inv.Expression != wantExpr {
		t.Fatalf("got %q, want %q", inv.Expression, wantExpr)
	}

	wantSum := sha256.Sum256([]byte(customFeed))
	if got, want := reg.Digests()[ScriptFeed], hex.EncodeToString(wantSum[:]); got != want {
		t.Fatalf("override digest = %q, want %q", got, want)
	}

	// Non-overridden scripts still fall back to embedded FS.
	if got, want := reg.Digests()[ScriptProfile], DefaultRegistry().Digests()[ScriptProfile]; got != want {
		t.Fatalf("fallback profile digest = %q, want %q", got, want)
	}
	profileInv, err := reg.Profile()
	if err != nil || profileInv.Expression == "" {
		t.Fatalf("fallback Profile() failed: %+v (%v)", profileInv, err)
	}
}

func TestNewRegistryRejectsEmptyScriptAndNonNotExistErrors(t *testing.T) {
	if _, err := NewRegistry(WithOverrideFS(fstest.MapFS{
		"feed.js": {Data: []byte("   \n\t  ")},
	})); err == nil {
		t.Fatal("expected error for whitespace-only override script")
	}

	sentinel := errors.New("permission denied")
	if _, err := NewRegistry(WithOverrideFS(failingFS{err: sentinel})); !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

func TestBuildArgumentEncodingAndValidation(t *testing.T) {
	override := fstest.MapFS{
		"feed.js":                {Data: []byte("limit => limit")},
		"jobs.js":                {Data: []byte("(cards, limit, source) => ({cards, limit, source})")},
		"job_cards.js":           {Data: []byte("() => 'cards'")},
		"job.js":                 {Data: []byte("(cards, desc) => ({cards, desc})")},
		"job_description.js":     {Data: []byte("box => box")},
		"job_locations.js":       {Data: []byte("q => q")},
		"job_companies.js":       {Data: []byte("(q, rec) => ({q, rec})")},
		"job_company_records.js": {Data: []byte("b => b")},
		"profile.js":             {Data: []byte("async () => ({profile: {}})")},
		"company.js":             {Data: []byte("() => ({company: {}})")},
	}
	reg, err := NewRegistry(WithOverrideFS(override))
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	locInv, err := reg.JobLocations("Côte d'Azur \"France\"\n<script>")
	if err != nil {
		t.Fatalf("JobLocations failed: %v", err)
	}
	wantLoc := `() => (q => q)("Côte d'Azur \"France\"\n\u003cscript\u003e")`
	if locInv.Expression != wantLoc {
		t.Fatalf("got %q, want %q", locInv.Expression, wantLoc)
	}

	compInv, err := reg.JobCompanies(`ACME & "Co"`)
	if err != nil {
		t.Fatalf("JobCompanies failed: %v", err)
	}
	wantComp := `() => ((q, rec) => ({q, rec}))("ACME \u0026 \"Co\"", (b => b))`
	if compInv.Expression != wantComp {
		t.Fatalf("got %q, want %q", compInv.Expression, wantComp)
	}

	jobsInv, err := reg.Jobs(25, "profile_match")
	if err != nil {
		t.Fatalf("Jobs failed: %v", err)
	}
	wantJobs := `() => ((cards, limit, source) => ({cards, limit, source}))((() => 'cards'), 25, "profile_match")`
	if jobsInv.Expression != wantJobs {
		t.Fatalf("got %q, want %q", jobsInv.Expression, wantJobs)
	}

	detailInv, err := reg.JobDetail()
	if err != nil {
		t.Fatalf("JobDetail failed: %v", err)
	}
	wantDetail := `() => ((cards, desc) => ({cards, desc}))((() => 'cards'), (box => box))`
	if detailInv.Expression != wantDetail {
		t.Fatalf("got %q, want %q", detailInv.Expression, wantDetail)
	}

	// Argument validation errors.
	for _, tc := range []struct {
		name string
		id   ScriptID
		args []any
	}{
		{"unknown script", ScriptID("nope"), nil},
		{"feed missing arg", ScriptFeed, nil},
		{"feed wrong arg type", ScriptFeed, []any{"5"}},
		{"jobs wrong arity", ScriptJobs, []any{5}},
		{"jobs wrong source type", ScriptJobs, []any{5, 99}},
		{"locations wrong type", ScriptJobLocations, []any{42}},
		{"companies wrong arity", ScriptJobCompanies, []any{"a", "b"}},
		{"profile extra arg", ScriptProfile, []any{"extra"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := reg.Build(tc.id, tc.args...); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}
