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
	dashboardweb "github.com/momobasehq/momobase/web/dashboard"
)

// RouterDeps contains the services, handlers, and settings required to build
// the application HTTP router.
type RouterDeps struct {
	Logger    *slog.Logger
	AdminAuth *services.AdminAuthService
	AppAuth   *services.AppAuthService
	// DashboardEnabled serves the administration dashboard at /dashboard/. It only
	// takes effect in a binary built with the dashboard tag, which is what carries
	// the assets; see web/dashboard.
	DashboardEnabled   bool
	CORSAllowedOrigins []string
	Public             *publich.Handler
	Admin              *adminh.Handler
	Webhooks           *webhookh.Handler
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
// public, administrative, webhook, health, and optional dashboard routes.
func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", publich.Ping)
	mux.HandleFunc("GET /healthz", publich.Health)
	// Available is false unless this binary was built with the dashboard tag, so a
	// deployment that sets the flag against an untagged build serves nothing here
	// rather than an empty shell whose scripts 404.
	if d.DashboardEnabled && dashboardweb.Available() {
		mux.Handle("GET /dashboard/", http.StripPrefix("/dashboard/", newDashboardHandler(dashboardweb.FS())))
		mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/dashboard/", http.StatusFound) })
		// The panel that lived here was replaced by the dashboard. Redirecting
		// permanently keeps existing bookmarks and runbooks working instead of
		// answering them with a bare 404.
		movedToDashboard := func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
		}
		mux.HandleFunc("GET /admin/", movedToDashboard)
		mux.HandleFunc("GET /admin", movedToDashboard)
	}
	tokens := middlewarex.RateLimitByIP(20, time.Minute)
	publicLimit := middlewarex.RateLimitByIP(120, time.Minute)
	adminLimit := middlewarex.RateLimitByIP(120, time.Minute)
	webhookLimit := middlewarex.RateLimitByIP(300, time.Minute)
	route(mux, "POST /api/v1/token", publich.ClientToken(d.AppAuth), tokens)
	route(mux, "POST /api/v1/token/refresh", publich.AppRefreshToken(d.AppAuth), tokens)
	// Discovery is what a checkout screen calls before it has any payment details,
	// so it needs a session but no create scope and no JSON body.
	route(mux, "GET /api/v1/payment-methods", d.Public.ListPaymentMethods, publicLimit, middlewarex.WithAppBearer(d.AppAuth))
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
	// Every administrative route names exactly one permission. Before this, most reads
	// were reachable by any authenticated administrator and every write was compared
	// against a literal role name in this file.
	add := func(pattern string, handler http.HandlerFunc, permission string, extra ...middleware) {
		guards := base
		if permission != "" {
			guards = append(append([]middleware{}, base...), middlewarex.RequirePermission(permission))
		}
		route(mux, pattern, handler, append(guards, extra...)...)
	}
	// Logout and identity are self-service: they act on the caller's own session, so
	// gating them on a permission would let a role lock someone out of signing out.
	add("POST /api/admin/logout", h.Logout, "")
	add("GET /api/admin/me", h.Me, "")

	add("GET /api/admin/permissions", h.ListPermissions, "roles:read")
	add("GET /api/admin/roles", h.ListRoles, "roles:read")
	add("POST /api/admin/roles", h.CreateRole, "roles:create", middlewarex.JSONOnly)
	add("PATCH /api/admin/roles/{name}", h.UpdateRole, "roles:update", middlewarex.JSONOnly)
	add("DELETE /api/admin/roles/{name}", h.DeleteRole, "roles:delete")

	add("GET /api/admin/transactions", h.ListTransactions, "transactions:read")
	// Aggregates of the same rows transactions:read already exposes, so it needs no
	// permission of its own; a role that can read transactions can chart them.
	add("GET /api/admin/analytics/transactions", h.TransactionAnalytics, "transactions:read")
	add("GET /api/admin/audit-logs", h.ListAuditLogs, "audit:read")
	add("GET /api/admin/system/info", h.SystemInfo, "system:read")
	add("GET /api/admin/system/health", h.SystemHealth, "system:read")
	add("GET /api/admin/workers", h.Workers, "system:read")

	add("GET /api/admin/users", h.ListAdmins, "users:read")
	add("POST /api/admin/users", h.CreateAdminUser, "users:create", middlewarex.JSONOnly)
	// Self-service password changes are allowed without users:update; the service
	// distinguishes the caller's own account from someone else's.
	add("PATCH /api/admin/users/{id}/password", h.ChangeAdminPassword, "", middlewarex.JSONOnly)
	add("PATCH /api/admin/users/{id}/status", h.ChangeAdminStatus, "users:update", middlewarex.JSONOnly)
	add("PATCH /api/admin/users/{id}/role", h.ChangeAdminRole, "users:update", middlewarex.JSONOnly)

	add("GET /api/admin/apps", h.ListApps, "apps:read")
	add("POST /api/admin/apps", h.CreateApp, "apps:create", middlewarex.JSONOnly)
	add("GET /api/admin/apps/{id}", h.GetApp, "apps:read")
	add("PATCH /api/admin/apps/{id}", h.UpdateApp, "apps:update", middlewarex.JSONOnly)
	add("PATCH /api/admin/apps/{id}/status", h.ChangeAppStatus, "apps:update", middlewarex.JSONOnly)
	add("GET /api/admin/apps/{id}/credentials", h.ListCredentials, "credentials:read")
	add("POST /api/admin/apps/{id}/credentials", h.CreateCredential, "credentials:create", middlewarex.JSONOnly)
	add("PATCH /api/admin/apps/{id}/credentials/{credentialID}/revoke", h.RevokeCredential, "credentials:update")
	add("POST /api/admin/apps/{id}/credentials/{credentialID}/rotate", h.RotateCredential, "credentials:update")

	add("GET /api/admin/providers", h.ListProviders, "providers:read")
	add("GET /api/admin/providers/registry", h.ProviderRegistry, "providers:read")
	add("GET /api/admin/health/providers", h.ListProviderHealth, "providers:read")
	add("GET /api/admin/runtime/providers", h.RuntimeProviders, "providers:read")
	add("POST /api/admin/providers/accounts", h.CreateProvider, "providers:create", middlewarex.JSONOnly)
	add("PATCH /api/admin/providers/accounts/{id}/countries", h.UpdateProviderCountries, "providers:update", middlewarex.JSONOnly)
	add("PATCH /api/admin/providers/accounts/{id}/config", h.UpdateProviderConfig, "providers:update", middlewarex.JSONOnly)
	add("PATCH /api/admin/providers/accounts/{id}/activate", h.ActivateProvider, "providers:update")
	add("PATCH /api/admin/providers/accounts/{id}/deactivate", h.DeactivateProvider, "providers:update")
	add("POST /api/admin/providers/accounts/{id}/test", h.TestProvider, "providers:test")
	// Balances reach the provider's API rather than the database, which is why they
	// are their own permission and why read_only does not hold it.
	add("GET /api/admin/balances/providers", h.ActiveProviderBalances, "balances:read")
	add("GET /api/admin/providers/accounts/{id}/balance", h.ProviderBalance, "balances:read")

	add("GET /api/admin/routes", h.ListRoutes, "routes:read")
	add("POST /api/admin/routes", h.CreateRoute, "routes:create", middlewarex.JSONOnly)
	add("PATCH /api/admin/routes/{id}", h.UpdateRoute, "routes:update", middlewarex.JSONOnly)
}
