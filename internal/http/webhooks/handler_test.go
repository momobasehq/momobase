package webhooks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
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
		Countries:           []string{"UG"},
		Active:              true,
		ConfigVersion:       1,
		EncryptedConfigJSON: ciphertext,
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create provider account: %v", err)
	}
	registry := providers.NewRegistry()
	registry.Register("test", func(*slog.Logger) providers.PaymentProvider { return &webhookProvider{} })
	runtime := services.NewProviderRuntimeManager(db, registry, encryptor, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := runtime.LoadActive(context.Background()); err != nil {
		t.Fatalf("LoadActive() error = %v", err)
	}
	return NewHandler(services.NewWebhookService(db, runtime)), db
}

func TestProviderWebhookAcceptsVerifiedEvent(t *testing.T) {
	h, db := webhookHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/provider-1", strings.NewReader(`{"status":"pending"}`))
	req.SetPathValue("providerAccountID", "provider-1")
	req.Header.Set("X-Webhook-Secret", "hook-secret")
	recorder := httptest.NewRecorder()
	h.ProviderWebhook(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"ok":true`) {
		t.Fatalf("ProviderWebhook() = %d %s", recorder.Code, recorder.Body.String())
	}
	var count int64
	if err := db.Model(&domain.WebhookEvent{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("webhook event count = %d, %v", count, err)
	}
}

func TestProviderWebhookRejectsInvalidSecret(t *testing.T) {
	h, _ := webhookHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/provider-1", strings.NewReader(`{}`))
	req.SetPathValue("providerAccountID", "provider-1")
	req.Header.Set("X-Webhook-Secret", "wrong")
	recorder := httptest.NewRecorder()
	h.ProviderWebhook(recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid webhook secret") {
		t.Fatalf("ProviderWebhook(invalid secret) = %d %s", recorder.Code, recorder.Body.String())
	}
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingBody) Close() error             { return nil }

func TestProviderWebhookReportsBodyReadError(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/provider-1", nil)
	req.Body = failingBody{}
	recorder := httptest.NewRecorder()
	h.ProviderWebhook(recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "read failed") {
		t.Fatalf("ProviderWebhook(read error) = %d %s", recorder.Code, recorder.Body.String())
	}
}
