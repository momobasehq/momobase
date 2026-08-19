package routing

import "testing"

// This test covers an unexported helper, so it stays in-package and must not reach
// for internal/testsupport — that would be an import cycle. It is a pure function
// and needs no database.

func TestCountryEligible(t *testing.T) {
	if !countryEligible(nil, "UG") || !countryEligible(nil, "") {
		t.Error("countryEligible() = false, want an account with no countries to be unrestricted")
	}
	if !countryEligible([]string{"UG"}, "UG") {
		t.Error("countryEligible(UG, UG) = false, want true")
	}
	if countryEligible([]string{"UG"}, "RW") || countryEligible([]string{"UG"}, "") {
		t.Error("countryEligible() = true, want a declared country to be required")
	}
}
