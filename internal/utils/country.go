package utils

import (
	"errors"
	"strings"

	"golang.org/x/text/language"
)

// NormalizeTransactionCountry returns a validated uppercase ISO 3166-1 alpha-2 country code.
//
// The length guard is load-bearing: language.ParseRegion accepts alpha-3 input and
// silently rewrites it, so "USA" would otherwise be stored as "US". IsCountry then
// rejects the reserved and grouping regions ParseRegion allows through — ZZ, XX, QO,
// EU, 419. The stored value is the caller's own input uppercased rather than the
// parsed region, which is what keeps a rewrite from reaching the database.
func NormalizeTransactionCountry(country string) (string, error) {
	country = strings.TrimSpace(country)
	region, err := language.ParseRegion(country)
	if len(country) != 2 || err != nil || !region.IsCountry() {
		return "", errors.New("country must be a supported ISO-3166 alpha-2 code")
	}
	return strings.ToUpper(country), nil
}

// NormalizeOptionalCountry validates a country code that a request may omit.
func NormalizeOptionalCountry(country string) (string, error) {
	if strings.TrimSpace(country) == "" {
		return "", nil
	}
	return NormalizeTransactionCountry(country)
}
