package providers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestAmountConversion(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		currency  string
		minor     int64
		formatted string
	}{
		{"whole two-decimal", "12", "USD", 1200, "12.00"},
		{"one decimal", "12.3", "USD", 1230, "12.30"},
		{"two decimals", "12.34", "USD", 1234, "12.34"},
		{"negative", "-12.34", "USD", -1234, "-12.34"},
		{"explicit positive", "+0.05", "USD", 5, "0.05"},
		{"zero-decimal currency", "2500", "UGX", 2500, "2500"},
		{"empty", "", "USD", 0, "0.00"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			minor, err := ParseAmountToMinor(test.raw, test.currency)
			if err != nil {
				t.Fatalf("ParseAmountToMinor() error = %v", err)
			}
			if minor != test.minor {
				t.Fatalf("ParseAmountToMinor() = %d, want %d", minor, test.minor)
			}
			if got := FormatAmountMinor(minor, test.currency); got != test.formatted {
				t.Fatalf("FormatAmountMinor() = %q, want %q", got, test.formatted)
			}
		})
	}
	for _, raw := range []string{"1.234", "word", "1.x"} {
		if _, err := ParseAmountToMinor(raw, "USD"); err == nil {
			t.Fatalf("ParseAmountToMinor() accepted %q", raw)
		}
	}
	if amount, err := OptionalAmount(" ", "USD"); err != nil || amount != nil {
		t.Fatalf("OptionalAmount(empty) = %v, %v", amount, err)
	}
	if amount, err := OptionalAmount("1.25", "USD"); err != nil || amount == nil || *amount != 125 {
		t.Fatalf("OptionalAmount() = %v, %v", amount, err)
	}
}

func TestPaymentStatusMappings(t *testing.T) {
	tests := map[string]string{
		" successful ": "succeeded",
		"DECLINED":     "failed",
		"pending":      "processing",
		"":             "processing",
		"unexpected":   "unknown",
		"CANCELED":     "cancelled",
		"timed_out":    "expired",
	}
	for input, want := range tests {
		if got := PaymentStatus(input); got != want {
			t.Fatalf("PaymentStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestPaymentStatusIsIdempotent guards the property the provider contract relies
// on: adapters report normalized statuses, and the reconciliation and webhook
// paths normalize again. A status that did not map to itself would be silently
// rewritten, which is how a settled payment can end up reported as unknown.
func TestPaymentStatusIsIdempotent(t *testing.T) {
	for _, status := range []string{"succeeded", "failed", "processing", "unknown", "cancelled", "expired"} {
		if got := PaymentStatus(status); got != status {
			t.Fatalf("PaymentStatus(%q) = %q, want the same status", status, got)
		}
	}
}

func TestDoJSONRequestResponseAndErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			http.Error(w, "token=super-secret", http.StatusBadGateway)
			return
		}
		if r.Method != http.MethodPost || r.Header.Get("X-Test") != "yes" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s, headers=%v", r.Method, r.Header)
		}
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if input["request"] != "value" {
			t.Fatalf("request body = %v", input)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"response": "ok"})
	}))
	t.Cleanup(server.Close)

	var output map[string]string
	err := DoJSON(
		context.Background(),
		server.Client(),
		http.MethodPost,
		server.URL,
		map[string]string{"X-Test": "yes", "X-Empty": ""},
		map[string]string{"request": "value"},
		&output,
	)
	if err != nil || output["response"] != "ok" {
		t.Fatalf("DoJSON() = %v, %v", output, err)
	}

	err = DoJSON(context.Background(), server.Client(), http.MethodGet, server.URL+"/error", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Bad Gateway") || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("DoJSON(error) = %v", err)
	}
	if !strings.Contains(err.Error(), "redacted") {
		t.Fatalf("DoJSON(error) was not redacted: %v", err)
	}
	if err := DoJSON(context.Background(), server.Client(), http.MethodPost, server.URL, nil, make(chan int), nil); err == nil {
		t.Fatal("DoJSON() accepted an unmarshalable input")
	}
	if err := DoJSON(context.Background(), server.Client(), http.MethodGet, "://bad-url", nil, nil, nil); err == nil {
		t.Fatal("DoJSON() accepted an invalid URL")
	}
}

func TestRedact(t *testing.T) {
	for _, value := range []string{"Bearer abc", "api_key=value", "PASSWORD=secret"} {
		if got := Redact(value); got != "[redacted provider error]" {
			t.Fatalf("Redact(%q) = %q", value, got)
		}
	}
	long := strings.Repeat("x", 600)
	if got := Redact(long); len(got) != 500 {
		t.Fatalf("Redact(long) length = %d", len(got))
	}
	if got := Redact("safe error"); got != "safe error" {
		t.Fatalf("Redact(safe) = %q", got)
	}
}

type testProvider struct{}

func (*testProvider) Capabilities() []Capability                 { return nil }
func (*testProvider) Init(context.Context, ProviderConfig) error { return nil }
func (*testProvider) HealthCheck(context.Context) error          { return nil }
func (*testProvider) Collect(context.Context, PaymentRequest) (*ProviderPaymentResponse, error) {
	return nil, nil
}
func (*testProvider) Disburse(context.Context, PaymentRequest) (*ProviderPaymentResponse, error) {
	return nil, nil
}
func (*testProvider) QueryTransaction(context.Context, string, string) (*ProviderTransactionStatus, error) {
	return nil, nil
}
func (*testProvider) QueryBalance(context.Context, string) (*ProviderBalance, error) {
	return nil, nil
}
func (*testProvider) VerifyWebhook(context.Context, []byte, map[string]string) (*ProviderWebhookEvent, error) {
	return nil, nil
}

func TestRegistryCapabilitiesAndReferences(t *testing.T) {
	registry := NewRegistry()
	registry.Register("test", func(*slog.Logger) PaymentProvider { return &testProvider{} })
	created, err := registry.Create("test", nil)
	if err != nil || created == nil {
		t.Fatalf("Registry.Create() = %T, %v", created, err)
	}
	if _, err := registry.Create("missing", nil); err == nil {
		t.Fatal("Registry.Create() accepted an unregistered provider")
	}
	caps := []Capability{{ServiceType: "collection"}}
	if !Supports(caps, "collection") || Supports(caps, "disbursement") {
		t.Fatal("Supports() returned an unexpected result")
	}

	first, second := RandomRef("ref-"), RandomRef("ref-")
	if first == second || !strings.HasPrefix(first, "ref-") {
		t.Fatalf("RandomRef() = %q, %q", first, second)
	}
}

func TestRegistryLookupAndFactoryGuards(t *testing.T) {
	registry := NewRegistry()
	registry.Register("second", func(*slog.Logger) PaymentProvider { return &testProvider{} })
	registry.Register("first", func(log *slog.Logger) PaymentProvider {
		if log == nil {
			t.Error("Registry.Create() passed a nil logger to a factory")
		}
		return &testProvider{}
	})

	if !registry.Has("first") || registry.Has("missing") {
		t.Fatal("Registry.Has() returned an unexpected result")
	}
	if codes := registry.List(); !slices.Equal(codes, []string{"first", "second"}) {
		t.Fatalf("Registry.List() = %v, want sorted codes", codes)
	}
	if _, err := registry.Create("first", nil); err != nil {
		t.Fatalf("Registry.Create() error = %v", err)
	}

	registry.Register("empty", func(*slog.Logger) PaymentProvider { return nil })
	if _, err := registry.Create("empty", nil); err == nil {
		t.Fatal("Registry.Create() accepted a factory that returned no provider")
	}
}
