package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	httpx "github.com/momobasehq/momobase/internal/http"
	adminh "github.com/momobasehq/momobase/internal/http/admin"
	publich "github.com/momobasehq/momobase/internal/http/public"
	webhookh "github.com/momobasehq/momobase/internal/http/webhooks"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
	"github.com/momobasehq/momobase/internal/workers"
)

// App owns the service's runtime dependencies and their lifecycle.
type App struct {
	Logger     *slog.Logger
	DB         *gorm.DB
	Runtime    *services.ProviderRuntimeManager
	Workers    *workers.Manager
	Server     *http.Server
	AdminUsers *services.AdminUserService

	lifecycleMu sync.Mutex
	serveCancel context.CancelFunc
	serveDone   chan struct{}
	closed      bool
	closeOnce   sync.Once
	closeErr    error
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
		log = NewLogger(cfg.Log.Level)
	}
	db, err := OpenDatabase(cfg)
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
		WarnPendingMigrations(context.Background(), db, log)
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

	runtime := services.NewProviderRuntimeManager(db, registry, enc, log)

	audit := services.NewAuditService(db, log)

	adminAuth := services.NewAdminAuthService(
		db,
		cfg.Security.AdminAccessTTL,
		cfg.Security.AdminRefreshTTL,
		audit,
		adminTokens,
	)
	adminUsers := services.NewAdminUserService(db, audit)
	appAuth := services.NewAppAuthService(
		db,
		cfg.Security.AppClientIDPrefix,
		cfg.Security.AppClientSecretPrefix,
		cfg.Security.AppAccessTTL,
		cfg.Security.AppRefreshTTL,
		appTokens,
	)
	apps := services.NewAppService(db, appAuth, audit)
	providerAdmin := services.NewProviderAdminService(db, audit, enc, registry, runtime)
	routeAdmin := services.NewRouteAdminService(db, audit)
	routeEngine := services.NewRouteEngine(db, runtime)
	payments := services.NewPaymentOrchestrator(db, routeEngine, services.NewProviderExecutor(runtime))
	webhooks := services.NewWebhookService(db, runtime)
	health := services.NewHealthService(db, runtime)
	recon := services.NewReconciliationService(db, runtime, webhooks, log)
	manager := workers.NewManager(log, workerTasks(cfg, db, health, recon)...)

	info := adminh.SystemInfo{
		AppName:        cfg.App.Name,
		AppEnv:         cfg.App.Env,
		DBType:         cfg.DB.Type,
		Addr:           cfg.App.Addr,
		WorkersEnabled: cfg.Workers.Enabled,
		WorkerNames:    manager.Names(),
	}

	publicHandler := publich.NewHandler(payments, routeEngine, db)
	adminHandler := adminh.NewHandler(
		db,
		adminAuth,
		adminUsers,
		providerAdmin,
		routeAdmin,
		apps,
		runtime,
		audit,
		info,
	)
	router := httpx.NewRouter(httpx.RouterDeps{
		Logger:             log,
		AdminAuth:          adminAuth,
		AppAuth:            appAuth,
		DashboardEnabled:   cfg.Features.DashboardEnabled,
		CORSAllowedOrigins: cfg.App.CORSAllowedOrigins,
		Public:             publicHandler,
		Admin:              adminHandler,
		Webhooks:           webhookh.NewHandler(webhooks),
	})

	app := &App{
		Logger:  log,
		DB:      db,
		Runtime: runtime,
		Workers: manager,
		Server: &http.Server{
			Addr:              cfg.App.Addr,
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       65 * time.Second,
			WriteTimeout:      65 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		AdminUsers: adminUsers,
	}
	databaseOwned = false
	return app, nil
}

func workerTasks(c Config, db *gorm.DB, health *services.HealthService, recon *services.ReconciliationService) []workers.Task {
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
		tasks = append(tasks, workers.Task{Name: "cleanup", Interval: c.Workers.CleanupInterval, Run: func(ctx context.Context) error {
			now := time.Now().UTC()
			return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := tx.Where("expires_at < ?", now).Delete(&domain.AdminSession{}).Error; err != nil {
					return err
				}
				return tx.Where("expires_at < ?", now).Delete(&domain.AppSession{}).Error
			})
		}})
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
	go func() { errCh <- a.Server.ListenAndServe() }()
	a.Logger.Info("server starting", slog.String("addr", a.Server.Addr))
	select {
	case <-serveCtx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.Server.Shutdown(shutdown); err != nil {
			return err
		}
		return serveCtx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
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
