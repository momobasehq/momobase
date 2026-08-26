package utils

import (
	"errors"
	"strings"
)

// NormalizeCurrency validates and uppercases a three-letter currency code.
func NormalizeCurrency(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 3 {
		return "", errors.New("currency must be a three-letter code")
	}
	for _, char := range value {
		if char < 'A' || char > 'Z' {
			return "", errors.New("currency must be a three-letter code")
		}
	}
	return value, nil
}
