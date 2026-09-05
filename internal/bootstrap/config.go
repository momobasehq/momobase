package bootstrap

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// AppConfig contains process-level application settings.
type AppConfig struct {
	// Name identifies the application in runtime metadata.
	Name string
	// Env selects development, staging, or production safety checks.
	Env string
	// Addr is the address passed to the Fiber listener.
	Addr string
	// PublicURL is the externally reachable base URL.
	PublicURL string
	// CORSAllowedOrigins lists browser origins allowed to call the API.
	CORSAllowedOrigins []string
	// TrustedProxyCIDRs names the proxies in front of this deployment, as addresses or
	// CIDRs. Empty means no forwarded header is believed, so rate limiting keys on the
	// immediate peer; behind a proxy that would put every client in one bucket.
	TrustedProxyCIDRs []string
}

// LogConfig contains structured logging settings.
type LogConfig struct {
	// Level is the minimum structured log level.
	Level string
}

// DatabaseConfig contains settings shared by the supported database drivers.
type DatabaseConfig struct {
	// Type selects sqlite, postgres, or mysql.
	Type string
	// Path is the SQLite database path.
	Path string
	// Host is the PostgreSQL or MySQL host.
	Host string
	// Port is the PostgreSQL or MySQL port.
	Port string
	// User is the PostgreSQL or MySQL user.
	User string
	// Password is the PostgreSQL or MySQL password.
	Password string
	// Name is the PostgreSQL or MySQL database name.
	Name string
	// SSLMode is the PostgreSQL TLS mode.
	SSLMode string
}

// SecurityConfig contains encryption, token, and application credential settings.
type SecurityConfig struct {
	// EncryptionMasterKeyBase64 is a base64-encoded 32-byte AES key.
	EncryptionMasterKeyBase64 string
	// AdminOAuthSecret signs administrator tokens.
	AdminOAuthSecret string
	// AppOAuthSecret signs application tokens.
	AppOAuthSecret string
	// AdminAccessTTL controls administrator access-token lifetime.
	AdminAccessTTL time.Duration
	// AdminRefreshTTL controls administrator refresh-token lifetime.
	AdminRefreshTTL time.Duration
	// AppAccessTTL controls application access-token lifetime.
	AppAccessTTL time.Duration
	// AppRefreshTTL controls application refresh-token lifetime.
	AppRefreshTTL time.Duration
	// AppClientIDPrefix prefixes generated application client IDs.
	AppClientIDPrefix string
	// AppClientSecretPrefix prefixes generated application client secrets.
	AppClientSecretPrefix string
}

// WorkersConfig controls background task activation and scheduling.
type WorkersConfig struct {
	// Enabled controls all background workers.
	Enabled bool
	// HealthEnabled controls provider health checks.
	HealthEnabled bool
	// ReconciliationEnabled controls transaction reconciliation.
	ReconciliationEnabled bool
	// CleanupEnabled controls expired-data cleanup.
	CleanupEnabled bool
	// HealthInterval is the provider health-check interval.
	HealthInterval time.Duration
	// ReconciliationInterval is the transaction reconciliation interval.
	ReconciliationInterval time.Duration
	// CleanupInterval is the expired-data cleanup interval.
	CleanupInterval time.Duration
}

// FeaturesConfig controls optional package behavior.
type FeaturesConfig struct {
	// AutoMigrate applies pending schema changes while constructing an instance.
	AutoMigrate bool
}

// Config contains all application configuration groups.
type Config struct {
	// App contains process and HTTP settings.
	App AppConfig
	// Log contains structured logging settings.
	Log LogConfig
	// DB contains database connection settings.
	DB DatabaseConfig
	// Security contains encryption and token settings.
	Security SecurityConfig
	// Workers contains background task settings.
	Workers WorkersConfig
	// Features contains optional package behavior.
	Features FeaturesConfig
}

// DefaultConfig returns the configuration Momobase runs with when a host supplies
// none. It is a development baseline: SQLite in ./data, a placeholder encryption
// key, and placeholder token secrets, all of which Validate rejects for staging
// and production. Every field is a plain value, so a host copies it and edits
// what it needs.
//
// Momobase reads no environment variables of its own. A host that configures from
// the environment reads it itself and assigns the fields.
func DefaultConfig() Config {
	return Config{
		App: AppConfig{
			Name:               "momobase",
			Env:                "development",
			Addr:               ":9090",
			PublicURL:          "http://localhost:9090",
			CORSAllowedOrigins: []string{"http://localhost:9090"},
		},
		Log: LogConfig{Level: "info"},
		DB: DatabaseConfig{
			Type:    "sqlite",
			Path:    "./data/momobase.db",
			Host:    "localhost",
			Port:    "5432",
			User:    "momobase",
			Name:    "momobase",
			SSLMode: "disable",
		},
		Security: SecurityConfig{
			EncryptionMasterKeyBase64: DefaultEncryptionMasterKeyBase64,
			AdminOAuthSecret:          DefaultAdminOAuthSecret,
			AppOAuthSecret:            DefaultAppOAuthSecret,
			AdminAccessTTL:            15 * time.Minute,
			AdminRefreshTTL:           24 * time.Hour,
			AppAccessTTL:              30 * time.Minute,
			AppRefreshTTL:             24 * time.Hour,
			AppClientIDPrefix:         "app_client",
			AppClientSecretPrefix:     "mb_test",
		},
		Workers: WorkersConfig{
			Enabled:                true,
			HealthEnabled:          true,
			ReconciliationEnabled:  true,
			CleanupEnabled:         true,
			HealthInterval:         30 * time.Second,
			ReconciliationInterval: 60 * time.Second,
			CleanupInterval:        300 * time.Second,
		},
		Features: FeaturesConfig{AutoMigrate: true},
	}
}

// The placeholder credentials DefaultConfig carries. They are long enough to run a
// development instance unchanged and are rejected by Validate for staging and
// production, so a deployment that forgets to replace one fails at startup.
const (
	// DefaultEncryptionMasterKeyBase64 is the all-zero development AES key.
	DefaultEncryptionMasterKeyBase64 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	// DefaultAdminOAuthSecret is the development administrator token secret.
	DefaultAdminOAuthSecret = "change-me-admin-oauth-secret-at-least-32-chars"
	// DefaultAppOAuthSecret is the development application token secret.
	DefaultAppOAuthSecret = "change-me-app-oauth-secret-at-least-32-chars"
)

// Validate rejects configuration that is unsafe for staging or production.
func (c Config) Validate() error {
	if c.App.Env != "production" && c.App.Env != "staging" {
		return nil
	}
	switch {
	case c.Security.EncryptionMasterKeyBase64 == DefaultEncryptionMasterKeyBase64:
		return fmt.Errorf("Security.EncryptionMasterKeyBase64 must be changed for %s", c.App.Env)
	case len(c.Security.AdminOAuthSecret) < 32 || strings.HasPrefix(c.Security.AdminOAuthSecret, "change-me-"):
		return fmt.Errorf("Security.AdminOAuthSecret must be at least 32 non-default characters for %s", c.App.Env)
	case len(c.Security.AppOAuthSecret) < 32 || strings.HasPrefix(c.Security.AppOAuthSecret, "change-me-"):
		return fmt.Errorf("Security.AppOAuthSecret must be at least 32 non-default characters for %s", c.App.Env)
	case !strings.HasPrefix(c.App.PublicURL, "https://"):
		return fmt.Errorf("App.PublicURL must use https:// for %s", c.App.Env)
	}
	if slices.Contains(c.App.CORSAllowedOrigins, "*") {
		return errors.New("App.CORSAllowedOrigins must not contain * in production")
	}
	if c.DB.Type == "postgres" && c.DB.SSLMode == "disable" && c.DB.Host != "db" && c.DB.Host != "localhost" && c.DB.Host != "127.0.0.1" {
		return errors.New("DB.SSLMode must be enabled for remote postgres")
	}
	return nil
}
