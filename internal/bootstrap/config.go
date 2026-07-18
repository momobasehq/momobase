package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// AppConfig contains process-level application settings.
type AppConfig struct {
	Name               string
	Env                string
	Addr               string
	PublicURL          string
	CORSAllowedOrigins []string
}

// LogConfig contains structured logging settings.
type LogConfig struct {
	Level string
}

// DatabaseConfig contains settings shared by the supported database drivers.
type DatabaseConfig struct {
	Type     string
	Path     string
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// SecurityConfig contains encryption, token, and application credential settings.
type SecurityConfig struct {
	EncryptionMasterKeyBase64 string
	AdminOAuthSecret          string
	AppOAuthSecret            string
	AdminAccessTTL            time.Duration
	AdminRefreshTTL           time.Duration
	AppAccessTTL              time.Duration
	AppRefreshTTL             time.Duration
	AppClientIDPrefix         string
	AppClientSecretPrefix     string
}

// WorkersConfig controls background task activation and scheduling.
type WorkersConfig struct {
	Enabled                bool
	HealthEnabled          bool
	ReconciliationEnabled  bool
	CleanupEnabled         bool
	HealthInterval         time.Duration
	ReconciliationInterval time.Duration
	CleanupInterval        time.Duration
}

// FeaturesConfig controls optional application behavior.
type FeaturesConfig struct {
	AdminFrontendEnabled bool
	AutoMigrate          bool
}

// Config contains all application configuration groups.
type Config struct {
	App      AppConfig
	Log      LogConfig
	DB       DatabaseConfig
	Security SecurityConfig
	Workers  WorkersConfig
	Features FeaturesConfig
}

// LoadConfig reads configuration from the environment and rejects invalid
// explicitly configured boolean and duration values.
func LoadConfig() (Config, error) {
	var c Config
	// app
	c.App.Name = env("APP_NAME", "momobase")
	c.App.Env = env("APP_ENV", "development")
	c.App.Addr = env("APP_ADDR", ":9090")
	c.App.PublicURL = env("APP_PUBLIC_URL", "http://localhost:9090")
	c.App.CORSAllowedOrigins = list("CORS_ALLOWED_ORIGINS", "http://localhost:9090")
	// logs
	c.Log.Level = env("LOG_LEVEL", "info")
	// database
	c.DB.Type = env("DB_TYPE", "sqlite")
	c.DB.Path = env("DB_PATH", "./data/momobase.db")
	c.DB.Host = env("DB_HOST", "localhost")
	c.DB.Port = env("DB_PORT", "5432")
	c.DB.User = env("DB_USER", "momobase")
	c.DB.Password = env("DB_PASSWORD", "")
	c.DB.Name = env("DB_NAME", "momobase")
	c.DB.SSLMode = env("DB_SSLMODE", "disable")
	// security
	c.Security.EncryptionMasterKeyBase64 = env("ENCRYPTION_MASTER_KEY_BASE64", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	c.Security.AdminOAuthSecret = env("ADMIN_OAUTH_SECRET", "change-me-admin-oauth-secret")
	c.Security.AppOAuthSecret = env("APP_OAUTH_SECRET", "change-me-app-oauth-secret")
	var err error
	c.Security.AdminAccessTTL, err = duration("ADMIN_ACCESS_TTL_MINUTES", 15, time.Minute)
	if err != nil {
		return Config{}, err
	}
	c.Security.AdminRefreshTTL, err = duration("ADMIN_REFRESH_TTL_HOURS", 24, time.Hour)
	if err != nil {
		return Config{}, err
	}
	c.Security.AppAccessTTL, err = duration("APP_ACCESS_TTL_MINUTES", 30, time.Minute)
	if err != nil {
		return Config{}, err
	}
	c.Security.AppRefreshTTL, err = duration("APP_REFRESH_TTL_HOURS", 24, time.Hour)
	if err != nil {
		return Config{}, err
	}
	c.Security.AppClientIDPrefix = env("APP_CLIENT_ID_PREFIX", "app_client")
	c.Security.AppClientSecretPrefix = env("APP_CLIENT_SECRET_PREFIX", "mb_test")
	// workers
	c.Workers.Enabled, err = boolean("WORKERS_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	c.Workers.HealthEnabled, err = boolean("HEALTH_WORKER_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	c.Workers.ReconciliationEnabled, err = boolean("RECONCILIATION_WORKER_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	c.Workers.CleanupEnabled, err = boolean("CLEANUP_WORKER_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	c.Workers.HealthInterval, err = duration("HEALTH_CHECK_INTERVAL_SECONDS", 30, time.Second)
	if err != nil {
		return Config{}, err
	}
	c.Workers.ReconciliationInterval, err = duration("RECONCILIATION_INTERVAL_SECONDS", 60, time.Second)
	if err != nil {
		return Config{}, err
	}
	c.Workers.CleanupInterval, err = duration("CLEANUP_INTERVAL_SECONDS", 300, time.Second)
	if err != nil {
		return Config{}, err
	}
	// features
	c.Features.AdminFrontendEnabled, err = boolean("ADMIN_FRONTEND_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	c.Features.AutoMigrate, err = boolean("AUTO_MIGRATE", true)
	if err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate rejects configuration that is unsafe for staging or production.
func (c Config) Validate() error {
	if c.App.Env != "production" && c.App.Env != "staging" {
		return nil
	}
	switch {
	case c.Security.EncryptionMasterKeyBase64 == "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=":
		return fmt.Errorf("ENCRYPTION_MASTER_KEY_BASE64 must be changed for %s", c.App.Env)
	case len(c.Security.AdminOAuthSecret) < 32 || strings.HasPrefix(c.Security.AdminOAuthSecret, "change-me-"):
		return fmt.Errorf("ADMIN_OAUTH_SECRET must be at least 32 non-default characters for %s", c.App.Env)
	case len(c.Security.AppOAuthSecret) < 32 || strings.HasPrefix(c.Security.AppOAuthSecret, "change-me-"):
		return fmt.Errorf("APP_OAUTH_SECRET must be at least 32 non-default characters for %s", c.App.Env)
	case !strings.HasPrefix(c.App.PublicURL, "https://"):
		return fmt.Errorf("APP_PUBLIC_URL must use https:// for %s", c.App.Env)
	}
	for _, origin := range c.App.CORSAllowedOrigins {
		if origin == "*" {
			return errors.New("CORS_ALLOWED_ORIGINS must not contain * in production")
		}
	}
	if c.DB.Type == "postgres" && c.DB.SSLMode == "disable" && c.DB.Host != "db" && c.DB.Host != "localhost" && c.DB.Host != "127.0.0.1" {
		return errors.New("DB_SSLMODE must be enabled for remote postgres")
	}
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func boolean(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean, got %q: %w", key, v, err)
	}
	return b, nil
}

func duration(key string, fallback int, unit time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return time.Duration(fallback) * unit, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a positive integer, got %q: %w", key, raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero, got %d", key, value)
	}
	return time.Duration(value) * unit, nil
}

func list(key, fallback string) []string {
	values := strings.Split(env(key, fallback), ",")
	out := values[:0]
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
