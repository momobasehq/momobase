package admin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/momobasehq/momobase/internal/audit"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/http/apidoc"
	"github.com/momobasehq/momobase/internal/platform"
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
	runtime := services.NewProviderRuntimeManager(db, registry, encryptor, log)
	audit := audit.New(db, log)
	apps := services.NewAppService(db, nil, nil)
	return NewHandler(
		db,
		nil,
		nil,
		services.NewProviderAdminService(db, audit, encryptor, registry, runtime),
		nil,
		apps,
		runtime,
		audit,
		services.NewAuthzService(db, audit),
		services.NewAnalyticsService(db),
		SystemInfo{
			AppName:        "momobase-test",
			AppEnv:         "test",
			DBType:         "sqlite",
			Addr:           ":9090",
			WorkersEnabled: true,
			WorkerNames:    []string{"health", "cleanup"},
		},
	)
}

func TestHandlerProviderRegistryListsRegisteredCodes(t *testing.T) {
	registry := providers.NewRegistry()
	for _, code := range []string{"zeta_pay", "acme_pay"} {
		registry.Register(code, func(*slog.Logger) providers.PaymentProvider { return nil })
	}
	h := testHandlerWithProviders(t, registry)

	recorder := httptest.NewRecorder()
	h.ProviderRegistry(recorder, httptest.NewRequest(http.MethodGet, "/providers/registry", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("ProviderRegistry() status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"providers":["acme_pay","zeta_pay"]`) {
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
		handler http.HandlerFunc
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
			recorder := httptest.NewRecorder()
			test.handler(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			for _, want := range test.want {
				if !strings.Contains(recorder.Body.String(), want) {
					t.Fatalf("body %s does not contain %q", recorder.Body.String(), want)
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
	recorder := httptest.NewRecorder()
	h.ListAdmins(recorder, httptest.NewRequest(http.MethodGet, "/admins?per_page=1", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"total":2`) || !strings.Contains(recorder.Body.String(), `"count":1`) {
		t.Fatalf("ListAdmins() response = %d %s", recorder.Code, recorder.Body.String())
	}

	app := domain.App{BaseModel: domain.BaseModel{ID: "app-1"}, Name: "App", Status: "active", Environment: "sandbox"}
	if err := h.db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/apps/app-1", nil)
	req.SetPathValue("id", "app-1")
	recorder = httptest.NewRecorder()
	h.GetApp(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "app-1") {
		t.Fatalf("GetApp() response = %d %s", recorder.Code, recorder.Body.String())
	}
	req.SetPathValue("id", "missing")
	recorder = httptest.NewRecorder()
	h.GetApp(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GetApp(missing) status = %d", recorder.Code)
	}
}

func TestHandlerListHonorsCanceledContext(t *testing.T) {
	h := testHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/admins", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	h.ListAdmins(recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("ListAdmins() with canceled context status = %d", recorder.Code)
	}
}

func TestHandlerCreateHandlersRejectInvalidJSON(t *testing.T) {
	h := testHandler(t)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(`{"unknown":true}`))
	h.CreateAdminUser(recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("CreateAdminUser(validation) response = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/apps", strings.NewReader(`{"unknown":true}`))
	h.CreateApp(recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("CreateApp(validation) response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestReplySuccessAndError(t *testing.T) {
	recorder := httptest.NewRecorder()
	reply(recorder, http.StatusAccepted, "FAILED", func() (apidoc.OK, error) { return apidoc.OK{OK: true}, nil })
	if recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"ok":true`) {
		t.Fatalf("reply(success) = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	reply(recorder, http.StatusOK, "FAILED", func() (apidoc.OK, error) { return apidoc.OK{OK: true}, io.EOF })
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "FAILED") {
		t.Fatalf("reply(error) = %d %s", recorder.Code, recorder.Body.String())
	}
}
