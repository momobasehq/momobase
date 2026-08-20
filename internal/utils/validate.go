package utils

import (
	"strings"
	"unicode"
)

// ValidIdentifier reports whether a rail identifier — a payment method or an
// account scheme — is safely comparable. Both name provider-specific values rather
// than a fixed set, so they are checked structurally instead of against a list of
// known ones. The empty string is valid; callers that require a value check it.
func ValidIdentifier(value string) bool {
	if len(value) > 64 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

// ValidAccount reports whether an account identifier fits the column it is stored
// in and is safe to log and compare. Its meaning stays opaque to the engine.
func ValidAccount(account string) bool {
	return len(account) <= 255 && strings.IndexFunc(account, unicode.IsControl) < 0
}
