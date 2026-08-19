package utils

import "strings"

// RedactRawMap replaces the values of credential-bearing keys with a placeholder so
// a provider's raw payload can be logged or persisted. Keys are matched loosely and
// case-insensitively: anything naming a token, secret, key, or password is masked.
func RedactRawMap(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		key := strings.ToLower(k)
		if strings.Contains(key, "token") ||
			strings.Contains(key, "secret") ||
			strings.Contains(key, "key") ||
			strings.Contains(key, "password") {
			v = "[redacted]"
		}
		out[k] = v
	}
	return out
}
