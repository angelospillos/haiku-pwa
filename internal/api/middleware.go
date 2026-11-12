package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

const contentSecurityPolicy = "default-src 'self'; " +
	"img-src 'self' data: blob: https: http:; " +
	"style-src 'self'; script-src 'self'; connect-src 'self'; " +
	"manifest-src 'self'; worker-src 'self'; " +
	"base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

type requestIDKey struct{}

// withMiddleware wraps next with request IDs, structured access logging, panic
// recovery and security headers.
func withMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return withRequestLog(logger, withSecurityHeaders(next))
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Content-Security-Policy", contentSecurityPolicy)
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		headers.Set("Permissions-Policy", "geolocation=(), microphone=()")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(data []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(data)
}

func withRequestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := newRequestID()
		recorder := &statusRecorder{ResponseWriter: w}
		recorder.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))

		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(r.Context(), "panic while serving request",
					"request_id", id, "panic", recovered, "stack", string(debug.Stack()))
				if recorder.status == 0 {
					writeError(recorder, http.StatusInternalServerError, "Something went wrong on the server")
				}
			}
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			logger.InfoContext(r.Context(), "request",
				"request_id", id,
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}()

		next.ServeHTTP(recorder, r)
	})
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(buf)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
