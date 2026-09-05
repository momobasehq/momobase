package momobase_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momobasehq/momobase"
	"github.com/momobasehq/momobase/hooks"
	"github.com/momobasehq/momobase/providers"
)

// stubProvider is built only from the package's exported types, so it fails to
// compile if the provider contract stops being usable from outside the module.
type stubProvider struct{ log *slog.Logger }

func newStubProvider(log *slog.Logger) providers.PaymentProvider { return &stubProvider{log: log} }

func (p *stubProvider) Init(context.Context, providers.ProviderConfig) error { return nil }
func (p *stubProvider) HealthCheck(context.Context) error                    { return nil }

func (p *stubProvider) Capabilities() []providers.Capability {
	return []providers.Capability{
		{ServiceType: providers.ServiceCollection, PaymentMethod: providers.PaymentMethodMomo},
	}
}

func (p *stubProvider) Collect(
	context.Context,
	providers.PaymentRequest,
) (*providers.ProviderPaymentResponse, error) {
	return &providers.ProviderPaymentResponse{Status: providers.TxProcessing}, nil
}

func (p *stubProvider) QueryTransaction(
	_ context.Context,
	reference string,
	_ string,
) (*providers.ProviderTransactionStatus, error) {
	return &providers.ProviderTransactionStatus{ProviderReference: reference, Status: providers.TxSucceeded}, nil
}

var _ providers.PaymentProvider = (*stubProvider)(nil)
var _ providers.Collector = (*stubProvider)(nil)
var _ providers.TransactionQuerier = (*stubProvider)(nil)
var _ providers.Factory = newStubProvider

// testConfig builds a valid configuration backed by a temporary SQLite database.
func testConfig(t *testing.T) momobase.Config {
	t.Helper()
	return momobase.Config{
		App: momobase.AppConfig{Name: "momobase", Env: "test", Addr: "127.0.0.1:0"},
		Log: momobase.LogConfig{Level: "error"},
		DB:  momobase.DatabaseConfig{Type: "sqlite", Path: filepath.Join(t.TempDir(), "momobase.db")},
		Security: momobase.SecurityConfig{
			EncryptionMasterKeyBase64: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			AdminOAuthSecret:          strings.Repeat("a", 32),
			AppOAuthSecret:            strings.Repeat("b", 32),
		},
		Features: momobase.FeaturesConfig{AutoMigrate: true},
	}
}

func TestNewBuildsInstanceWithCustomProvider(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	instance, err := momobase.New(
		nil,
		momobase.WithConfig(testConfig(t)),
		momobase.WithLogger(logger),
		momobase.WithProvider("stub_pay", newStubProvider),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := instance.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	if instance.App() == nil {
		t.Error("App() = nil, want the configured Fiber application")
	}
	if instance.DB() == nil {
		t.Error("DB() = nil, want the opened database handle")
	}
	if instance.Logger() != logger {
		t.Error("Logger() did not return the configured logger")
	}
	removePaymentHook := instance.OnPaymentRequest().Bind(func(context.Context, hooks.PaymentRequestEvent) error {
		return nil
	})
	removeTransactionHook := instance.OnTransactionChanged().Bind(func(context.Context, hooks.TransactionChangedEvent) error {
		return nil
	})
	removePaymentHook()
	removeTransactionHook()
	if got := instance.Addr(); got != "127.0.0.1:0" {
		t.Errorf("Addr() = %q, want the configured address", got)
	}
}

func TestInstancePublicOperations(t *testing.T) {
	instance, err := momobase.New(
		momobase.WithConfig(testConfig(t)),
		momobase.WithProvider("stub_pay", newStubProvider),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = instance.Close() }()

	ctx := context.Background()
	if err := instance.Migrate(ctx); err != nil {
		t.Errorf("Migrate() error = %v", err)
	}
	if err := instance.SeedAdmin(ctx, "admin@example.com", "correct-horse-battery-staple", "Admin"); err != nil {
		t.Errorf("SeedAdmin() error = %v", err)
	}
	response, err := instance.App().Test(httptest.NewRequest(http.MethodGet, "/ping", nil))
	if err != nil {
		t.Fatalf("App().Test() error = %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Errorf("GET /ping status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestNewAppliesConfigOptionsInOrder(t *testing.T) {
	instance, err := momobase.New(
		momobase.WithConfig(testConfig(t)),
		momobase.WithProvider("stub_pay", newStubProvider),
		momobase.WithAddr("127.0.0.1:7777"),
		momobase.WithConfigFunc(func(cfg *momobase.Config) { cfg.App.Addr = "127.0.0.1:8888" }),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = instance.Close() }()

	if got := instance.Addr(); got != "127.0.0.1:8888" {
		t.Errorf("Addr() = %q, want the last applied override", got)
	}
}

func TestNewWithoutAnyProviderFails(t *testing.T) {
	instance, err := momobase.New(momobase.WithConfig(testConfig(t)))
	if err == nil {
		_ = instance.Close()
		t.Fatal("New() error = nil, want an error when no provider is registered")
	}
	if !strings.Contains(err.Error(), "no payment providers registered") {
		t.Errorf("New() error = %v, want a missing-provider error", err)
	}
}

func TestNewRegistersProvidersFromAMap(t *testing.T) {
	instance, err := momobase.New(
		momobase.WithConfig(testConfig(t)),
		momobase.WithProviders(map[string]providers.Factory{"stub_pay": newStubProvider}),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := instance.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestNewRejectsInvalidProviders(t *testing.T) {
	tests := []struct {
		name   string
		option momobase.Option
	}{
		{"blank code", momobase.WithProvider(" ", newStubProvider)},
		{"nil factory", momobase.WithProvider("stub_pay", nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance, err := momobase.New(momobase.WithConfig(testConfig(t)), test.option)
			if err == nil {
				_ = instance.Close()
				t.Fatal("New() error = nil, want an invalid-provider error")
			}
		})
	}
}

func TestNewRejectsPersistedUnknownPaymentMethods(t *testing.T) {
	cfg := testConfig(t)
	instance, err := momobase.New(
		momobase.WithConfig(cfg),
		momobase.WithProvider("stub_pay", newStubProvider),
	)
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}
	if err := instance.DB().Exec(`
		INSERT INTO payment_routes
			(id, service_type, payment_method, provider_account_id, priority, active, created_at, updated_at)
		VALUES
			('route_legacy', 'collection', 'cash', 'pacc_legacy', 1, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`).Error; err != nil {
		_ = instance.Close()
		t.Fatalf("insert legacy route: %v", err)
	}
	if err := instance.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := momobase.New(
		momobase.WithConfig(cfg),
		momobase.WithProvider("stub_pay", newStubProvider),
	)
	if err == nil {
		_ = reopened.Close()
		t.Fatal("second New() error = nil, want unsupported payment method")
	}
	if !strings.Contains(err.Error(), "unsupported methods: cash") {
		t.Fatalf("second New() error = %v, want legacy method named", err)
	}
}

func TestServeReturnsNilWhenContextIsCancelled(t *testing.T) {
	instance, err := momobase.New(
		momobase.WithConfig(testConfig(t)),
		momobase.WithProvider("stub_pay", newStubProvider),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = instance.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := instance.Serve(ctx); err != nil {
		t.Errorf("Serve() error = %v, want nil after cancellation", err)
	}
}

// TestNewWithoutConfigUsesDefaults proves the package needs nothing from the
// environment: with no configuration supplied it builds against DefaultConfig
// alone. It runs in a temporary working directory because the default database
// path is relative.
func TestNewWithoutConfigUsesDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	instance, err := momobase.New(
		momobase.WithProvider("stub_pay", newStubProvider),
		momobase.WithAddr("127.0.0.1:0"),
		momobase.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = instance.Close() }()

	if _, err := os.Stat(filepath.FromSlash(momobase.DefaultConfig().DB.Path)); err != nil {
		t.Errorf("default database was not created: %v", err)
	}
}
