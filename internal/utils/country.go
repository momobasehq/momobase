package utils

import (
	"errors"
	"strings"

	"golang.org/x/text/language"
)

// NormalizeProviderCountries validates and deduplicates a provider account's country
// list, preserving the caller's ordering. An empty list normalizes to nil, which
// marks the account as unrestricted.
func NormalizeProviderCountries(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		country, err := NormalizeTransactionCountry(value)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[country]; !exists {
			seen[country] = struct{}{}
			out = append(out, country)
		}
	}
	return out, nil
}

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

// NormalizeOptionalCountry validates a country code that a request may omit,
// returning an empty string when none was supplied. A payment without a country
// can only be routed to a provider account that declares no countries.
func NormalizeOptionalCountry(country string) (string, error) {
	if strings.TrimSpace(country) == "" {
		return "", nil
	}
	return NormalizeTransactionCountry(country)
}
