package webhooks

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

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/internal/service/webhook"
	"github.com/momobasehq/momobase/providers"
)

type webhookProvider struct{}

func (*webhookProvider) Capabilities() []providers.Capability                 { return nil }
func (*webhookProvider) Init(context.Context, providers.ProviderConfig) error { return nil }
func (*webhookProvider) HealthCheck(context.Context) error                    { return nil }
func (*webhookProvider) Collect(context.Context, providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return nil, nil
}
func (*webhookProvider) Disburse(context.Context, providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return nil, nil
}
func (*webhookProvider) QueryTransaction(context.Context, string, string) (*providers.ProviderTransactionStatus, error) {
	return nil, nil
}
func (*webhookProvider) QueryBalance(context.Context, string) (*providers.ProviderBalance, error) {
	return nil, nil
}
func (*webhookProvider) VerifyWebhook(context.Context, []byte, map[string]string) (*providers.ProviderWebhookEvent, error) {
	return &providers.ProviderWebhookEvent{
		ProviderReference: "provider-ref",
		Status:            domain.TxProcessing,
		EventType:         "payment.updated",
		Raw:               map[string]any{"status": "pending"},
	}, nil
}

func webhookHandler(t *testing.T) (*Handler, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&domain.ProviderAccount{}, &domain.WebhookEvent{}, &domain.Transaction{}, &domain.TransactionAttempt{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	encryptor, err := platform.NewEncryptor("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}
	ciphertext, err := encryptor.Encrypt([]byte(`{"webhook_secret":"hook-secret"}`))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	account := domain.ProviderAccount{
		BaseModel:           domain.BaseModel{ID: "provider-1"},
		ProviderCode:        "test",
		Name:                "Test",
		Environment:         "sandbox",
		Country:             "UG",
		Currency:            "UGX",
		Active:              true,
		ConfigVersion:       1,
		EncryptedConfigJSON: ciphertext,
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create provider account: %v", err)
	}
	registry := providers.NewRegistry()
	registry.Register("test", func(*slog.Logger) providers.PaymentProvider { return &webhookProvider{} })
	repos := repository.New(db)
	runtime := provider.NewRuntimeManager(repos, registry, encryptor, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := runtime.LoadActive(context.Background()); err != nil {
		t.Fatalf("LoadActive() error = %v", err)
	}
	return NewHandler(webhook.New(repos, runtime)), db
}

func TestProviderWebhookAcceptsVerifiedEvent(t *testing.T) {
	h, db := webhookHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/provider-1", strings.NewReader(`{"status":"pending"}`))
	req.Header.Set("X-Webhook-Secret", "hook-secret")
	res := call(t, h, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body, `"ok":true`) {
		t.Fatalf("ProviderWebhook() = %d %s", res.Code, res.Body)
	}
	var count int64
	if err := db.Model(&domain.WebhookEvent{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("webhook event count = %d, %v", count, err)
	}
}

func TestProviderWebhookRejectsInvalidSecret(t *testing.T) {
	h, _ := webhookHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/provider-1", strings.NewReader(`{}`))
	req.Header.Set("X-Webhook-Secret", "wrong")
	res := call(t, h, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body, "invalid webhook secret") {
		t.Fatalf("ProviderWebhook(invalid secret) = %d %s", res.Code, res.Body)
	}
}

// TestProviderWebhookRejectsAnUnknownAccount covers the path a body read error used to.
// fasthttp buffers the request before a handler runs, so a body that fails mid-read is
// no longer a failure the handler can observe; an account the runtime has never loaded
// is, and it reaches the same branch.
func TestProviderWebhookRejectsAnUnknownAccount(t *testing.T) {
	h, _ := webhookHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/absent", strings.NewReader(`{}`))
	req.Header.Set("X-Webhook-Secret", "hook-secret")
	res := call(t, h, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("ProviderWebhook(unknown account) = %d %s", res.Code, res.Body)
	}
}

// response is one recorded reply. A fiber.Ctx cannot be built directly, so a handler is
// exercised by mounting it on a throwaway app and driving a request through it.
type response struct {
	Code int
	Body string
}

func call(t *testing.T, h *Handler, req *http.Request) response {
	t.Helper()
	app := fiber.New()
	app.Post("/webhooks/:providerAccountID", h.ProviderWebhook)
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
