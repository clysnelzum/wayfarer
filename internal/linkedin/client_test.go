package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin/scripts"
)

type readerFunc func(context.Context, string, string) (json.RawMessage, error)

func (f readerFunc) Read(ctx context.Context, pageURL, expression string) (json.RawMessage, error) {
	return f(ctx, pageURL, expression)
}

type renderedReaderMock struct {
	readFn         func(context.Context, string, string) (json.RawMessage, error)
	readRenderedFn func(context.Context, string, string) (json.RawMessage, error)
	readCalls      []string
	renderedCalls  []string
}

var _ chrome.RenderedPageReader = (*renderedReaderMock)(nil)

func (m *renderedReaderMock) Read(ctx context.Context, pageURL, expression string) (json.RawMessage, error) {
	m.readCalls = append(m.readCalls, pageURL)
	if m.readFn != nil {
		return m.readFn(ctx, pageURL, expression)
	}
	return nil, errors.New("unexpected Read call")
}

func (m *renderedReaderMock) ReadRendered(ctx context.Context, pageURL, expression string) (json.RawMessage, error) {
	m.renderedCalls = append(m.renderedCalls, pageURL)
	if m.readRenderedFn != nil {
		return m.readRenderedFn(ctx, pageURL, expression)
	}
	return nil, errors.New("unexpected ReadRendered call")
}

type scriptCall struct {
	id   scripts.ScriptID
	args []any
}

type fakeScriptProvider struct {
	calls []scriptCall
}

func (f *fakeScriptProvider) record(id scripts.ScriptID, args ...any) (scripts.Invocation, error) {
	f.calls = append(f.calls, scriptCall{id: id, args: args})
	return scripts.Invocation{
		ID:         id,
		Expression: "() => ({stub: " + strconv.Quote(string(id)) + "})",
	}, nil
}

func (f *fakeScriptProvider) Build(id scripts.ScriptID, args ...any) (scripts.Invocation, error) {
	return f.record(id, args...)
}
func (f *fakeScriptProvider) Feed(limit int) (scripts.Invocation, error) {
	return f.record(scripts.ScriptFeed, limit)
}
func (f *fakeScriptProvider) Jobs(limit int, source string) (scripts.Invocation, error) {
	return f.record(scripts.ScriptJobs, limit, source)
}
func (f *fakeScriptProvider) JobDetail() (scripts.Invocation, error) {
	return f.record(scripts.ScriptJobDetail)
}
func (f *fakeScriptProvider) JobLocations(query string) (scripts.Invocation, error) {
	return f.record(scripts.ScriptJobLocations, query)
}
func (f *fakeScriptProvider) JobCompanies(query string) (scripts.Invocation, error) {
	return f.record(scripts.ScriptJobCompanies, query)
}
func (f *fakeScriptProvider) Profile() (scripts.Invocation, error) {
	return f.record(scripts.ScriptProfile)
}
func (f *fakeScriptProvider) Company() (scripts.Invocation, error) {
	return f.record(scripts.ScriptCompany)
}
func (f *fakeScriptProvider) AuthGuard() string {
	return "() => false"
}

func mustNewClient(t *testing.T, reader chrome.PageReader, opts ...Option) *Client {
	t.Helper()
	c, err := NewClient(reader, opts...)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	return c
}

func assertContractError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected contract error, got nil")
	}
	if !errors.Is(err, ErrUpstreamContract) {
		t.Fatalf("expected errors.Is(err, ErrUpstreamContract), got %v", err)
	}
	var ce *ContractError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ContractError, got %T (%v)", err, err)
	}
}

func TestNewClientValidationAndOptions(t *testing.T) {
	if _, err := NewClient(nil); err == nil {
		t.Fatal("expected error for nil reader")
	}
	for _, badBase := range []string{"", "ftp://www.linkedin.com", "https://", "https://user:pass@www.linkedin.com", "https://www.linkedin.com?x=1", "https://www.linkedin.com#frag"} {
		if _, err := NewClient(readerFunc(nil), WithBaseURL(badBase)); err == nil {
			t.Fatalf("expected error for invalid baseURL %q", badBase)
		}
	}

	provider := &fakeScriptProvider{}
	c := mustNewClient(
		t,
		readerFunc(func(_ context.Context, raw, _ string) (json.RawMessage, error) {
			if raw != "https://sandbox.linkedin.com/jobs/view/42/" {
				t.Fatalf("unexpected custom base URL target: %q", raw)
			}
			return json.RawMessage(`{"job":{"id":"42","title":"Engineer","company":"ACME","description":"Build"}}`), nil
		}),
		WithScripts(provider),
		WithBaseURL("https://sandbox.linkedin.com/"),
		WithLogger(nil),
	)
	if c.logger == nil {
		t.Fatal("expected non-nil logger even when WithLogger(nil) is used")
	}
	job, err := c.Job(context.Background(), "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.URL != "https://sandbox.linkedin.com/jobs/view/42/" {
		t.Fatalf("expected canonicalized custom base URL, got %q", job.URL)
	}

	// Default logger / scripts / base URL.
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	def := mustNewClient(t, readerFunc(nil), WithScripts(nil), WithLogger(discardLogger))
	if def.baseURL != defaultBaseURL || def.scripts == nil || def.logger != discardLogger {
		t.Fatalf("unexpected client defaults: %+v", def)
	}

	// Zero/nil client guard should return an error rather than panic.
	var nilClient *Client
	if _, err := nilClient.Feed(context.Background(), 0); err == nil {
		t.Fatal("expected error calling method on nil *Client")
	}
}

func TestTransportErrorTranslation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		chromeErr error
		want      error
	}{
		{"auth required", chrome.ErrAuthRequired, ErrAuthRequired},
		{"wrapped auth", fmt.Errorf("redirected: %w", chrome.ErrAuth), ErrAuthRequired},
		{"busy", chrome.ErrBusy, ErrUpstreamBusy},
		{"debugger unavailable", chrome.ErrDebuggerUnavailable, ErrDebuggerUnavailable},
		{"debugger unavailable wrapping deadline exceeded", fmt.Errorf("%w: %w", chrome.ErrDebuggerUnavailable, context.DeadlineExceeded), ErrDebuggerUnavailable},
		{"closed", chrome.ErrClosed, ErrDebuggerUnavailable},
		{"deadline exceeded", context.DeadlineExceeded, context.DeadlineExceeded},
		{"canceled", context.Canceled, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
				return nil, tc.chromeErr
			}))
			_, err := c.Feed(context.Background(), 0)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected errors.Is(%v, %v)", err, tc.want)
			}
			if (tc.chromeErr == context.DeadlineExceeded || tc.chromeErr == context.Canceled) && err != tc.chromeErr {
				t.Fatalf("expected context error to remain unwrapped, got %v", err)
			}
			if tc.chromeErr == chrome.ErrAuthRequired && !errors.Is(err, chrome.ErrAuthRequired) {
				t.Fatalf("expected underlying chrome.ErrAuthRequired to remain wrapped, got %v", err)
			}
		})
	}
}

func TestContractErrorUnwrapAndWirePayloadErrors(t *testing.T) {
	inner := errors.New("syntax failure")
	ce := &ContractError{Operation: "feed", Reason: "bad json", Err: inner}
	if !errors.Is(ce, ErrUpstreamContract) || !errors.Is(ce, inner) {
		t.Fatalf("ContractError did not unwrap both sentinel and inner error: %v", ce)
	}
	if !strings.Contains(ce.Error(), "feed") || !strings.Contains(ce.Error(), "bad json") || !strings.Contains(ce.Error(), "syntax failure") {
		t.Fatalf("unexpected ContractError message: %q", ce.Error())
	}

	ceNoInner := &ContractError{Operation: "job", Reason: "missing field"}
	if !errors.Is(ceNoInner, ErrUpstreamContract) {
		t.Fatalf("ContractError without inner error must unwrap to ErrUpstreamContract")
	}

	for _, raw := range []string{``, `null`, `{invalid json`, `[]`} {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		c := mustNewClient(t, readerFunc(func(context.Context, string, string) (json.RawMessage, error) {
			return json.RawMessage(raw), nil
		}), WithLogger(logger))
		_, err := c.Feed(context.Background(), 0)
		assertContractError(t, err)
		logged := logs.String()
		if !strings.Contains(logged, `"msg":"linkedin wire contract error"`) || !strings.Contains(logged, `"op":"feed"`) {
			t.Fatalf("expected wire contract error log, got: %s", logged)
		}
	}
}

func TestRenderedPageReaderRouting(t *testing.T) {
	mock := &renderedReaderMock{
		readFn: func(_ context.Context, pageURL, _ string) (json.RawMessage, error) {
			switch pageURL {
			case defaultBaseURL + "/jobs/":
				return json.RawMessage(`{"items":[],"page":0}`), nil
			case defaultBaseURL + "/jobs/search-results/?keywords=Go":
				return json.RawMessage(`{"items":[],"page":1,"match_mode":"no_results","query_evidence":{"keywords":["Go"]}}`), nil
			case defaultBaseURL + "/company/example-co/about/":
				return json.RawMessage(`{"company":{"slug":"example-co","name":"Example Co"}}`), nil
			default:
				return nil, fmt.Errorf("unexpected Read URL %q", pageURL)
			}
		},
		readRenderedFn: func(_ context.Context, pageURL, _ string) (json.RawMessage, error) {
			switch pageURL {
			case defaultBaseURL + "/jobs/":
				return json.RawMessage(`{"items":[],"page":0,"recommendation_status":"no_more_observed"}`), nil
			case defaultBaseURL + "/in/example-person/":
				return json.RawMessage(`{"profile":{"slug":"example-person","name":"Example Person"}}`), nil
			default:
				return nil, fmt.Errorf("unexpected ReadRendered URL %q", pageURL)
			}
		},
	}

	c := mustNewClient(t, mock)
	ctx := context.Background()

	if _, err := c.Jobs(ctx, JobQuery{}); err != nil {
		t.Fatalf("initial Jobs failed: %v", err)
	}
	if _, err := c.Jobs(ctx, JobQuery{Keywords: "Go", Page: 1}); err != nil {
		t.Fatalf("search Jobs failed: %v", err)
	}
	if _, err := c.Company(ctx, "example-co"); err != nil {
		t.Fatalf("Company failed: %v", err)
	}
	if _, err := c.Jobs(ctx, JobQuery{RecommendationLimit: 10}); err != nil {
		t.Fatalf("limited Jobs failed: %v", err)
	}
	if _, err := c.Profile(ctx, "example-person"); err != nil {
		t.Fatalf("Profile failed: %v", err)
	}

	wantRead := []string{
		defaultBaseURL + "/jobs/",
		defaultBaseURL + "/jobs/search-results/?keywords=Go",
		defaultBaseURL + "/company/example-co/about/",
	}
	wantRendered := []string{
		defaultBaseURL + "/jobs/",
		defaultBaseURL + "/in/example-person/",
	}
	if !reflect.DeepEqual(mock.readCalls, wantRead) {
		t.Fatalf("readCalls = %v, want %v", mock.readCalls, wantRead)
	}
	if !reflect.DeepEqual(mock.renderedCalls, wantRendered) {
		t.Fatalf("renderedCalls = %v, want %v", mock.renderedCalls, wantRendered)
	}
}

func TestJobDescriptionBlocks(t *testing.T) {
	for _, blocks := range []string{"", `,"description_blocks":[{"kind":"list","ordered":false,"items":["Go","Kubernetes"]}]`} {
		provider := &fakeScriptProvider{}
		c := mustNewClient(t, readerFunc(func(_ context.Context, raw, expr string) (json.RawMessage, error) {
			if raw != "https://www.linkedin.com/jobs/view/42/" || expr == "" {
				t.Fatalf("unexpected job read: %q (expr=%q)", raw, expr)
			}
			return json.RawMessage(`{"job":{"id":"42","title":"Engineer","company":"ACME","description":"Requirements"` + blocks + `}}`), nil
		}), WithScripts(provider))
		job, err := c.Job(context.Background(), "42")
		if err != nil || job.Description != "Requirements" || job.URL != "https://www.linkedin.com/jobs/view/42/" {
			t.Fatalf("unexpected job: %+v (%v)", job, err)
		}
		if !reflect.DeepEqual(provider.calls, []scriptCall{{id: scripts.ScriptJobDetail}}) {
			t.Fatalf("unexpected script calls: %+v", provider.calls)
		}
		out, err := json.Marshal(job)
		if err != nil || strings.Contains(string(out), `"description_blocks"`) != (blocks != "") ||
			(blocks != "" && !strings.Contains(string(out), `"ordered":false,"items":["Go","Kubernetes"]`)) {
			t.Fatalf("unexpected job JSON: %s (%v)", out, err)
		}
	}
}

func TestJobsPagination(t *testing.T) {
	const item42 = `{"id":"42","title":"Engineer","company":"ACME","location":"Remote","url":"https://www.linkedin.com/jobs/view/42/"}`
	for _, tc := range []struct {
		name   string
		query  JobQuery
		path   string
		offset string
		body   string
		more   bool
		next   int
		bad    bool
	}{
		{"recommendations", JobQuery{}, "/jobs/", "", `{"items":[],"page":0}`, false, 0, false},
		{"first page", JobQuery{Keywords: "Go", Page: 1}, "/jobs/search-results/", "", `{"items":[],"page":1,"match_mode":"no_results","query_evidence":{"keywords":["Go"]}}`, false, 0, false},
		{"second page", JobQuery{Keywords: "C++ & Go", Page: 2}, "/jobs/search-results/", "25", `{"items":[` + item42 + `],"page":2,"has_more":true,"match_mode":"results","query_evidence":{"keywords":["C++ & Go"],"start":["25"]}}`, true, 3, false},
		{"page cap", JobQuery{Keywords: "Go", Page: 100}, "/jobs/search-results/", "2475", `{"items":[` + item42 + `],"page":100,"has_more":true,"match_mode":"results","query_evidence":{"keywords":["Go"],"start":["2475"]}}`, false, 0, false},
		{"unexpected upstream page", JobQuery{Keywords: "Go", Page: 2}, "/jobs/search-results/", "25", `{"items":[` + item42 + `],"page":1,"match_mode":"results"}`, false, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := mustNewClient(t, readerFunc(func(_ context.Context, raw, expr string) (json.RawMessage, error) {
				u, err := url.Parse(raw)
				if err != nil || u.Host != "www.linkedin.com" || u.Path != tc.path ||
					u.Query().Get("keywords") != tc.query.Keywords || u.Query().Get("start") != tc.offset || expr == "" {
					t.Fatalf("unexpected browser request: %q (%v)", raw, err)
				}
				return json.RawMessage(tc.body), nil
			}))
			got, err := c.Jobs(context.Background(), tc.query)
			if (err != nil) != tc.bad {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.bad {
				if !errors.Is(err, ErrSearchMismatch) {
					t.Fatalf("expected ErrSearchMismatch, got %v", err)
				}
				return
			}
			if got.Items == nil || (got.Pagination == nil) != (tc.query.Page == 0) {
				t.Fatalf("unexpected result: %+v", got)
			}
			if p := got.Pagination; p != nil && (p.Page != tc.query.Page || p.HasMore != tc.more || p.NextPage != tc.next) {
				t.Fatalf("unexpected pagination: %+v", p)
			}
		})
	}
}
