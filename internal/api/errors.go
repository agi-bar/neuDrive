package api

import (
	"log/slog"
	"net/http"
)

// Standard API error codes
const (
	ErrCodeBadRequest   = "bad_request"
	ErrCodeUnauthorized = "unauthorized"
	ErrCodeForbidden    = "forbidden"
	ErrCodeNotFound     = "not_found"
	ErrCodeConflict     = "conflict"
	ErrCodeInternal     = "internal_error"
	ErrCodeUnsupported  = "unsupported"
	ErrCodeRateLimit    = "rate_limit_exceeded"
	ErrCodeValidation   = "validation_error"
)

// APIError is the standard error response envelope.
type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// APISuccess is the standard success response envelope.
type APISuccess struct {
	OK   bool        `json:"ok"`
	Data interface{} `json:"data"`
}

// respondOK writes a 200 response with the data wrapped in an APISuccess envelope.
func respondOK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, APISuccess{OK: true, Data: data})
}

// respondCreated writes a 201 response with the data wrapped in an APISuccess envelope.
func respondCreated(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusCreated, APISuccess{OK: true, Data: data})
}

// respondError writes an error response with the given status, code, and message.
func respondError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, APIError{Code: code, Message: message})
}

// respondValidationError writes a 422 validation error for a specific field.
func respondValidationError(w http.ResponseWriter, field, message string) {
	writeJSON(w, http.StatusUnprocessableEntity, APIError{
		Code:    ErrCodeValidation,
		Message: message,
		Details: map[string]string{"field": field},
	})
}

// respondNotFound writes a 404 not-found error for the given resource type.
func respondNotFound(w http.ResponseWriter, resource string) {
	respondError(w, http.StatusNotFound, ErrCodeNotFound, resource+" not found")
}

// respondForbidden writes a 403 forbidden error with the given message.
func respondForbidden(w http.ResponseWriter, message string) {
	respondError(w, http.StatusForbidden, ErrCodeForbidden, message)
}

// respondUnauthorized writes a 401 unauthorized error.
func respondUnauthorized(w http.ResponseWriter) {
	respondError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "missing or invalid authentication")
}

// respondInternalError logs the actual error and returns a generic 500 message.
func respondInternalError(w http.ResponseWriter, err error) {
	slog.Error("internal error", "error", err)
	respondError(w, http.StatusInternalServerError, ErrCodeInternal, "an internal error occurred")
}
