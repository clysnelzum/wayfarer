package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, logger *slog.Logger, status int, body any) {
	data, err := json.Marshal(body)
	if err != nil {
		if logger != nil {
			logger.Error("marshal json response", "error", err)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"Failed to encode response."}}` + "\n"))
		return
	}
	data = append(data, '\n')
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil && logger != nil {
		logger.Debug("write json response", "error", err)
	}
}

func writeError(w http.ResponseWriter, logger *slog.Logger, status int, code, message string) {
	writeJSON(w, logger, status, errorResponse{
		Error: errorDetail{Code: code, Message: message},
	})
}

func writeResult(w http.ResponseWriter, logger *slog.Logger, value any, err error) {
	if err == nil {
		writeJSON(w, logger, http.StatusOK, value)
		return
	}
	status, code, message, retryAfter := mapError(err)
	if status == 0 {
		return
	}
	if status >= http.StatusInternalServerError && logger != nil {
		logger.Warn("upstream request failed", "status", status, "code", code, "err", err)
	}
	if retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	writeError(w, logger, status, code, message)
}

func mapError(err error) (status int, code, message, retryAfter string) {
	switch {
	case errors.Is(err, linkedin.ErrSearchMismatch):
		return http.StatusBadGateway, "upstream_search_changed", "LinkedIn did not retain the requested job search, page, or filters.", ""
	case errors.Is(err, linkedin.ErrJobSourceUnavailable):
		return http.StatusNotFound, "recommendation_source_unavailable", "LinkedIn did not show this recommendation source in the current session.", ""
	case errors.Is(err, linkedin.ErrJobNotFound):
		return http.StatusNotFound, "job_not_found", "LinkedIn reports that the job is unavailable.", ""
	case errors.Is(err, linkedin.ErrProfileNotFound):
		return http.StatusNotFound, "profile_not_found", "LinkedIn reports that the personal profile is unavailable.", ""
	case errors.Is(err, linkedin.ErrCompanyNotFound):
		return http.StatusNotFound, "company_not_found", "LinkedIn reports that the company profile is unavailable.", ""
	case errors.Is(err, linkedin.ErrInvalidInput):
		return http.StatusBadRequest, "invalid_query", err.Error(), ""
	case errors.Is(err, linkedin.ErrDebuggerUnavailable):
		return http.StatusServiceUnavailable, "upstream_debugger_unavailable", "Check the Chrome debugger endpoint and approve the connection in Chrome if prompted.", ""
	case errors.Is(err, linkedin.ErrAuthRequired):
		return http.StatusServiceUnavailable, "upstream_auth_required", "Sign in or complete the verification in Chrome.", ""
	case errors.Is(err, linkedin.ErrUpstreamBusy):
		return http.StatusServiceUnavailable, "upstream_busy", "Chrome has too many active queries.", "2"
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "upstream_timeout", "Chrome did not provide the expected page data in time.", ""
	case errors.Is(err, context.Canceled):
		return 0, "", "", ""
	case errors.Is(err, linkedin.ErrUpstreamContract):
		return http.StatusBadGateway, "upstream_error", "Could not read the LinkedIn page via Chrome.", ""
	default:
		return http.StatusBadGateway, "upstream_error", "Could not read the LinkedIn page via Chrome.", ""
	}
}
