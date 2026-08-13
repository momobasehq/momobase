package providers

import "strings"

// Redact removes provider error text that appears to contain sensitive values and
// truncates other text to a safe maximum length.
func Redact(value string) string {
	lower := strings.ToLower(value)
	for _, secret := range []string{
		"token",
		"secret",
		"api_key",
		"subscription_key",
		"password",
		"authorization",
		"bearer",
	} {
		if strings.Contains(lower, secret) {
			return "[redacted provider error]"
		}
	}
	if len(value) > 500 {
		return value[:500]
	}
	return value
}
