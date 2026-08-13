package providers

import (
	"fmt"
	"strconv"
	"strings"
)

// PaymentStatus normalizes a provider status value to a Momobase transaction status.
func PaymentStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TS", "SUCCESS", "SUCCESSFUL", "COMPLETED", "200":
		return "succeeded"
	case "TF", "FAILED", "FAILURE", "DECLINED":
		return "failed"
	case "TIP", "PENDING", "IN_PROGRESS", "PROCESSING", "":
		return "processing"
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

func exponent(currency string) int {
	if strings.Contains(
		" BIF CLP DJF GNF ISK JPY KMF KRW PYG RWF UGX VND VUV XAF XOF XPF ",
		" "+strings.ToUpper(strings.TrimSpace(currency))+" ",
	) {
		return 0
	}
	return 2
}

// FormatAmountMinor formats an amount in minor units for the currency's precision.
func FormatAmountMinor(amount int64, currency string) string {
	if exponent(currency) == 0 {
		return strconv.FormatInt(amount, 10)
	}
	fraction := amount % 100
	if fraction < 0 {
		fraction = -fraction
	}
	return fmt.Sprintf("%d.%02d", amount/100, fraction)
}

// ParseAmountToMinor converts a provider amount string to minor currency units.
func ParseAmountToMinor(raw, currency string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	if exponent(currency) == 0 {
		return strconv.ParseInt(value, 10, 64)
	}
	sign := int64(1)
	if strings.HasPrefix(value, "-") {
		sign, value = -1, value[1:]
	} else {
		value = strings.TrimPrefix(value, "+")
	}
	parts := strings.SplitN(value, ".", 2)
	if len(parts) == 2 && len(parts[1]) > 2 {
		return 0, fmt.Errorf(
			"amount %q has too many decimals for %s",
			raw,
			strings.ToUpper(currency),
		)
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	whole, err := strconv.ParseInt(First(parts[0], "0"), 10, 64)
	if err != nil {
		return 0, err
	}
	minor, err := strconv.ParseInt(
		First(fraction+strings.Repeat("0", 2-len(fraction)), "0"),
		10,
		64,
	)
	return sign * (whole*100 + minor), err
}
