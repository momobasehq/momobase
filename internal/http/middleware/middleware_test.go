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
	handler := RateLimitByIP(2, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	if got := clientIP(req); got != "198.51.100.2" {
		t.Fatalf("clientIP() = %q", got)
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

	verified := &domain.AdminUser{Role: "operations"}
	auth := authenticate(adminKey, func(ctx context.Context, token string) (*domain.AdminUser, error) {
		if marked, _ := ctx.Value(requestMarker{}).(bool); token == "token-value" && !marked {
			t.Fatal("request context was not passed to authentication")
		}
		if token != "token-value" {
			return nil, errors.New("bad token")
		}
		return verified, nil
	})
	protected := auth(RequireRole("operations")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	forbidden := httptest.NewRequest(http.MethodGet, "/", nil)
	forbidden = forbidden.WithContext(context.WithValue(forbidden.Context(), adminKey, &domain.AdminUser{Role: "viewer"}))
	recorder = httptest.NewRecorder()
	RequireRole("operations")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("forbidden handler was called")
	})).ServeHTTP(recorder, forbidden)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("RequireRole() status = %d", recorder.Code)
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
	if !hasScope("*", "anything") || hasScope("read", "write") {
		t.Fatal("hasScope() wildcard or mismatch behavior is incorrect")
	}
}

func TestStructuredLoggerRecordsRequest(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	handler := StructuredLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/payments", nil)
	req.RemoteAddr = "192.0.2.10:4321"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	log := output.String()
	for _, value := range []string{"http_request", "POST", "/payments", `"status":201`, "192.0.2.10:4321"} {
		if !strings.Contains(log, value) {
			t.Fatalf("StructuredLogger() log %q does not contain %q", log, value)
		}
	}
}
