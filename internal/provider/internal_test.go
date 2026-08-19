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
