package utils_test

import (
	"testing"

	"github.com/momobasehq/momobase/internal/utils"
)

func TestCountryNormalization(t *testing.T) {
	if country, err := utils.NormalizeOptionalCountry("  "); err != nil || country != "" {
		t.Fatalf("utils.NormalizeOptionalCountry(blank) = %q, %v, want an empty country", country, err)
	}
	if country, err := utils.NormalizeOptionalCountry(" ug "); err != nil || country != "UG" {
		t.Fatalf("utils.NormalizeOptionalCountry(%q) = %q, %v", " ug ", country, err)
	}

	// language.ParseRegion is deliberately not trusted on its own: it accepts the
	// reserved and grouping regions, and rewrites alpha-3 input into alpha-2 rather
	// than rejecting it, so "USA" would otherwise be stored as "US".
	for _, country := range []string{"XX", "ZZ", "QO", "EU", "419", "USA", "U", "UGX"} {
		if got, err := utils.NormalizeTransactionCountry(country); err == nil {
			t.Errorf("utils.NormalizeTransactionCountry(%q) = %q, want an error", country, got)
		}
	}
	if _, err := utils.NormalizeTransactionCountry(""); err == nil {
		t.Error("utils.NormalizeTransactionCountry(\"\") = nil, want an error for a required country")
	}
	// UK is exceptionally reserved for GB rather than assigned, and ParseRegion
	// resolves it. Storing the caller's own input keeps it out of the canonical
	// rewrite path, so what round-trips is exactly what was sent.
	if country, err := utils.NormalizeTransactionCountry("uk"); err != nil || country != "UK" {
		t.Errorf("utils.NormalizeTransactionCountry(%q) = %q, %v, want the input uppercased", "uk", country, err)
	}
}
