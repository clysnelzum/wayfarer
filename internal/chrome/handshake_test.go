package chrome

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCanceledCallerDoesNotRestartHandshake(t *testing.T) {
	var handshakes atomic.Int32
	started := make(chan struct{})
	allow := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handshakes.Add(1) == 1 {
			close(started)
		}
		select {
		case <-allow:
		case <-r.Context().Done():
			return
		}
		up := websocket.Upgrader{}
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer ws.Close()
		var m message
		_ = ws.ReadJSON(&m)
	}))
	defer server.Close()
	defer func() {
		select {
		case <-allow:
		default:
			close(allow)
		}
	}()
	b, err := New("ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ctx, stop := context.WithTimeout(context.Background(), 4*time.Second)
	defer stop()
	first, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := b.acquire(first); result <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("handshake did not start")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller: %v", err)
	}
	close(allow)
	c, err := b.acquire(ctx)
	if err != nil || c == nil || !c.alive() || handshakes.Load() != 1 {
		t.Fatalf("second caller error=%v, handshakes=%d", err, handshakes.Load())
	}
}
