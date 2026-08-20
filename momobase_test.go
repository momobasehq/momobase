package momobase_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momobasehq/momobase"
)

// stubProvider is built only from the package's exported types, so it fails to
// compile if the provider contract stops being usable from outside the module.
type stubProvider struct{ log *slog.Logger }

func newStubProvider(log *slog.Logger) momobase.PaymentProvider { return &stubProvider{log: log} }

func (p *stubProvider) Init(context.Context, momobase.ProviderConfig) error { return nil }
func (p *stubProvider) HealthCheck(context.Context) error                   { return nil }

func (p *stubProvider) Capabilities() []momobase.Capability {
	return []momobase.Capability{
		{ServiceType: momobase.ServiceCollection},
	}
}

func (p *stubProvider) Collect(
	context.Context,
	momobase.PaymentRequest,
) (*momobase.ProviderPaymentResponse, error) {
	return &momobase.ProviderPaymentResponse{Status: momobase.TxProcessing}, nil
}

func (p *stubProvider) Disburse(
	context.Context,
	momobase.PaymentRequest,
) (*momobase.ProviderPaymentResponse, error) {
	return nil, errors.New("disbursement is not supported")
}

func (p *stubProvider) QueryTransaction(
	_ context.Context,
	reference string,
	_ string,
) (*momobase.ProviderTransactionStatus, error) {
	return &momobase.ProviderTransactionStatus{ProviderReference: reference, Status: momobase.TxSucceeded}, nil
}

func (p *stubProvider) QueryBalance(context.Context, string) (*momobase.ProviderBalance, error) {
	return &momobase.ProviderBalance{Currency: "UGX"}, nil
}

func (p *stubProvider) VerifyWebhook(
	context.Context,
	[]byte,
	map[string]string,
) (*momobase.ProviderWebhookEvent, error) {
	return &momobase.ProviderWebhookEvent{Status: momobase.TxSucceeded}, nil
}

var _ momobase.PaymentProvider = (*stubProvider)(nil)
var _ momobase.ProviderFactory = newStubProvider

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

	if instance.Handler() == nil {
		t.Error("Handler() = nil, want the configured HTTP handler")
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
		momobase.WithProviders(map[string]momobase.ProviderFactory{"stub_pay": newStubProvider}),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := instance.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
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

// TestExportedProviderHelpers exercises the helpers the root package re-exports
// for provider authors. It lives in the external test package deliberately: a
// helper that stops being reachable from outside the module breaks this test
// rather than silently regressing a documented part of the public surface.
func TestExportedProviderHelpers(t *testing.T) {
	config := momobase.ProviderConfig{
		"name":    "  Acme  ",
		"enabled": "TRUE",
		"retries": "3",
		"nested":  map[string]any{"inner": map[string]any{"leaf": "value"}},
	}
	if got := momobase.ConfigString(config, "name"); got != "Acme" {
		t.Errorf("ConfigString() = %q, want a trimmed value", got)
	}
	if !momobase.ConfigBool(config, "enabled") {
		t.Error("ConfigBool() = false, want true")
	}
	if got := momobase.ConfigInt(config, "retries"); got != 3 {
		t.Errorf("ConfigInt() = %d, want 3", got)
	}
	if got := momobase.ConfigPath(config, "nested.inner.leaf"); got != "value" {
		t.Errorf("ConfigPath() = %q, want the nested value", got)
	}
	if got := momobase.First("", "  ", "chosen"); got != "chosen" {
		t.Errorf("First() = %q, want the first nonblank value", got)
	}

	if got := momobase.PaymentStatus("SUCCESSFUL"); got != momobase.TxSucceeded {
		t.Errorf("PaymentStatus() = %q, want %q", got, momobase.TxSucceeded)
	}
	minor, err := momobase.ParseAmountToMinor("12.34", "USD")
	if err != nil || minor != 1234 {
		t.Fatalf("ParseAmountToMinor() = %d, %v", minor, err)
	}
	if got := momobase.FormatAmountMinor(minor, "USD"); got != "12.34" {
		t.Errorf("FormatAmountMinor() = %q, want 12.34", got)
	}
	optional, err := momobase.OptionalAmount("", "USD")
	if err != nil || optional != nil {
		t.Errorf("OptionalAmount(blank) = %v, %v, want nil", optional, err)
	}
	if optional, err = momobase.OptionalAmount("1.25", "USD"); err != nil || optional == nil || *optional != 125 {
		t.Errorf("OptionalAmount() = %v, %v, want 125", optional, err)
	}

	if got := momobase.Redact("bearer abc123"); got == "bearer abc123" {
		t.Error("Redact() returned credential-like text unchanged")
	}
	if got := momobase.Redact("upstream refused the payment"); got != "upstream refused the payment" {
		t.Errorf("Redact() = %q, want safe text preserved", got)
	}
	if got := momobase.RandomRef("acme"); !strings.HasPrefix(got, "acme") || len(got) != len("acme")+32 {
		t.Errorf("RandomRef() = %q, want the prefix followed by 32 hexadecimal characters", got)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "on" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)

	var out struct {
		Status string `json:"status"`
	}
	headers := map[string]string{"X-Test": "on"}
	if err = momobase.DoJSON(context.Background(), server.Client(), http.MethodPost, server.URL, headers, map[string]any{"a": 1}, &out); err != nil {
		t.Fatalf("DoJSON() error = %v", err)
	}
	if out.Status != "ok" {
		t.Errorf("DoJSON() decoded %q, want ok", out.Status)
	}
	err = momobase.DoJSON(context.Background(), server.Client(), http.MethodGet, server.URL, nil, nil, nil)
	if err == nil {
		t.Error("DoJSON() on a non-2xx response = nil, want an error")
	}
}
