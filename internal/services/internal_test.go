package services

import (
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/providers"
)

// These tests cover unexported helpers, so they stay in-package and must not reach
// for internal/testsupport — that would be an import cycle. Each one is a pure
// function, so none of them needs a database.

func TestWebhookAccountMatching(t *testing.T) {
	tx := &domain.Transaction{
		Amount:          1500,
		Currency:        "UGX",
		Country:         "UG",
		Reference:       "ORDER-1",
		CustomerAccount: "256770000000",
	}
	matching := &VerifiedWebhook{ProviderWebhookEvent: providers.ProviderWebhookEvent{Account: "256770000000"}}
	if err := validateWebhook(matching, tx); err != nil {
		t.Fatalf("validateWebhook() error = %v, want the recorded account to match", err)
	}
	absent := &VerifiedWebhook{}
	if err := validateWebhook(absent, tx); err != nil {
		t.Fatalf("validateWebhook() error = %v, want an event without an account to skip the check", err)
	}
	// A provider that reports an unnormalized account no longer matches: the engine
	// compares exactly, so normalization is the provider's to keep consistent.
	other := &VerifiedWebhook{ProviderWebhookEvent: providers.ProviderWebhookEvent{Account: "0770000000"}}
	if err := validateWebhook(other, tx); err == nil {
		t.Fatal("validateWebhook() = nil, want a mismatched account rejected")
	}
}
