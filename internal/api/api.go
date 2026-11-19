package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// NewHandler constructs the Wayfarer HTTP API handler with bearer token auth,
// security headers, panic recovery, and structured request logging.
func NewHandler(token string, service Service, opts ...Option) (http.Handler, error) {
	if token == "" {
		return nil, errors.New("api token must not be empty")
	}
	if service == nil {
		return nil, errors.New("api service must not be nil")
	}
	cfg := handlerConfig{
		logger: slog.Default(),
		now:    time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.logger == nil {
		cfg.logger = slog.Default()
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}

	refs, err := newSourceRefs(cfg.now)
	if err != nil {
		return nil, err
	}

	h := &handler{
		service: service,
		refs:    refs,
		logger:  cfg.logger,
	}

	mux := newRouter(h)
	chain := withGetOnly(cfg.logger)(mux)
	chain = withBearerAuth(token, cfg.logger)(chain)
	chain = withRecover(cfg.logger)(chain)
	chain = withRequestLog(cfg.logger)(chain)
	chain = withSecurityHeaders()(chain)
	return chain, nil
}

// New constructs the Wayfarer HTTP API handler using default options and panics
// if initialization fails.
func New(token string, service Service) http.Handler {
	h, err := NewHandler(token, service)
	if err != nil {
		panic(err)
	}
	return h
}
