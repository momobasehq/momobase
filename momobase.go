package momobase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/momobasehq/momobase/hooks"
	"github.com/momobasehq/momobase/internal/bootstrap"
	"github.com/momobasehq/momobase/providers"
)

// Configuration groups. Config carries every setting the server needs; the
// remaining types are its fields, exposed so that they can be built directly.
type (
	// Config contains all application configuration groups.
	Config = bootstrap.Config
	// AppConfig contains process-level application settings.
	AppConfig = bootstrap.AppConfig
	// LogConfig contains structured logging settings.
	LogConfig = bootstrap.LogConfig
	// DatabaseConfig contains settings shared by the supported database drivers.
	DatabaseConfig = bootstrap.DatabaseConfig
	// SecurityConfig contains encryption, token, and application credential settings.
	SecurityConfig = bootstrap.SecurityConfig
	// WorkersConfig controls background task activation and scheduling.
	WorkersConfig = bootstrap.WorkersConfig
	// FeaturesConfig controls optional application behavior.
	FeaturesConfig = bootstrap.FeaturesConfig
)

// DefaultConfig returns Momobase's own configuration: a development baseline of
// plain values that a host copies and edits. New uses it when no configuration is
// supplied through WithConfig.
//
// Momobase reads no environment variables. A host that configures from the
// environment, a file, or a secret manager reads it itself and assigns the fields:
//
//	cfg := momobase.DefaultConfig()
//	cfg.App.Env = "production"
//	cfg.App.Addr = os.Getenv("PORT")
//	cfg.Security.AdminOAuthSecret = os.Getenv("ADMIN_OAUTH_SECRET")
//
// The placeholder credentials in the returned configuration are rejected by
// Config.Validate when App.Env is staging or production.
func DefaultConfig() Config {
	return bootstrap.DefaultConfig()
}

// The placeholder credentials DefaultConfig carries, exported so that a host can
// assert it replaced them. Config.Validate rejects all three when App.Env is
// staging or production.
const (
	// DefaultEncryptionMasterKeyBase64 is the all-zero development AES key.
	DefaultEncryptionMasterKeyBase64 = bootstrap.DefaultEncryptionMasterKeyBase64
	// DefaultAdminOAuthSecret is the development administrator token secret.
	DefaultAdminOAuthSecret = bootstrap.DefaultAdminOAuthSecret
	// DefaultAppOAuthSecret is the development application token secret.
	DefaultAppOAuthSecret = bootstrap.DefaultAppOAuthSecret
)

// Option customizes the instance constructed by New.
type Option func(*options)

type options struct {
	config    *Config
	mutators  []func(*Config)
	logger    *slog.Logger
	factories map[string]providers.Factory
}

// WithConfig uses cfg instead of DefaultConfig.
func WithConfig(cfg Config) Option {
	return func(o *options) { o.config = &cfg }
}

// WithConfigFunc applies fn to the resolved configuration before the instance is
// built. Functions run in the order supplied, after any WithConfig value.
func WithConfigFunc(fn func(*Config)) Option {
	return func(o *options) { o.mutators = append(o.mutators, fn) }
}

// WithAddr overrides the address the HTTP server listens on, such as ":9090".
func WithAddr(addr string) Option {
	return WithConfigFunc(func(cfg *Config) { cfg.App.Addr = addr })
}

// WithLogger uses log instead of a logger derived from the configured log level.
func WithLogger(log *slog.Logger) Option {
	return func(o *options) { o.logger = log }
}

// WithProvider registers a payment provider under code, replacing any provider
// previously registered under the same code. The code is the value operators
// select when creating a provider account through the Admin API.
//
// Momobase registers no providers of its own, so at least one is required. The
// reference adapter in providers/dummy is registered the same way as any other:
//
//	momobase.New(momobase.WithProvider("acme_pay", acme.New))
func WithProvider(code string, factory providers.Factory) Option {
	return func(o *options) {
		if o.factories == nil {
			o.factories = map[string]providers.Factory{}
		}
		o.factories[code] = factory
	}
}

// WithProviders registers each payment provider in factories by its code.
func WithProviders(factories map[string]providers.Factory) Option {
	return func(o *options) {
		if o.factories == nil {
			o.factories = make(map[string]providers.Factory, len(factories))
		}
		for code, factory := range factories {
			o.factories[code] = factory
		}
	}
}

func providerRegistry(factories map[string]providers.Factory) (providers.Registry, error) {
	if len(factories) == 0 {
		return nil, errors.New("no payment providers registered: supply at least one with WithProvider")
	}
	registry := providers.NewRegistry()
	for code, factory := range factories {
		if strings.TrimSpace(code) == "" {
			return nil, errors.New("provider code is required")
		}
		if factory == nil {
			return nil, fmt.Errorf("provider factory is required: %s", code)
		}
		registry.Register(code, factory)
	}
	return registry, nil
}

// Instance is a configured Momobase server and the runtime dependencies it owns.
type Instance struct {
	app *bootstrap.App
}

// New builds an instance from the supplied options, opening the database and
// preparing the HTTP server, providers, and background workers. Configuration is
// DefaultConfig unless WithConfig is supplied.
// The caller owns the returned instance and must Close it to release its connections.
func New(opts ...Option) (*Instance, error) {
	o := &options{}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	cfg := DefaultConfig()
	if o.config != nil {
		cfg = *o.config
	}
	for _, mutate := range o.mutators {
		mutate(&cfg)
	}
	registry, err := providerRegistry(o.factories)
	if err != nil {
		return nil, err
	}
	app, err := bootstrap.NewApp(cfg, o.logger, registry)
	if err != nil {
		return nil, err
	}
	return &Instance{app: app}, nil
}

// Serve starts the provider runtimes, background workers, and HTTP server, and
// blocks until ctx is cancelled or the server stops. Shutting down because ctx
// was cancelled is not reported as an error. An instance may only be served once.
func (i *Instance) Serve(ctx context.Context) error {
	if err := i.app.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// Run serves the instance until the process receives an interrupt or
// termination signal, then shuts it down gracefully.
func (i *Instance) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return i.Serve(ctx)
}

// Close stops an active server and its workers before closing the database connection
// pool. It is safe to call Close more than once.
func (i *Instance) Close() error {
	return i.app.Close()
}

// Migrate applies pending schema migrations and converges the schema with the
// current models. It runs automatically during New unless the AutoMigrate
// feature is disabled, and is safe to call more than once.
func (i *Instance) Migrate(ctx context.Context) error {
	return bootstrap.Migrate(ctx, i.app.DB, i.app.Logger)
}

// SeedAdmin creates a super administrator with the supplied credentials.
func (i *Instance) SeedAdmin(ctx context.Context, email, password, name string) error {
	return i.app.SeedAdmin(ctx, email, password, name)
}

// App returns the Fiber application, which may be mounted in an existing Fiber app,
// exercised with App().Test, or extended with additional routes instead of calling
// Serve or Run.
//
// Momobase runs on fasthttp rather than net/http, so this is a *fiber.App and not an
// http.Handler; an application serving Momobase alongside net/http routes has to
// adapt at its own boundary.
func (i *Instance) App() *fiber.App {
	return i.app.Fiber
}

// Addr returns the address the HTTP server listens on. A configured port of 0 reads
// back as the port the kernel chose once Serve has bound the listener.
func (i *Instance) Addr() string {
	return i.app.ListenAddr()
}

// DB returns the instance's database handle.
func (i *Instance) DB() *gorm.DB {
	return i.app.DB
}

// Logger returns the instance's structured logger.
func (i *Instance) Logger() *slog.Logger {
	return i.app.Logger
}

// OnPaymentRequest returns the blocking hook invoked for each normalized new
// payment request before routing and persistence. Idempotent replays skip it.
func (i *Instance) OnPaymentRequest() *hooks.Hook[hooks.PaymentRequestEvent] {
	return i.app.Hooks.OnPaymentRequest()
}

// OnTransactionChanged returns the post-commit hook invoked when a transaction
// status is persisted from the request, webhook, or reconciliation path.
func (i *Instance) OnTransactionChanged() *hooks.Hook[hooks.TransactionChangedEvent] {
	return i.app.Hooks.OnTransactionChanged()
}
