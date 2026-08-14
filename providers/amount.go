package providers

import (
	"fmt"
	"strconv"
	"strings"
)

// PaymentStatus normalizes a provider status value to a Momobase transaction
// status. It recognizes common vocabularies only; a provider using bespoke
// status codes should map them itself and return a Tx constant directly.
//
// It is idempotent: an already-normalized status maps to itself, so a provider
// that reports normalized statuses is not corrupted by a second pass through
// this function on the reconciliation and webhook paths.
func PaymentStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "SUCCESS", "SUCCESSFUL", "SUCCEEDED", "COMPLETED", "200":
		return "succeeded"
	case "FAILED", "FAILURE", "DECLINED":
		return "failed"
	case "PENDING", "IN_PROGRESS", "PROCESSING", "":
		return "processing"
	case "CANCELLED", "CANCELED":
		return "cancelled"
	case "EXPIRED", "TIMEOUT", "TIMED_OUT":
		return "expired"
	default:
		return "unknown"
	}
}

// OptionalAmount parses an amount when raw is nonblank and returns nil otherwise.
func OptionalAmount(raw, currency string) (*int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := ParseAmountToMinor(raw, currency)
	return &value, err
}

// Currencies whose minor unit differs from the ISO 4217 default of two decimal
// places. Everything absent from both lists is treated as two-decimal.
const (
	zeroDecimalCurrencies  = " BIF CLP DJF GNF ISK JPY KMF KRW PYG RWF UGX VND VUV XAF XOF XPF "
	threeDecimalCurrencies = " BHD IQD JOD KWD LYD OMR TND "
)

// exponent returns the number of decimal places in the currency's minor unit.
func exponent(currency string) int {
	code := " " + strings.ToUpper(strings.TrimSpace(currency)) + " "
	switch {
	case strings.Contains(zeroDecimalCurrencies, code):
		return 0
	case strings.Contains(threeDecimalCurrencies, code):
		return 3
	default:
		return 2
	}
}

// scale returns the number of minor units in one major unit for exp decimal places.
func scale(exp int) int64 {
	factor := int64(1)
	for range exp {
		factor *= 10
	}
	return factor
}

// FormatAmountMinor formats an amount in minor units for the currency's precision.
func FormatAmountMinor(amount int64, currency string) string {
	exp := exponent(currency)
	if exp == 0 {
		return strconv.FormatInt(amount, 10)
	}
	sign := ""
	if amount < 0 {
		sign, amount = "-", -amount
	}
	factor := scale(exp)
	return fmt.Sprintf("%s%d.%0*d", sign, amount/factor, exp, amount%factor)
}

// ParseAmountToMinor converts a provider amount string to minor currency units.
func ParseAmountToMinor(raw, currency string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	exp := exponent(currency)
	if exp == 0 {
		return strconv.ParseInt(value, 10, 64)
	}
	sign := int64(1)
	if strings.HasPrefix(value, "-") {
		sign, value = -1, value[1:]
	} else {
		value = strings.TrimPrefix(value, "+")
	}
	whole, fraction, _ := strings.Cut(value, ".")
	if len(fraction) > exp {
		return 0, fmt.Errorf(
			"amount %q has too many decimals for %s",
			raw,
			strings.ToUpper(currency),
		)
	}
	units, err := strconv.ParseInt(First(whole, "0"), 10, 64)
	if err != nil {
		return 0, err
	}
	minor, err := strconv.ParseInt(
		First(fraction+strings.Repeat("0", exp-len(fraction)), "0"),
		10,
		64,
	)
	return sign * (units*scale(exp) + minor), err
}
