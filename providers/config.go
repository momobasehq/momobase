package providers

import (
	"fmt"
	"strconv"
	"strings"
)

// Slash ensures value begins with a forward slash.
func Slash(value string) string {
	if strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}

// FirstError returns primary when it is non-nil and fallback otherwise.
func FirstError(primary, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}

// First returns the first nonblank value after trimming surrounding whitespace.
func First(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func text(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// String returns a trimmed textual representation of a configuration value.
func String(c ProviderConfig, key string) string {
	return text(c[key])
}

// Bool reports whether a configuration value is "true", case-insensitively, or "1".
func Bool(c ProviderConfig, key string) bool {
	value := strings.ToLower(text(c[key]))
	return value == "true" || value == "1"
}

// Int converts a configuration value to an integer, returning zero when invalid.
func Int(c ProviderConfig, key string) int {
	value, _ := strconv.Atoi(text(c[key]))
	return value
}

// Path returns the textual value at a dot-separated path through nested maps.
func Path(values map[string]any, path string) string {
	var value any = values
	for _, key := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value = object[key]
	}
	return text(value)
}
