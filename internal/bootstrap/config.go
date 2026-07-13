package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App struct {
		Name, Env, Addr, PublicURL string
		CORSAllowedOrigins         []string
	}
	Log      struct{ Level string }
	DB       struct{ Type, Path, Host, Port, User, Password, Name, SSLMode string }
	Security struct {
		EncryptionMasterKeyBase64, AdminOAuthSecret, AppOAuthSecret  string
		AdminAccessTTL, AdminRefreshTTL, AppAccessTTL, AppRefreshTTL time.Duration
		AppClientIDPrefix, AppClientSecretPrefix                     string
	}
	Workers struct {
		Enabled, HealthEnabled, ReconciliationEnabled, CleanupEnabled bool
		HealthInterval, ReconciliationInterval, CleanupInterval       time.Duration
	}
	Features struct{ AdminFrontendEnabled, AutoMigrate bool }
}

func LoadConfig() Config {
	var c Config
	c.App.Name, c.App.Env, c.App.Addr = env("APP_NAME", "momobase"), env("APP_ENV", "development"), env("APP_ADDR", ":8080")
	c.App.PublicURL, c.App.CORSAllowedOrigins = env("APP_PUBLIC_URL", "http://localhost:8080"), list("CORS_ALLOWED_ORIGINS", "http://localhost:8080")
	c.Log.Level = env("LOG_LEVEL", "info")
	c.DB.Type, c.DB.Path, c.DB.Host, c.DB.Port = env("DB_TYPE", "sqlite"), env("DB_PATH", "./data/momobase.db"), env("DB_HOST", "localhost"), env("DB_PORT", "5432")
	c.DB.User, c.DB.Password, c.DB.Name, c.DB.SSLMode = env("DB_USER", "momobase"), env("DB_PASSWORD", ""), env("DB_NAME", "momobase"), env("DB_SSLMODE", "disable")
	c.Security.EncryptionMasterKeyBase64 = env("ENCRYPTION_MASTER_KEY_BASE64", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	c.Security.AdminOAuthSecret, c.Security.AppOAuthSecret = env("ADMIN_OAUTH_SECRET", "change-me-admin-oauth-secret"), env("APP_OAUTH_SECRET", "change-me-app-oauth-secret")
	c.Security.AdminAccessTTL, c.Security.AdminRefreshTTL = duration("ADMIN_ACCESS_TTL_MINUTES", 15, time.Minute), duration("ADMIN_REFRESH_TTL_HOURS", 24, time.Hour)
	c.Security.AppAccessTTL, c.Security.AppRefreshTTL = duration("APP_ACCESS_TTL_MINUTES", 30, time.Minute), duration("APP_REFRESH_TTL_HOURS", 24, time.Hour)
	c.Security.AppClientIDPrefix, c.Security.AppClientSecretPrefix = env("APP_CLIENT_ID_PREFIX", "app_client"), env("APP_CLIENT_SECRET_PREFIX", "mb_test")
	c.Workers.Enabled, c.Workers.HealthEnabled = boolean("WORKERS_ENABLED", true), boolean("HEALTH_WORKER_ENABLED", true)
	c.Workers.ReconciliationEnabled, c.Workers.CleanupEnabled = boolean("RECONCILIATION_WORKER_ENABLED", true), boolean("CLEANUP_WORKER_ENABLED", true)
	c.Workers.HealthInterval = duration("HEALTH_CHECK_INTERVAL_SECONDS", 30, time.Second)
	c.Workers.ReconciliationInterval = duration("RECONCILIATION_INTERVAL_SECONDS", 60, time.Second)
	c.Workers.CleanupInterval = duration("CLEANUP_INTERVAL_SECONDS", 300, time.Second)
	c.Features.AdminFrontendEnabled, c.Features.AutoMigrate = boolean("ADMIN_FRONTEND_ENABLED", false), boolean("AUTO_MIGRATE", true)
	return c
}
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
func boolean(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
func duration(key string, fallback int, unit time.Duration) time.Duration {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		value = fallback
	}
	return time.Duration(value) * unit
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
