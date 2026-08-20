package bootstrap

import (
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/providers"
)

type fakeProvider struct{ providers.PaymentProvider }

func fakeFactory(*slog.Logger) providers.PaymentProvider { return &fakeProvider{} }

func TestBuildRegistryContainsOnlyRegisteredProviders(t *testing.T) {
	opts := []Option{WithProvider("zeta_pay", fakeFactory), WithProvider("acme_pay", fakeFactory)}
	registry, err := newOptions(opts).buildRegistry()
	if err != nil {
		t.Fatalf("buildRegistry() error = %v", err)
	}

	if got := registry.List(); !slices.Equal(got, []string{"acme_pay", "zeta_pay"}) {
		t.Errorf("List() = %v, want only the registered providers", got)
	}
	provider, err := registry.Create("acme_pay", nil)
	if err != nil {
		t.Fatalf("Create(acme_pay) error = %v", err)
	}
	if _, ok := provider.(*fakeProvider); !ok {
		t.Errorf("Create(acme_pay) = %T, want the registered provider", provider)
	}
}

func TestBuildRegistryLastProviderForACodeWins(t *testing.T) {
	replaced := func(*slog.Logger) providers.PaymentProvider { return nil }
	opts := []Option{WithProvider("acme_pay", replaced), WithProvider("acme_pay", fakeFactory)}
	registry, err := newOptions(opts).buildRegistry()
	if err != nil {
		t.Fatalf("buildRegistry() error = %v", err)
	}

	if got := registry.List(); !slices.Equal(got, []string{"acme_pay"}) {
		t.Errorf("List() = %v, want a single registration", got)
	}
	provider, err := registry.Create("acme_pay", nil)
	if err != nil {
		t.Fatalf("Create(acme_pay) error = %v", err)
	}
	if _, ok := provider.(*fakeProvider); !ok {
		t.Errorf("Create(acme_pay) = %T, want the last registered provider", provider)
	}
}

func TestBuildRegistryRejectsEmptyRegistry(t *testing.T) {
	_, err := newOptions(nil).buildRegistry()
	if err == nil || !strings.Contains(err.Error(), "no payment providers registered") {
		t.Fatalf("buildRegistry() error = %v, want a missing-provider error", err)
	}
}

func TestBuildRegistryRejectsInvalidProviders(t *testing.T) {
	tests := map[string]struct {
		option Option
		want   string
	}{
		"blank code":  {option: WithProvider("  ", fakeFactory), want: "provider code is required"},
		"nil factory": {option: WithProvider("acme_pay", nil), want: "provider factory is required"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newOptions([]Option{test.option}).buildRegistry()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("buildRegistry() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWithRegistryUsesSuppliedRegistry(t *testing.T) {
	base := providers.NewRegistry()
	base.Register("acme_pay", fakeFactory)

	registry, err := newOptions([]Option{withRegistry(base)}).buildRegistry()
	if err != nil {
		t.Fatalf("buildRegistry() error = %v", err)
	}
	if !registry.Has("acme_pay") {
		t.Error("Has(acme_pay) = false, want the supplied registry to be used")
	}
}
