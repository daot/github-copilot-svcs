package internal

import (
	"fmt"
	"net/http"
)

// Error types for different categories of errors
type (
	// AuthenticationError represents authentication-related errors
	AuthenticationError struct {
		Message string
		Err     error
	}

	// ConfigurationError represents configuration-related errors
	ConfigurationError struct {
		Field   string
		Value   interface{}
		Message string
		Err     error
	}

	// NetworkError represents network-related errors
	NetworkError struct {
		Operation string
		URL       string
		Message   string
		Err       error
	}

	// ValidationError represents validation errors
	ValidationError struct {
		Field   string
		Value   interface{}
		Message string
		Err     error
	}

	// ProxyError represents proxy operation errors
	ProxyError struct {
		Operation string
		Message   string
		Err       error
	}
)

// Error implementations
func (e *AuthenticationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("authentication error: %s: %v", e.Message, e.Err)
	}
	return fmt.Sprintf("authentication error: %s", e.Message)
}

func (e *AuthenticationError) Unwrap() error {
	return e.Err
}

func (e *ConfigurationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("configuration error for %s=%v: %s: %v", e.Field, e.Value, e.Message, e.Err)
	}
	return fmt.Sprintf("configuration error for %s=%v: %s", e.Field, e.Value, e.Message)
}

func (e *ConfigurationError) Unwrap() error {
	return e.Err
}

func (e *NetworkError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("network error during %s to %s: %s: %v", e.Operation, e.URL, e.Message, e.Err)
	}
	return fmt.Sprintf("network error during %s to %s: %s", e.Operation, e.URL, e.Message)
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

func (e *ValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("validation error for %s=%v: %s: %v", e.Field, e.Value, e.Message, e.Err)
	}
	return fmt.Sprintf("validation error for %s=%v: %s", e.Field, e.Value, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

func (e *ProxyError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("proxy error during %s: %s: %v", e.Operation, e.Message, e.Err)
	}
	return fmt.Sprintf("proxy error during %s: %s", e.Operation, e.Message)
}

func (e *ProxyError) Unwrap() error {
	return e.Err
}

// Error constructors for common scenarios
func NewAuthError(message string, err error) *AuthenticationError {
	return &AuthenticationError{Message: message, Err: err}
}

func NewConfigError(field string, value interface{}, message string, err error) *ConfigurationError {
	return &ConfigurationError{Field: field, Value: value, Message: message, Err: err}
}

func NewNetworkError(operation, url, message string, err error) *NetworkError {
	return &NetworkError{Operation: operation, URL: url, Message: message, Err: err}
}

func NewValidationError(field string, value interface{}, message string, err error) *ValidationError {
	return &ValidationError{Field: field, Value: value, Message: message, Err: err}
}

func NewProxyError(operation, message string, err error) *ProxyError {
	return &ProxyError{Operation: operation, Message: message, Err: err}
}

// HTTP error helpers
func WriteHTTPError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `{"error": {"message": "%s", "type": "error", "code": %d}}`, message, statusCode)
}

func WriteHTTPErrorWithDetails(w http.ResponseWriter, statusCode int, errorType, message, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `{"error": {"message": "%s", "type": "%s", "code": %d, "details": "%s"}}`,
		message, errorType, statusCode, details)
}

// Common HTTP error responses
func WriteAuthenticationError(w http.ResponseWriter) {
	WriteHTTPError(w, http.StatusUnauthorized, "Authentication required")
}

func WriteAuthorizationError(w http.ResponseWriter) {
	WriteHTTPError(w, http.StatusForbidden, "Insufficient permissions")
}

func WriteValidationError(w http.ResponseWriter, message string) {
	WriteHTTPError(w, http.StatusBadRequest, message)
}

func WriteInternalError(w http.ResponseWriter) {
	WriteHTTPError(w, http.StatusInternalServerError, "Internal server error")
}

func WriteServiceUnavailableError(w http.ResponseWriter) {
	WriteHTTPError(w, http.StatusServiceUnavailable, "Service temporarily unavailable")
}

func WriteRateLimitError(w http.ResponseWriter) {
	WriteHTTPError(w, http.StatusTooManyRequests, "Rate limit exceeded")
}

// Error classification helpers
func IsAuthenticationError(err error) bool {
	_, ok := err.(*AuthenticationError)
	return ok
}

func IsConfigurationError(err error) bool {
	_, ok := err.(*ConfigurationError)
	return ok
}

func IsNetworkError(err error) bool {
	_, ok := err.(*NetworkError)
	return ok
}

func IsValidationError(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}

func IsProxyError(err error) bool {
	_, ok := err.(*ProxyError)
	return ok
}
