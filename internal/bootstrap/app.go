package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/cache"
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
)

// App owns the service's runtime dependencies and their lifecycle.
type App struct {
	Logger     *slog.Logger
	DB         *gorm.DB
	Cache      cache.Store
	Runtime    *provider.RuntimeManager
	Workers    *workers.Manager
	Fiber      *fiber.App
	Addr       string
	AdminUsers *identity.AdminUserService

	lifecycleMu sync.Mutex
	serveCancel context.CancelFunc
	serveDone   chan struct{}
	closed      bool
	closeOnce   sync.Once
	closeErr    error
	redisClient *redis.Client
}

// NewApp validates cfg and constructs the application and all owned runtime
// dependencies. Options customize the logger and the set of payment providers
// available to the application.
func NewApp(cfg Config, opts ...Option) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	o := newOptions(opts)
	registry, err := o.buildRegistry()
	if err != nil {
		return nil, err
	}
	log := o.logger
	if log == nil {
		log = newLogger(cfg.Log.Level)
	}
	if cfg.Cache.TTL <= 0 {
		cfg.Cache.TTL = defaultCacheTTL
	}
	redisClient := newRedisClient(cfg)
	cacheStore := cache.NewRedisStore(redisClient, cfg.Cache.TTL, log)
	cacheOwned := true
	defer func() {
		if cacheOwned {
			if closeErr := redisClient.Close(); closeErr != nil {
				log.Error("close Redis client after startup failure", slog.String("error", closeErr.Error()))
			}
		}
	}()
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

	runtime := provider.NewRuntimeManager(repos, registry, enc, log)

	audit := audit.New(repos, log)

	// Seeded before anything can authenticate: the catalogue and the system roles are
	// what every authorization check resolves against, so a boot that skipped this
	// would authorize nothing.
	authz := identity.NewAuthzService(repos, audit, cacheStore)
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
	apps := identity.NewAppService(repos, appAuth, audit, cacheStore)
	providerAdmin := provider.NewAdminService(repos, audit, enc, registry, runtime)
	routeAdmin := routing.NewAdminService(repos, audit, cacheStore)
	routeEngine := routing.NewEngine(repos, runtime, cacheStore)
	payments := payment.NewOrchestrator(repos, routeEngine, provider.NewExecutor(runtime), cacheStore)
	webhooks := webhook.New(repos, runtime)
	health := provider.NewHealthService(repos, runtime)
	recon := reconciliation.New(repos, runtime, webhooks, log)
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
		Analytics: identity.NewAnalyticsService(repos, cacheStore),
		System:    info,
	})
	// Parsed here rather than in the router so a malformed CIDR fails at start-up with
	// a clear error instead of silently disabling forwarded-header trust.
	router := httpx.NewRouter(httpx.RouterDeps{
		Logger:             log,
		AdminAuth:          adminAuth,
		AppAuth:            appAuth,
		DashboardEnabled:   cfg.Features.DashboardEnabled,
		CORSAllowedOrigins: cfg.App.CORSAllowedOrigins,
		TrustedProxyCIDRs:  cfg.App.TrustedProxyCIDRs,
		Public:             publicHandler,
		Admin:              adminHandler,
		Webhooks:           webhookh.NewHandler(webhooks),
	})

	app := &App{
		Logger:      log,
		DB:          db,
		Cache:       cacheStore,
		Runtime:     runtime,
		Workers:     manager,
		Fiber:       router,
		Addr:        cfg.App.Addr,
		AdminUsers:  adminUsers,
		redisClient: redisClient,
	}
	cacheOwned = false
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
		a.Logger.Error("load active providers", slog.String("error", err.Error()))
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

// Close stops an active server and its workers before closing the shared Redis
// client and database connection pool. It is safe to call Close more than once.
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
		a.closeErr = errors.Join(
			closeRedis(a.redisClient),
			closeDatabase(a.DB),
		)
	})
	return a.closeErr
}

func closeRedis(client *redis.Client) error {
	if client == nil {
		return nil
	}
	if err := client.Close(); err != nil {
		return fmt.Errorf("close Redis client: %w", err)
	}
	return nil
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
