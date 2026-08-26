package utils_test

import (
	"testing"

	"github.com/momobasehq/momobase/internal/utils"
)

func TestNormalizeCurrency(t *testing.T) {
	if got, err := utils.NormalizeCurrency(" ugx "); err != nil || got != "UGX" {
		t.Fatalf("NormalizeCurrency() = %q, %v, want UGX", got, err)
	}
	for _, value := range []string{"", "UG", "US1", "USDX"} {
		if got, err := utils.NormalizeCurrency(value); err == nil {
			t.Errorf("NormalizeCurrency(%q) = %q, want an error", value, got)
		}
	}
}
