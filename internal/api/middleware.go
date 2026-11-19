package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func withSecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			next.ServeHTTP(w, r)
		})
	}
}

func withRecover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					if logger != nil {
						logger.Error("http handler panic",
							"method", r.Method,
							"path", r.URL.Path,
							"panic", fmt.Sprint(rec),
						)
					}
					writeError(w, logger, http.StatusInternalServerError, "internal_error", "Internal server error.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (sr *statusRecorder) WriteHeader(status int) {
	if !sr.wrote {
		sr.status = status
		sr.wrote = true
	}
	sr.ResponseWriter.WriteHeader(status)
}

func (sr *statusRecorder) Write(p []byte) (int, error) {
	if !sr.wrote {
		sr.status = http.StatusOK
		sr.wrote = true
	}
	return sr.ResponseWriter.Write(p)
}

func (sr *statusRecorder) Unwrap() http.ResponseWriter {
	return sr.ResponseWriter
}

func withRequestLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if logger == nil {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration", time.Since(start),
			)
		})
	}
}

func withBearerAuth(token string, logger *slog.Logger) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			hash := sha256.Sum256([]byte(got))
			if token == "" || !ok || subtle.ConstantTimeCompare(want[:], hash[:]) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, logger, http.StatusUnauthorized, "unauthorized", "A valid API token is required.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func withGetOnly(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				writeError(w, logger, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is supported.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
