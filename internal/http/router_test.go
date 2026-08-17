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

	adminh "github.com/momobasehq/momobase/internal/http/admin"
	publich "github.com/momobasehq/momobase/internal/http/public"
	webhookh "github.com/momobasehq/momobase/internal/http/webhooks"
	"github.com/momobasehq/momobase/internal/platform"
)

func testRouter() http.Handler {
	return testRouterWith(false)
}

func testRouterWith(dashboard bool) http.Handler {
	return NewRouter(RouterDeps{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DashboardEnabled:   dashboard,
		CORSAllowedOrigins: []string{"https://console.example.com"},
		Public:             publich.NewHandler(nil, nil, nil),
		Admin:              adminh.NewHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, adminh.SystemInfo{}),
		Webhooks:           webhookh.NewHandler(nil),
	})
}

func TestRouterHealthEndpointsAndCORS(t *testing.T) {
	router := testRouter()

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /ping status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"ok":true`) {
		t.Fatalf("GET /healthz response = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/collections", nil)
	preflight.Header.Set("Origin", "https://console.example.com")
	router.ServeHTTP(recorder, preflight)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("CORS preflight status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://console.example.com" {
		t.Fatalf("CORS allow origin = %q", got)
	}
	if !strings.Contains(recorder.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key") {
		t.Fatalf("CORS allow headers = %q", recorder.Header().Get("Access-Control-Allow-Headers"))
	}

	recorder = httptest.NewRecorder()
	disallowed := httptest.NewRequest(http.MethodGet, "/ping", nil)
	disallowed.Header.Set("Origin", "https://attacker.example")
	router.ServeHTTP(recorder, disallowed)
	if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("disallowed CORS origin was reflected")
	}
}

func TestTokenHandlerValidatesGrantAndReportsErrors(t *testing.T) {
	called := false
	handler := token("client_credentials", func(r *http.Request) (any, error) {
		called = true
		return map[string]string{"token": r.Form.Get("client_id")}, nil
	})

	recorder := httptest.NewRecorder()
	form := url.Values{"grant_type": {"password"}, "client_id": {"client-1"}}
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest || called {
		t.Fatalf("wrong grant response = %d, called = %v", recorder.Code, called)
	}

	recorder = httptest.NewRecorder()
	form.Set("grant_type", "client_credentials")
	req = httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || !called || !strings.Contains(recorder.Body.String(), "client-1") {
		t.Fatalf("valid grant response = %d %s, called = %v", recorder.Code, recorder.Body.String(), called)
	}

	wantErr := errors.New("invalid credentials")
	recorder = httptest.NewRecorder()
	failing := token("password", func(*http.Request) (any, error) { return nil, wantErr })
	req = httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	failing.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), wantErr.Error()) {
		t.Fatalf("issue error response = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	malformed := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader("grant_type=%zz"))
	malformed.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(recorder, malformed)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "BAD_REQUEST") {
		t.Fatalf("malformed form response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func token(grant string, issue func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			platform.Error(w, 400, "BAD_REQUEST", err.Error())
			return
		}
		actual := r.Form.Get("grant_type")
		if actual != grant && (grant != "password" || actual != "") {
			platform.Error(w, 400, "UNSUPPORTED_GRANT", "grant_type must be "+grant)
			return
		}
		out, err := issue(r)
		if err != nil {
			platform.Error(w, 401, "UNAUTHORIZED", err.Error())
			return
		}
		platform.RawJSON(w, 200, out)
	}
}

func TestChainAppliesMiddlewareInDeclarationOrder(t *testing.T) {
	var order []string
	wrap := func(name string) middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+"-before")
				next.ServeHTTP(w, r)
				order = append(order, name+"-after")
			})
		}
	}
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusNoContent)
	}), wrap("one"), wrap("two"))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if got := strings.Join(order, ","); got != "one-before,two-before,handler,two-after,one-after" {
		t.Fatalf("chain order = %q", got)
	}
}
