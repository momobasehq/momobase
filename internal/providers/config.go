package providers

import (
	"fmt"
	"strconv"
	"strings"
)

func Slash(value string) string {
	if strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}

func FirstError(primary, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}

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

func String(c ProviderConfig, key string) string {
	return text(c[key])
}

func Bool(c ProviderConfig, key string) bool {
	value := strings.ToLower(text(c[key]))
	return value == "true" || value == "1"
}

func Int(c ProviderConfig, key string) int {
	value, _ := strconv.Atoi(text(c[key]))
	return value
}

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
