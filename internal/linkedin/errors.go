package linkedin

import (
	"context"
	"errors"
	"fmt"

	"github.com/clysnelzum/wayfarer/internal/chrome"
)

var (
	// ErrJobNotFound indicates that LinkedIn reported the requested job posting as unavailable.
	ErrJobNotFound = errors.New("job not found")
	// ErrNotFound is an alias for ErrJobNotFound preserved for compatibility.
	ErrNotFound = ErrJobNotFound
	// ErrProfileNotFound indicates that LinkedIn reported the personal profile as unavailable.
	ErrProfileNotFound = errors.New("profile not found")
	// ErrCompanyNotFound indicates that LinkedIn reported the company profile as unavailable.
	ErrCompanyNotFound = errors.New("company not found")
	// ErrJobSourceUnavailable indicates that LinkedIn did not offer the requested recommendation source.
	ErrJobSourceUnavailable = errors.New("job recommendation source unavailable")
	// ErrSearchMismatch indicates that LinkedIn altered or dropped requested job search parameters.
	ErrSearchMismatch = errors.New("upstream search changed")
	// ErrInvalidInput indicates that caller-supplied parameters, IDs, or slugs failed validation.
	ErrInvalidInput = errors.New("invalid input")
	// ErrUpstreamContract indicates that a LinkedIn browser extractor violated the expected schema or invariants.
	ErrUpstreamContract = errors.New("upstream contract violation")
	// ErrAuthRequired indicates that Chrome's LinkedIn session requires authentication or verification.
	ErrAuthRequired = errors.New("upstream authentication required")
	// ErrUpstreamBusy indicates that the Chrome browser reader is at concurrency capacity.
	ErrUpstreamBusy = errors.New("upstream browser busy")
	// ErrDebuggerUnavailable indicates that the Chrome debugger connection is unreachable or closed.
	ErrDebuggerUnavailable = errors.New("upstream debugger unavailable")
)

// ContractError describes a schema or invariant violation in an upstream browser payload.
type ContractError struct {
	Operation string
	Reason    string
	Err       error
}

// Error returns a concise description of the upstream contract violation.
func (e *ContractError) Error() string {
	msg := ErrUpstreamContract.Error()
	if e.Operation != "" {
		msg += ": " + e.Operation
	}
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Unwrap returns ErrUpstreamContract joined with any underlying error so callers
// can match both ErrUpstreamContract and the root cause via errors.Is/As.
func (e *ContractError) Unwrap() error {
	if e.Err == nil {
		return ErrUpstreamContract
	}
	return errors.Join(ErrUpstreamContract, e.Err)
}

func contractError(operation, reason string, err error) error {
	return &ContractError{
		Operation: operation,
		Reason:    reason,
		Err:       err,
	}
}

func translateTransportError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, chrome.ErrAuthRequired), errors.Is(err, chrome.ErrAuth):
		return fmt.Errorf("%w: %w", ErrAuthRequired, err)
	case errors.Is(err, chrome.ErrBusy):
		return fmt.Errorf("%w: %w", ErrUpstreamBusy, err)
	case errors.Is(err, chrome.ErrDebuggerUnavailable), errors.Is(err, chrome.ErrClosed):
		return fmt.Errorf("%w: %w", ErrDebuggerUnavailable, err)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return err
	default:
		return err
	}
}
