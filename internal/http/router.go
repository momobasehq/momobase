package httpx

import (
	"log/slog"
	"net/http"
	"time"

	adminh "github.com/momobasehq/momobase/internal/http/admin"
	middlewarex "github.com/momobasehq/momobase/internal/http/middleware"
	publich "github.com/momobasehq/momobase/internal/http/public"
	webhookh "github.com/momobasehq/momobase/internal/http/webhooks"
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

type middleware = middlewarex.Middleware

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
	mux.HandleFunc("GET /ping", publich.Ping)
	mux.HandleFunc("GET /healthz", publich.Health)
	if d.AdminFrontendEnabled {
		mux.Handle("GET /admin/", http.StripPrefix("/admin", http.FileServer(http.FS(adminweb.FS()))))
		mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin/", http.StatusFound) })
	}
	tokens := middlewarex.RateLimitByIP(20, time.Minute)
	publicLimit := middlewarex.RateLimitByIP(120, time.Minute)
	adminLimit := middlewarex.RateLimitByIP(120, time.Minute)
	webhookLimit := middlewarex.RateLimitByIP(300, time.Minute)
	route(mux, "POST /api/v1/token", publich.ClientToken(d.AppAuth), tokens)
	route(mux, "POST /api/v1/token/refresh", publich.AppRefreshToken(d.AppAuth), tokens)
	app := []middleware{publicLimit, middlewarex.WithAppBearer(d.AppAuth), middlewarex.JSONOnly}
	route(mux, "POST /api/v1/collections", d.Public.CreateCollection, append(app, middlewarex.RequireAppScope("collections:create"))...)
	route(mux, "POST /api/v1/disbursements", d.Public.CreateDisbursement, append(app, middlewarex.RequireAppScope("disbursements:create"))...)
	route(
		mux,
		"GET /api/v1/transactions/by-reference/{reference}",
		d.Public.GetTransactionByReference,
		publicLimit,
		middlewarex.WithAppBearer(d.AppAuth),
		middlewarex.RequireAppScope("transactions:read"),
	)
	route(
		mux,
		"GET /api/v1/transactions/{id}",
		d.Public.GetTransaction,
		publicLimit,
		middlewarex.WithAppBearer(d.AppAuth),
		middlewarex.RequireAppScope("transactions:read"),
	)

	route(mux, "POST /api/admin/token", d.Admin.Token, tokens)
	route(mux, "POST /api/admin/login", d.Admin.Token, tokens)
	route(mux, "POST /api/admin/token/refresh", d.Admin.RefreshToken, tokens)
	adminRoutes(mux, d.Admin, adminLimit, middlewarex.WithAdminBearer(d.AdminAuth), middlewarex.NoCache)
	route(mux, "POST /webhooks/{providerAccountID}", d.Webhooks.ProviderWebhook, webhookLimit, middlewarex.MaxBodyBytes(256<<10))
	return chain(
		mux,
		middlewarex.Recover(d.Logger),
		middlewarex.MaxBodyBytes(1<<20),
		middlewarex.StructuredLogger(d.Logger),
		middlewarex.CORS(d.CORSAllowedOrigins),
	)
}

func adminRoutes(mux *http.ServeMux, h *adminh.Handler, base ...middleware) {
	super := middlewarex.RequireRole("super_admin")
	ops := middlewarex.RequireRole("super_admin", "operations")
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
	add("POST /api/admin/users", h.CreateAdminUser, super, middlewarex.JSONOnly)
	add("PATCH /api/admin/users/{id}/password", h.ChangeAdminPassword, super, middlewarex.JSONOnly)
	add("PATCH /api/admin/users/{id}/status", h.ChangeAdminStatus, super, middlewarex.JSONOnly)
	add("GET /api/admin/apps", h.ListApps)
	add("POST /api/admin/apps", h.CreateApp, super, middlewarex.JSONOnly)
	add("GET /api/admin/apps/{id}", h.GetApp)
	add("PATCH /api/admin/apps/{id}", h.UpdateApp, super, middlewarex.JSONOnly)
	add("PATCH /api/admin/apps/{id}/status", h.ChangeAppStatus, super, middlewarex.JSONOnly)
	add("GET /api/admin/apps/{id}/credentials", h.ListCredentials)
	add(
		"POST /api/admin/apps/{id}/credentials",
		h.CreateCredential,
		super,
		middlewarex.JSONOnly,
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
	add("GET /api/admin/providers/registry", h.ProviderRegistry)
	add("POST /api/admin/providers/accounts", h.CreateProvider, super, middlewarex.JSONOnly)
	add(
		"PATCH /api/admin/providers/accounts/{id}/countries",
		h.UpdateProviderCountries,
		super,
		middlewarex.JSONOnly,
	)
	add(
		"PATCH /api/admin/providers/accounts/{id}/config",
		h.UpdateProviderConfig,
		super,
		middlewarex.JSONOnly,
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
	add("POST /api/admin/routes", h.CreateRoute, super, middlewarex.JSONOnly)
	add("PATCH /api/admin/routes/{id}", h.UpdateRoute, super, middlewarex.JSONOnly)
}
