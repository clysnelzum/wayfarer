package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
			b, err := New("ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			read := b.Read
			if state == "rendered" {
				read = b.ReadRendered
			}
			got, err := read(ctx, "https://www.linkedin.com/jobs/", "() => ({count: 1})")
			if state == "auth" {
				if !errors.Is(err, ErrAuth) {
					t.Fatalf("got %v, want ErrAuth", err)
				}
			} else {
				var value struct{ Count int }
				if err != nil || json.Unmarshal(got, &value) != nil || value.Count != 1 {
					t.Fatalf("unexpected result: %s (%v)", got, err)
				}
			}
			want := []string{"Target.createTarget", "Target.attachToTarget"}
			if state == "rendered" {
				want = append(want, "Emulation.setFocusEmulationEnabled")
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
