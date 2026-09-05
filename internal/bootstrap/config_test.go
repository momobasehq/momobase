package bootstrap

import (
	"strings"
	"testing"
	"time"
)

// TestDefaultConfigRunsUnchanged pins the development baseline: a host that supplies
// no configuration still gets one that starts. The token secrets are the trap — the
// manager rejects anything shorter than 32 characters, so a shortened placeholder
// would fail every instance built without WithConfig.
func TestDefaultConfigRunsUnchanged(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate() error = %v", err)
	}
	if len(cfg.Security.AdminOAuthSecret) < 32 || len(cfg.Security.AppOAuthSecret) < 32 {
		t.Fatalf("default token secrets are too short to sign with: %+v", cfg.Security)
	}
	if cfg.App.Addr != ":9090" || cfg.DB.Type != "sqlite" || !cfg.Features.AutoMigrate {
		t.Fatalf("unexpected development defaults: %+v", cfg)
	}
	if cfg.Workers.HealthInterval != 30*time.Second || cfg.Security.AdminAccessTTL != 15*time.Minute {
		t.Fatalf("unexpected default intervals: %+v %+v", cfg.Workers, cfg.Security)
	}
	// Returned by value, so one host's edit cannot reach another's configuration.
	cfg.App.CORSAllowedOrigins[0] = "https://edited.example"
	if DefaultConfig().App.CORSAllowedOrigins[0] == "https://edited.example" {
		t.Fatal("DefaultConfig() shares its slices between calls")
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
			func(c *Config) { c.Security.EncryptionMasterKeyBase64 = DefaultEncryptionMasterKeyBase64 },
			"Security.EncryptionMasterKeyBase64",
		},
		{"short admin secret", func(c *Config) { c.Security.AdminOAuthSecret = "short" }, "Security.AdminOAuthSecret"},
		{"short app secret", func(c *Config) { c.Security.AppOAuthSecret = "short" }, "Security.AppOAuthSecret"},
		{"insecure public URL", func(c *Config) { c.App.PublicURL = "http://api.example.com" }, "App.PublicURL"},
		{"wildcard CORS", func(c *Config) { c.App.CORSAllowedOrigins = []string{"*"} }, "App.CORSAllowedOrigins"},
		{"remote postgres without TLS", func(c *Config) { c.DB.SSLMode = "disable" }, "DB.SSLMode"},
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
