package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/services"
)

func TestRequestPolicyMiddleware(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	JSONOnly(next).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnsupportedMediaType || called {
		t.Fatalf("JSONOnly() status/called = %d, %v", recorder.Code, called)
	}

	called = false
	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "Application/JSON; charset=utf-8")
	NoCache(JSONOnly(next)).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted || !called {
		t.Fatalf("JSONOnly(valid) status/called = %d, %v", recorder.Code, called)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("NoCache() header = %q", got)
	}
}

func TestMaxBodyBytesEnforcesLimit(t *testing.T) {
	handler := MaxBodyBytes(4)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345"))
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("MaxBodyBytes() status = %d", recorder.Code)
	}
}

func TestRecoverConvertsPanicToJSONError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "SERVER_ERROR") {
		t.Fatalf("Recover() response = %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(logs.String(), "http panic") || !strings.Contains(logs.String(), "boom") {
		t.Fatalf("Recover() log = %s", logs.String())
	}
}

func TestRateLimitByIPIsolatedByClient(t *testing.T) {
	handler := RateLimitByIP(2, time.Minute, RemoteClientIP)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for attempt := 1; attempt <= 3; attempt++ {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		handler.ServeHTTP(recorder, req)
		want := http.StatusNoContent
		if attempt == 3 {
			want = http.StatusTooManyRequests
		}
		if recorder.Code != want {
			t.Fatalf("attempt %d status = %d, want %d", attempt, recorder.Code, want)
		}
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.2"
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("different client status = %d", recorder.Code)
	}
	if got := RemoteClientIP(req); got != "198.51.100.2" {
		t.Fatalf("RemoteClientIP() = %q", got)
	}
}

func TestAuthenticationContextAndAuthorization(t *testing.T) {
	if got := BearerToken(httptest.NewRequest(http.MethodGet, "/", nil)); got != "" {
		t.Fatalf("BearerToken() without header = %q", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer  token-value ")
	type requestMarker struct{}
	req = req.WithContext(context.WithValue(req.Context(), requestMarker{}, true))
	if got := BearerToken(req); got != "token-value" {
		t.Fatalf("BearerToken() = %q", got)
	}

	verified := &domain.AdminUser{Role: "operations", Permissions: []string{"transactions:read"}}
	auth := authenticate(adminKey, func(ctx context.Context, token string) (*domain.AdminUser, error) {
		if marked, _ := ctx.Value(requestMarker{}).(bool); token == "token-value" && !marked {
			t.Fatal("request context was not passed to authentication")
		}
		if token != "token-value" {
			return nil, errors.New("bad token")
		}
		return verified, nil
	})
	protected := auth(RequirePermission("transactions:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AdminUser(r) != verified {
			t.Fatal("authenticated admin was not stored in context")
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	recorder := httptest.NewRecorder()
	protected.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d", recorder.Code)
	}

	bad := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder = httptest.NewRecorder()
	protected.ServeHTTP(recorder, bad)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", recorder.Code)
	}

	// A role that grants nothing is refused, which is also what an administrator whose
	// role was deleted looks like: no permissions resolve, so nothing is authorized.
	forbidden := httptest.NewRequest(http.MethodGet, "/", nil)
	forbidden = forbidden.WithContext(context.WithValue(forbidden.Context(), adminKey, &domain.AdminUser{Role: "viewer"}))
	recorder = httptest.NewRecorder()
	RequirePermission("transactions:read")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("forbidden handler was called")
	})).ServeHTTP(recorder, forbidden)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("RequirePermission() status = %d", recorder.Code)
	}
}

// TestRequirePermissionHonorsTheWildcard pins the mechanism that keeps super_admin
// correct as permissions are added: the role holds "*", not an enumerated set, so a
// permission introduced by a later release needs no migration to reach it.
func TestRequirePermissionHonorsTheWildcard(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(
		req.Context(),
		adminKey,
		&domain.AdminUser{Role: "super_admin", Permissions: []string{domain.PermissionWildcard}},
	))
	for _, permission := range []string{"transactions:read", "roles:delete", "something:invented:later"} {
		recorder := httptest.NewRecorder()
		RequirePermission(permission)(next).ServeHTTP(recorder, req)
		if recorder.Code != http.StatusNoContent {
			t.Errorf("RequirePermission(%q) with the wildcard = %d, want %d", permission, recorder.Code, http.StatusNoContent)
		}
	}
}

func TestRequireAppScope(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	identity := &services.AppIdentity{Credential: domain.AppCredential{Scopes: "transactions:read collections:create"}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), appKey, identity))
	recorder := httptest.NewRecorder()
	RequireAppScope("transactions:read")(next).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent || App(req) != identity {
		t.Fatalf("RequireAppScope(valid) status = %d", recorder.Code)
	}

	identity.Credential.Scopes = "collections:create"
	recorder = httptest.NewRecorder()
	RequireAppScope("transactions:read")(next).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("RequireAppScope(missing) status = %d", recorder.Code)
	}
	// Admin roles and app credentials share one wildcard rule, so the two paths
	// cannot come to disagree on what "*" grants.
	if !granted([]string{"*"}, "anything") || granted([]string{"read"}, "write") {
		t.Fatal("granted() wildcard or mismatch behavior is incorrect")
	}
}

func TestStructuredLoggerRecordsRequest(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	// Wrapped in RequestID so the log carries the identifier, which is the point of
	// having one: it is what ties a report to the line that produced it.
	handler := RequestID(StructuredLogger(logger, RemoteClientIP)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})))
	req := httptest.NewRequest(http.MethodPost, "/payments", nil)
	req.RemoteAddr = "192.0.2.10:4321"
	req.Header.Set(RequestIDHeader, "trace-1")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	log := output.String()
	// The address is the resolved client, without the ephemeral port: it has to match
	// what the rate limiter keys on, or a 429 and its log line name different things.
	for _, value := range []string{"http_request", "POST", "/payments", `"status":201`, `"ip":"192.0.2.10"`, `"request_id":"trace-1"`} {
		if !strings.Contains(log, value) {
			t.Fatalf("StructuredLogger() log %q does not contain %q", log, value)
		}
	}
}
