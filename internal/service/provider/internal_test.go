package provider

import (
	"testing"

	"github.com/momobasehq/momobase/providers"
)

// This test covers an unexported helper, so it stays in-package and must not import
// internal/testsupport — that would be a cycle. It is pure and needs no database.

func TestGuardValidatedRequest(t *testing.T) {
	if err := guardValidatedRequest(
		providers.PaymentRequest{Account: "256770000000"},
		&providers.PaymentRequest{},
	); err == nil {
		t.Fatal("guardValidatedRequest() = nil, want an error for an emptied account")
	}
}

func TestProviderInitConfigAddsAuthoritativeEnvironment(t *testing.T) {
	plain := providers.ProviderConfig{
		"api_key":     "secret",
		"environment": "sandbox",
	}
	config := providerInitConfig(plain, "production")
	if got := providers.ConfigString(config, "environment"); got != "production" {
		t.Errorf("environment = %v, want production", got)
	}
	if got := providers.ConfigString(config, "api_key"); got != "secret" {
		t.Errorf("primary = %v, want original value", got)
	}
	if got := providers.ConfigString(plain, "environment"); got != "sandbox" {
		t.Errorf("original environment = %v, want unchanged sandbox", got)
	}
}
