package models

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by value objects and repositories. Callers match them
// with errors.Is; the HTTP layer maps them to 400 and 404 respectively.
var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	ErrConflict        = errors.New("conflict")
)

// invalidError carries the human-readable detail while still matching
// errors.Is(err, ErrInvalidArgument).
type invalidError struct {
	detail string
}

func (e invalidError) Error() string {
	return e.detail
}

func (e invalidError) Unwrap() error {
	return ErrInvalidArgument
}

// Invalid wraps a validation failure in the ErrInvalidArgument chain.
func Invalid(detail string) error {
	return invalidError{detail: detail}
}

// Invalidf is the formatted variant of Invalid.
func Invalidf(format string, args ...any) error {
	return invalidError{detail: fmt.Sprintf(format, args...)}
}
