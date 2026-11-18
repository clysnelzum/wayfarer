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

	mu     sync.Mutex
	idle   []*cdp
	closed bool
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

type cdp struct {
	ws *websocket.Conn
	id int
}

type message struct {
	ID        int             `json:"id"`
	Method    string          `json:"method"`
	SessionID string          `json:"sessionId,omitempty"`
	Params    any             `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Code int `json:"code"`
	} `json:"error,omitempty"`
}

func (c *cdp) call(ctx context.Context, session, method string, params, result any) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	defer func() {
		if err == nil {
			return
		}
		if cause := ctx.Err(); cause != nil {
			err = cause
		} else if !deadline.IsZero() && !time.Now().Before(deadline) {
			err = context.DeadlineExceeded
		}
	}()
	_ = c.ws.SetWriteDeadline(deadline)
	_ = c.ws.SetReadDeadline(deadline)
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = c.ws.UnderlyingConn().SetDeadline(time.Now())
		close(interrupted)
	})
	defer func() {
		if !stop() {
			<-interrupted
		}
	}()
	c.id++
	if err := c.ws.WriteJSON(message{ID: c.id, Method: method, SessionID: session, Params: params}); err != nil {
		return err
	}
	for {
		var reply message
		if err := c.ws.ReadJSON(&reply); err != nil {
			return err
		}
		if reply.ID != c.id {
			continue // CDP events can be interleaved with responses.
		}
		if reply.Error != nil {
			return &commandError{method: method, code: reply.Error.Code}
		}
		if result != nil {
			return json.Unmarshal(reply.Result, result)
		}
		return nil
	}
}

// Close releases idle debugger connections. It does not close Chrome. In-flight
// reads release their connections instead of returning them to the pool.
func (b *Browser) Close() {
	b.mu.Lock()
	b.closed = true
	idle := b.idle
	b.idle = nil
	b.mu.Unlock()
	for _, c := range idle {
		_ = c.ws.Close()
	}
}

func (b *Browser) acquire(ctx context.Context) (*cdp, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrDebuggerUnavailable
	}
	var cached *cdp
	if n := len(b.idle); n != 0 {
		cached = b.idle[n-1]
		b.idle[n-1] = nil
		b.idle = b.idle[:n-1]
	}
	b.mu.Unlock()
	if cached != nil {
		// Retry a stale connection only before any navigation has been sent.
		probe, cancel := context.WithTimeout(ctx, time.Second)
		err := cached.call(probe, "", "Browser.getVersion", nil, nil)
		cancel()
		if err == nil {
			return cached, nil
		}
		_ = cached.ws.Close()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 12 * time.Second}
	ws, resp, err := dialer.DialContext(ctx, b.endpoint, nil)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrDebuggerUnavailable, err)
	}
	ws.SetReadLimit(4 << 20)
	return &cdp{ws: ws}, nil
}

func (b *Browser) release(c *cdp, reusable bool) {
	if reusable {
		b.mu.Lock()
		if !b.closed {
			b.idle = append(b.idle, c)
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()
	}
	_ = c.ws.Close()
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
	reusable := false
	defer func() { b.release(c, reusable) }()
	var target struct{ TargetID string }
	if err := c.call(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank", "background": true}, &target); err != nil {
		return nil, err
	}
	if target.TargetID == "" {
		return nil, errors.New("Chrome returned no target ID")
	}
	defer func() {
		// Try to close the tab even after cancellation. Never reuse the socket
		// unless the browser acknowledged closing it and the read completed.
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var closed struct{ Success bool }
		err := c.call(cleanup, "", "Target.closeTarget", map[string]string{"targetId": target.TargetID}, &closed)
		reusable = reusable && err == nil && closed.Success
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
				reusable = true
				return nil, ErrAuth
			case "ok":
				reusable = true
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

type commandError struct {
	method string
	code   int
}

func (e *commandError) Error() string { return fmt.Sprintf("CDP %s failed (%d)", e.method, e.code) }

var (
	ErrAuth                = errors.New("Chrome session is not authenticated or needs attention")
	ErrBusy                = errors.New("Chrome navigation capacity reached")
	ErrDebuggerUnavailable = errors.New("Chrome debugger connection failed")
)
