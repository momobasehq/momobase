package utils

import (
	"fmt"
	"strconv"
	"strings"
)

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

// String returns a trimmed textual representation of the value stored at key.
func String(values map[string]any, key string) string {
	return text(values[key])
}

// Bool reports whether the value stored at key is "true", case-insensitively, or "1".
func Bool(values map[string]any, key string) bool {
	value := strings.ToLower(text(values[key]))
	return value == "true" || value == "1"
}

// Int converts the value stored at key to an integer, returning zero when invalid.
func Int(values map[string]any, key string) int {
	value, _ := strconv.Atoi(text(values[key]))
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
