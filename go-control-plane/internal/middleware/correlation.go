package middleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/logging"
)

const (
	// CorrelationIDHeader is the HTTP header for correlation IDs
	CorrelationIDHeader = "X-Correlation-ID"
	// RequestIDHeader is the HTTP header for request IDs
	RequestIDHeader = "X-Request-ID"
)

// CorrelationID middleware adds correlation and request IDs to context
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get or generate correlation ID
		correlationID := r.Header.Get(CorrelationIDHeader)
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Get or generate request ID
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add IDs to context
		ctx = logging.WithCorrelationID(ctx, correlationID)
		ctx = logging.WithRequestID(ctx, requestID)

		// Add IDs to response headers for tracing
		w.Header().Set(CorrelationIDHeader, correlationID)
		w.Header().Set(RequestIDHeader, requestID)

		// Continue with updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestLogger middleware logs HTTP requests with correlation IDs
func RequestLogger(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create response writer wrapper to capture status code
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Log request
			logger.WithContext(r.Context()).WithFields(map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
				"remote": r.RemoteAddr,
				"agent":  r.UserAgent(),
			}).Info("HTTP request received")

			// Process request
			next.ServeHTTP(rw, r)

			// Log response
			logger.WithContext(r.Context()).WithFields(map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
				"status": rw.statusCode,
			}).Info("HTTP request completed")
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Made with Bob
