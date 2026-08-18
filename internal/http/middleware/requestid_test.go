package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestID(t *testing.T) {
	var seen string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	// An identifier a proxy or calling service already started is adopted, so one trace
	// stays one trace across both sets of logs.
	t.Run("adopts a plausible inbound identifier", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set(RequestIDHeader, "trace-from-the-proxy")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if seen != "trace-from-the-proxy" || recorder.Header().Get(RequestIDHeader) != "trace-from-the-proxy" {
			t.Errorf("context %q header %q, want the inbound value", seen, recorder.Header().Get(RequestIDHeader))
		}
	})

	t.Run("generates one when absent", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		if seen == "" || !strings.HasPrefix(seen, "req") {
			t.Errorf("generated identifier = %q", seen)
		}
		if recorder.Header().Get(RequestIDHeader) != seen {
			t.Error("the response header does not match the context value")
		}
	})

	// It reaches the logs, so an unbounded or control-laden value from a caller would be
	// a cheap way to bloat or corrupt them.
	t.Run("refuses an implausible inbound identifier", func(t *testing.T) {
		for name, value := range map[string]string{
			"oversize":         strings.Repeat("x", maxInboundRequestID+1),
			"newline":          "trace\nspoofed=true",
			"non ascii":        "trace-\xff",
			"control chararcs": "trace\x00",
		} {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set(RequestIDHeader, value)
			handler.ServeHTTP(httptest.NewRecorder(), request)
			if seen == value {
				t.Errorf("%s identifier was adopted", name)
			}
		}
	})
}

// RequestIDFrom must be safe on a request the middleware never touched, so a handler
// can log it unconditionally.
func TestRequestIDFromWithoutTheMiddleware(t *testing.T) {
	if got := RequestIDFrom(httptest.NewRequest(http.MethodGet, "/", nil).Context()); got != "" {
		t.Errorf("RequestIDFrom() = %q, want empty", got)
	}
}
