package admin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/momobasehq/momobase/internal/audit"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/http/apidoc"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/provider"
	"github.com/momobasehq/momobase/internal/services"
	"github.com/momobasehq/momobase/providers"
)

func handlerDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.AdminUser{},
		&domain.App{},
		&domain.ProviderAccount{},
		&domain.ProviderHealthSnapshot{},
		&domain.PaymentRoute{},
		&domain.Transaction{},
		&domain.AuditLog{},
		&domain.AppCredential{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func testHandler(t *testing.T) *Handler {
	t.Helper()
	return testHandlerWithProviders(t, providers.NewRegistry())
}

// testHandlerWithProviders builds a handler whose provider administration
// service is backed by registry.
func testHandlerWithProviders(t *testing.T, registry providers.Registry) *Handler {
	t.Helper()
	db := handlerDatabase(t)
	encryptor, err := platform.NewEncryptor("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runtime := provider.NewRuntimeManager(db, registry, encryptor, log)
	audit := audit.New(db, log)
	apps := services.NewAppService(db, nil, nil)
	return NewHandler(Deps{
		DB:        db,
		Providers: provider.NewAdminService(db, audit, encryptor, registry, runtime),
		Apps:      apps,
		Runtime:   runtime,
		Audit:     audit,
		Authz:     services.NewAuthzService(db, audit),
		Analytics: services.NewAnalyticsService(db),
		System: SystemInfo{
			AppName:        "momobase-test",
			AppEnv:         "test",
			DBType:         "sqlite",
			Addr:           ":9090",
			WorkersEnabled: true,
			WorkerNames:    []string{"health", "cleanup"},
		},
	})
}

func TestHandlerProviderRegistryListsRegisteredCodes(t *testing.T) {
	registry := providers.NewRegistry()
	for _, code := range []string{"zeta_pay", "acme_pay"} {
		registry.Register(code, func(*slog.Logger) providers.PaymentProvider { return nil })
	}
	h := testHandlerWithProviders(t, registry)

	res := serve(t, "/providers/registry", h.ProviderRegistry,
		httptest.NewRequest(http.MethodGet, "/providers/registry", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("ProviderRegistry() status = %d, body = %s", res.Code, res.Body)
	}
	if body := res.Body; !strings.Contains(body, `"providers":["acme_pay","zeta_pay"]`) {
		t.Fatalf("ProviderRegistry() body = %s, want the registered codes in ascending order", body)
	}
}

func TestParseExpiry(t *testing.T) {
	if value, err := parseExpiry(""); err != nil || value != nil {
		t.Fatalf("parseExpiry(empty) = %v, %v", value, err)
	}
	value, err := parseExpiry("2030-01-02T03:04:05Z")
	if err != nil || value == nil || value.Year() != 2030 {
		t.Fatalf("parseExpiry(valid) = %v, %v", value, err)
	}
	if _, err := parseExpiry("tomorrow"); err == nil {
		t.Fatal("parseExpiry() accepted an invalid timestamp")
	}
}

func TestHandlerSystemEndpoints(t *testing.T) {
	h := testHandler(t)
	if err := h.db.Create(&domain.ProviderAccount{
		BaseModel:           domain.BaseModel{ID: "active"},
		Name:                "Active",
		Active:              true,
		EncryptedConfigJSON: "encrypted",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}

	tests := []struct {
		name    string
		handler fiber.Handler
		path    string
		want    []string
	}{
		{"system info", h.SystemInfo, "/system/info", []string{"momobase-test", "sqlite", "health"}},
		{"system health", h.SystemHealth, "/system/health", []string{`"database":"ok"`, `"active_provider_account_count":1`}},
		{"workers", h.Workers, "/workers?per_page=1", []string{`"total":2`, `"count":1`, "managed_by_single_binary"}},
		{"runtime providers", h.RuntimeProviders, "/runtime/providers", []string{`"total":0`, `"count":0`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := serve(t, "/*", test.handler, httptest.NewRequest(http.MethodGet, test.path, nil))
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body)
			}
			for _, want := range test.want {
				if !strings.Contains(res.Body, want) {
					t.Fatalf("body %s does not contain %q", res.Body, want)
				}
			}
		})
	}
}

func TestHandlerListAndGetApp(t *testing.T) {
	h := testHandler(t)
	for _, user := range []domain.AdminUser{
		{BaseModel: domain.BaseModel{ID: "admin-1"}, Name: "One", Email: "one@example.com", Role: "operations", Status: "active"},
		{BaseModel: domain.BaseModel{ID: "admin-2"}, Name: "Two", Email: "two@example.com", Role: "operations", Status: "active"},
	} {
		if err := h.db.Create(&user).Error; err != nil {
			t.Fatalf("create admin: %v", err)
		}
	}
	res := serve(t, "/admins", h.ListAdmins, httptest.NewRequest(http.MethodGet, "/admins?per_page=1", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body, `"total":2`) || !strings.Contains(res.Body, `"count":1`) {
		t.Fatalf("ListAdmins() response = %d %s", res.Code, res.Body)
	}

	app := domain.App{BaseModel: domain.BaseModel{ID: "app-1"}, Name: "App", Status: "active", Environment: "sandbox"}
	if err := h.db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	res = serve(t, "/apps/:id", h.GetApp, httptest.NewRequest(http.MethodGet, "/apps/app-1", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body, "app-1") {
		t.Fatalf("GetApp() response = %d %s", res.Code, res.Body)
	}
	res = serve(t, "/apps/:id", h.GetApp, httptest.NewRequest(http.MethodGet, "/apps/missing", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("GetApp(missing) status = %d", res.Code)
	}
}

func TestHandlerListHonorsCanceledContext(t *testing.T) {
	h := testHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// The cancelled context is installed the way RequestContext installs the real one,
	// which is the only way in: fiber.Ctx itself can never be cancelled.
	cancelled := func(c fiber.Ctx) error {
		c.SetContext(ctx)
		return c.Next()
	}
	res := serve(t, "/admins", h.ListAdmins, httptest.NewRequest(http.MethodGet, "/admins", nil), cancelled)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("ListAdmins() with canceled context status = %d", res.Code)
	}
}

func TestHandlerCreateHandlersRejectInvalidJSON(t *testing.T) {
	h := testHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(`{"unknown":true}`))
	res := serve(t, "/admin/users", h.CreateAdminUser, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body, "VALIDATION_ERROR") {
		t.Fatalf("CreateAdminUser(validation) response = %d %s", res.Code, res.Body)
	}

	req = httptest.NewRequest(http.MethodPost, "/admin/apps", strings.NewReader(`{"unknown":true}`))
	res = serve(t, "/admin/apps", h.CreateApp, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body, "VALIDATION_ERROR") {
		t.Fatalf("CreateApp(validation) response = %d %s", res.Code, res.Body)
	}
}

func TestReplySuccessAndError(t *testing.T) {
	res := serve(t, "/", func(c fiber.Ctx) error {
		return reply(c, http.StatusAccepted, "FAILED", func() (apidoc.OK, error) { return apidoc.OK{OK: true}, nil })
	}, httptest.NewRequest(http.MethodGet, "/", nil))
	if res.Code != http.StatusAccepted || !strings.Contains(res.Body, `"ok":true`) {
		t.Fatalf("reply(success) = %d %s", res.Code, res.Body)
	}

	res = serve(t, "/", func(c fiber.Ctx) error {
		return reply(c, http.StatusOK, "FAILED", func() (apidoc.OK, error) { return apidoc.OK{OK: true}, io.EOF })
	}, httptest.NewRequest(http.MethodGet, "/", nil))
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body, "FAILED") {
		t.Fatalf("reply(error) = %d %s", res.Code, res.Body)
	}
}

// response is one recorded reply. A fiber.Ctx cannot be built directly, so a handler is
// exercised by mounting it on a throwaway app and driving a request through it.
type response struct {
	Code int
	Body string
}

func serve(t *testing.T, pattern string, handler fiber.Handler, req *http.Request, guards ...fiber.Handler) response {
	t.Helper()
	app := fiber.New()
	chain := append(append([]fiber.Handler{}, guards...), handler)
	rest := make([]any, 0, len(chain)-1)
	for _, link := range chain[1:] {
		rest = append(rest, link)
	}
	app.All(pattern, chain[0], rest...)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	_ = res.Body.Close()
	return response{Code: res.StatusCode, Body: string(raw)}
}
