package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// PageReader reads data from a page by evaluating a JavaScript expression.
type PageReader interface {
	Read(ctx context.Context, pageURL, expression string) (json.RawMessage, error)
}

// RenderedPageReader extends PageReader with visibility-emulated reads for
// pages whose DOM depends on layout or focus.
type RenderedPageReader interface {
	PageReader
	ReadRendered(ctx context.Context, pageURL, expression string) (json.RawMessage, error)
}

// ReadOptions configures a single Browser.ReadPage call.
type ReadOptions struct {
	// Rendered enables focus and viewport emulation for visibility-dependent pages.
	Rendered bool
	// Timeout overrides the browser's default timeout for this read when positive.
	Timeout time.Duration
}

// Option configures a Browser constructed with New.
type Option func(*browserConfig)

type browserConfig struct {
	allowedHosts    []string
	authGuard       string
	maxConcurrency  int
	standardTimeout time.Duration
	renderedTimeout time.Duration
	pollInterval    time.Duration
	logger          *slog.Logger
}

// WithAllowedHosts restricts page navigations to the provided host[:port] values.
// By default, only "www.linkedin.com" is allowed.
func WithAllowedHosts(hosts ...string) Option {
	return func(c *browserConfig) {
		c.allowedHosts = append([]string(nil), hosts...)
	}
}

// WithAuthGuard configures a JavaScript predicate injected into page evaluation
// to detect authentication walls. When the predicate returns a truthy value,
// ReadPage returns ErrAuthRequired. An empty predicate disables the check.
func WithAuthGuard(predicateJS string) Option {
	return func(c *browserConfig) {
		c.authGuard = predicateJS
	}
}

// WithMaxConcurrency sets the maximum number of concurrent page reads.
// Non-positive values are ignored. Defaults to 2.
func WithMaxConcurrency(n int) Option {
	return func(c *browserConfig) {
		if n > 0 {
			c.maxConcurrency = n
		}
	}
}

// WithStandardTimeout sets the default timeout for non-rendered reads.
// Non-positive values are ignored. Defaults to 25s.
func WithStandardTimeout(d time.Duration) Option {
	return func(c *browserConfig) {
		if d > 0 {
			c.standardTimeout = d
		}
	}
}

// WithRenderedTimeout sets the default timeout for rendered reads.
// Non-positive values are ignored. Defaults to 90s.
func WithRenderedTimeout(d time.Duration) Option {
	return func(c *browserConfig) {
		if d > 0 {
			c.renderedTimeout = d
		}
	}
}

// WithPollInterval sets the polling interval while waiting for a page extractor
// to produce a non-null result. Non-positive values are ignored. Defaults to 300ms.
func WithPollInterval(d time.Duration) Option {
	return func(c *browserConfig) {
		if d > 0 {
			c.pollInterval = d
		}
	}
}

// WithLogger sets the structured logger used for non-fatal warnings such as tab
// cleanup failures. Passing nil disables logging; by default slog.Default is used.
func WithLogger(logger *slog.Logger) Option {
	return func(c *browserConfig) {
		if logger == nil {
			c.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			return
		}
		c.logger = logger
	}
}

var (
	// ErrAuthRequired indicates that Chrome was redirected to an untrusted origin
	// or triggered the configured authentication guard.
	ErrAuthRequired = errors.New("Chrome session is not authenticated or needs attention")
	// ErrAuth is an alias for ErrAuthRequired preserved for backward compatibility.
	ErrAuth = ErrAuthRequired
	// ErrBusy indicates that the maximum concurrent read capacity has been reached.
	ErrBusy = errors.New("Chrome navigation capacity reached")
	// ErrClosed indicates that Browser.Close has already been called.
	ErrClosed = errors.New("chrome browser closed")
	// ErrDebuggerUnavailable indicates that the Chrome DevTools WebSocket could not
	// be reached or was disconnected unexpectedly.
	ErrDebuggerUnavailable = errors.New("Chrome debugger connection failed")
	// ErrNavigationFailed indicates that Chrome reported a page navigation failure.
	ErrNavigationFailed = errors.New("Chrome navigation failed")
)

// NavigationError describes a navigation failure reported by Chrome's Page.navigate.
type NavigationError struct {
	ErrorText string
}

// Error returns a concise description of the navigation failure.
func (e *NavigationError) Error() string {
	if e.ErrorText == "" {
		return ErrNavigationFailed.Error()
	}
	return fmt.Sprintf("%s: %s", ErrNavigationFailed, e.ErrorText)
}

// Unwrap returns ErrNavigationFailed so callers can match with errors.Is.
func (e *NavigationError) Unwrap() error {
	return ErrNavigationFailed
}

// ExtractorError describes a JavaScript exception thrown while evaluating a page extractor.
type ExtractorError struct {
	Message string
}

// Error returns a concise description of the extractor exception.
func (e *ExtractorError) Error() string {
	if e.Message == "" {
		return "page extractor failed"
	}
	return fmt.Sprintf("page extractor failed: %s", e.Message)
}

// Browser uses a user-supplied browser-level CDP endpoint. Chrome owns the
// authenticated profile; Wayfarer never reads or exports its cookies.
type Browser struct {
	endpoint        string
	allowedHosts    map[string]struct{}
	allowedOrigins  []string
	authGuard       string
	standardTimeout time.Duration
	renderedTimeout time.Duration
	pollInterval    time.Duration
	logger          *slog.Logger
	slots           chan struct{}

	mu      sync.Mutex
	conn    *cdp
	dialing *dialAttempt
	closed  bool
}

var (
	_ RenderedPageReader = (*Browser)(nil)
	_ io.Closer          = (*Browser)(nil)
)

// New validates a loopback Chrome DevTools browser WebSocket endpoint and returns
// a Browser configured with opts.
func New(endpoint string, opts ...Option) (*Browser, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "ws" || u.User != nil || u.Port() == "" ||
		u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/devtools/browser/") ||
		u.Path == "/devtools/browser/" {
		return nil, errors.New("expected ws://127.0.0.1:port/devtools/browser/id")
	}
	if ip := net.ParseIP(u.Hostname()); ip == nil || !ip.IsLoopback() {
		return nil, errors.New("Chrome debugger must use a loopback IP")
	}
	cfg := browserConfig{
		allowedHosts:    []string{"www.linkedin.com"},
		maxConcurrency:  2,
		standardTimeout: 25 * time.Second,
		renderedTimeout: 90 * time.Second,
		pollInterval:    300 * time.Millisecond,
		logger:          slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	allowedSet := make(map[string]struct{}, len(cfg.allowedHosts))
	allowedOrigins := make([]string, 0, len(cfg.allowedHosts))
	for _, h := range cfg.allowedHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if _, exists := allowedSet[h]; exists {
			continue
		}
		allowedSet[h] = struct{}{}
		allowedOrigins = append(allowedOrigins, "https://"+h)
	}
	return &Browser{
		endpoint:        endpoint,
		allowedHosts:    allowedSet,
		allowedOrigins:  allowedOrigins,
		authGuard:       cfg.authGuard,
		standardTimeout: cfg.standardTimeout,
		renderedTimeout: cfg.renderedTimeout,
		pollInterval:    cfg.pollInterval,
		logger:          cfg.logger,
		slots:           make(chan struct{}, cfg.maxConcurrency),
	}, nil
}

type dialAttempt struct {
	done   chan struct{}
	cancel context.CancelFunc
	conn   *cdp
	err    error
}

// Close releases the shared debugger connection. It does not close Chrome.
func (b *Browser) Close() error {
	b.mu.Lock()
	b.closed = true
	if b.dialing != nil {
		b.dialing.cancel()
	}
	c := b.conn
	b.conn = nil
	b.mu.Unlock()
	if c != nil {
		c.stop(ErrClosed)
	}
	return nil
}

func (b *Browser) isClosed() bool {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	return closed
}

func (b *Browser) acquire(ctx context.Context) (*cdp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrClosed
	}
	if c := b.conn; c != nil && c.alive() {
		b.mu.Unlock()
		return c, nil
	}
	attempt := b.dialing
	if attempt == nil {
		// The handshake belongs to the browser, not to the HTTP caller that
		// happens to arrive first. Concurrent calls wait on the same attempt.
		dialCtx, cancel := context.WithTimeout(context.Background(), b.standardTimeout)
		attempt = &dialAttempt{done: make(chan struct{}), cancel: cancel}
		b.dialing = attempt
		go b.connect(dialCtx, attempt)
	}
	b.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-attempt.done:
		return attempt.conn, attempt.err
	}
}

func (b *Browser) connect(ctx context.Context, attempt *dialAttempt) {
	defer attempt.cancel()
	dialer := websocket.Dialer{}
	ws, resp, err := dialer.DialContext(ctx, b.endpoint, nil)
	if err != nil && resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	defer close(attempt.done)
	b.dialing = nil
	switch {
	case b.closed:
		if ws != nil {
			_ = ws.Close()
		}
		attempt.err = ErrClosed
	case err != nil:
		attempt.err = fmt.Errorf("%w: %w", ErrDebuggerUnavailable, err)
	default:
		b.conn = newCDP(ws)
		attempt.conn = b.conn
	}
}

// Read navigates a temporary tab in Chrome's existing default browser context.
// expression is supplied only by trusted extractors compiled into the binary.
// It may return a promise and must resolve to null until there is a usable result.
func (b *Browser) Read(ctx context.Context, pageURL, expression string) (json.RawMessage, error) {
	return b.ReadPage(ctx, pageURL, expression, ReadOptions{})
}

// ReadRendered allows visibility-dependent DOM loading in a temporary tab.
// The emulation is scoped to that tab and does not bring it to the foreground.
func (b *Browser) ReadRendered(ctx context.Context, pageURL, expression string) (json.RawMessage, error) {
	return b.ReadPage(ctx, pageURL, expression, ReadOptions{Rendered: true})
}

// ReadPage navigates a temporary tab to pageURL and polls expression until it
// resolves to a non-null JSON value, an error occurs, or ctx expires.
func (b *Browser) ReadPage(ctx context.Context, pageURL, expression string, opts ReadOptions) (json.RawMessage, error) {
	if err := b.validateURL(pageURL); err != nil {
		return nil, err
	}
	if b.isClosed() {
		return nil, ErrClosed
	}
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
	default:
		return nil, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, b.timeoutFor(opts))
	defer cancel()
	c, err := b.acquire(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	targetID, err := createTemporaryTarget(c)
	if err != nil {
		return nil, err
	}
	defer b.closeTemporaryTarget(c, targetID)

	sessionID, err := attachTarget(ctx, c, targetID)
	if err != nil {
		return nil, err
	}
	if opts.Rendered {
		if err := enableVisibilityEmulation(ctx, c, sessionID); err != nil {
			return nil, err
		}
	}
	var nav struct{ ErrorText string }
	if err := c.call(ctx, sessionID, "Page.navigate", map[string]string{"url": pageURL}, &nav); err != nil {
		return nil, err
	}
	if nav.ErrorText != "" {
		return nil, &NavigationError{ErrorText: nav.ErrorText}
	}
	return b.pollEvaluate(ctx, c, sessionID, b.buildEvaluateJS(expression))
}

func (b *Browser) validateURL(pageURL string) error {
	u, err := url.Parse(pageURL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return errors.New("unexpected upstream URL")
	}
	if _, ok := b.allowedHosts[strings.ToLower(u.Host)]; !ok {
		return errors.New("unexpected upstream URL")
	}
	return nil
}

func (b *Browser) timeoutFor(opts ReadOptions) time.Duration {
	if opts.Timeout > 0 {
		return opts.Timeout
	}
	if opts.Rendered {
		return b.renderedTimeout
	}
	return b.standardTimeout
}

func createTemporaryTarget(c *cdp) (string, error) {
	var target struct{ TargetID string }
	// Once creation is sent, wait briefly for the target ID even if the
	// caller leaves, so the tab can still be closed.
	create, stopCreate := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCreate()
	if err := c.call(create, "", "Target.createTarget", map[string]any{"url": "about:blank", "background": true}, &target); err != nil {
		return "", err
	}
	if target.TargetID == "" {
		return "", errors.New("Chrome returned no target ID")
	}
	return target.TargetID, nil
}

func (b *Browser) closeTemporaryTarget(c *cdp, targetID string) {
	// Close only this request's tab, even if the HTTP request was canceled.
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.call(cleanup, "", "Target.closeTarget", map[string]string{"targetId": targetID}, nil); err != nil {
		b.logger.Warn("failed to close temporary Chrome target", "target_id", targetID, "error", err)
	}
}

func attachTarget(ctx context.Context, c *cdp, targetID string) (string, error) {
	var attached struct{ SessionID string }
	if err := c.call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, &attached); err != nil {
		return "", err
	}
	if attached.SessionID == "" {
		return "", errors.New("Chrome returned no session ID")
	}
	return attached.SessionID, nil
}

func enableVisibilityEmulation(ctx context.Context, c *cdp, sessionID string) error {
	if err := c.call(ctx, sessionID, "Emulation.setFocusEmulationEnabled", map[string]bool{"enabled": true}, nil); err != nil {
		return err
	}
	return c.call(ctx, sessionID, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width":             1280,
		"height":            900,
		"deviceScaleFactor": 1,
		"mobile":            false,
	}, nil)
}

func (b *Browser) buildEvaluateJS(expression string) string {
	originsJSON, _ := json.Marshal(b.allowedOrigins)
	var authCheck string
	if guard := strings.TrimSpace(b.authGuard); guard != "" {
		authCheck = fmt.Sprintf(`
		const __authGuard = (%s);
		if (typeof __authGuard === 'function' ? await __authGuard() : Boolean(__authGuard)) return {state:'auth'};`, guard)
	}
	return fmt.Sprintf(`(async () => {
		if (location.href === 'about:blank') return null;
		if (!%s.includes(location.origin)) return {state:'auth'};%s
		if (document.readyState === 'loading' && !document.querySelector('main, [role="main"]')) return null;
		const value = await (%s)();
		return value === null ? null : {state:'ok', value};
	})()`, originsJSON, authCheck, expression)
}

func (b *Browser) pollEvaluate(ctx context.Context, c *cdp, sessionID, js string) (json.RawMessage, error) {
	tick := time.NewTicker(b.pollInterval)
	defer tick.Stop()
	for {
		var evaluated struct {
			Result           struct{ Value json.RawMessage }
			ExceptionDetails json.RawMessage
		}
		err := c.call(ctx, sessionID, "Runtime.evaluate", map[string]any{"expression": js, "returnByValue": true, "awaitPromise": true}, &evaluated)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// A navigation may replace the JavaScript execution context.
			var netErr net.Error
			if errors.As(err, &netErr) {
				return nil, err
			}
			var protocolErr *CommandError
			if !errors.As(err, &protocolErr) || protocolErr.Code != -32000 {
				return nil, err
			}
		} else if len(evaluated.ExceptionDetails) > 0 && string(evaluated.ExceptionDetails) != "null" {
			return nil, &ExtractorError{Message: extractExceptionMessage(evaluated.ExceptionDetails)}
		} else if v := evaluated.Result.Value; len(v) > 0 && string(v) != "null" {
			return decodeEvaluateEnvelope(v)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}

func extractExceptionMessage(raw json.RawMessage) string {
	var details struct {
		Text      string `json:"text"`
		Exception struct {
			Description string `json:"description"`
		} `json:"exception"`
	}
	if err := json.Unmarshal(raw, &details); err != nil {
		return ""
	}
	if details.Text != "" {
		return details.Text
	}
	return details.Exception.Description
}

func decodeEvaluateEnvelope(raw json.RawMessage) (json.RawMessage, error) {
	var envelope struct {
		State string
		Value json.RawMessage
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode extractor envelope: %w", err)
	}
	switch envelope.State {
	case "auth":
		return nil, ErrAuthRequired
	case "ok":
		return envelope.Value, nil
	default:
		return nil, errors.New("unexpected extractor result")
	}
}
