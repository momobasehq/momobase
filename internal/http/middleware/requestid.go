package middleware

import (
	"context"
	"net/http"

	"github.com/momobasehq/momobase/internal/platform"
)

// RequestIDHeader carries the identifier in and out, so a proxy or caller that already
// has one can have it adopted rather than replaced.
const RequestIDHeader = "X-Request-Id"

// maxInboundRequestID bounds an adopted identifier. It reaches the logs, and an
// unbounded value from a caller would be a cheap way to bloat them.
const maxInboundRequestID = 64

// RequestID attaches an identifier to every request, echoes it in the response, and
// makes it available to handlers and the request log.
//
// An inbound X-Request-Id is adopted when it is present and plausibly sized, so a
// trace started by a proxy or a calling service stays one trace across both logs;
// otherwise one is generated. It is a correlation aid rather than a credential, so
// adopting a caller's value is safe — nothing authorizes on it.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" || len(id) > maxInboundRequestID || !printableASCII(id) {
			id = platform.NewID("req")
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// RequestIDFrom returns the request's identifier, or an empty string when the
// RequestID middleware did not run.
func RequestIDFrom(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

// printableASCII rejects control characters and non-ASCII bytes, which would otherwise
// reach a log line and could break a line-oriented log parser.
func printableASCII(value string) bool {
	for i := range len(value) {
		if value[i] < 0x20 || value[i] > 0x7e {
			return false
		}
	}
	return true
}
