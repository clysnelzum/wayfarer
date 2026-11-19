package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type message struct {
	ID        int             `json:"id"`
	Method    string          `json:"method"`
	SessionID string          `json:"sessionId,omitempty"`
	Params    any             `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// cdp uses one read pump and serializes WebSocket writes. Chrome can reply out
// of order across flattened target sessions, so callers wait by command ID.
type cdp struct {
	ws      *websocket.Conn
	writers chan struct{}
	done    chan struct{}
	once    sync.Once
	err     error // Set before closing done.

	mu      sync.Mutex
	id      int
	pending map[int]chan message
}

func newCDP(ws *websocket.Conn) *cdp {
	ws.SetReadLimit(4 << 20)
	c := &cdp{
		ws: ws, writers: make(chan struct{}, 1), done: make(chan struct{}),
		pending: make(map[int]chan message),
	}
	go c.readLoop()
	return c
}

func (c *cdp) stop(err error) {
	c.once.Do(func() {
		if errors.Is(err, ErrClosed) {
			c.err = err
		} else {
			c.err = fmt.Errorf("%w: %w", ErrDebuggerUnavailable, err)
		}
		_ = c.ws.Close()
		close(c.done)
	})
}

func (c *cdp) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *cdp) readLoop() {
	for {
		var reply message
		if err := c.ws.ReadJSON(&reply); err != nil {
			c.stop(err)
			return
		}
		if reply.ID == 0 {
			continue // An unsolicited event, not a command reply.
		}
		c.mu.Lock()
		waiter := c.pending[reply.ID]
		delete(c.pending, reply.ID)
		if waiter != nil {
			waiter <- reply
		}
		c.mu.Unlock()
	}
}

func (c *cdp) call(ctx context.Context, session, method string, params, result any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.err
	case c.writers <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		<-c.writers
		return err
	}

	waiter := make(chan message, 1)
	c.mu.Lock()
	c.id++
	id := c.id
	c.pending[id] = waiter
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	// Bound transport writes independently of individual request deadlines.
	_ = c.ws.SetWriteDeadline(time.Now().Add(2 * time.Second))
	err := c.ws.WriteJSON(message{ID: id, Method: method, SessionID: session, Params: params})
	<-c.writers
	if err != nil {
		// Gorilla cannot continue after a failed write. Ordinary cancellation
		// while waiting for a reply does not affect the WebSocket.
		c.stop(err)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return c.err
	}

	decode := func(reply message) error {
		if reply.Error != nil {
			return &CommandError{
				Method:  method,
				Code:    reply.Error.Code,
				Message: reply.Error.Message,
			}
		}
		if result != nil {
			if err := json.Unmarshal(reply.Result, result); err != nil {
				return fmt.Errorf("decode CDP %s result: %w", method, err)
			}
		}
		return nil
	}
	select {
	case reply := <-waiter:
		return decode(reply)
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		// Chrome may close directly after a reply. Honor an already received
		// command result rather than report it as an interrupted request.
		select {
		case reply := <-waiter:
			return decode(reply)
		default:
			return c.err
		}
	}
}

// CommandError describes a protocol error returned by Chrome DevTools Protocol.
type CommandError struct {
	Method  string
	Code    int
	Message string
}

// Error returns a concise description of the failed CDP command.
func (e *CommandError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("CDP %s failed (%d): %s", e.Method, e.Code, e.Message)
	}
	return fmt.Sprintf("CDP %s failed (%d)", e.Method, e.Code)
}
