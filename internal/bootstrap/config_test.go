package bootstrap

import (
	"strings"
	"testing"
	"time"
)

func TestBooleanRejectsInvalidValue(t *testing.T) {
	t.Setenv("TEST_BOOLEAN", "sometimes")

	_, err := boolean("TEST_BOOLEAN", true)
	if err == nil || !strings.Contains(err.Error(), "TEST_BOOLEAN") {
		t.Fatalf("boolean() error = %v, want error naming TEST_BOOLEAN", err)
	}
}

func TestDurationRejectsInvalidValue(t *testing.T) {
	for _, value := range []string{"not-a-number", "0", "-1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("TEST_DURATION", value)

			_, err := duration("TEST_DURATION", 30, time.Second)
			if err == nil || !strings.Contains(err.Error(), "TEST_DURATION") {
				t.Fatalf("duration() error = %v, want error naming TEST_DURATION", err)
			}
		})
	}
}

func TestLoadConfigReturnsParsingError(t *testing.T) {
	setValidParsingEnvironment(t)
	t.Setenv("WORKERS_ENABLED", "sometimes")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "WORKERS_ENABLED") {
		t.Fatalf("LoadConfig() error = %v, want error naming WORKERS_ENABLED", err)
	}
}

func setValidParsingEnvironment(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"ADMIN_ACCESS_TTL_MINUTES":        "15",
		"ADMIN_REFRESH_TTL_HOURS":         "24",
		"APP_ACCESS_TTL_MINUTES":          "30",
		"APP_REFRESH_TTL_HOURS":           "24",
		"WORKERS_ENABLED":                 "true",
		"HEALTH_WORKER_ENABLED":           "true",
		"RECONCILIATION_WORKER_ENABLED":   "true",
		"CLEANUP_WORKER_ENABLED":          "true",
		"HEALTH_CHECK_INTERVAL_SECONDS":   "30",
		"RECONCILIATION_INTERVAL_SECONDS": "60",
		"CLEANUP_INTERVAL_SECONDS":        "300",
		"ADMIN_FRONTEND_ENABLED":          "false",
		"AUTO_MIGRATE":                    "true",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}
