package openmeteo

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrUnavailable marks temporary upstream failures: network, 5xx, timeouts.
	ErrUnavailable = errors.New("open-meteo source unavailable")
	// ErrRateLimited marks HTTP 429 after the retry budget is exhausted.
	ErrRateLimited = errors.New("open-meteo rate limit exceeded")
	// ErrInvalidResponse marks a 4xx answer or a body that cannot be decoded.
	ErrInvalidResponse = errors.New("open-meteo invalid response")
)

type statusError struct {
	kind       error
	statusCode int
	reason     string
	retryAfter time.Duration
}

func (e *statusError) Error() string {
	if e.reason == "" {
		return fmt.Sprintf("%s: http %d", e.kind, e.statusCode)
	}

	return fmt.Sprintf("%s: http %d: %s", e.kind, e.statusCode, e.reason)
}

func (e *statusError) Unwrap() error {
	return e.kind
}
