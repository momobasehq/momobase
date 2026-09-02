package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/momobasehq/momobase/hooks"
	"github.com/momobasehq/momobase/internal/domain"
	httpx "github.com/momobasehq/momobase/internal/http"
	adminh "github.com/momobasehq/momobase/internal/http/admin"
	publich "github.com/momobasehq/momobase/internal/http/public"
	webhookh "github.com/momobasehq/momobase/internal/http/webhooks"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/service/identity"
	"github.com/momobasehq/momobase/internal/service/payment"
	"github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/internal/service/reconciliation"
	"github.com/momobasehq/momobase/internal/service/routing"
	"github.com/momobasehq/momobase/internal/service/webhook"
	"github.com/momobasehq/momobase/internal/workers"
	providerapi "github.com/momobasehq/momobase/providers"
)

// App owns the service's runtime dependencies and their lifecycle.
type App struct {
	Logger     *slog.Logger
	DB         *gorm.DB
	Runtime    *provider.RuntimeManager
	Workers    *workers.Manager
	Fiber      *fiber.App
	Hooks      *hooks.Registry
	Addr       string
	AdminUsers *identity.AdminUserService

	lifecycleMu sync.Mutex
	serveCancel context.CancelFunc
	serveDone   chan struct{}
	closed      bool
	closeOnce   sync.Once
	closeErr    error
}

// NewApp validates cfg and constructs the application and all owned runtime dependencies.
func NewApp(cfg Config, log *slog.Logger, registry providerapi.Registry) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if log == nil {
		log = newLogger(cfg.Log.Level)
	}
	extensionHooks := hooks.NewRegistry(log)
	db, err := openDatabase(cfg)
	if err != nil {
		return nil, err
	}
	databaseOwned := true
	defer func() {
		if databaseOwned {
			_ = closeDatabase(db)
		}
	}()
	// NewApp has no context of its own: the schema is settled before the server
	// and its workers exist, so there is nothing yet to cancel the work from.
	if cfg.Features.AutoMigrate {
		if err = Migrate(context.Background(), db, log); err != nil {
			return nil, err
		}
	} else {
		warnPendingMigrations(context.Background(), db, log)
	}
	enc, err := platform.NewEncryptor(cfg.Security.EncryptionMasterKeyBase64)
	if err != nil {
		return nil, err
	}
	adminTokens, err := platform.NewTokenManager(cfg.Security.AdminOAuthSecret)
	if err != nil {
		return nil, err
	}
	appTokens, err := platform.NewTokenManager(cfg.Security.AppOAuthSecret)
	if err != nil {
		return nil, err
	}

	// One unit of work over the open handle: every service reaches the database only
	// through the repositories it hands out, and Within is the sole transaction boundary.
	repos := repository.New(db)
	unsupportedMethods, err := repos.PaymentRoutes.UnsupportedMethods(context.Background())
	if err != nil {
		return nil, err
	}
	if len(unsupportedMethods) > 0 {
		return nil, fmt.Errorf("payment routes use unsupported methods: %s", strings.Join(unsupportedMethods, ", "))
	}

	runtime := provider.NewRuntimeManager(repos, registry, enc, log)

	audit := audit.New(repos, log)

	// Seeded before anything can authenticate: the catalogue and the system roles are
	// what every authorization check resolves against, so a boot that skipped this
	// would authorize nothing.
	authz := identity.NewAuthzService(repos, audit)
	if err = authz.Seed(context.Background()); err != nil {
		return nil, err
	}
	adminAuth := identity.NewAdminAuthService(repos,
		cfg.Security.AdminAccessTTL,
		cfg.Security.AdminRefreshTTL,
		audit,
		adminTokens,
		authz,
	)
	adminUsers := identity.NewAdminUserService(repos, audit, authz)
	appAuth := identity.NewAppAuthService(repos,
		cfg.Security.AppClientIDPrefix,
		cfg.Security.AppClientSecretPrefix,
		cfg.Security.AppAccessTTL,
		cfg.Security.AppRefreshTTL,
		appTokens,
	)
	apps := identity.NewAppService(repos, appAuth, audit)
	providerAdmin := provider.NewAdminService(repos, audit, enc, registry, runtime)
	routeAdmin := routing.NewAdminService(repos, audit, runtime)
	routeEngine := routing.NewEngine(repos, runtime)
	payments := payment.NewOrchestrator(
		repos,
		routeEngine,
		provider.NewExecutor(runtime),
		extensionHooks,
	)
	webhooks := webhook.New(repos, runtime, extensionHooks)
	health := provider.NewHealthService(repos, runtime)
	recon := reconciliation.New(reconciliation.Deps{
		Repos:   repos,
		Runtime: runtime,
		Webhook: webhooks,
		Logger:  log,
		Hooks:   extensionHooks,
	})
	manager := workers.NewManager(log, workerTasks(cfg, repos, health, recon)...)

	info := adminh.SystemInfo{
		AppName:        cfg.App.Name,
		AppEnv:         cfg.App.Env,
		DBType:         cfg.DB.Type,
		Addr:           cfg.App.Addr,
		WorkersEnabled: cfg.Workers.Enabled,
		WorkerNames:    manager.Names(),
	}

	publicHandler := publich.NewHandler(payments, routeEngine, repos)
	adminHandler := adminh.NewHandler(adminh.Deps{
		Repos:     repos,
		Auth:      adminAuth,
		Users:     adminUsers,
		Providers: providerAdmin,
		Routes:    routeAdmin,
		Apps:      apps,
		Runtime:   runtime,
		Audit:     audit,
		Authz:     authz,
		Analytics: identity.NewAnalyticsService(repos),
		System:    info,
	})
	// Parsed here rather than in the router so a malformed CIDR fails at start-up with
	// a clear error instead of silently disabling forwarded-header trust.
	router := httpx.NewRouter(httpx.RouterDeps{
		Logger:             log,
		AdminAuth:          adminAuth,
		AppAuth:            appAuth,
		CORSAllowedOrigins: cfg.App.CORSAllowedOrigins,
		TrustedProxyCIDRs:  cfg.App.TrustedProxyCIDRs,
		Public:             publicHandler,
		Admin:              adminHandler,
		Webhooks:           webhookh.NewHandler(webhooks),
	})

	app := &App{
		Logger:     log,
		DB:         db,
		Runtime:    runtime,
		Workers:    manager,
		Fiber:      router,
		Hooks:      extensionHooks,
		Addr:       cfg.App.Addr,
		AdminUsers: adminUsers,
	}
	databaseOwned = false
	return app, nil
}

func workerTasks(
	c Config,
	repos *repository.UnitOfWork,
	health *provider.HealthService,
	recon *reconciliation.Service,
) []workers.Task {
	if !c.Workers.Enabled {
		return nil
	}
	tasks := make([]workers.Task, 0, 3)
	if c.Workers.HealthEnabled {
		tasks = append(tasks, workers.Task{
			Name:     "health",
			Interval: c.Workers.HealthInterval,
			Run:      health.CheckAll,
		})
	}
	if c.Workers.ReconciliationEnabled {
		tasks = append(tasks, workers.Task{
			Name:     "reconciliation",
			Interval: c.Workers.ReconciliationInterval,
			Run: func(ctx context.Context) error {
				return recon.RunOnce(ctx, 100)
			},
		})
	}
	if c.Workers.CleanupEnabled {
		tasks = append(tasks, workers.Task{
			Name:     "cleanup",
			Interval: c.Workers.CleanupInterval,
			Run: func(ctx context.Context) error {
				now := time.Now().UTC()
				return repos.Within(ctx, func(r *repository.Set) error {
					if _, err := r.AdminSessions.DeleteExpired(ctx, now); err != nil {
						return err
					}
					_, err := r.AppSessions.DeleteExpired(ctx, now)
					return err
				})
			},
		})
	}
	return tasks
}

// Serve starts the configured provider runtimes, background workers, and HTTP
// server, and shuts them down when ctx is cancelled.
func (a *App) Serve(ctx context.Context) error {
	a.lifecycleMu.Lock()
	if a.closed {
		a.lifecycleMu.Unlock()
		return errors.New("application is closed")
	}
	if a.serveDone != nil {
		a.lifecycleMu.Unlock()
		return errors.New("application has already been served")
	}
	serveCtx, cancelServe := context.WithCancel(ctx)
	serveDone := make(chan struct{})
	a.serveCancel = cancelServe
	a.serveDone = serveDone
	a.lifecycleMu.Unlock()

	defer func() {
		cancelServe()
		a.Workers.Wait()
		close(serveDone)
	}()

	if err := a.Runtime.LoadActive(serveCtx); err != nil {
		if serveCtx.Err() != nil {
			return serveCtx.Err()
		}
		return fmt.Errorf("load active providers: %w", err)
	}
	if serveCtx.Err() != nil {
		return serveCtx.Err()
	}
	a.Workers.Start(serveCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Fiber.Listen(a.Addr, fiber.ListenConfig{
			DisableStartupMessage: true,
			ListenerAddrFunc:      a.recordListenAddr,
		})
	}()
	a.Logger.Info("server starting", slog.String("addr", a.Addr))
	select {
	case <-serveCtx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Listen returns nil once the graceful shutdown completes, so the goroutine
		// above is drained by the caller rather than left holding the channel.
		if err := a.Fiber.ShutdownWithContext(shutdown); err != nil {
			return err
		}
		return serveCtx.Err()
	case err := <-errCh:
		return err
	}
}

// recordListenAddr stores the address the listener actually bound. A configured port
// of 0 asks the kernel to choose one, and an embedding application has no other way to
// learn which.
func (a *App) recordListenAddr(addr net.Addr) {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	a.Addr = addr.String()
}

// ListenAddr returns the address the server is bound to, which is the configured one
// until the listener resolves a port of 0.
func (a *App) ListenAddr() string {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	return a.Addr
}

// Close stops an active server and its workers before closing the database
// connection pool. It is safe to call Close more than once.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		a.lifecycleMu.Lock()
		a.closed = true
		cancelServe := a.serveCancel
		serveDone := a.serveDone
		a.lifecycleMu.Unlock()

		if cancelServe != nil {
			cancelServe()
		}
		if serveDone != nil {
			<-serveDone
		}
		a.closeErr = closeDatabase(a.DB)
	})
	return a.closeErr
}

func closeDatabase(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("access database pool: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close database pool: %w", err)
	}
	return nil
}

// SeedAdmin creates a super administrator using the application's admin user
// service.
func (a *App) SeedAdmin(ctx context.Context, email, password, name string) error {
	if email == "" || password == "" {
		return errors.New("email and password are required")
	}
	_, err := a.AdminUsers.Create(
		ctx,
		&domain.AdminUser{
			BaseModel: domain.BaseModel{ID: "system"},
			Role:      "super_admin",
		},
		name,
		email,
		password,
		"super_admin",
	)
	return err
}
