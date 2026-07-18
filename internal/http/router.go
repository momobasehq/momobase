package httpx

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	adminh "github.com/momobasehq/momobase/internal/http/admin"
	authmw "github.com/momobasehq/momobase/internal/http/middleware"
	publich "github.com/momobasehq/momobase/internal/http/public"
	webhookh "github.com/momobasehq/momobase/internal/http/webhooks"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
)

type RouterDeps struct {
	Logger               *slog.Logger
	AdminAuth            *services.AdminAuthService
	AppAuth              *services.AppAuthService
	AdminFrontendEnabled bool
	CORSAllowedOrigins   []string
	Public               *publich.Handler
	Admin                *adminh.Handler
	Webhooks             *webhookh.Handler
}
type middleware func(http.Handler) http.Handler

func chain(h http.Handler, middlewares ...middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
func route(mux *http.ServeMux, pattern string, h http.HandlerFunc, middlewares ...middleware) {
	mux.Handle(pattern, chain(h, middlewares...))
}
func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { platform.JSON(w, 200, map[string]bool{"ok": true}) })
	if d.AdminFrontendEnabled {
		if _, err := os.Stat("web/admin/index.html"); err == nil {
			mux.Handle("GET /admin/", http.StripPrefix("/admin", http.FileServer(http.Dir("web/admin"))))
			mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin/", http.StatusFound) })
		}
	}
	tokens := authmw.RateLimitByIP(20, time.Minute)
	publicLimit := authmw.RateLimitByIP(120, time.Minute)
	adminLimit := authmw.RateLimitByIP(120, time.Minute)
	webhookLimit := authmw.RateLimitByIP(300, time.Minute)
	route(mux, "POST /api/v1/token", token("client_credentials", func(r *http.Request) (any, error) {
		return d.AppAuth.IssueClientToken(r.Form.Get("client_id"), r.Form.Get("client_secret"))
	}), tokens)
	route(mux, "POST /api/v1/token/refresh", token("refresh_token", func(r *http.Request) (any, error) {
		return d.AppAuth.RefreshToken(r.Form.Get("refresh_token"))
	}), tokens)
	app := []middleware{publicLimit, authmw.WithAppBearer(d.AppAuth), authmw.JSONOnly}
	route(mux, "POST /api/v1/collections", d.Public.CreateCollection, append(app, authmw.RequireAppScope("collections:create"))...)
	route(mux, "POST /api/v1/disbursements", d.Public.CreateDisbursement, append(app, authmw.RequireAppScope("disbursements:create"))...)
	route(
		mux,
		"GET /api/v1/transactions/by-reference/{reference}",
		d.Public.GetTransactionByReference,
		publicLimit,
		authmw.WithAppBearer(d.AppAuth),
		authmw.RequireAppScope("transactions:read"),
	)
	route(
		mux,
		"GET /api/v1/transactions/{id}",
		d.Public.GetTransaction,
		publicLimit,
		authmw.WithAppBearer(d.AppAuth),
		authmw.RequireAppScope("transactions:read"),
	)

	adminToken := token("password", func(r *http.Request) (any, error) {
		return d.AdminAuth.IssuePasswordToken(r.Form.Get("username"), r.Form.Get("password"), r.RemoteAddr, r.UserAgent())
	})
	route(mux, "POST /api/admin/token", adminToken, tokens)
	route(mux, "POST /api/admin/login", adminToken, tokens)
	route(mux, "POST /api/admin/token/refresh", token("refresh_token", func(r *http.Request) (any, error) {
		return d.AdminAuth.RefreshToken(r.Form.Get("refresh_token"), r.RemoteAddr, r.UserAgent())
	}), tokens)
	adminRoutes(mux, d.Admin, adminLimit, authmw.WithAdminBearer(d.AdminAuth), authmw.NoCache)
	route(mux, "POST /webhooks/{providerAccountID}", d.Webhooks.ProviderWebhook, webhookLimit, authmw.MaxBodyBytes(256<<10))
	return chain(mux, authmw.Recover(d.Logger), authmw.MaxBodyBytes(1<<20), authmw.StructuredLogger(d.Logger), cors(d.CORSAllowedOrigins))
}

func adminRoutes(mux *http.ServeMux, h *adminh.Handler, base ...middleware) {
	super := authmw.RequireRole("super_admin")
	ops := authmw.RequireRole("super_admin", "operations")
	add := func(pattern string, handler http.HandlerFunc, extra ...middleware) {
		route(mux, pattern, handler, append(base, extra...)...)
	}
	add("POST /api/admin/logout", h.Action(200, "LOGOUT_FAILED", "logout", false))
	add("GET /api/admin/me", h.Me)
	add("GET /api/admin/transactions", h.List("transactions"))
	add("GET /api/admin/audit-logs", h.List("audit"))
	add("GET /api/admin/health/providers", h.List("health"))
	add("GET /api/admin/balances/providers", h.ActiveProviderBalances, ops)
	add("GET /api/admin/system/info", h.SystemInfo)
	add("GET /api/admin/system/health", h.SystemHealth)
	add("GET /api/admin/workers", h.Workers)
	add("GET /api/admin/runtime/providers", h.RuntimeProviders)
	add("GET /api/admin/users", h.List("admins"))
	add("POST /api/admin/users", h.Action(201, "ADMIN_CREATE_FAILED", "admin.create", true), super, authmw.JSONOnly)
	add("PATCH /api/admin/users/{id}/password", h.Action(200, "PASSWORD_CHANGE_FAILED", "admin.password", true), super, authmw.JSONOnly)
	add("PATCH /api/admin/users/{id}/status", h.Action(200, "STATUS_CHANGE_FAILED", "admin.status", true), super, authmw.JSONOnly)
	add("GET /api/admin/apps", h.List("apps"))
	add("POST /api/admin/apps", h.Action(201, "APP_CREATE_FAILED", "app.create", true), super, authmw.JSONOnly)
	add("GET /api/admin/apps/{id}", h.GetApp)
	add("PATCH /api/admin/apps/{id}", h.Action(200, "APP_UPDATE_FAILED", "app.update", true), super, authmw.JSONOnly)
	add("PATCH /api/admin/apps/{id}/status", h.Action(200, "APP_STATUS_CHANGE_FAILED", "app.status", true), super, authmw.JSONOnly)
	add("GET /api/admin/apps/{id}/credentials", h.List("credentials"))
	add(
		"POST /api/admin/apps/{id}/credentials",
		h.Action(201, "APP_CREDENTIAL_CREATE_FAILED", "credential.create", true),
		super,
		authmw.JSONOnly,
	)
	add(
		"PATCH /api/admin/apps/{id}/credentials/{credentialID}/revoke",
		h.Action(200, "APP_CREDENTIAL_REVOKE_FAILED", "credential.revoke", false),
		super,
	)
	add(
		"POST /api/admin/apps/{id}/credentials/{credentialID}/rotate",
		h.Action(200, "APP_CREDENTIAL_ROTATE_FAILED", "credential.rotate", false),
		super,
	)
	add("GET /api/admin/providers", h.List("providers"))
	add("POST /api/admin/providers/accounts", h.Action(201, "PROVIDER_CREATE_FAILED", "provider.create", true), super, authmw.JSONOnly)
	add(
		"PATCH /api/admin/providers/accounts/{id}/countries",
		h.Action(200, "COUNTRIES_UPDATE_FAILED", "provider.countries", true),
		super,
		authmw.JSONOnly,
	)
	add(
		"PATCH /api/admin/providers/accounts/{id}/config",
		h.Action(200, "CONFIG_UPDATE_FAILED", "provider.config", true),
		super,
		authmw.JSONOnly,
	)
	add("PATCH /api/admin/providers/accounts/{id}/activate", h.Action(200, "PROVIDER_ACTIVATE_FAILED", "provider.activate", false), super)
	add(
		"PATCH /api/admin/providers/accounts/{id}/deactivate",
		h.Action(200, "PROVIDER_DEACTIVATE_FAILED", "provider.deactivate", false),
		super,
	)
	add("POST /api/admin/providers/accounts/{id}/test", h.Action(200, "PROVIDER_TEST_FAILED", "provider.test", false), super)
	add("GET /api/admin/providers/accounts/{id}/balance", h.ProviderBalance, ops)
	add("GET /api/admin/routes", h.List("routes"))
	add("POST /api/admin/routes", h.Action(201, "ROUTE_CREATE_FAILED", "route.create", true), super, authmw.JSONOnly)
	add("PATCH /api/admin/routes/{id}", h.Action(200, "ROUTE_UPDATE_FAILED", "route.update", true), super, authmw.JSONOnly)
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
func cors(origins []string) middleware {
	allowed := map[string]bool{}
	if len(origins) == 0 {
		origins = []string{"http://localhost:9090"}
	}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowed[origin] || allowed["*"]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().
					Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Idempotency-Key, X-CSRF-Token, X-Webhook-Secret")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, HEAD, OPTIONS")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
