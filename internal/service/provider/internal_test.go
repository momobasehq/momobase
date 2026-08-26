package provider

import (
	"testing"

	"github.com/momobasehq/momobase/providers"
)

// This test covers an unexported helper, so it stays in-package and must not reach
// for internal/testsupport — that would be an import cycle. It is a pure function
// and needs no database.

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
		"environment": "sandbox",
		"api_key":     "secret",
	}
	config := providerInitConfig(plain, "production")
	if got := config["environment"]; got != "production" {
		t.Errorf("environment = %v, want production", got)
	}
	if got := config["api_key"]; got != "secret" {
		t.Errorf("api_key = %v, want original value", got)
	}
	if got := plain["environment"]; got != "sandbox" {
		t.Errorf("original environment = %v, want unchanged sandbox", got)
	}
}
