package httpx

import (
	"log/slog"
	"net/http"
	"time"

	adminh "github.com/momobasehq/momobase/internal/http/admin"
	authmw "github.com/momobasehq/momobase/internal/http/middleware"
	publich "github.com/momobasehq/momobase/internal/http/public"
	webhookh "github.com/momobasehq/momobase/internal/http/webhooks"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
	adminweb "github.com/momobasehq/momobase/web/admin"
)

// RouterDeps contains the services, handlers, and settings required to build
// the application HTTP router.
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

// NewRouter constructs the complete application HTTP handler, including
// public, administrative, webhook, health, and optional admin frontend routes.
func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { platform.JSON(w, 200, map[string]bool{"ok": true}) })
	if d.AdminFrontendEnabled {
		mux.Handle("GET /admin/", http.StripPrefix("/admin", http.FileServer(http.FS(adminweb.FS()))))
		mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin/", http.StatusFound) })
	}
	tokens := authmw.RateLimitByIP(20, time.Minute)
	publicLimit := authmw.RateLimitByIP(120, time.Minute)
	adminLimit := authmw.RateLimitByIP(120, time.Minute)
	webhookLimit := authmw.RateLimitByIP(300, time.Minute)
	route(mux, "POST /api/v1/token", token("client_credentials", func(r *http.Request) (any, error) {
		return d.AppAuth.IssueClientToken(r.Context(), r.Form.Get("client_id"), r.Form.Get("client_secret"))
	}), tokens)
	route(mux, "POST /api/v1/token/refresh", token("refresh_token", func(r *http.Request) (any, error) {
		return d.AppAuth.RefreshToken(r.Context(), r.Form.Get("refresh_token"))
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
		return d.AdminAuth.IssuePasswordToken(r.Context(), r.Form.Get("username"), r.Form.Get("password"), r.RemoteAddr, r.UserAgent())
	})
	route(mux, "POST /api/admin/token", adminToken, tokens)
	route(mux, "POST /api/admin/login", adminToken, tokens)
	route(mux, "POST /api/admin/token/refresh", token("refresh_token", func(r *http.Request) (any, error) {
		return d.AdminAuth.RefreshToken(r.Context(), r.Form.Get("refresh_token"), r.RemoteAddr, r.UserAgent())
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
	add("POST /api/admin/logout", h.Logout)
	add("GET /api/admin/me", h.Me)
	add("GET /api/admin/transactions", h.ListTransactions)
	add("GET /api/admin/audit-logs", h.ListAuditLogs)
	add("GET /api/admin/health/providers", h.ListProviderHealth)
	add("GET /api/admin/balances/providers", h.ActiveProviderBalances, ops)
	add("GET /api/admin/system/info", h.SystemInfo)
	add("GET /api/admin/system/health", h.SystemHealth)
	add("GET /api/admin/workers", h.Workers)
	add("GET /api/admin/runtime/providers", h.RuntimeProviders)
	add("GET /api/admin/users", h.ListAdmins)
	add("POST /api/admin/users", h.CreateAdminUser, super, authmw.JSONOnly)
	add("PATCH /api/admin/users/{id}/password", h.ChangeAdminPassword, super, authmw.JSONOnly)
	add("PATCH /api/admin/users/{id}/status", h.ChangeAdminStatus, super, authmw.JSONOnly)
	add("GET /api/admin/apps", h.ListApps)
	add("POST /api/admin/apps", h.CreateApp, super, authmw.JSONOnly)
	add("GET /api/admin/apps/{id}", h.GetApp)
	add("PATCH /api/admin/apps/{id}", h.UpdateApp, super, authmw.JSONOnly)
	add("PATCH /api/admin/apps/{id}/status", h.ChangeAppStatus, super, authmw.JSONOnly)
	add("GET /api/admin/apps/{id}/credentials", h.ListCredentials)
	add(
		"POST /api/admin/apps/{id}/credentials",
		h.CreateCredential,
		super,
		authmw.JSONOnly,
	)
	add(
		"PATCH /api/admin/apps/{id}/credentials/{credentialID}/revoke",
		h.RevokeCredential,
		super,
	)
	add(
		"POST /api/admin/apps/{id}/credentials/{credentialID}/rotate",
		h.RotateCredential,
		super,
	)
	add("GET /api/admin/providers", h.ListProviders)
	add("POST /api/admin/providers/accounts", h.CreateProvider, super, authmw.JSONOnly)
	add(
		"PATCH /api/admin/providers/accounts/{id}/countries",
		h.UpdateProviderCountries,
		super,
		authmw.JSONOnly,
	)
	add(
		"PATCH /api/admin/providers/accounts/{id}/config",
		h.UpdateProviderConfig,
		super,
		authmw.JSONOnly,
	)
	add("PATCH /api/admin/providers/accounts/{id}/activate", h.ActivateProvider, super)
	add(
		"PATCH /api/admin/providers/accounts/{id}/deactivate",
		h.DeactivateProvider,
		super,
	)
	add("POST /api/admin/providers/accounts/{id}/test", h.TestProvider, super)
	add("GET /api/admin/providers/accounts/{id}/balance", h.ProviderBalance, ops)
	add("GET /api/admin/routes", h.ListRoutes)
	add("POST /api/admin/routes", h.CreateRoute, super, authmw.JSONOnly)
	add("PATCH /api/admin/routes/{id}", h.UpdateRoute, super, authmw.JSONOnly)
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
