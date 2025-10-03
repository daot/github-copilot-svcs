// Package internal provides HTTP middleware for github-copilot-svcs.
package internal

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// HTTP status code constants
const (
	statusServerError = 500
	statusClientError = 400
)

// LoggingResponseWriter wraps http.ResponseWriter to capture response data and status code.
type LoggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

// NewLoggingResponseWriter ...
func NewLoggingResponseWriter(w http.ResponseWriter) *LoggingResponseWriter {
	return &LoggingResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           bytes.NewBuffer(nil),
	}
}

// WriteHeader ...
func (lrw *LoggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *LoggingResponseWriter) Write(body []byte) (int, error) {
	// Write to both the original response and our buffer
	lrw.body.Write(body)
	return lrw.ResponseWriter.Write(body)
}

// Hijack ...
func (lrw *LoggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := lrw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// StatusCode ...
func (lrw *LoggingResponseWriter) StatusCode() int {
	return lrw.statusCode
}

// Body ...
func (lrw *LoggingResponseWriter) Body() []byte {
	return lrw.body.Bytes()
}

// LoggingMiddleware logs HTTP requests and responses, including status code and duration.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create logging response writer
		lrw := NewLoggingResponseWriter(w)

		// Read and store request body for logging (if reasonable size)
		var requestBody []byte
		if r.Body != nil && r.ContentLength > 0 && r.ContentLength < 1024*1024 { // Max 1MB for logging
			requestBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Log request
		Info("HTTP Request",
			"method", r.Method,
			"url", r.URL.String(),
			"remote_addr", getClientIP(r),
			"user_agent", r.UserAgent(),
			"content_length", r.ContentLength,
			"has_body", len(requestBody) > 0,
		)

		// Process request
		next.ServeHTTP(lrw, r)

		// Calculate duration
		duration := time.Since(start)

		// Determine log level based on status code
		statusCode := lrw.StatusCode()
		responseSize := len(lrw.Body())

		logArgs := []interface{}{
			"method", r.Method,
			"url", r.URL.String(),
			"status_code", statusCode,
			"duration_ms", duration.Milliseconds(),
			"response_size", responseSize,
			"remote_addr", getClientIP(r),
		}

		// Log response with appropriate level
		switch {
		case statusCode >= statusServerError:
			Error("HTTP Response", logArgs...)
		case statusCode >= statusClientError:
			Warn("HTTP Response", logArgs...)
		default:
			Info("HTTP Response", logArgs...)
		}

		// Log response body for debugging if it's small and there was an error
		if statusCode >= 400 && responseSize > 0 && responseSize < 1024 {
			Debug("HTTP Response Body", "body", string(lrw.Body()))
		}
	})
}

// RecoveryMiddleware ...
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				Error("HTTP Handler Panic",
					"error", err,
					"method", r.Method,
					"url", r.URL.String(),
					"remote_addr", getClientIP(r),
				)

				WriteInternalError(w)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware ...
func CORSMiddleware(config *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Set CORS headers based on configuration
			if len(config.CORS.AllowedOrigins) > 0 {
				if containsOrigin(config.CORS.AllowedOrigins, origin) || containsOrigin(config.CORS.AllowedOrigins, "*") {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			if len(config.CORS.AllowedHeaders) > 0 {
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.CORS.AllowedHeaders, ", "))
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersMiddleware ...
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Only set HSTS for HTTPS requests
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// TimeoutMiddleware sets a timeout for HTTP requests using http.TimeoutHandler.
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, timeout, "Request timeout")
	}
}

// CompressionResponseWriter wraps http.ResponseWriter to handle compression
type CompressionResponseWriter struct {
	http.ResponseWriter
	gzipWriter *gzip.Writer
	compressed bool
}

// NewCompressionResponseWriter creates a new compression response writer
func NewCompressionResponseWriter(w http.ResponseWriter, r *http.Request) *CompressionResponseWriter {
	// Check if client accepts gzip encoding
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		gz := gzip.NewWriter(w)
		return &CompressionResponseWriter{
			ResponseWriter: w,
			gzipWriter:     gz,
			compressed:     true,
		}
	}

	return &CompressionResponseWriter{
		ResponseWriter: w,
		compressed:     false,
	}
}

// WriteHeader handles the status code and sets compression headers if needed
func (crw *CompressionResponseWriter) WriteHeader(statusCode int) {
	if crw.compressed {
		crw.ResponseWriter.Header().Set("Content-Encoding", "gzip")
		crw.ResponseWriter.Header().Set("Vary", "Accept-Encoding")
	}
	crw.ResponseWriter.WriteHeader(statusCode)
}

// Write writes data, compressing if enabled
func (crw *CompressionResponseWriter) Write(data []byte) (int, error) {
	if crw.compressed {
		return crw.gzipWriter.Write(data)
	}
	return crw.ResponseWriter.Write(data)
}

// Close closes the gzip writer if compression is enabled
func (crw *CompressionResponseWriter) Close() error {
	if crw.compressed {
		return crw.gzipWriter.Close()
	}
	return nil
}

// CompressionMiddleware adds gzip compression for compressible content
func CompressionMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only compress certain content types
			contentType := r.Header.Get("Content-Type")
			shouldCompress := strings.Contains(contentType, "text/") ||
				strings.Contains(contentType, "application/json") ||
				strings.Contains(contentType, "application/javascript") ||
				strings.Contains(contentType, "text/css") ||
				strings.Contains(contentType, "text/html")

			// Don't compress if client doesn't accept gzip or content is already compressed
			acceptEncoding := r.Header.Get("Accept-Encoding")
			if !shouldCompress || !strings.Contains(acceptEncoding, "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			// Create compression writer
			crw := NewCompressionResponseWriter(w, r)
			defer func() {
				_ = crw.Close() // Ignore error as response is already sent
			}()

			// Serve the request with compression
			next.ServeHTTP(crw, r)
		})
	}
}

// Helper functions
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header (nginx)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}

	return r.RemoteAddr
}

func containsOrigin(origins []string, origin string) bool {
	for _, o := range origins {
		if o == origin || o == "*" {
			return true
		}
	}
	return false
}
