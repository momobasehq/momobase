package hooks_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/momobasehq/momobase/hooks"
)

func TestHookRunsInOrderAndUnbinds(t *testing.T) {
	hook := hooks.Hook[int]{}
	calls := []int{}
	unbind := hook.Bind(func(_ context.Context, event int) error {
		calls = append(calls, event)
		return nil
	})
	hook.Bind(func(context.Context, int) error {
		calls = append(calls, 2)
		return errors.New("stop")
	})
	hook.Bind(func(context.Context, int) error {
		calls = append(calls, 3)
		return nil
	})

	if err := hook.Trigger(context.Background(), 1); err == nil {
		t.Fatal("Trigger() error = nil, want the handler failure")
	}
	if len(calls) != 2 || calls[0] != 1 || calls[1] != 2 {
		t.Fatalf("Trigger() calls = %v, want [1 2]", calls)
	}

	unbind()
	unbind()
	calls = calls[:0]
	if err := hook.Trigger(context.Background(), 1); err == nil {
		t.Fatal("Trigger() after unbind error = nil, want the remaining handler failure")
	}
	if len(calls) != 1 || calls[0] != 2 {
		t.Fatalf("Trigger() calls after unbind = %v, want [2]", calls)
	}
}

func TestHookConcurrentBindingAndTriggering(t *testing.T) {
	hook := hooks.Hook[int]{}
	var calls atomic.Int64
	hook.Bind(func(context.Context, int) error {
		calls.Add(1)
		return nil
	})

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unbind := hook.Bind(func(context.Context, int) error {
				calls.Add(1)
				return nil
			})
			if err := hook.Trigger(context.Background(), 1); err != nil {
				t.Errorf("Trigger() error = %v", err)
			}
			unbind()
		}()
	}
	wg.Wait()
	if calls.Load() == 0 {
		t.Fatal("Trigger() did not invoke any handler")
	}
}

func TestTransactionObserversContinueAfterAnError(t *testing.T) {
	registry := hooks.NewRegistry(nil)
	calls := 0
	registry.OnTransactionChanged().Bind(func(context.Context, hooks.TransactionChangedEvent) error {
		calls++
		return errors.New("first observer failed")
	})
	registry.OnTransactionChanged().Bind(func(context.Context, hooks.TransactionChangedEvent) error {
		calls++
		return nil
	})

	registry.NotifyTransactionChanged(context.Background(), hooks.TransactionChangedEvent{})
	if calls != 2 {
		t.Fatalf("transaction observer calls = %d, want 2", calls)
	}
}
