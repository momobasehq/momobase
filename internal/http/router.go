package httpx

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	adminh "github.com/momobasehq/momobase/internal/http/admin"
	middlewarex "github.com/momobasehq/momobase/internal/http/middleware"
	publich "github.com/momobasehq/momobase/internal/http/public"
	webhookh "github.com/momobasehq/momobase/internal/http/webhooks"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/service/identity"
)

// Request and body limits. maxRequestBytes bounds every route; webhooks are allowed
// more because a provider payload is not ours to shrink.
const (
	maxRequestBytes = 1 << 20
	maxWebhookBytes = 256 << 10
)

// Per-window request allowances. Token endpoints are the tightest because they are
// the only unauthenticated write path.
const (
	rateLimitWindow  = time.Minute
	tokenRateLimit   = 20
	publicRateLimit  = 120
	adminRateLimit   = 120
	webhookRateLimit = 300
)

// RouterDeps contains the services, handlers, and settings required to build
// the application HTTP router.
type RouterDeps struct {
	Logger    *slog.Logger
	AdminAuth *identity.AdminAuthService
	AppAuth   *identity.AppAuthService
	// DashboardEnabled serves the embedded administration dashboard.
	DashboardEnabled   bool
	DashboardPath      string
	CORSAllowedOrigins []string
	// TrustedProxyCIDRs names the proxies in front of this deployment. Fiber reads a
	// forwarded address only from a peer in this list, so an empty list means the
	// immediate peer is the client, which is the only safe default.
	TrustedProxyCIDRs []string
	Public            *publich.Handler
	Admin             *adminh.Handler
	Webhooks          *webhookh.Handler
}

// NewRouter constructs the complete application, including public,
// administrative, webhook, health, and optional dashboard routes.
func NewRouter(d RouterDeps) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "momobase",
		// Values read from a request are copies rather than views into fasthttp's
		// pooled buffer. Without this a string taken from Params, Query or a header
		// stays valid only until the handler returns, and anything that outlives the
		// request — a provider account id used as a runtime map key, an id carried
		// into a background retry — is silently rewritten when the buffer is reused
		// by a later request. The failure is invisible in tests and looks like a
		// routing outage in production, which is not a trade worth an allocation.
		Immutable:        true,
		BodyLimit:        maxRequestBytes,
		ReadTimeout:      65 * time.Second,
		WriteTimeout:     65 * time.Second,
		IdleTimeout:      120 * time.Second,
		ErrorHandler:     errorHandler,
		TrustProxy:       len(d.TrustedProxyCIDRs) > 0,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: d.TrustedProxyCIDRs},
		ProxyHeader:      fiber.HeaderXForwardedFor,
	})

	app.Use(
		// Outermost, so every later middleware and every log line for this request —
		// including a recovered panic — can name the same identifier.
		middlewarex.RequestContext,
		middlewarex.BoundRequestID,
		requestid.New(requestid.Config{Generator: func() string { return platform.NewID("req") }}),
		// Ahead of Recover rather than behind it: a panic unwinds through the logger on
		// its way out, so recording the request from inside would report the status
		// nobody sent instead of the 500 the error handler answers with.
		middlewarex.RequestLogger(d.Logger),
		recover.New(),
		helmet.New(),
		cors.New(corsConfig(d.CORSAllowedOrigins)),
		compress.New(),
	)

	// Liveness is answered from the middleware stack, so /ping and /healthz keep
	// answering even when every route behind them is failing.
	app.Get("/ping", healthcheck.New())
	app.Get("/healthz", publich.Health)
	mountDashboard(app, d.DashboardEnabled, d.DashboardPath)
	mountPublic(app, d)
	mountAdmin(app, d)
	app.Post(
		"/webhooks/:providerAccountID",
		limitBy(webhookRateLimit),
		bodyLimit(maxWebhookBytes),
		d.Webhooks.ProviderWebhook,
	)
	return app
}

// errorHandler renders an error a handler returned in the same envelope every other
// response uses, so a client never has to parse two shapes.
func errorHandler(c fiber.Ctx, err error) error {
	status, code := fiber.StatusInternalServerError, "SERVER_ERROR"
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		status = fiberErr.Code
		code = "REQUEST_ERROR"
	}
	return platform.Error(c, status, code, err.Error())
}

// limitBy caps a route group at max requests per window, keyed on the client address
// Fiber resolved under the configured proxy trust.
func limitBy(max int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          max,
		Expiration:   rateLimitWindow,
		KeyGenerator: func(c fiber.Ctx) string { return c.IP() },
		LimitReached: func(c fiber.Ctx) error {
			return platform.Error(c, fiber.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
		},
	})
}

// bodyLimit refuses a payload larger than limit before a handler sees it. Fiber's
// BodyLimit is a whole-app setting, so a per-route cap is checked here.
func bodyLimit(limit int) fiber.Handler {
	return func(c fiber.Ctx) error {
		if len(c.Body()) > limit {
			return platform.Error(c, fiber.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body is too large")
		}
		return c.Next()
	}
}

// corsConfig allows the dashboard's origin and nothing else by default. Every method
// the router actually serves is listed: one missing fails preflight rather than the
// request, which the browser reports as a CORS error with no status to trace.
func corsConfig(origins []string) cors.Config {
	if len(origins) == 0 {
		origins = []string{"http://localhost:9090"}
	}
	return cors.Config{
		AllowOrigins: origins,
		AllowMethods: []string{
			fiber.MethodGet,
			fiber.MethodPost,
			fiber.MethodPatch,
			fiber.MethodDelete,
			fiber.MethodOptions,
		},
		AllowHeaders: []string{
			fiber.HeaderAuthorization,
			fiber.HeaderContentType,
			fiber.HeaderXRequestID,
			"Idempotency-Key",
		},
		ExposeHeaders:    []string{fiber.HeaderXRequestID},
		AllowCredentials: false,
	}
}

func mountPublic(app *fiber.App, d RouterDeps) {
	api := app.Group("/api/v1")
	// One limiter instance per allowance, shared by every route that names it, so the
	// budget is per caller rather than per caller per route.
	tokens, limit := limitBy(tokenRateLimit), limitBy(publicRateLimit)
	bearer := middlewarex.WithAppBearer(d.AppAuth)

	route(api, fiber.MethodPost, "/token", tokens, publich.ClientToken(d.AppAuth))
	route(api, fiber.MethodPost, "/token/refresh", tokens, publich.AppRefreshToken(d.AppAuth))

	// Guards are attached per route rather than to a group. A Fiber group registers its
	// middleware for the whole prefix and runs it before the method is matched, so an
	// authenticated group sharing this prefix would also guard the token endpoints, and
	// only their registration order would keep them reachable.
	authed := func(extra ...fiber.Handler) []fiber.Handler {
		return append([]fiber.Handler{limit, bearer}, extra...)
	}
	// Discovery is what a checkout screen calls before it has any payment details, so
	// it needs a session but no create scope and no JSON body.
	route(api, fiber.MethodGet, "/payment-methods", authed(d.Public.ListPaymentMethods)...)
	route(api, fiber.MethodPost, "/collections", authed(
		middlewarex.JSONOnly,
		middlewarex.RequireAppScope("collections:create"),
		d.Public.CreateCollection,
	)...)
	route(api, fiber.MethodPost, "/disbursements", authed(
		middlewarex.JSONOnly,
		middlewarex.RequireAppScope("disbursements:create"),
		d.Public.CreateDisbursement,
	)...)
	route(api, fiber.MethodGet, "/transactions/by-reference/:reference", authed(
		middlewarex.RequireAppScope("transactions:read"),
		d.Public.GetTransactionByReference,
	)...)
	route(api, fiber.MethodGet, "/transactions/:id", authed(
		middlewarex.RequireAppScope("transactions:read"),
		d.Public.GetTransaction,
	)...)
}

func mountAdmin(app *fiber.App, d RouterDeps) {
	admin, h := app.Group("/api/admin"), d.Admin
	tokens := limitBy(tokenRateLimit)
	route(admin, fiber.MethodPost, "/token", tokens, h.Token)
	route(admin, fiber.MethodPost, "/login", tokens, h.Token)
	route(admin, fiber.MethodPost, "/token/refresh", tokens, h.RefreshToken)

	// Attached per route for the same reason as the public group: a Fiber group runs
	// its middleware ahead of method matching, so guarding the prefix would also guard
	// the three token endpoints above.
	base := []fiber.Handler{limitBy(adminRateLimit), middlewarex.WithAdminBearer(d.AdminAuth), middlewarex.NoCache}
	// Every administrative route names exactly one permission. Logout and identity are
	// the two exceptions: they act on the caller's own session, so gating them on a
	// permission would let a role lock someone out of signing out.
	chain := func(permission string, extra []fiber.Handler, handler fiber.Handler) []fiber.Handler {
		out := append(append([]fiber.Handler{}, base...), extra...)
		if permission != "" {
			out = append(out, middlewarex.RequirePermission(permission))
		}
		return append(out, handler)
	}
	get := func(path, permission string, handler fiber.Handler) {
		route(admin, fiber.MethodGet, path, chain(permission, nil, handler)...)
	}
	post := func(path, permission string, handler fiber.Handler, extra ...fiber.Handler) {
		route(admin, fiber.MethodPost, path, chain(permission, extra, handler)...)
	}
	patch := func(path, permission string, handler fiber.Handler, extra ...fiber.Handler) {
		route(admin, fiber.MethodPatch, path, chain(permission, extra, handler)...)
	}

	route(admin, fiber.MethodPost, "/logout", chain("", nil, h.Logout)...)
	route(admin, fiber.MethodGet, "/me", chain("", nil, h.Me)...)

	get("/permissions", "roles:read", h.ListPermissions)
	get("/roles", "roles:read", h.ListRoles)
	post("/roles", "roles:create", h.CreateRole, middlewarex.JSONOnly)
	patch("/roles/:name", "roles:update", h.UpdateRole, middlewarex.JSONOnly)
	route(admin, fiber.MethodDelete, "/roles/:name", chain("roles:delete", nil, h.DeleteRole)...)

	get("/transactions", "transactions:read", h.ListTransactions)
	// Aggregates of the same rows transactions:read already exposes, so it needs no
	// permission of its own; a role that can read transactions can chart them.
	get("/analytics/transactions", "transactions:read", h.TransactionAnalytics)
	get("/audit-logs", "audit:read", h.ListAuditLogs)
	get("/system/info", "system:read", h.SystemInfo)
	get("/system/health", "system:read", h.SystemHealth)
	get("/workers", "system:read", h.Workers)

	get("/users", "users:read", h.ListAdmins)
	post("/users", "users:create", h.CreateAdminUser, middlewarex.JSONOnly)
	// Self-service password changes are allowed without users:update; the service
	// distinguishes the caller's own account from someone else's.
	patch("/users/:id/password", "", h.ChangeAdminPassword, middlewarex.JSONOnly)
	patch("/users/:id/status", "users:update", h.ChangeAdminStatus, middlewarex.JSONOnly)
	patch("/users/:id/role", "users:update", h.ChangeAdminRole, middlewarex.JSONOnly)

	get("/apps", "apps:read", h.ListApps)
	post("/apps", "apps:create", h.CreateApp, middlewarex.JSONOnly)
	get("/apps/:id", "apps:read", h.GetApp)
	patch("/apps/:id", "apps:update", h.UpdateApp, middlewarex.JSONOnly)
	patch("/apps/:id/status", "apps:update", h.ChangeAppStatus, middlewarex.JSONOnly)
	get("/apps/:id/credentials", "credentials:read", h.ListCredentials)
	post("/apps/:id/credentials", "credentials:create", h.CreateCredential, middlewarex.JSONOnly)
	patch("/apps/:id/credentials/:credentialID/revoke", "credentials:update", h.RevokeCredential)
	post("/apps/:id/credentials/:credentialID/rotate", "credentials:update", h.RotateCredential)

	get("/providers", "providers:read", h.ListProviders)
	get("/providers/registry", "providers:read", h.ProviderRegistry)
	get("/providers/accounts/:id", "providers:read", h.GetProviderAccount)
	get("/health/providers", "providers:read", h.ListProviderHealth)
	get("/runtime/providers", "providers:read", h.RuntimeProviders)
	post("/providers/accounts", "providers:create", h.CreateProvider, middlewarex.JSONOnly)
	patch("/providers/accounts/:id/settings", "providers:update", h.UpdateProviderSettings, middlewarex.JSONOnly)
	patch("/providers/accounts/:id/config", "providers:update", h.UpdateProviderConfig, middlewarex.JSONOnly)
	patch("/providers/accounts/:id/activate", "providers:update", h.ActivateProvider)
	patch("/providers/accounts/:id/deactivate", "providers:update", h.DeactivateProvider)
	post("/providers/accounts/:id/test", "providers:test", h.TestProvider)
	// Balances reach the provider's API rather than the database, which is why they
	// are their own permission and why read_only does not hold it.
	get("/balances/providers", "balances:read", h.ActiveProviderBalances)
	get("/providers/accounts/:id/balance", "balances:read", h.ProviderBalance)

	get("/routes", "routes:read", h.ListRoutes)
	post("/routes", "routes:create", h.CreateRoute, middlewarex.JSONOnly)
	patch("/routes/:id", "routes:update", h.UpdateRoute, middlewarex.JSONOnly)
}

// route registers one handler chain. Fiber's route methods take an `any` variadic, so a
// chain built as a slice cannot be spread into them directly.
func route(r fiber.Router, method, path string, handlers ...fiber.Handler) {
	rest := make([]any, 0, len(handlers)-1)
	for _, handler := range handlers[1:] {
		rest = append(rest, handler)
	}
	r.Add([]string{method}, path, handlers[0], rest...)
}
