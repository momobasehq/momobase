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
	c.Security.AdminAccessTTL = duration("ADMIN_ACCESS_TTL_MINUTES", 15, time.Minute)
	c.Security.AdminRefreshTTL = duration("ADMIN_REFRESH_TTL_HOURS", 24, time.Hour)
	c.Security.AppAccessTTL = duration("APP_ACCESS_TTL_MINUTES", 30, time.Minute)
	c.Security.AppRefreshTTL = duration("APP_REFRESH_TTL_HOURS", 24, time.Hour)
	c.Security.AppClientIDPrefix = env("APP_CLIENT_ID_PREFIX", "app_client")
	c.Security.AppClientSecretPrefix = env("APP_CLIENT_SECRET_PREFIX", "mb_test")
	// workers
	c.Workers.Enabled = boolean("WORKERS_ENABLED", true)
	c.Workers.HealthEnabled = boolean("HEALTH_WORKER_ENABLED", true)
	c.Workers.ReconciliationEnabled = boolean("RECONCILIATION_WORKER_ENABLED", true)
	c.Workers.CleanupEnabled = boolean("CLEANUP_WORKER_ENABLED", true)
	c.Workers.HealthInterval = duration("HEALTH_CHECK_INTERVAL_SECONDS", 30, time.Second)
	c.Workers.ReconciliationInterval = duration("RECONCILIATION_INTERVAL_SECONDS", 60, time.Second)
	c.Workers.CleanupInterval = duration("CLEANUP_INTERVAL_SECONDS", 300, time.Second)
	// features
	c.Features.AdminFrontendEnabled = boolean("ADMIN_FRONTEND_ENABLED", false)
	c.Features.AutoMigrate = boolean("AUTO_MIGRATE", true)
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
