package workers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagerRunsImmediatelyRepeatsAndStops(t *testing.T) {
	var calls atomic.Int32
	ran := make(chan struct{}, 4)
	manager := NewManager(nil, Task{
		Name:     "health",
		Interval: 10 * time.Millisecond,
		Run: func(context.Context) error {
			calls.Add(1)
			ran <- struct{}{}
			return nil
		},
	})
	names := manager.Names()
	if len(names) != 1 || names[0] != "health" {
		t.Fatalf("Names() = %v", names)
	}
	names[0] = "changed"
	if manager.Names()[0] != "health" {
		t.Fatal("Names() exposed the manager's internal task slice")
	}

	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)
	for range 2 {
		select {
		case <-ran:
		case <-time.After(time.Second):
			t.Fatal("worker did not run")
		}
	}
	cancel()
	done := make(chan struct{})
	go func() {
		manager.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Wait() did not return after cancellation")
	}
	if calls.Load() < 2 {
		t.Fatalf("worker ran %d times", calls.Load())
	}
}

func TestManagerLogsTaskErrorsButNotCancellation(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))

	ctx, cancel := context.WithCancel(context.Background())
	manager := NewManager(logger,
		Task{
			Name:     "failing",
			Interval: time.Hour,
			Run: func(context.Context) error {
				cancel()
				return errors.New("provider unavailable")
			},
		},
		Task{
			Name:     "cancelled",
			Interval: time.Hour,
			Run: func(context.Context) error {
				return context.Canceled
			},
		},
	)
	manager.Start(ctx)
	manager.Wait()
	log := output.String()
	if !strings.Contains(log, "worker iteration failed") || !strings.Contains(log, "failing") || !strings.Contains(log, "provider unavailable") {
		t.Fatalf("worker error log = %s", log)
	}
	if strings.Contains(log, "cancelled") {
		t.Fatalf("context cancellation was logged as a worker failure: %s", log)
	}
}
