package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

// Service defines the LinkedIn read operations exposed by the HTTP API.
type Service interface {
	Feed(ctx context.Context, limit int) (linkedin.FeedResults, error)
	Jobs(ctx context.Context, query linkedin.JobQuery) (linkedin.JobResults, error)
	JobLocations(ctx context.Context, query string) (linkedin.JobLocations, error)
	JobCompanies(ctx context.Context, query string) (linkedin.JobCompanies, error)
	Job(ctx context.Context, id string) (linkedin.Job, error)
	Profile(ctx context.Context, slug string) (linkedin.Profile, error)
	Company(ctx context.Context, slug string) (linkedin.Company, error)
}

// Option configures an API handler constructed with NewHandler.
type Option func(*handlerConfig)

type handlerConfig struct {
	logger *slog.Logger
	now    func() time.Time
}

// WithLogger sets the structured logger used for request logs, panic recovery,
// and JSON serialization errors. Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(cfg *handlerConfig) {
		if logger != nil {
			cfg.logger = logger
		}
	}
}

// WithClock overrides the clock used for source_ref sealing and expiration.
// Defaults to time.Now.
func WithClock(now func() time.Time) Option {
	return func(cfg *handlerConfig) {
		if now != nil {
			cfg.now = now
		}
	}
}
