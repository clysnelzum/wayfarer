package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Browser uses a user-supplied browser-level CDP endpoint. Chrome owns the
// authenticated profile; Wayfarer never reads or exports its cookies.
type Browser struct {
	endpoint string
	slots    chan struct{}

	mu      sync.Mutex
	conn    *cdp
	dialing *dialAttempt
	closed  bool
}

func New(endpoint string) (*Browser, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "ws" || u.User != nil || u.Port() == "" ||
		u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/devtools/browser/") ||
		u.Path == "/devtools/browser/" {
		return nil, errors.New("expected ws://127.0.0.1:port/devtools/browser/id")
	}
	if ip := net.ParseIP(u.Hostname()); ip == nil || !ip.IsLoopback() {
		return nil, errors.New("Chrome debugger must use a loopback IP")
	}
	return &Browser{endpoint: endpoint, slots: make(chan struct{}, 2)}, nil
}

type dialAttempt struct {
	done   chan struct{}
	cancel context.CancelFunc
	conn   *cdp
	err    error
}

// Close releases the shared debugger connection. It does not close Chrome.
func (b *Browser) Close() {
	b.mu.Lock()
	b.closed = true
	if b.dialing != nil {
		b.dialing.cancel()
	}
	c := b.conn
	b.conn = nil
	b.mu.Unlock()
	if c != nil {
		c.stop(errors.New("Wayfarer closed the debugger connection"))
	}
}

func (b *Browser) acquire(ctx context.Context) (*cdp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrDebuggerUnavailable
	}
	if c := b.conn; c != nil && c.alive() {
		b.mu.Unlock()
		return c, nil
	}
	attempt := b.dialing
	if attempt == nil {
		// The handshake belongs to the browser, not to the HTTP caller that
		// happens to arrive first. Concurrent calls wait on the same attempt.
		dialCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
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
		attempt.err = ErrDebuggerUnavailable
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
	return b.read(ctx, pageURL, expression, false)
}

// ReadRendered allows visibility-dependent DOM loading in a temporary tab.
// The emulation is scoped to that tab and does not bring it to the foreground.
func (b *Browser) ReadRendered(ctx context.Context, pageURL, expression string) (json.RawMessage, error) {
	return b.read(ctx, pageURL, expression, true)
}

func (b *Browser) read(ctx context.Context, pageURL, expression string, render bool) (json.RawMessage, error) {
	u, err := url.Parse(pageURL)
	if err != nil || u.Scheme != "https" || u.Host != "www.linkedin.com" || u.User != nil {
		return nil, errors.New("unexpected upstream URL")
	}
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
	default:
		return nil, ErrBusy
	}
	timeout := 20 * time.Second
	if render {
		timeout = 24 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c, err := b.acquire(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var target struct{ TargetID string }
	// Once creation is sent, wait briefly for the target ID even if the
	// caller leaves, so the tab can still be closed.
	create, stopCreate := context.WithTimeout(context.Background(), 2*time.Second)
	err = c.call(create, "", "Target.createTarget", map[string]any{"url": "about:blank", "background": true}, &target)
	stopCreate()
	if err != nil {
		return nil, err
	}
	if target.TargetID == "" {
		return nil, errors.New("Chrome returned no target ID")
	}
	defer func() {
		// Close only this request's tab, even if the HTTP request was canceled.
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = c.call(cleanup, "", "Target.closeTarget", map[string]string{"targetId": target.TargetID}, nil)
	}()
	var attached struct{ SessionID string }
	if err := c.call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, &attached); err != nil {
		return nil, err
	}
	if attached.SessionID == "" {
		return nil, errors.New("Chrome returned no session ID")
	}
	if render {
		if err := c.call(ctx, attached.SessionID, "Emulation.setFocusEmulationEnabled", map[string]bool{"enabled": true}, nil); err != nil {
			return nil, err
		}
	}
	var nav struct{ ErrorText string }
	if err := c.call(ctx, attached.SessionID, "Page.navigate", map[string]string{"url": pageURL}, &nav); err != nil {
		return nil, err
	}
	if nav.ErrorText != "" {
		return nil, errors.New("Chrome navigation failed")
	}

	// Check origin before invoking the extractor, including after redirects.
	js := `(async () => {
		if (location.href === 'about:blank' || document.readyState === 'loading') return null;
		if (location.origin !== 'https://www.linkedin.com' || /^\/(login|uas|checkpoint|authwall)(\/|$)/.test(location.pathname)) return {state:'auth'};
		if (document.querySelector('form[action*="login-submit"]')) return {state:'auth'};
		const value = await (` + expression + `)();
		return value === null ? null : {state:'ok', value};
	})()`
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		var evaluated struct {
			Result           struct{ Value json.RawMessage }
			ExceptionDetails json.RawMessage
		}
		err := c.call(ctx, attached.SessionID, "Runtime.evaluate", map[string]any{"expression": js, "returnByValue": true, "awaitPromise": true}, &evaluated)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// A navigation may replace the JavaScript execution context.
			var netErr net.Error
			if errors.As(err, &netErr) {
				return nil, err
			}
			var protocolErr *commandError
			if !errors.As(err, &protocolErr) || protocolErr.code != -32000 {
				return nil, err
			}
		} else if len(evaluated.ExceptionDetails) > 0 {
			return nil, errors.New("page extractor failed")
		} else if v := evaluated.Result.Value; len(v) > 0 && string(v) != "null" {
			var envelope struct {
				State string
				Value json.RawMessage
			}
			if err := json.Unmarshal(v, &envelope); err != nil {
				return nil, err
			}
			switch envelope.State {
			case "auth":
				return nil, ErrAuth
			case "ok":
				return envelope.Value, nil
			default:
				return nil, errors.New("unexpected extractor result")
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}

var (
	ErrAuth                = errors.New("Chrome session is not authenticated or needs attention")
	ErrBusy                = errors.New("Chrome navigation capacity reached")
	ErrDebuggerUnavailable = errors.New("Chrome debugger connection failed")
)
