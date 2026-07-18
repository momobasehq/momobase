package bootstrap

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/workers"
)

func TestAppCloseStopsWorkersAndClosesDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:app-close?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("access test database pool: %v", err)
	}

	started := make(chan struct{})
	stopped := make(chan struct{})
	manager := workers.NewManager(nil, workers.Task{
		Name:     "blocking",
		Interval: time.Hour,
		Run: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		},
	})
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	manager.Start(workerCtx)
	workersDone := make(chan struct{})
	go func() {
		manager.Wait()
		close(workersDone)
	}()

	app := &App{
		DB:          db,
		Workers:     manager,
		serveCancel: cancelWorkers,
		serveDone:   workersDone,
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	if err := app.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("Close() returned before worker stopped")
	}
	if err := app.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if err := sqlDB.Ping(); err == nil {
		t.Fatal("database Ping() succeeded after Close()")
	}
	if err := app.Serve(context.Background()); err == nil {
		t.Fatal("Serve() succeeded after Close()")
	}
}
