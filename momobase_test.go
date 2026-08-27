package momobase_test

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momobasehq/momobase"
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
		{ServiceType: providers.ServiceCollection},
	}
}

func (p *stubProvider) Collect(
	context.Context,
	providers.PaymentRequest,
) (*providers.ProviderPaymentResponse, error) {
	return &providers.ProviderPaymentResponse{Status: providers.TxProcessing}, nil
}

func (p *stubProvider) Disburse(
	context.Context,
	providers.PaymentRequest,
) (*providers.ProviderPaymentResponse, error) {
	return nil, errors.New("disbursement is not supported")
}

func (p *stubProvider) QueryTransaction(
	_ context.Context,
	reference string,
	_ string,
) (*providers.ProviderTransactionStatus, error) {
	return &providers.ProviderTransactionStatus{ProviderReference: reference, Status: providers.TxSucceeded}, nil
}

func (p *stubProvider) QueryBalance(context.Context, string) (*providers.ProviderBalance, error) {
	return &providers.ProviderBalance{Currency: "UGX"}, nil
}

func (p *stubProvider) VerifyWebhook(
	context.Context,
	[]byte,
	map[string]string,
) (*providers.ProviderWebhookEvent, error) {
	return &providers.ProviderWebhookEvent{Status: providers.TxSucceeded}, nil
}

var _ providers.PaymentProvider = (*stubProvider)(nil)
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
	instance, err := momobase.New(
		momobase.WithConfig(testConfig(t)),
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
	if instance.Logger() == nil {
		t.Error("Logger() = nil, want the instance logger")
	}
	if got := instance.Addr(); got != "127.0.0.1:0" {
		t.Errorf("Addr() = %q, want the configured address", got)
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
