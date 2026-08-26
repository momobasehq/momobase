package testsupport

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/dto"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/service/identity"
	"github.com/momobasehq/momobase/internal/service/payment"
	"github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/internal/service/routing"
	"github.com/momobasehq/momobase/providers"
	"github.com/momobasehq/momobase/providers/dummy"
)

// Method is the payment method the fixtures route on. Momobase ships no
// payment-method constants: a method is a free-form label an operator picks when
// creating a route, and the engine only ever compares it.
const Method = "momo"

// ProviderCode is the registry code the fixture registers the dummy adapter under.
const ProviderCode = "test_provider"

// DummyConfig builds a dummy provider configuration, merging overrides over the
// minimum viable settings. Every call site states the behavior it depends on
// rather than relying on the adapter's defaults.
func DummyConfig(overrides map[string]any) map[string]any {
	config := map[string]any{
		"webhook_secret": "test-webhook-signing-credential",
		"outcome":        dummy.OutcomeSucceed,
	}
	for key, value := range overrides {
		config[key] = value
	}
	return config
}

// Stack is a wired set of services sharing one throwaway database.
type Stack struct {
	DB            *gorm.DB
	Repos         *repository.UnitOfWork
	Auth          *identity.AppAuthService
	Apps          *identity.AppService
	ProviderAdmin *provider.AdminService
	Runtime       *provider.RuntimeManager
	Routes        *routing.AdminService
	Routing       *routing.Engine
	Payments      *payment.Orchestrator
	Authz         *identity.AuthzService
	Analytics     *identity.AnalyticsService
	Registry      providers.Registry
	Actor         *domain.AdminUser
}

// Must returns value, panicking when err is non-nil. Fixture setup has no useful
// recovery, so a failure aborts the test rather than propagating a second error.
func Must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// NoError panics when err is non-nil.
func NoError(err error) {
	if err != nil {
		panic(err)
	}
}

// New opens an isolated in-memory database, migrates every model, and wires the
// services against it. Each call gets its own database and its own provider
// registry, so tests never observe one another.
func New(t *testing.T) *Stack {
	t.Helper()
	// Silent, because a not-found read is an ordinary outcome here — an idempotency
	// miss, a provider that has never been probed — and GORM logs each one at error
	// level, which buries a real failure in the noise.
	db := Must(gorm.Open(
		sqlite.Open("file:"+platform.NewID("test")+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)},
	))
	NoError(db.AutoMigrate(
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
	enc := Must(platform.NewEncryptor("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="))
	registry := providers.NewRegistry()
	registry.Register(ProviderCode, dummy.New)
	repos := repository.New(db)
	runtime, recorder := provider.NewRuntimeManager(repos, registry, enc, log), audit.New(repos, log)
	tokens := Must(platform.NewTokenManager("test-app-token-secret-must-be-long-1234567890"))
	auth := identity.NewAppAuthService(repos, "app_test", "secret_test", 30*time.Minute, 24*time.Hour, tokens)
	apps, routes := identity.NewAppService(repos, auth, recorder), routing.NewAdminService(repos, recorder)
	routing := routing.NewEngine(repos, runtime)
	authz := identity.NewAuthzService(repos, recorder)
	NoError(authz.Seed(context.Background()))
	return &Stack{
		db,
		repos,
		auth,
		apps,
		provider.NewAdminService(repos, recorder, enc, registry, runtime),
		runtime,
		routes,
		routing,
		payment.NewOrchestrator(repos, routing, provider.NewExecutor(runtime)),
		authz,
		identity.NewAnalyticsService(repos),
		registry,
		&domain.AdminUser{
			BaseModel: domain.BaseModel{ID: "admin"},
			Role:      "super_admin",
			Status:    "active",
		},
	}
}

// App creates an application with one credential, returning the plaintext secret,
// which is only ever available at creation time.
func (s *Stack) App(t *testing.T) (*domain.App, *domain.AppCredential, string) {
	t.Helper()
	app := Must(s.Apps.CreateApp(context.Background(), s.Actor, identity.CreateAppInput{
		Name: "Test", Environment: "sandbox", Currency: "UGX",
	}))
	created := Must(s.Apps.CreateCredential(context.Background(), s.Actor, app.ID, "Default", "", nil))
	return app, &created.Credential, created.ClientSecret
}

// Provider creates and activates an account for the bundled dummy adapter.
func (s *Stack) Provider(t *testing.T, config map[string]any, requestedCountry ...string) *domain.ProviderAccount {
	t.Helper()
	return s.ProviderFor(t, ProviderCode, config, requestedCountry...)
}

// ProviderFor creates and activates an account for a registered provider code,
// which lets a test install its own adapter and route payments through it.
func (s *Stack) ProviderFor(
	t *testing.T,
	code string,
	config map[string]any,
	requestedCountry ...string,
) *domain.ProviderAccount {
	t.Helper()
	country := "UG"
	if len(requestedCountry) > 0 {
		country = requestedCountry[0]
	}
	account := Must(s.ProviderAdmin.CreateAccount(
		context.Background(),
		s.Actor,
		provider.CreateAccountInput{
			ProviderCode: code,
			Name:         "Test",
			Environment:  "sandbox",
			Country:      country,
			Currency:     "UGX",
			Config:       config,
		},
	))
	NoError(s.ProviderAdmin.Activate(context.Background(), s.Actor, account.ID))
	return account
}

// Route points a collection on Method at a provider account.
func (s *Stack) Route(t *testing.T, id string, priority int) {
	t.Helper()
	Must(s.Routes.Create(context.Background(), s.Actor, domain.ServiceCollection, Method, id, priority, true))
}

// RegisterValidator installs an adapter implementing providers.RequestValidator under
// a provider code, so a test can observe what the engine hands a provider and what it
// does with the request the provider hands back. Each stack owns its registry, so the
// closure cannot leak into another test.
func (s *Stack) RegisterValidator(code string, validate func(*providers.PaymentRequest) error) {
	s.Registry.Register(code, func(log *slog.Logger) providers.PaymentProvider {
		return &requestValidator{PaymentProvider: dummy.New(log), validate: validate}
	})
}

// requestValidator wraps the dummy adapter with the optional RequestValidator hook.
type requestValidator struct {
	providers.PaymentProvider
	validate func(*providers.PaymentRequest) error
}

func (p *requestValidator) ValidateRequest(_ context.Context, req *providers.PaymentRequest) error {
	return p.validate(req)
}

// PaymentRequest builds a minimal collection request on Method, which most payment
// and routing tests only need to vary by reference, country, and account.
func PaymentRequest(reference, country, account string) *dto.CreatePayment {
	return &dto.CreatePayment{
		PaymentMethod: Method,
		Amount:        1500,
		Currency:      "UGX",
		Country:       country,
		Reference:     reference,
		Account:       account,
	}
}
