package apperrors

import (
	"fmt"
	"net/http"
)

// AppError is the standard error struct that wraps an internal error
// and carries HTTP-specific metadata for the API wrapper to consume.
type AppError struct {
	Code    string
	Message string
	Status  int
	Err     error // Underlying wrapped error (never sent to client)
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// Canonical error codes
const (
	CodeValidation       = "VALIDATION_ERROR"
	CodeUnauthorized     = "UNAUTHORIZED"
	CodeForbidden        = "FORBIDDEN"
	CodeNotFound         = "NOT_FOUND"
	CodeInternal         = "INTERNAL_ERROR"
	CodeQuotaExceeded    = "QUOTA_EXCEEDED"
	CodeRateLimited      = "RATE_LIMITED"
	CodeBadRequest       = "BAD_REQUEST"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeConflict         = "CONFLICT"
)

// --- Helper Constructors ---

func NewValidation(msg string, err error) *AppError {
	return &AppError{
		Code:    CodeValidation,
		Message: msg,
		Status:  http.StatusUnprocessableEntity,
		Err:     err,
	}
}

func NewUnauthorized(msg string, err error) *AppError {
	return &AppError{
		Code:    CodeUnauthorized,
		Message: msg,
		Status:  http.StatusUnauthorized,
		Err:     err,
	}
}

func NewForbidden(msg string, err error) *AppError {
	return &AppError{
		Code:    CodeForbidden,
		Message: msg,
		Status:  http.StatusForbidden,
		Err:     err,
	}
}

func NewNotFound(msg string, err error) *AppError {
	return &AppError{
		Code:    CodeNotFound,
		Message: msg,
		Status:  http.StatusNotFound,
		Err:     err,
	}
}

func NewInternal(err error) *AppError {
	return &AppError{
		Code:    CodeInternal,
		Message: "An internal server error occurred",
		Status:  http.StatusInternalServerError,
		Err:     err, // The wrapper will log this securely and push to Sentry!
	}
}

func NewBadRequest(msg string, err error) *AppError {
	return &AppError{
		Code:    CodeBadRequest,
		Message: msg,
		Status:  http.StatusBadRequest,
		Err:     err,
	}
}

func NewMethodNotAllowed(msg string) *AppError {
	return &AppError{
		Code:    CodeMethodNotAllowed,
		Message: msg,
		Status:  http.StatusMethodNotAllowed,
		Err:     nil,
	}
}

func NewConflict(msg string, err error) *AppError {
	return &AppError{
		Code:    CodeConflict,
		Message: msg,
		Status:  http.StatusConflict,
		Err:     err,
	}
}

func NewQuotaExceeded(msg string) *AppError {
	return &AppError{
		Code:    CodeQuotaExceeded,
		Message: msg,
		Status:  http.StatusTooManyRequests,
		Err:     nil,
	}
}

func NewRateLimited(msg string) *AppError {
	return &AppError{
		Code:    CodeRateLimited,
		Message: msg,
		Status:  http.StatusTooManyRequests,
		Err:     nil,
	}
}
