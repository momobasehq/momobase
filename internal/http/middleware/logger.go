package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader records code before forwarding it to the wrapped response writer.
func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// StructuredLogger records the method, path, response status, duration, request
// identifier, and client address of each request using the supplied structured logger.
//
// The client address comes from the same resolver the rate limiter uses, so a log line
// and a 429 name the same address. Logging the raw peer while limiting on a forwarded
// one would make an incident impossible to follow.
func StructuredLogger(log *slog.Logger, ip ClientIP) func(http.Handler) http.Handler {
	if ip == nil {
		ip = RemoteClientIP
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rec, r)
			log.Info(
				"http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("ip", ip(r)),
				slog.String("request_id", RequestIDFrom(r.Context())),
			)
		})
	}
}
