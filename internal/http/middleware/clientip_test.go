package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func request(remote, forwarded string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	if forwarded != "" {
		r.Header.Set("X-Forwarded-For", forwarded)
	}
	return r
}

// TestRemoteClientIPIgnoresForwardedHeaders is the safety default: a header any caller
// can set must not decide which rate-limit bucket they land in.
func TestRemoteClientIPIgnoresForwardedHeaders(t *testing.T) {
	if got := RemoteClientIP(request("192.0.2.10:5555", "203.0.113.9")); got != "192.0.2.10" {
		t.Errorf("RemoteClientIP() = %q, want the peer address", got)
	}
	// A RemoteAddr with no port still resolves, since not every transport supplies one.
	if got := RemoteClientIP(request("192.0.2.10", "")); got != "192.0.2.10" {
		t.Errorf("RemoteClientIP() without a port = %q", got)
	}
}

func TestNewForwardedClientIPWithoutTrustIsTheDefault(t *testing.T) {
	resolve := mustResolver(t, nil)
	if got := resolve(request("192.0.2.10:5555", "203.0.113.9")); got != "192.0.2.10" {
		t.Errorf("an empty trust list honoured a forwarded header: %q", got)
	}
}

// TestForwardedClientIPTrustIsDirectional is the property that makes the header usable
// at all: it is believed only when the request arrived from a proxy we named.
func TestForwardedClientIPTrustIsDirectional(t *testing.T) {
	resolve := mustResolver(t, []string{"10.0.0.0/8", "192.0.2.1"})

	tests := []struct {
		name      string
		remote    string
		forwarded string
		want      string
	}{
		{"trusted peer yields the client", "10.0.0.5:1234", "203.0.113.9", "203.0.113.9"},
		{"a single trusted host is accepted without a mask", "192.0.2.1:1234", "203.0.113.9", "203.0.113.9"},
		// The walk stops at the first untrusted hop from the right: anything further
		// left was supplied by something upstream and cannot be verified.
		{"walks past trusted proxies only", "10.0.0.5:1234", "198.51.100.7, 203.0.113.9, 10.0.0.6", "203.0.113.9"},
		// The spoofing case: an untrusted caller setting the header changes nothing.
		{"untrusted peer keeps its own address", "203.0.113.50:1234", "10.0.0.9", "203.0.113.50"},
		{"a malformed hop ends the walk", "10.0.0.5:1234", "not-an-ip", "10.0.0.5"},
		{"no header falls back to the peer", "10.0.0.5:1234", "", "10.0.0.5"},
		{"every hop trusted falls back to the peer", "10.0.0.5:1234", "10.0.0.7, 10.0.0.8", "10.0.0.5"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolve(request(test.remote, test.forwarded)); got != test.want {
				t.Errorf("resolved %q, want %q", got, test.want)
			}
		})
	}
}

// An IPv4-mapped IPv6 peer must match an IPv4 prefix, or a proxy reaching the server
// over a dual-stack listener would silently stop being trusted.
func TestForwardedClientIPUnmapsIPv4(t *testing.T) {
	resolve := mustResolver(t, []string{"10.0.0.0/8"})
	if got := resolve(request("[::ffff:10.0.0.5]:1234", "203.0.113.9")); got != "203.0.113.9" {
		t.Errorf("an IPv4-mapped trusted peer was not recognised: %q", got)
	}
}

func TestNewForwardedClientIPRejectsMalformedTrust(t *testing.T) {
	for _, entry := range []string{"not-an-ip", "10.0.0.0/999", "10.0.0.0/", "example.com"} {
		if _, err := NewForwardedClientIP([]string{entry}); err == nil {
			t.Errorf("NewForwardedClientIP(%q) = nil, want an error", entry)
		}
	}
}

// TestRateLimitByIPUsesTheResolver proves the limiter keys on the resolved client
// rather than the peer, which is the whole point behind a proxy.
func TestRateLimitByIPUsesTheResolver(t *testing.T) {
	resolve := mustResolver(t, []string{"10.0.0.0/8"})
	handler := RateLimitByIP(1, time.Minute, resolve)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// Two requests through the same proxy from different clients each get their own
	// bucket; before this they shared the proxy's one.
	for _, client := range []string{"203.0.113.9", "203.0.113.10"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request("10.0.0.5:1234", client))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("first request for %s = %d", client, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request("10.0.0.5:1234", "203.0.113.9"))
	if recorder.Code != http.StatusTooManyRequests {
		t.Errorf("second request for one client = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
}

func mustResolver(t *testing.T, cidrs []string) ClientIP {
	t.Helper()
	resolve, err := NewForwardedClientIP(cidrs)
	if err != nil {
		t.Fatalf("NewForwardedClientIP() error = %v", err)
	}
	return resolve
}
