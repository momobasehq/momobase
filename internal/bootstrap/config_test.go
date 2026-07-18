package bootstrap

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigDefaultsAndOverrides(t *testing.T) {
	for _, key := range []string{
		"APP_NAME",
		"APP_ENV",
		"APP_ADDR",
		"APP_PUBLIC_URL",
		"CORS_ALLOWED_ORIGINS",
		"LOG_LEVEL",
		"DB_TYPE",
		"DB_PATH",
		"DB_HOST",
		"DB_PORT",
		"DB_USER",
		"DB_PASSWORD",
		"DB_NAME",
		"DB_SSLMODE",
		"ENCRYPTION_MASTER_KEY_BASE64",
		"ADMIN_OAUTH_SECRET",
		"APP_OAUTH_SECRET",
		"APP_CLIENT_ID_PREFIX",
		"APP_CLIENT_SECRET_PREFIX",
	} {
		t.Setenv(key, "")
	}
	setValidParsingEnvironment(t)
	t.Setenv("APP_NAME", "payments")
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://one.example, ,https://two.example ")
	t.Setenv("ADMIN_ACCESS_TTL_MINUTES", "7")
	t.Setenv("HEALTH_CHECK_INTERVAL_SECONDS", "9")
	t.Setenv("ADMIN_FRONTEND_ENABLED", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.App.Name != "payments" || cfg.App.Env != "development" || cfg.DB.Type != "sqlite" {
		t.Fatalf("unexpected defaults and overrides: %+v", cfg)
	}
	if got := strings.Join(cfg.App.CORSAllowedOrigins, ","); got != "https://one.example,https://two.example" {
		t.Fatalf("CORSAllowedOrigins = %q", got)
	}
	if cfg.Security.AdminAccessTTL != 7*time.Minute {
		t.Fatalf("AdminAccessTTL = %v", cfg.Security.AdminAccessTTL)
	}
	if cfg.Workers.HealthInterval != 9*time.Second || !cfg.Features.AdminFrontendEnabled {
		t.Fatalf("unexpected worker/features config: %+v %+v", cfg.Workers, cfg.Features)
	}
}

func TestConfigValidateProductionSafety(t *testing.T) {
	valid := Config{
		App: AppConfig{
			Env:                "production",
			PublicURL:          "https://api.example.com",
			CORSAllowedOrigins: []string{"https://console.example.com"},
		},
		DB: DatabaseConfig{Type: "postgres", Host: "db.example.com", SSLMode: "require"},
		Security: SecurityConfig{
			EncryptionMasterKeyBase64: "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=",
			AdminOAuthSecret:          strings.Repeat("a", 32),
			AppOAuthSecret:            strings.Repeat("b", 32),
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			"default encryption key",
			func(c *Config) { c.Security.EncryptionMasterKeyBase64 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" },
			"ENCRYPTION_MASTER_KEY_BASE64",
		},
		{"short admin secret", func(c *Config) { c.Security.AdminOAuthSecret = "short" }, "ADMIN_OAUTH_SECRET"},
		{"short app secret", func(c *Config) { c.Security.AppOAuthSecret = "short" }, "APP_OAUTH_SECRET"},
		{"insecure public URL", func(c *Config) { c.App.PublicURL = "http://api.example.com" }, "APP_PUBLIC_URL"},
		{"wildcard CORS", func(c *Config) { c.App.CORSAllowedOrigins = []string{"*"} }, "CORS_ALLOWED_ORIGINS"},
		{"remote postgres without TLS", func(c *Config) { c.DB.SSLMode = "disable" }, "DB_SSLMODE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := valid
			test.edit(&cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}

	development := valid
	development.App.Env = "development"
	development.Security.AdminOAuthSecret = "short"
	if err := development.Validate(); err != nil {
		t.Fatalf("development config rejected: %v", err)
	}
}

func TestParsingHelpersUseFallbacks(t *testing.T) {
	t.Setenv("TEST_BOOLEAN", "")
	gotBool, err := boolean("TEST_BOOLEAN", true)
	if err != nil || !gotBool {
		t.Fatalf("boolean fallback = %v, %v", gotBool, err)
	}
	t.Setenv("TEST_DURATION", "")
	gotDuration, err := duration("TEST_DURATION", 3, time.Minute)
	if err != nil || gotDuration != 3*time.Minute {
		t.Fatalf("duration fallback = %v, %v", gotDuration, err)
	}
}

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
