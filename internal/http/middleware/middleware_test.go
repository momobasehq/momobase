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

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
)

// serve runs a chain against req on a throwaway app. A fiber.Ctx cannot be built
// directly, so middleware is exercised through a request rather than in isolation.
func serve(t *testing.T, req *http.Request, chain ...fiber.Handler) *http.Response {
	t.Helper()
	app := fiber.New()
	rest := make([]any, 0, len(chain)-1)
	for _, handler := range chain[1:] {
		rest = append(rest, handler)
	}
	app.All("/*", chain[0], rest...)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	return res
}

func accepted(c fiber.Ctx) error { return c.SendStatus(http.StatusAccepted) }

func TestRequestPolicyMiddleware(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	res := serve(t, req, JSONOnly, accepted)
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("JSONOnly() status = %d", res.StatusCode)
	}

	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header.Set(fiber.HeaderContentType, "Application/JSON; charset=utf-8")
	res = serve(t, req, NoCache, JSONOnly, accepted)
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("JSONOnly(valid) status = %d", res.StatusCode)
	}
	if got := res.Header.Get(fiber.HeaderCacheControl); got != "no-store" {
		t.Fatalf("NoCache() header = %q", got)
	}
}

// TestBoundRequestIDDropsAnOversizedHeader pins the half of the request-id contract
// Fiber's middleware does not cover. It refuses anything but visible ASCII; it does
// not bound the length, and an unbounded value from a caller reaches every log line.
func TestBoundRequestIDDropsAnOversizedHeader(t *testing.T) {
	echo := func(c fiber.Ctx) error { return c.SendString(requestid.FromContext(c)) }

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(fiber.HeaderXRequestID, "trace-from-the-proxy")
	res := serve(t, req, BoundRequestID, requestid.New(), echo)
	if got := readBody(t, res); got != "trace-from-the-proxy" {
		t.Fatalf("adopted request id = %q, want the inbound value", got)
	}

	oversized := strings.Repeat("x", maxInboundRequestID+1)
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(fiber.HeaderXRequestID, oversized)
	res = serve(t, req, BoundRequestID, requestid.New(), echo)
	if got := readBody(t, res); got == oversized || got == "" {
		t.Fatalf("request id = %q, want a generated replacement", got)
	}
}

func TestAuthenticationContextAndAuthorization(t *testing.T) {
	bearer := func(c fiber.Ctx) error { return c.SendString(BearerToken(c)) }
	if got := readBody(t, serve(t, httptest.NewRequest(http.MethodGet, "/", nil), bearer)); got != "" {
		t.Fatalf("BearerToken() without header = %q", got)
	}
	authorized := httptest.NewRequest(http.MethodGet, "/", nil)
	authorized.Header.Set(fiber.HeaderAuthorization, "Bearer  token-value ")
	if got := readBody(t, serve(t, authorized, bearer)); got != "token-value" {
		t.Fatalf("BearerToken() = %q", got)
	}

	verified := &domain.AdminUser{Role: "operations", Permissions: []string{"transactions:read"}}
	verify := func(token string) (*platform.TokenClaims, error) {
		if token != "token-value" {
			return nil, errors.New("bad token")
		}
		return &platform.TokenClaims{SubjectID: "admin-1"}, nil
	}
	resolve := func(ctx context.Context, claims *platform.TokenClaims) (*domain.AdminUser, error) {
		if ctx == nil {
			t.Fatal("request context was not passed to authentication")
		}
		if claims.SubjectID != "admin-1" {
			return nil, errors.New("unknown subject")
		}
		return verified, nil
	}
	guarded := func(c fiber.Ctx) error {
		if AdminUser(c) != verified {
			t.Fatal("authenticated admin was not stored on the request")
		}
		return c.SendStatus(http.StatusNoContent)
	}
	chain := []fiber.Handler{
		authenticate(adminKey, verify, resolve),
		RequirePermission("transactions:read"),
		guarded,
	}

	authorized = httptest.NewRequest(http.MethodGet, "/", nil)
	authorized.Header.Set(fiber.HeaderAuthorization, "Bearer token-value")
	if res := serve(t, authorized, chain...); res.StatusCode != http.StatusNoContent {
		t.Fatalf("authenticated status = %d", res.StatusCode)
	}
	if res := serve(t, httptest.NewRequest(http.MethodGet, "/", nil), chain...); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", res.StatusCode)
	}

	// A role that grants nothing is refused, which is also what an administrator whose
	// role was deleted looks like: no permissions resolve, so nothing is authorized.
	res := serve(t, httptest.NewRequest(http.MethodGet, "/", nil),
		store(adminKey, &domain.AdminUser{Role: "viewer"}),
		RequirePermission("transactions:read"),
		func(fiber.Ctx) error {
			t.Fatal("forbidden handler was called")
			return nil
		},
	)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("RequirePermission() status = %d", res.StatusCode)
	}
}

// TestRequirePermissionHonorsTheWildcard pins the mechanism that keeps super_admin
// correct as permissions are added: the role holds "*", not an enumerated set, so a
// permission introduced by a later release needs no migration to reach it.
func TestRequirePermissionHonorsTheWildcard(t *testing.T) {
	admin := &domain.AdminUser{Role: "super_admin", Permissions: []string{domain.PermissionWildcard}}
	for _, permission := range []string{"transactions:read", "roles:delete", "something:invented:later"} {
		res := serve(t, httptest.NewRequest(http.MethodGet, "/", nil),
			store(adminKey, admin), RequirePermission(permission),
			func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
		if res.StatusCode != http.StatusNoContent {
			t.Errorf("RequirePermission(%q) with the wildcard = %d", permission, res.StatusCode)
		}
	}
}

func TestRequireAppScope(t *testing.T) {
	identity := &services.AppIdentity{
		Credential: domain.AppCredential{Scopes: "transactions:read collections:create"},
	}
	guarded := func(c fiber.Ctx) error {
		if App(c) != identity {
			t.Fatal("app identity was not stored on the request")
		}
		return c.SendStatus(http.StatusNoContent)
	}
	res := serve(t, httptest.NewRequest(http.MethodGet, "/", nil),
		store(appKey, identity), RequireAppScope("transactions:read"), guarded)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("RequireAppScope(valid) status = %d", res.StatusCode)
	}

	identity.Credential.Scopes = "collections:create"
	res = serve(t, httptest.NewRequest(http.MethodGet, "/", nil),
		store(appKey, identity), RequireAppScope("transactions:read"), guarded)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("RequireAppScope(missing) status = %d", res.StatusCode)
	}
	// Admin roles and app credentials share one wildcard rule, so the two paths
	// cannot come to disagree on what "*" grants.
	if !granted([]string{"*"}, "anything") || granted([]string{"read"}, "write") {
		t.Fatal("granted() wildcard or mismatch behavior is incorrect")
	}
}

func TestRequestLoggerRecordsRequest(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	req := httptest.NewRequest(http.MethodPost, "/payments", nil)
	req.Header.Set(fiber.HeaderXRequestID, "trace-1")
	serve(t, req,
		BoundRequestID, requestid.New(), RequestLogger(logger),
		func(c fiber.Ctx) error { return c.SendStatus(http.StatusCreated) })
	log := output.String()
	for _, value := range []string{"http_request", "POST", "/payments", `"status":201`, `"request_id":"trace-1"`} {
		if !strings.Contains(log, value) {
			t.Fatalf("RequestLogger() log %q does not contain %q", log, value)
		}
	}
}

// TestRequestLoggerReportsTheStatusAnErrorWillProduce covers the case the recorded
// status cannot answer: a handler that returned an error has not reached the error
// handler yet, so the response still carries whatever was set before it failed.
func TestRequestLoggerReportsTheStatusAnErrorWillProduce(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	serve(t, httptest.NewRequest(http.MethodGet, "/missing", nil),
		RequestLogger(logger),
		func(fiber.Ctx) error { return fiber.ErrNotFound })
	if !strings.Contains(output.String(), `"status":404`) {
		t.Fatalf("RequestLogger() log = %s, want the status the error carries", output.String())
	}
}

// store puts a value on the request the way an authenticating middleware would, so an
// authorization test does not have to mint a real token first.
func store(k key, value any) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Locals(k, value)
		return c.Next()
	}
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	_ = res.Body.Close()
	return string(raw)
}
