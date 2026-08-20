package webhook

import (
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/providers"
)

// This test covers an unexported helper, so it stays in-package and must not reach
// for internal/testsupport — that would be an import cycle. It is a pure function
// and needs no database.

func TestWebhookAccountMatching(t *testing.T) {
	tx := &domain.Transaction{
		Amount:          1500,
		Currency:        "UGX",
		Country:         "UG",
		Reference:       "ORDER-1",
		CustomerAccount: "256770000000",
	}
	matching := &verifiedWebhook{ProviderWebhookEvent: providers.ProviderWebhookEvent{Account: "256770000000"}}
	if err := validateWebhook(matching, tx); err != nil {
		t.Fatalf("validateWebhook() error = %v, want the recorded account to match", err)
	}
	absent := &verifiedWebhook{}
	if err := validateWebhook(absent, tx); err != nil {
		t.Fatalf("validateWebhook() error = %v, want an event without an account to skip the check", err)
	}
	// A provider that reports an unnormalized account no longer matches: the engine
	// compares exactly, so normalization is the provider's to keep consistent.
	other := &verifiedWebhook{ProviderWebhookEvent: providers.ProviderWebhookEvent{Account: "0770000000"}}
	if err := validateWebhook(other, tx); err == nil {
		t.Fatal("validateWebhook() = nil, want a mismatched account rejected")
	}
}
