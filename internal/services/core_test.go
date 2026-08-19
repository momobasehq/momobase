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

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
	"github.com/momobasehq/momobase/providers/dummy"
)

// testMethod is the payment method the fixtures route on. Momobase ships no
// payment-method constants: a method is a free-form label an operator picks when
// creating a route, and the engine only ever compares it.
const testMethod = "momo"

// dummyConfig builds a dummy provider configuration, merging overrides over the
// minimum viable settings. Every call site states the behavior it depends on
// rather than relying on the adapter's defaults.
func dummyConfig(overrides map[string]any) map[string]any {
	config := map[string]any{
		"webhook_secret": "test-webhook-signing-credential",
		"outcome":        dummy.OutcomeSucceed,
	}
	for key, value := range overrides {
		config[key] = value
	}
	return config
}

type testStack struct {
	db            *gorm.DB
	auth          *AppAuthService
	apps          *AppService
	providerAdmin *ProviderAdminService
	runtime       *ProviderRuntimeManager
	routes        *RouteAdminService
	routing       *RouteEngine
	payments      *PaymentOrchestrator
	authz         *AuthzService
	analytics     *AnalyticsService
	registry      providers.Registry
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
	noError(db.AutoMigrate(
		&domain.AdminUser{},
		&domain.Permission{},
		&domain.Role{},
		&domain.AuditLog{},
		&domain.App{},
		&domain.AppCredential{},
		&domain.AppSession{},
		&domain.ProviderAccount{},
		&domain.ProviderHealthSnapshot{},
		&domain.PaymentRoute{},
		&domain.Transaction{},
		&domain.TransactionAttempt{},
		&domain.WebhookEvent{},
	))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	enc := must(platform.NewEncryptor("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="))
	registry := providers.NewRegistry()
	registry.Register("test_provider", dummy.New)
	runtime, audit := NewProviderRuntimeManager(db, registry, enc, log), NewAuditService(db, log)
	tokens := must(platform.NewTokenManager("test-app-token-secret-must-be-long-1234567890"))
	auth := NewAppAuthService(db, "app_test", "secret_test", 30*time.Minute, 24*time.Hour, tokens)
	apps, routes := NewAppService(db, auth, audit), NewRouteAdminService(db, audit)
	routing := NewRouteEngine(db, runtime)
	authz := NewAuthzService(db, audit)
	noError(authz.Seed(context.Background()))
	return &testStack{
		db,
		auth,
		apps,
		NewProviderAdminService(db, audit, enc, registry, runtime),
		runtime,
		routes,
		routing,
		NewPaymentOrchestrator(db, routing, NewProviderExecutor(runtime)),
		authz,
		NewAnalyticsService(db),
		registry,
		&domain.AdminUser{
			BaseModel: domain.BaseModel{ID: "admin"},
			Role:      "super_admin",
			Status:    "active",
		},
	}
}
func (s *testStack) app(t *testing.T) (*domain.App, *domain.AppCredential, string) {
	app := must(s.apps.CreateApp(context.Background(), s.actor, "Test", "", "sandbox"))
	created := must(s.apps.CreateCredential(context.Background(), s.actor, app.ID, "Default", defaultScopes, nil))
	return app, &created.Credential, created.ClientSecret
}
func (s *testStack) provider(t *testing.T, config map[string]any, countries ...string) *domain.ProviderAccount {
	return s.providerFor(t, "test_provider", config, countries...)
}

// providerFor creates and activates an account for a registered provider code,
// which lets a test install its own adapter and route payments through it.
func (s *testStack) providerFor(
	t *testing.T,
	code string,
	config map[string]any,
	countries ...string,
) *domain.ProviderAccount {
	t.Helper()
	account := must(s.providerAdmin.CreateAccount(
		context.Background(),
		s.actor,
		code,
		"Test",
		"sandbox",
		countries,
		config,
	))
	noError(s.providerAdmin.Activate(context.Background(), s.actor, account.ID))
	return account
}
func (s *testStack) route(t *testing.T, id string, priority int) {
	must(s.routes.Create(context.Background(), s.actor, domain.ServiceCollection, testMethod, id, priority, true))
}

func TestCoreFlows(t *testing.T) {
	t.Run("credential rotation", func(t *testing.T) {
		s := stack(t)
		app, credential, secret := s.app(t)
		token := must(s.auth.IssueClientToken(context.Background(), credential.ClientID, secret))
		rotated := must(s.apps.RotateCredential(context.Background(), s.actor, app.ID, credential.ID))
		if _, err := s.auth.ValidateClientCredentials(context.Background(), credential.ClientID, secret); err == nil {
			t.Fatal("old secret accepted")
		}
		must(s.auth.ValidateClientCredentials(context.Background(), credential.ClientID, rotated.ClientSecret))
		if _, err := s.auth.AuthenticateBearer(context.Background(), token.AccessToken); err == nil {
			t.Fatal("old access token accepted")
		}
	})
	t.Run("idempotency", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, dummyConfig(nil), "UG")
		s.route(t, account.ID, 1)
		app, _, _ := s.app(t)
		req := &CreatePaymentRequest{
			PaymentMethod: testMethod,
			Amount:        50000,
			Currency:      "UGX",
			Country:       "UG",
			Reference:     "ORDER-1",
			Account:       "256770000000",
			Customer:      &PartyPayload{Name: "Test Customer"},
		}
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
	t.Run("reconciliation settles a processing payment", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, dummyConfig(map[string]any{"settle_after": 2}), "UG")
		s.route(t, account.ID, 1)
		app, _, _ := s.app(t)
		created := must(s.payments.Create(context.Background(), app.ID, domain.ServiceCollection, "idem-recon", &CreatePaymentRequest{
			PaymentMethod: testMethod,
			Amount:        1500,
			Currency:      "UGX",
			Country:       "UG",
			Reference:     "ORDER-RECON",
			Account:       "256770000000",
			Customer:      &PartyPayload{Name: "Test Customer"},
		}))
		if created.Status != domain.TxProcessing {
			t.Fatalf("Create() status = %q, want %q", created.Status, domain.TxProcessing)
		}

		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		recon := NewReconciliationService(s.db, s.runtime, NewWebhookService(s.db, s.runtime), log)
		// The orchestrator schedules the next attempt a minute out, so each pass
		// clears the backoff to make the run deterministic.
		for pass := range 2 {
			noError(s.db.Model(&domain.Transaction{}).Where("id = ?", created.TransactionID).Update("next_reconcile_at", nil).Error)
			noError(recon.RunOnce(context.Background(), 10))
			var tx domain.Transaction
			noError(s.db.First(&tx, "id = ?", created.TransactionID).Error)
			want := domain.TxProcessing
			if pass == 1 {
				want = domain.TxSucceeded
			}
			if tx.Status != want {
				t.Fatalf("status after pass %d = %q, want %q", pass+1, tx.Status, want)
			}
		}

		var attempt domain.TransactionAttempt
		noError(s.db.First(&attempt, "transaction_id = ?", created.TransactionID).Error)
		if attempt.Status != domain.TxSucceeded || attempt.CompletedAt == nil {
			t.Fatalf("attempt = %q completed=%v, want a completed successful attempt", attempt.Status, attempt.CompletedAt)
		}
	})
	t.Run("country routing and health fallback", func(t *testing.T) {
		s := stack(t)
		primary, backup, kenya :=
			s.provider(t, dummyConfig(nil), "UG", "RW"),
			s.provider(t, dummyConfig(nil), "UG"),
			s.provider(t, dummyConfig(nil), "KE")
		s.route(t, primary.ID, 1)
		s.route(t, backup.ID, 2)
		s.route(t, kenya.ID, 0)
		selected := must(s.routing.SelectProvider(context.Background(), domain.ServiceCollection, testMethod, "UG"))
		if selected.Account.ID != primary.ID {
			t.Fatal("highest-priority provider supporting UG was not selected")
		}
		noError(s.db.Save(&domain.ProviderHealthSnapshot{
			ProviderAccountID: primary.ID,
			Status:            domain.ProviderDown,
			CircuitState:      domain.CircuitOpen,
		}).Error)
		selected = must(s.routing.SelectProvider(context.Background(), domain.ServiceCollection, testMethod, "UG"))
		if selected.Account.ID != backup.ID {
			t.Fatal("healthy UG backup was not selected")
		}
		if _, err := s.routing.SelectProvider(
			context.Background(),
			domain.ServiceCollection,
			testMethod,
			"TZ",
		); !errors.Is(err, ErrNoRouteAvailable) {
			t.Fatalf("unexpected fallback for unsupported country: %v", err)
		}
	})
	t.Run("failed reload keeps runtime", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, dummyConfig(nil), "UG")
		before, _ := s.runtime.Get(account.ID)
		if err := s.providerAdmin.UpdateConfig(
			context.Background(),
			s.actor,
			account.ID,
			dummyConfig(map[string]any{"fail_init": true}),
		); err == nil {
			t.Fatal("bad config accepted")
		}
		after, ok := s.runtime.Get(account.ID)
		if !ok || after.ConfigVersion != before.ConfigVersion {
			t.Fatal("failed reload replaced the healthy runtime")
		}
	})
}

func TestCountryNormalization(t *testing.T) {
	countries, err := utils.NormalizeProviderCountries([]string{" ug ", "RW", "UG"})
	if err != nil || len(countries) != 2 || countries[0] != "UG" || countries[1] != "RW" {
		t.Fatalf("countries=%v err=%v", countries, err)
	}
	if countries, err = utils.NormalizeProviderCountries(nil); err != nil || countries != nil {
		t.Fatalf("utils.NormalizeProviderCountries(nil) = %v, %v, want an unrestricted account", countries, err)
	}
	if country, err := utils.NormalizeOptionalCountry("  "); err != nil || country != "" {
		t.Fatalf("utils.NormalizeOptionalCountry(blank) = %q, %v, want an empty country", country, err)
	}
	if country, err := utils.NormalizeOptionalCountry(" ug "); err != nil || country != "UG" {
		t.Fatalf("utils.NormalizeOptionalCountry(%q) = %q, %v", " ug ", country, err)
	}

	// language.ParseRegion is deliberately not trusted on its own: it accepts the
	// reserved and grouping regions, and rewrites alpha-3 input into alpha-2 rather
	// than rejecting it, so "USA" would otherwise be stored as "US".
	for _, country := range []string{"XX", "ZZ", "QO", "EU", "419", "USA", "U", "UGX"} {
		if got, err := utils.NormalizeTransactionCountry(country); err == nil {
			t.Errorf("utils.NormalizeTransactionCountry(%q) = %q, want an error", country, got)
		}
	}
	if _, err := utils.NormalizeTransactionCountry(""); err == nil {
		t.Error("utils.NormalizeTransactionCountry(\"\") = nil, want an error for a required country")
	}
	// UK is exceptionally reserved for GB rather than assigned, and ParseRegion
	// resolves it. Storing the caller's own input keeps it out of the canonical
	// rewrite path, so what round-trips is exactly what was sent.
	if country, err := utils.NormalizeTransactionCountry("uk"); err != nil || country != "UK" {
		t.Errorf("utils.NormalizeTransactionCountry(%q) = %q, %v, want the input uppercased", "uk", country, err)
	}
}

func TestServiceQueriesHonorCanceledContext(t *testing.T) {
	s := stack(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.apps.GetApp(ctx, "missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetApp() error = %v, want context canceled", err)
	}
}
