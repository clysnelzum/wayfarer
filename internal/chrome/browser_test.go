package chrome

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReadReusesDebuggerAndReconnectsBeforeNavigation(t *testing.T) {
	var connections, navigations, closes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer ws.Close()
		connection := connections.Add(1)
		_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			var m message
			if err := ws.ReadJSON(&m); err != nil {
				return
			}
			var result any = struct{}{}
			switch m.Method {
			case "Target.createTarget":
				result = map[string]string{"targetId": "temporary"}
			case "Target.attachToTarget":
				result = map[string]string{"sessionId": "session"}
			case "Emulation.setFocusEmulationEnabled", "Emulation.setDeviceMetricsOverride":
			case "Page.navigate":
				navigations.Add(1)
			case "Runtime.evaluate":
				result = map[string]any{"result": map[string]any{"value": map[string]any{"state": "ok", "value": 42}}}
			case "Target.closeTarget":
				closes.Add(1)
				result = map[string]bool{"success": true}
			default:
				t.Errorf("unexpected method %s", m.Method)
				return
			}
			if err := ws.WriteJSON(map[string]any{"id": m.ID, "result": result}); err != nil {
				return
			}
			if connection == 1 && closes.Load() == 2 {
				return // Disconnect after acknowledging the second tab's cleanup.
			}
		}
	}))
	defer server.Close()
	b, err := New("ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for i, wantConnections := range []int32{1, 1, 2} {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		if i == 2 {
			select {
			case <-b.conn.done:
			case <-ctx.Done():
				t.Fatal("idle reader did not detect debugger disconnect")
			}
		}
		value, err := b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
		cancel()
		if err != nil || string(value) != "42" || connections.Load() != wantConnections || closes.Load() != int32(i+1) {
			t.Fatalf("read %d: %s (%v), connections=%d, closes=%d", i+1, value, err, connections.Load(), closes.Load())
		}
	}
	if navigations.Load() != 3 {
		t.Fatalf("expected exactly 3 navigations, got %d", navigations.Load())
	}
}

func TestReadUsesAndClosesTemporaryTarget(t *testing.T) {
	for _, state := range []string{"ok", "auth", "rendered"} {
		t.Run(state, func(t *testing.T) {
			observed := make(chan []string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var methods []string
				defer func() { observed <- methods }()
				up := websocket.Upgrader{}
				ws, err := up.Upgrade(w, r, nil)
				if err != nil {
					t.Errorf("upgrade: %v", err)
					return
				}
				defer ws.Close()
				_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
				eval := 0
				for {
					var m struct {
						ID        int
						Method    string
						SessionID string
						Params    map[string]any
					}
					if err := ws.ReadJSON(&m); err != nil {
						return
					}
					methods = append(methods, m.Method)
					var payload any
					switch m.Method {
					case "Target.createTarget":
						if m.Params["url"] != "about:blank" || m.Params["background"] != true {
							t.Errorf("unexpected creation params: %v", m.Params)
						}
						payload = map[string]string{"targetId": "test-target"}
					case "Target.attachToTarget":
						if m.Params["targetId"] != "test-target" || m.Params["flatten"] != true {
							t.Errorf("unexpected attach params: %v", m.Params)
						}
						payload = map[string]string{"sessionId": "test-session"}
					case "Emulation.setFocusEmulationEnabled":
						if state != "rendered" || m.SessionID != "test-session" || m.Params["enabled"] != true {
							t.Errorf("unexpected focus emulation: %+v", m)
						}
						payload = struct{}{}
					case "Emulation.setDeviceMetricsOverride":
						if state != "rendered" || m.SessionID != "test-session" || m.Params["width"] != float64(1280) || m.Params["height"] != float64(900) || m.Params["deviceScaleFactor"] != float64(1) || m.Params["mobile"] != false {
							t.Errorf("unexpected device metrics override: %+v", m)
						}
						payload = struct{}{}
					case "Page.navigate":
						if m.SessionID != "test-session" || m.Params["url"] != "https://www.linkedin.com/jobs/" {
							t.Errorf("unexpected navigation: %+v", m)
						}
						payload = map[string]string{"frameId": "test-frame"}
					case "Runtime.evaluate":
						if m.Params["awaitPromise"] != true || m.Params["returnByValue"] != true {
							t.Errorf("expected evaluation by value with async support")
						}
						eval++
						if eval == 1 {
							_ = ws.WriteJSON(map[string]any{"id": m.ID, "error": map[string]int{"code": -32000}})
							continue
						}
						resultState := state
						if state == "rendered" {
							resultState = "ok"
						}
						payload = map[string]any{"result": map[string]any{"value": map[string]any{"state": resultState, "value": map[string]int{"count": 1}}}}
					case "Target.closeTarget":
						if m.Params["targetId"] != "test-target" {
							t.Errorf("unexpected target closed: %v", m.Params)
						}
						payload = map[string]bool{"success": true}
						_ = ws.WriteJSON(map[string]any{"id": m.ID, "result": payload})
						return
					default:
						t.Errorf("unexpected command: %s", m.Method)
						return
					}
					_ = ws.WriteJSON(map[string]any{"method": "Test.interleavedEvent"})
					_ = ws.WriteJSON(map[string]any{"id": m.ID, "result": payload})
				}
			}))
			defer server.Close()
			var opts []Option
			if state == "auth" {
				opts = append(opts, WithAuthGuard("() => window.__authWall"))
			}
			b, err := New("ws"+strings.TrimPrefix(server.URL, "http")+"/devtools/browser/test", opts...)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			read := b.Read
			if state == "rendered" {
				read = b.ReadRendered
			}
			got, err := read(ctx, "https://www.linkedin.com/jobs/", "() => ({count: 1})")
			if state == "auth" {
				if !errors.Is(err, ErrAuthRequired) || !errors.Is(err, ErrAuth) {
					t.Fatalf("got %v, want ErrAuthRequired / ErrAuth", err)
				}
			} else {
				var value struct{ Count int }
				if err != nil || json.Unmarshal(got, &value) != nil || value.Count != 1 {
					t.Fatalf("unexpected result: %s (%v)", got, err)
				}
			}
			want := []string{"Target.createTarget", "Target.attachToTarget"}
			if state == "rendered" {
				want = append(want, "Emulation.setFocusEmulationEnabled", "Emulation.setDeviceMetricsOverride")
			}
			want = append(want, "Page.navigate", "Runtime.evaluate", "Runtime.evaluate", "Target.closeTarget")
			select {
			case got := <-observed:
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("got commands %v, want %v", got, want)
				}
			case <-ctx.Done():
				t.Fatal("mock did not observe cleanup")
			}
		})
	}
}

type mockCDPRequest struct {
	ID        int            `json:"id"`
	Method    string         `json:"method"`
	SessionID string         `json:"sessionId"`
	Params    map[string]any `json:"params"`
}

func startMockCDPServer(t *testing.T, handler func(ws *websocket.Conn, req mockCDPRequest) (result any, cdpErr map[string]any, closeConn bool)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{}
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer ws.Close()
		_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			var req mockCDPRequest
			if err := ws.ReadJSON(&req); err != nil {
				return
			}
			result, cdpErr, closeConn := handler(ws, req)
			resp := map[string]any{"id": req.ID}
			if cdpErr != nil {
				resp["error"] = cdpErr
			} else {
				if result == nil {
					result = struct{}{}
				}
				resp["result"] = result
			}
			if err := ws.WriteJSON(resp); err != nil {
				return
			}
			if closeConn {
				return
			}
		}
	}))
}

func defaultMockCDPReply(req mockCDPRequest) (any, map[string]any, bool) {
	switch req.Method {
	case "Target.createTarget":
		return map[string]string{"targetId": "target-1"}, nil, false
	case "Target.attachToTarget":
		return map[string]string{"sessionId": "session-1"}, nil, false
	case "Page.navigate":
		return map[string]string{"frameId": "frame-1"}, nil, false
	case "Runtime.evaluate":
		return map[string]any{"result": map[string]any{"value": map[string]any{"state": "ok", "value": 42}}}, nil, false
	case "Target.closeTarget":
		return map[string]bool{"success": true}, nil, false
	default:
		return struct{}{}, nil, false
	}
}

func TestCloseReturnsNilAndSubsequentReadReturnsErrClosed(t *testing.T) {
	server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
		return defaultMockCDPReply(req)
	})
	defer server.Close()

	b, err := New("ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
	if err != nil || string(got) != "42" {
		t.Fatalf("initial read failed: %s (%v)", got, err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second Close() returned error: %v", err)
	}

	_, err = b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Read after Close: got %v, want ErrClosed", err)
	}
	if errors.Is(err, ErrDebuggerUnavailable) {
		t.Fatalf("Read after Close should not wrap ErrDebuggerUnavailable: %v", err)
	}
}

func TestCloseInterruptsInFlightCallWithErrClosed(t *testing.T) {
	evaluating := make(chan struct{})
	var once sync.Once
	server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
		if req.Method == "Runtime.evaluate" {
			once.Do(func() { close(evaluating) })
			time.Sleep(2 * time.Second)
		}
		return defaultMockCDPReply(req)
	})
	defer server.Close()

	b, err := New("ws"+strings.TrimPrefix(server.URL, "http")+"/devtools/browser/test", WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, err := b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
		errCh <- err
	}()

	select {
	case <-evaluating:
	case <-ctx.Done():
		t.Fatal("timed out waiting for Runtime.evaluate")
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	err = <-errCh
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("in-flight Read after Close: got %v, want ErrClosed", err)
	}
	if errors.Is(err, ErrDebuggerUnavailable) {
		t.Fatalf("in-flight Read after Close should not match ErrDebuggerUnavailable: %v", err)
	}
}

func TestCommandErrorPreservesMethodCodeAndMessage(t *testing.T) {
	server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
		if req.Method == "Page.navigate" {
			return nil, map[string]any{
				"code":    -32602,
				"message": "Invalid parameters: url is malformed",
			}, false
		}
		return defaultMockCDPReply(req)
	})
	defer server.Close()

	b, err := New("ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("got %T (%v), want *CommandError", err, err)
	}
	if cmdErr.Method != "Page.navigate" || cmdErr.Code != -32602 || cmdErr.Message != "Invalid parameters: url is malformed" {
		t.Fatalf("unexpected CommandError: %+v", cmdErr)
	}
	if !strings.Contains(cmdErr.Error(), "Page.navigate") || !strings.Contains(cmdErr.Error(), "Invalid parameters: url is malformed") {
		t.Fatalf("unexpected Error() string: %q", cmdErr.Error())
	}
}

func TestNavigationErrorPreservesErrorText(t *testing.T) {
	var closed atomic.Bool
	server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
		if req.Method == "Page.navigate" {
			return map[string]any{"frameId": "frame-1", "errorText": "net::ERR_NAME_NOT_RESOLVED"}, nil, false
		}
		if req.Method == "Target.closeTarget" {
			closed.Store(true)
		}
		return defaultMockCDPReply(req)
	})
	defer server.Close()

	b, err := New("ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
	if !errors.Is(err, ErrNavigationFailed) {
		t.Fatalf("got %v, want ErrNavigationFailed", err)
	}
	var navErr *NavigationError
	if !errors.As(err, &navErr) {
		t.Fatalf("got %T (%v), want *NavigationError", err, err)
	}
	if navErr.ErrorText != "net::ERR_NAME_NOT_RESOLVED" {
		t.Fatalf("got ErrorText=%q, want net::ERR_NAME_NOT_RESOLVED", navErr.ErrorText)
	}
	if !closed.Load() {
		t.Fatal("expected Target.closeTarget cleanup after navigation error")
	}
}

func TestExtractorErrorPreservesExceptionText(t *testing.T) {
	for _, tc := range []struct {
		name             string
		exceptionDetails any
		wantMessage      string
	}{
		{
			name:             "with text",
			exceptionDetails: map[string]any{"text": "Uncaught TypeError: cannot read properties"},
			wantMessage:      "Uncaught TypeError: cannot read properties",
		},
		{
			name:             "fallback to exception description",
			exceptionDetails: map[string]any{"exception": map[string]any{"description": "Error: bad selector"}},
			wantMessage:      "Error: bad selector",
		},
		{
			name:             "malformed shape",
			exceptionDetails: "unexpected-non-object",
			wantMessage:      "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
				if req.Method == "Runtime.evaluate" {
					return map[string]any{"exceptionDetails": tc.exceptionDetails}, nil, false
				}
				return defaultMockCDPReply(req)
			})
			defer server.Close()

			b, err := New("ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test")
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err = b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
			var extErr *ExtractorError
			if !errors.As(err, &extErr) {
				t.Fatalf("got %T (%v), want *ExtractorError", err, err)
			}
			if extErr.Message != tc.wantMessage {
				t.Fatalf("got Message=%q, want %q", extErr.Message, tc.wantMessage)
			}
		})
	}
}

func TestWithAllowedHosts(t *testing.T) {
	var seenExpressions []string
	var mu sync.Mutex
	server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
		if req.Method == "Runtime.evaluate" {
			mu.Lock()
			if expr, ok := req.Params["expression"].(string); ok {
				seenExpressions = append(seenExpressions, expr)
			}
			mu.Unlock()
		}
		return defaultMockCDPReply(req)
	})
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Default allows only www.linkedin.com.
	defaultBrowser, err := New(wsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer defaultBrowser.Close()
	if _, err := defaultBrowser.Read(ctx, "https://example.com/page", "() => 1"); err == nil {
		t.Fatal("expected default browser to reject non-linkedin host")
	}

	// Custom allowed hosts replace the default.
	customBrowser, err := New(wsURL, WithAllowedHosts("example.com", "Sub.Example.org"))
	if err != nil {
		t.Fatal(err)
	}
	defer customBrowser.Close()

	if _, err := customBrowser.Read(ctx, "https://www.linkedin.com/jobs/", "() => 1"); err == nil {
		t.Fatal("expected custom browser to reject unconfigured default host")
	}
	if _, err := customBrowser.Read(ctx, "http://example.com/page", "() => 1"); err == nil {
		t.Fatal("expected custom browser to reject non-https URL")
	}

	for _, allowedURL := range []string{"https://example.com/page", "https://sub.example.org/data"} {
		got, err := customBrowser.Read(ctx, allowedURL, "() => 42")
		if err != nil || string(got) != "42" {
			t.Fatalf("Read(%s): got %s (%v), want 42", allowedURL, got, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seenExpressions) != 2 {
		t.Fatalf("expected 2 evaluations, got %d", len(seenExpressions))
	}
	for _, expr := range seenExpressions {
		if !strings.Contains(expr, `"https://example.com"`) || !strings.Contains(expr, `"https://sub.example.org"`) {
			t.Fatalf("expected allowed origins in JS wrapper, got: %s", expr)
		}
		if strings.Contains(expr, "linkedin.com") {
			t.Fatalf("unexpected linkedin.com reference in custom-host JS wrapper: %s", expr)
		}
	}
}

func TestWithAuthGuard(t *testing.T) {
	t.Run("custom guard triggers ErrAuthRequired", func(t *testing.T) {
		var capturedExpr string
		server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
			if req.Method == "Runtime.evaluate" {
				capturedExpr, _ = req.Params["expression"].(string)
				return map[string]any{"result": map[string]any{"value": map[string]any{"state": "auth"}}}, nil, false
			}
			return defaultMockCDPReply(req)
		})
		defer server.Close()

		guard := `() => Boolean(document.querySelector('#custom-auth-wall'))`
		b, err := New("ws"+strings.TrimPrefix(server.URL, "http")+"/devtools/browser/test", WithAuthGuard(guard))
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err = b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
		if !errors.Is(err, ErrAuthRequired) {
			t.Fatalf("got %v, want ErrAuthRequired", err)
		}
		if !strings.Contains(capturedExpr, guard) || !strings.Contains(capturedExpr, "__authGuard") {
			t.Fatalf("expected custom auth guard in Runtime.evaluate expression, got: %s", capturedExpr)
		}
	})

	t.Run("empty guard injects no auth-wall check", func(t *testing.T) {
		var capturedExpr string
		server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
			if req.Method == "Runtime.evaluate" {
				capturedExpr, _ = req.Params["expression"].(string)
			}
			return defaultMockCDPReply(req)
		})
		defer server.Close()

		b, err := New("ws"+strings.TrimPrefix(server.URL, "http")+"/devtools/browser/test", WithAuthGuard(""))
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		got, err := b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
		if err != nil || string(got) != "42" {
			t.Fatalf("got %s (%v), want 42", got, err)
		}
		for _, forbidden := range []string{"__authGuard", "login-submit", "authwall", "checkpoint"} {
			if strings.Contains(capturedExpr, forbidden) {
				t.Fatalf("unexpected %q in JS wrapper with empty auth guard: %s", forbidden, capturedExpr)
			}
		}
	})
}

func TestWithMaxConcurrency(t *testing.T) {
	firstInFlight := make(chan struct{})
	releaseFirst := make(chan struct{})
	var evalCount atomic.Int32
	server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
		if req.Method == "Runtime.evaluate" && evalCount.Add(1) == 1 {
			close(firstInFlight)
			<-releaseFirst
		}
		return defaultMockCDPReply(req)
	})
	defer server.Close()

	b, err := New("ws"+strings.TrimPrefix(server.URL, "http")+"/devtools/browser/test", WithMaxConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	firstErr := make(chan error, 1)
	go func() {
		_, err := b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
		firstErr <- err
	}()

	select {
	case <-firstInFlight:
	case <-ctx.Done():
		t.Fatal("timed out waiting for first read to enter Runtime.evaluate")
	}

	_, err = b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
	if !errors.Is(err, ErrBusy) {
		close(releaseFirst)
		t.Fatalf("second concurrent Read with maxConcurrency=1: got %v, want ErrBusy", err)
	}

	close(releaseFirst)
	if err := <-firstErr; err != nil {
		t.Fatalf("first read failed: %v", err)
	}
}

func TestWithLoggerLogsCloseTargetWarningAndReadOptionsTimeout(t *testing.T) {
	var logBuf bytes.Buffer
	var mu sync.Mutex
	logger := slog.New(slog.NewTextHandler(&syncWriter{mu: &mu, buf: &logBuf}, nil))

	server := startMockCDPServer(t, func(_ *websocket.Conn, req mockCDPRequest) (any, map[string]any, bool) {
		if req.Method == "Runtime.evaluate" {
			return map[string]any{"result": map[string]any{"value": nil}}, nil, false
		}
		if req.Method == "Target.closeTarget" {
			return nil, map[string]any{"code": -32000, "message": "target already gone"}, false
		}
		return defaultMockCDPReply(req)
	})
	defer server.Close()

	b, err := New(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/devtools/browser/test",
		WithLogger(logger),
		WithPollInterval(10*time.Millisecond),
		WithStandardTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	_, err = b.ReadPage(context.Background(), "https://www.linkedin.com/jobs/", "() => null", ReadOptions{
		Timeout: 60 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReadPage with short ReadOptions.Timeout: got %v, want DeadlineExceeded", err)
	}

	mu.Lock()
	logged := logBuf.String()
	mu.Unlock()
	if !strings.Contains(logged, "failed to close temporary Chrome target") || !strings.Contains(logged, "target already gone") {
		t.Fatalf("expected warning log for Target.closeTarget failure, got: %q", logged)
	}
}

type syncWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}
