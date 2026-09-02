package httpx

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	adminh "github.com/momobasehq/momobase/internal/http/admin"
	httpcommon "github.com/momobasehq/momobase/internal/http/common"
	publich "github.com/momobasehq/momobase/internal/http/public"
	webhookh "github.com/momobasehq/momobase/internal/http/webhooks"
	"github.com/momobasehq/momobase/internal/service/identity"
)

func testRouter() *fiber.App {
	return NewRouter(RouterDeps{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORSAllowedOrigins: []string{"https://console.example.com"},
		Public:             publich.NewHandler(nil, nil, nil),
		Admin:              adminh.NewHandler(adminh.Deps{}),
		Webhooks:           webhookh.NewHandler(nil),
	})
}

// reply is one recorded response, in the shape the assertions below use.
type reply struct {
	Code   int
	Body   string
	Header http.Header
}

func send(t *testing.T, app *fiber.App, req *http.Request) reply {
	t.Helper()
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	_ = res.Body.Close()
	return reply{Code: res.StatusCode, Body: string(raw), Header: res.Header}
}

func TestRouterHealthEndpointsAndCORS(t *testing.T) {
	app := testRouter()

	if res := send(t, app, httptest.NewRequest(http.MethodGet, "/ping", nil)); res.Code != http.StatusOK {
		t.Fatalf("GET /ping status = %d", res.Code)
	}
	res := send(t, app, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body, `"ok":true`) {
		t.Fatalf("GET /healthz response = %d %s", res.Code, res.Body)
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/collections", nil)
	preflight.Header.Set("Origin", "https://console.example.com")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	res = send(t, app, preflight)
	if res.Code != http.StatusNoContent {
		t.Fatalf("CORS preflight status = %d", res.Code)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://console.example.com" {
		t.Fatalf("CORS allow origin = %q", got)
	}
	if !strings.Contains(res.Header.Get("Access-Control-Allow-Headers"), "Idempotency-Key") {
		t.Fatalf("CORS allow headers = %q", res.Header.Get("Access-Control-Allow-Headers"))
	}
	// Every method the router serves must be advertised. One missing fails preflight
	// rather than the request, which the browser reports as an opaque CORS error with
	// no status to trace — how DELETE went unnoticed after roles gained it.
	methods := res.Header.Get("Access-Control-Allow-Methods")
	for _, method := range []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
	} {
		if !strings.Contains(methods, method) {
			t.Errorf("CORS allow methods = %q, missing %s", methods, method)
		}
	}
	if !strings.Contains(res.Header.Get("Vary"), "Origin") {
		t.Errorf("CORS Vary = %q, want Origin", res.Header.Get("Vary"))
	}

	// A refused origin is never reflected back, on a preflight or on a plain request.
	refused := httptest.NewRequest(http.MethodOptions, "/api/v1/collections", nil)
	refused.Header.Set("Origin", "https://attacker.example")
	refused.Header.Set("Access-Control-Request-Method", http.MethodPost)
	if res = send(t, app, refused); res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("a refused origin was allowed")
	}
	disallowed := httptest.NewRequest(http.MethodGet, "/ping", nil)
	disallowed.Header.Set("Origin", "https://attacker.example")
	if res = send(t, app, disallowed); res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("disallowed CORS origin was reflected")
	}
}

// TestSecurityHeadersAreSet covers what replaced the hand-rolled security middleware:
// the headers now come from helmet, so this pins that it is actually installed.
func TestSecurityHeadersAreSet(t *testing.T) {
	res := send(t, testRouter(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "SAMEORIGIN",
	} {
		if got := res.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

// TestRequestIDIsEchoed pins the outermost middleware: a caller that already has a
// trace keeps it, and a caller that does not is given one.
func TestRequestIDIsEchoed(t *testing.T) {
	app := testRouter()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(fiber.HeaderXRequestID, "trace-from-the-proxy")
	if got := send(t, app, req).Header.Get(fiber.HeaderXRequestID); got != "trace-from-the-proxy" {
		t.Errorf("echoed request id = %q, want the inbound value", got)
	}
	if got := send(t, app, httptest.NewRequest(http.MethodGet, "/healthz", nil)).
		Header.Get(fiber.HeaderXRequestID); got == "" {
		t.Error("no request id was generated")
	}
}

func TestTokenHandlerValidatesGrantAndReportsErrors(t *testing.T) {
	called := false
	app := fiber.New()
	app.Post("/token", httpcommon.Token("client_credentials", func(c fiber.Ctx) (*identity.TokenResponse, error) {
		called = true
		return &identity.TokenResponse{AccessToken: c.FormValue("client_id")}, nil
	}))
	app.Post("/failing", httpcommon.Token("password", func(fiber.Ctx) (*identity.TokenResponse, error) {
		return nil, errors.New("invalid credentials")
	}))

	form := url.Values{"grant_type": {"password"}, "client_id": {"client-1"}}
	res := send(t, app, formRequest("/token", form))
	if res.Code != http.StatusBadRequest || called {
		t.Fatalf("wrong grant response = %d, called = %v", res.Code, called)
	}

	form.Set("grant_type", "client_credentials")
	res = send(t, app, formRequest("/token", form))
	if res.Code != http.StatusOK || !called || !strings.Contains(res.Body, "client-1") {
		t.Fatalf("valid grant response = %d %s, called = %v", res.Code, res.Body, called)
	}

	res = send(t, app, formRequest("/failing", url.Values{}))
	if res.Code != http.StatusUnauthorized || !strings.Contains(res.Body, "invalid credentials") {
		t.Fatalf("issue error response = %d %s", res.Code, res.Body)
	}
}

func formRequest(target string, form url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")
	return req
}

// TestErrorHandlerUsesTheResponseEnvelope pins that a handler returning an error is
// rendered the same way as one that wrote a failure itself. A client parses one shape.
func TestErrorHandlerUsesTheResponseEnvelope(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: errorHandler})
	app.Get("/boom", func(fiber.Ctx) error { return fiber.ErrTeapot })
	res := send(t, app, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if res.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusTeapot)
	}
	for _, want := range []string{`"success":false`, `"code":"REQUEST_ERROR"`} {
		if !strings.Contains(res.Body, want) {
			t.Errorf("error body %s does not contain %q", res.Body, want)
		}
	}
}

// TestRouterAnswers405ForAMethodMismatch pins behaviour the router already provides,
// so nobody adds a handler for it: a path registered for another method is a 405 with
// an Allow header, and an unknown path is still a 404, so the two stay distinguishable.
func TestRouterAnswers405ForAMethodMismatch(t *testing.T) {
	app := testRouter()

	res := send(t, app, httptest.NewRequest(http.MethodDelete, "/api/admin/users", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE on a GET/POST path = %d, want %d", res.Code, http.StatusMethodNotAllowed)
	}
	// The header is what makes the status actionable to a client.
	if allow := res.Header.Get("Allow"); !strings.Contains(allow, http.MethodPost) {
		t.Errorf("Allow = %q, want it to list POST", allow)
	}

	res = send(t, app, httptest.NewRequest(http.MethodGet, "/api/admin/nonexistent", nil))
	if res.Code != http.StatusNotFound {
		t.Errorf("GET on an unknown path = %d, want %d", res.Code, http.StatusNotFound)
	}
}

// TestRequestValuesSurviveTheRequestTheyCameFrom pins Immutable, and the reason for it.
//
// fasthttp pools the buffer a request is parsed into, so by default every string a
// handler reads from the path, the query or a header is a view into memory the next
// request overwrites. Anything that outlives the handler — a provider account id used
// as a key in the runtime map, an id carried into a retry — is then silently rewritten
// into a splice of two unrelated requests. Nothing fails at the point of the mistake:
// the write succeeds, the map has an entry, and routing simply stops matching.
func TestRequestValuesSurviveTheRequestTheyCameFrom(t *testing.T) {
	app := NewRouter(RouterDeps{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Public:   publich.NewHandler(nil, nil, nil),
		Admin:    adminh.NewHandler(adminh.Deps{}),
		Webhooks: webhookh.NewHandler(nil),
	})

	const want = "pacc_f505c8cc-927e-443a-9b68-ce32f5bd6afb"
	var retained []string
	app.Get("/retain/:id", func(c fiber.Ctx) error {
		retained = append(retained, c.Params("id"))
		return c.SendStatus(http.StatusNoContent)
	})
	send(t, app, httptest.NewRequest(http.MethodGet, "/retain/"+want, nil))

	// Drive enough differently shaped requests through the same app to have the
	// pooled buffer handed out and overwritten several times.
	for i := range 25 {
		target := "/retain/" + strings.Repeat("z", i) + "/../../api/admin/apps/" + strings.Repeat("q", i) + "/credentials"
		send(t, app, httptest.NewRequest(http.MethodGet, target, nil))
	}

	if retained[0] != want {
		t.Fatalf("retained path value = %q, want %q — request values are aliasing the pooled buffer", retained[0], want)
	}
}
