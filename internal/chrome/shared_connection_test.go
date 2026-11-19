package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestConcurrentReadsShareConnectionAfterCancellation(t *testing.T) {
	var connections, closes atomic.Int32
	firstEvaluation := make(chan struct{})
	secondEvaluation := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{}
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer ws.Close()
		if connections.Add(1) != 1 {
			t.Error("unexpected second WebSocket")
			return
		}
		var first, second message
		targets := 0
		respond := func(m message, result any) {
			if err := ws.WriteJSON(map[string]any{"id": m.ID, "sessionId": m.SessionID, "result": result}); err != nil {
				t.Errorf("reply: %v", err)
			}
		}
		respondEval := func(m message) {
			respond(m, map[string]any{"result": map[string]any{"value": map[string]any{"state": "ok", "value": m.SessionID}}})
		}
		for {
			var m message
			if err := ws.ReadJSON(&m); err != nil {
				return
			}
			params, _ := m.Params.(map[string]any)
			switch m.Method {
			case "Target.createTarget":
				targets++
				respond(m, map[string]string{"targetId": fmt.Sprint(targets)})
			case "Target.attachToTarget":
				respond(m, map[string]any{"sessionId": params["targetId"]})
			case "Page.navigate":
				respond(m, struct{}{})
			case "Runtime.evaluate":
				switch m.SessionID {
				case "1":
					first = m
					close(firstEvaluation)
				case "2":
					second = m
					close(secondEvaluation)
				default:
					respondEval(m)
				}
			case "Target.closeTarget":
				closes.Add(1)
				if params["targetId"] == "1" && second.ID != 0 {
					// Reply out of order, including a late reply for the
					// canceled evaluation. Neither may cross sessions.
					respondEval(second)
					respondEval(first)
				}
				respond(m, map[string]bool{"success": true})
			default:
				t.Errorf("unexpected command: %s", m.Method)
				return
			}
		}
	}))
	defer server.Close()
	b, err := New("ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ctx, stop := context.WithTimeout(context.Background(), 4*time.Second)
	defer stop()
	firstCtx, cancelFirst := context.WithCancel(ctx)
	defer cancelFirst()
	type outcome struct {
		value json.RawMessage
		err   error
	}
	read := func(ctx context.Context) outcome {
		value, err := b.Read(ctx, "https://www.linkedin.com/jobs/", "() => 42")
		return outcome{value, err}
	}
	await := func(signal <-chan struct{}) {
		t.Helper()
		select {
		case <-signal:
		case <-ctx.Done():
			t.Fatal("timed out waiting for evaluation")
		}
	}
	one, two := make(chan outcome, 1), make(chan outcome, 1)
	go func() { one <- read(firstCtx) }()
	await(firstEvaluation)
	go func() { two <- read(ctx) }()
	await(secondEvaluation)
	cancelFirst()
	if x := <-one; !errors.Is(x.err, context.Canceled) {
		t.Fatalf("first read: %v", x.err)
	}
	if x := <-two; x.err != nil || string(x.value) != `"2"` {
		t.Fatalf("second read: %s (%v)", x.value, x.err)
	}
	if x := read(ctx); x.err != nil || string(x.value) != `"3"` {
		t.Fatalf("third read: %s (%v)", x.value, x.err)
	}
	if connections.Load() != 1 || closes.Load() != 3 {
		t.Fatalf("connections=%d, closed tabs=%d", connections.Load(), closes.Load())
	}
}
