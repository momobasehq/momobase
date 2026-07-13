package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"momobase/internal/domain"
	"momobase/internal/platform"
	"momobase/internal/providers"
)

type testStack struct {
	db            *gorm.DB
	auth          *AppAuthService
	apps          *AppService
	providerAdmin *ProviderAdminService
	runtime       *ProviderRuntimeManager
	routes        *RouteAdminService
	routing       *RouteEngine
	payments      *PaymentOrchestrator
	actor         *domain.AdminUser
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
func noError(err error) {
	if err != nil {
		panic(err)
	}
}
func stack(t *testing.T) *testStack {
	t.Helper()
	db := must(gorm.Open(sqlite.Open("file:"+platform.NewID("test")+"?mode=memory&cache=shared"), &gorm.Config{}))
	noError(db.AutoMigrate(&domain.AdminUser{}, &domain.AuditLog{}, &domain.App{}, &domain.AppCredential{}, &domain.AppSession{}, &domain.ProviderAccount{}, &domain.ProviderHealthSnapshot{}, &domain.PaymentRoute{}, &domain.Transaction{}, &domain.TransactionAttempt{}, &domain.WebhookEvent{}))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	enc := must(platform.NewEncryptor("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="))
	registry := providers.NewRegistry()
	registry.Register("test_provider", func(*slog.Logger) providers.PaymentProvider { return &fakeProvider{} })
	runtime, audit := NewProviderRuntimeManager(db, registry, enc, log), NewAuditService(db, log)
	tokens := must(platform.NewTokenManager("test-app-token-secret-must-be-long-1234567890"))
	auth := NewAppAuthService(db, "app_test", "secret_test", 30*time.Minute, 24*time.Hour, tokens)
	apps, routes := NewAppService(db, auth, audit), NewRouteAdminService(db, audit)
	routing := NewRouteEngine(db, runtime)
	return &testStack{db, auth, apps, NewProviderAdminService(db, audit, enc, registry, runtime), runtime, routes, routing, NewPaymentOrchestrator(db, routing, NewProviderExecutor(runtime)), &domain.AdminUser{BaseModel: domain.BaseModel{ID: "admin"}, Role: "super_admin", Status: "active"}}
}
func (s *testStack) app(t *testing.T) (*domain.App, *domain.AppCredential, string) {
	app := must(s.apps.CreateApp(s.actor, "Test", "", "sandbox"))
	created := must(s.apps.CreateCredential(s.actor, app.ID, "Default", defaultScopes, nil))
	return app, &created.Credential, created.ClientSecret
}
func (s *testStack) provider(t *testing.T, country, mode string) *domain.ProviderAccount {
	account := must(s.providerAdmin.CreateAccount(s.actor, "test_provider", "Test", "sandbox", country, map[string]any{"mode": mode, "webhook_secret": "secret", "supports_global": country == domain.CountryGlobal}))
	noError(s.providerAdmin.Activate(context.Background(), s.actor, account.ID))
	return account
}
func (s *testStack) route(t *testing.T, id string, priority int) {
	must(s.routes.Create(s.actor, domain.ServiceCollection, domain.PaymentMethodMomo, id, priority, true))
}

func TestCoreFlows(t *testing.T) {
	t.Run("credential rotation", func(t *testing.T) {
		s := stack(t)
		app, credential, secret := s.app(t)
		token := must(s.auth.IssueClientToken(credential.ClientID, secret))
		rotated := must(s.apps.RotateCredential(s.actor, app.ID, credential.ID))
		if _, err := s.auth.ValidateClientCredentials(credential.ClientID, secret); err == nil {
			t.Fatal("old secret accepted")
		}
		must(s.auth.ValidateClientCredentials(credential.ClientID, rotated.ClientSecret))
		if _, err := s.auth.AuthenticateBearer(token.AccessToken); err == nil {
			t.Fatal("old access token accepted")
		}
	})
	t.Run("idempotency", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, "UG", "success")
		s.route(t, account.ID, 1)
		app, _, _ := s.app(t)
		req := &CreatePaymentRequest{PaymentMethod: domain.PaymentMethodMomo, Amount: 50000, Currency: "UGX", Country: "UG", Reference: "ORDER-1", Customer: &PartyPayload{Phone: "256770000000"}, Momo: &PartyPayload{Phone: "256770000000"}}
		first := must(s.payments.Create(context.Background(), app.ID, domain.ServiceCollection, "idem", req))
		second := must(s.payments.Create(context.Background(), app.ID, domain.ServiceCollection, "idem", req))
		if first.TransactionID != second.TransactionID {
			t.Fatal("idempotent request created a second transaction")
		}
		var attempts int64
		noError(s.db.Model(&domain.TransactionAttempt{}).Where("transaction_id = ?", first.TransactionID).Count(&attempts).Error)
		if attempts != 1 {
			t.Fatalf("attempts=%d", attempts)
		}
	})
	t.Run("country routing and health fallback", func(t *testing.T) {
		s := stack(t)
		global, local := s.provider(t, domain.CountryGlobal, "success"), s.provider(t, "UG", "success")
		s.route(t, global.ID, 1)
		s.route(t, local.ID, 10)
		selected := must(s.routing.SelectProvider(context.Background(), domain.ServiceCollection, domain.PaymentMethodMomo, "UG"))
		if selected.Account.ID != local.ID {
			t.Fatal("country-specific provider was not preferred")
		}
		noError(s.db.Save(&domain.ProviderHealthSnapshot{ProviderAccountID: local.ID, Status: domain.ProviderDown, CircuitState: domain.CircuitOpen}).Error)
		selected = must(s.routing.SelectProvider(context.Background(), domain.ServiceCollection, domain.PaymentMethodMomo, "UG"))
		if selected.Account.ID != global.ID {
			t.Fatal("healthy global provider was not selected")
		}
	})
	t.Run("failed reload keeps runtime", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, "UG", "success")
		before, _ := s.runtime.Get(account.ID)
		if err := s.providerAdmin.UpdateConfig(context.Background(), s.actor, account.ID, map[string]any{"mode": "init_error", "webhook_secret": "secret"}); err == nil {
			t.Fatal("bad config accepted")
		}
		after, ok := s.runtime.Get(account.ID)
		if !ok || after.ConfigVersion != before.ConfigVersion {
			t.Fatal("failed reload replaced the healthy runtime")
		}
	})
}

type fakeProvider struct{ mode string }

func (*fakeProvider) Capabilities() []providers.Capability {
	return []providers.Capability{{ServiceType: domain.ServiceCollection, PaymentMethod: domain.PaymentMethodMomo}, {ServiceType: domain.ServiceDisbursement, PaymentMethod: domain.PaymentMethodMomo}}
}
func (p *fakeProvider) Init(_ context.Context, config providers.ProviderConfig) error {
	p.mode, _ = config["mode"].(string)
	if p.mode == "init_error" {
		return errors.New("init")
	}
	return nil
}
func (p *fakeProvider) HealthCheck(context.Context) error {
	if p.mode == "down" {
		return errors.New("down")
	}
	return nil
}
func (p *fakeProvider) Collect(_ context.Context, request providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return p.pay(request.Reference)
}
func (p *fakeProvider) Disburse(_ context.Context, request providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return p.pay(request.Reference)
}
func (*fakeProvider) QueryTransaction(_ context.Context, ref string) (*providers.ProviderTransactionStatus, error) {
	return &providers.ProviderTransactionStatus{ProviderReference: ref, Status: domain.TxSucceeded}, nil
}
func (*fakeProvider) QueryBalance(context.Context) (*providers.ProviderBalance, error) {
	return &providers.ProviderBalance{Currency: "UGX", Available: 100000, Ledger: 100000}, nil
}
func (*fakeProvider) VerifyWebhook(context.Context, []byte, map[string]string) (*providers.ProviderWebhookEvent, error) {
	return &providers.ProviderWebhookEvent{ProviderReference: "test", Status: domain.TxSucceeded, EventType: "updated"}, nil
}
func (p *fakeProvider) pay(ref string) (*providers.ProviderPaymentResponse, error) {
	if p.mode == "error" {
		return nil, errors.New("provider")
	}
	return &providers.ProviderPaymentResponse{ProviderReference: "test_" + ref, Status: domain.TxSucceeded}, nil
}
