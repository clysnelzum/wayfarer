package scripts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"sync"
)

// ScriptID identifies a LinkedIn browser extractor script or composed invocation.
type ScriptID string

const (
	ScriptFeed              ScriptID = "feed"
	ScriptJobs              ScriptID = "jobs"
	ScriptJobCards          ScriptID = "job_cards"
	ScriptJobDetail         ScriptID = "job"
	ScriptJobDescription    ScriptID = "job_description"
	ScriptJobLocations      ScriptID = "job_locations"
	ScriptJobCompanies      ScriptID = "job_companies"
	ScriptJobCompanyRecords ScriptID = "job_company_records"
	ScriptProfile           ScriptID = "profile"
	ScriptCompany           ScriptID = "company"
)

// AllScriptIDs lists every required extractor script in deterministic order.
var AllScriptIDs = []ScriptID{
	ScriptFeed,
	ScriptJobs,
	ScriptJobCards,
	ScriptJobDetail,
	ScriptJobDescription,
	ScriptJobLocations,
	ScriptJobCompanies,
	ScriptJobCompanyRecords,
	ScriptProfile,
	ScriptCompany,
}

const linkedInAuthGuard = `() => location.origin !== 'https://www.linkedin.com' || /^\/(login|uas|checkpoint|authwall)(\/|$)/.test(location.pathname) || Boolean(document.querySelector('form[action*="login-submit"]'))`

// Invocation holds the target script identifier and composed JavaScript
// expression ready to be evaluated by the browser driver.
type Invocation struct {
	ID         ScriptID
	Expression string
}

// Provider builds JavaScript extractor invocations and supplies the LinkedIn
// auth-wall predicate.
type Provider interface {
	Build(id ScriptID, args ...any) (Invocation, error)
	Feed(limit int) (Invocation, error)
	Jobs(limit int, source string) (Invocation, error)
	JobDetail() (Invocation, error)
	JobLocations(query string) (Invocation, error)
	JobCompanies(query string) (Invocation, error)
	Profile() (Invocation, error)
	Company() (Invocation, error)
	AuthGuard() string
}

type registryConfig struct {
	override fs.FS
}

// Option configures NewRegistry.
type Option func(*registryConfig)

// WithOverrideFS layers an optional filesystem on top of the embedded scripts.
// Missing files fall back to the embedded FS; all other read errors fail fast.
func WithOverrideFS(override fs.FS) Option {
	return func(cfg *registryConfig) {
		cfg.override = override
	}
}

// Registry loads, validates, digests, and composes LinkedIn extractor scripts.
type Registry struct {
	sources map[ScriptID]string
	digests map[ScriptID]string
}

var defaultRegistry = sync.OnceValues(func() (*Registry, error) {
	return NewRegistry()
})

// DefaultRegistry returns the singleton Registry backed by the embedded scripts.
func DefaultRegistry() *Registry {
	reg, err := defaultRegistry()
	if err != nil {
		panic(fmt.Sprintf("linkedin/scripts: load embedded scripts: %v", err))
	}
	return reg
}

// NewRegistry constructs a Registry from the embedded scripts, optionally
// overridden by an overlay fs.FS.
func NewRegistry(opts ...Option) (*Registry, error) {
	var cfg registryConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	sources := make(map[ScriptID]string, len(AllScriptIDs))
	digests := make(map[ScriptID]string, len(AllScriptIDs))
	for _, id := range AllScriptIDs {
		filename := string(id) + ".js"
		data, err := readScript(cfg.override, filename)
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return nil, fmt.Errorf("script %s is empty", filename)
		}
		sum := sha256.Sum256(data)
		sources[id] = string(data)
		digests[id] = hex.EncodeToString(sum[:])
	}

	return &Registry{
		sources: sources,
		digests: digests,
	}, nil
}

func readScript(override fs.FS, filename string) ([]byte, error) {
	if override != nil {
		data, err := fs.ReadFile(override, filename)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("read override script %s: %w", filename, err)
		}
	}
	data, err := fs.ReadFile(FS, filename)
	if err != nil {
		return nil, fmt.Errorf("read embedded script %s: %w", filename, err)
	}
	return data, nil
}

// Digests returns a copy of the SHA-256 hex digests keyed by ScriptID.
func (r *Registry) Digests() map[ScriptID]string {
	return maps.Clone(r.digests)
}

// AuthGuard returns the JavaScript predicate function that detects LinkedIn
// auth walls, checkpoints, and off-origin redirects.
func (r *Registry) AuthGuard() string {
	return linkedInAuthGuard
}

// Feed builds the feed extractor invocation with the given item limit.
func (r *Registry) Feed(limit int) (Invocation, error) {
	return r.Build(ScriptFeed, limit)
}

// Jobs builds the jobs search/recommendations extractor invocation.
func (r *Registry) Jobs(limit int, source string) (Invocation, error) {
	return r.Build(ScriptJobs, limit, source)
}

// JobDetail builds the single-job detail extractor invocation.
func (r *Registry) JobDetail() (Invocation, error) {
	return r.Build(ScriptJobDetail)
}

// JobLocations builds the job location typeahead extractor invocation.
func (r *Registry) JobLocations(query string) (Invocation, error) {
	return r.Build(ScriptJobLocations, query)
}

// JobCompanies builds the job company typeahead extractor invocation.
func (r *Registry) JobCompanies(query string) (Invocation, error) {
	return r.Build(ScriptJobCompanies, query)
}

// Profile builds the personal profile extractor invocation.
func (r *Registry) Profile() (Invocation, error) {
	return r.Build(ScriptProfile)
}

// Company builds the company profile extractor invocation.
func (r *Registry) Company() (Invocation, error) {
	return r.Build(ScriptCompany)
}

// Build constructs an Invocation for the given ScriptID and arguments.
// Arguments are encoded uniformly via json.Marshal.
func (r *Registry) Build(id ScriptID, args ...any) (Invocation, error) {
	src, ok := r.sources[id]
	if !ok {
		return Invocation{}, fmt.Errorf("unknown script %q", id)
	}

	switch id {
	case ScriptFeed:
		limit, err := requireArg[int](id, args, 0, 1)
		if err != nil {
			return Invocation{}, err
		}
		encodedLimit, err := encodeJSArg(limit)
		if err != nil {
			return Invocation{}, err
		}
		return Invocation{
			ID:         id,
			Expression: "() => (" + src + ")(" + encodedLimit + ")",
		}, nil

	case ScriptJobs:
		if len(args) != 2 {
			return Invocation{}, fmt.Errorf("script %q expects 2 arguments (limit int, source string), got %d", id, len(args))
		}
		limit, err := requireArg[int](id, args, 0, 2)
		if err != nil {
			return Invocation{}, err
		}
		source, err := requireArg[string](id, args, 1, 2)
		if err != nil {
			return Invocation{}, err
		}
		encodedLimit, err := encodeJSArg(limit)
		if err != nil {
			return Invocation{}, err
		}
		encodedSource, err := encodeJSArg(source)
		if err != nil {
			return Invocation{}, err
		}
		return Invocation{
			ID:         id,
			Expression: "() => (" + src + ")((" + r.sources[ScriptJobCards] + "), " + encodedLimit + ", " + encodedSource + ")",
		}, nil

	case ScriptJobDetail:
		if len(args) != 0 {
			return Invocation{}, fmt.Errorf("script %q expects 0 arguments, got %d", id, len(args))
		}
		return Invocation{
			ID:         id,
			Expression: "() => (" + src + ")((" + r.sources[ScriptJobCards] + "), (" + r.sources[ScriptJobDescription] + "))",
		}, nil

	case ScriptJobLocations:
		query, err := requireArg[string](id, args, 0, 1)
		if err != nil {
			return Invocation{}, err
		}
		encodedQuery, err := encodeJSArg(query)
		if err != nil {
			return Invocation{}, err
		}
		return Invocation{
			ID:         id,
			Expression: "() => (" + src + ")(" + encodedQuery + ")",
		}, nil

	case ScriptJobCompanies:
		query, err := requireArg[string](id, args, 0, 1)
		if err != nil {
			return Invocation{}, err
		}
		encodedQuery, err := encodeJSArg(query)
		if err != nil {
			return Invocation{}, err
		}
		return Invocation{
			ID:         id,
			Expression: "() => (" + src + ")(" + encodedQuery + ", (" + r.sources[ScriptJobCompanyRecords] + "))",
		}, nil

	case ScriptProfile, ScriptCompany, ScriptJobCards, ScriptJobDescription, ScriptJobCompanyRecords:
		if len(args) != 0 {
			return Invocation{}, fmt.Errorf("script %q expects 0 arguments, got %d", id, len(args))
		}
		return Invocation{
			ID:         id,
			Expression: src,
		}, nil

	default:
		return Invocation{}, fmt.Errorf("unknown script %q", id)
	}
}

func requireArg[T any](id ScriptID, args []any, idx, wantLen int) (T, error) {
	var zero T
	if len(args) != wantLen {
		return zero, fmt.Errorf("script %q expects %d argument(s), got %d", id, wantLen, len(args))
	}
	v, ok := args[idx].(T)
	if !ok {
		return zero, fmt.Errorf("script %q argument %d has type %T, want %T", id, idx, args[idx], zero)
	}
	return v, nil
}

func encodeJSArg(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode script argument: %w", err)
	}
	return string(b), nil
}
