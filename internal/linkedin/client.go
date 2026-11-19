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
	"strings"

	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin/scripts"
)

const defaultBaseURL = "https://www.linkedin.com"

// Reader reads data from a LinkedIn page by evaluating a JavaScript extractor.
type Reader = chrome.PageReader

// Option configures a Client constructed with NewClient.
type Option func(*Client)

// Client reads LinkedIn feed, jobs, personal profile, and company pages via a
// Chrome PageReader and validates upstream payloads against the domain contract.
type Client struct {
	reader  chrome.PageReader
	scripts scripts.Provider
	baseURL string
	logger  *slog.Logger
}

// WithScripts overrides the JavaScript extractor provider used by the client.
// Passing nil leaves the default embedded script registry unchanged.
func WithScripts(p scripts.Provider) Option {
	return func(c *Client) {
		if p != nil {
			c.scripts = p
		}
	}
}

// WithBaseURL overrides the LinkedIn base URL targeted by page navigations and
// canonicalized entity URLs. Trailing slashes are trimmed.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

// WithLogger sets the structured logger used by the client. Passing nil disables
// logging; by default slog.Default() is used.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		if logger == nil {
			c.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			return
		}
		c.logger = logger
	}
}

// NewClient constructs a LinkedIn Client backed by reader and configured with opts.
func NewClient(reader chrome.PageReader, opts ...Option) (*Client, error) {
	if reader == nil {
		return nil, errors.New("linkedin reader must not be nil")
	}
	c := &Client{
		reader:  reader,
		scripts: scripts.DefaultRegistry(),
		baseURL: defaultBaseURL,
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	u, err := url.Parse(c.baseURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid base URL %q", c.baseURL)
	}
	if c.scripts == nil {
		c.scripts = scripts.DefaultRegistry()
	}
	if c.logger == nil {
		c.logger = slog.Default()
	}
	return c, nil
}

func (c *Client) ensureReady() error {
	if c == nil || c.reader == nil || c.scripts == nil {
		return errors.New("linkedin client is not initialized")
	}
	return nil
}

func (c *Client) readRaw(ctx context.Context, rendered bool, path, expr string) (json.RawMessage, error) {
	if err := c.ensureReady(); err != nil {
		return nil, err
	}
	targetURL := c.baseURL + path
	var (
		body json.RawMessage
		err  error
	)
	if rendered {
		if rp, ok := c.reader.(chrome.RenderedPageReader); ok {
			body, err = rp.ReadRendered(ctx, targetURL, expr)
		} else {
			body, err = c.reader.Read(ctx, targetURL, expr)
		}
	} else {
		body, err = c.reader.Read(ctx, targetURL, expr)
	}
	if err != nil {
		return nil, translateTransportError(err)
	}
	return body, nil
}

func (c *Client) logContractError(op string, err error) {
	if c == nil || c.logger == nil || err == nil {
		return
	}
	var ce *ContractError
	if errors.As(err, &ce) {
		c.logger.Warn("linkedin wire contract error", "op", op, "err", err)
	}
}

func readWire[T any](ctx context.Context, c *Client, rendered bool, op, path, expr string) (T, error) {
	var zero T
	body, err := c.readRaw(ctx, rendered, path, expr)
	if err != nil {
		return zero, err
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		cerr := contractError(op, "empty or null browser payload", nil)
		c.logContractError(op, cerr)
		return zero, cerr
	}
	var wire T
	if err := json.Unmarshal(trimmed, &wire); err != nil {
		cerr := contractError(op, "decode browser JSON", err)
		c.logContractError(op, cerr)
		return zero, cerr
	}
	return wire, nil
}

func readWireDomain[Wire any, Domain any](
	ctx context.Context,
	c *Client,
	rendered bool,
	op, path, expr string,
	convert func(Wire) (Domain, error),
) (Domain, error) {
	var zero Domain
	wire, err := readWire[Wire](ctx, c, rendered, op, path, expr)
	if err != nil {
		return zero, err
	}
	out, err := convert(wire)
	if err != nil {
		c.logContractError(op, err)
		return zero, err
	}
	return out, nil
}

func parseHTTPURL(raw string) (*url.URL, bool) {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return nil, false
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return nil, false
	}
	return u, true
}

func validHTTPURL(raw string) bool {
	_, ok := parseHTTPURL(raw)
	return ok
}

func optionalHTTPURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	u, ok := parseHTTPURL(trimmed)
	if !ok {
		return ""
	}
	if u.RawQuery != "" {
		if _, err := url.QueryUnescape(u.RawQuery); err != nil {
			return ""
		}
	}
	return trimmed
}
