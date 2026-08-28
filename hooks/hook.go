// Package hooks provides typed extension points for Momobase lifecycle events.
package hooks

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// Handler handles one typed hook event. The hook owner decides whether an error
// stops the invocation or is collected while the remaining handlers continue.
type Handler[T any] func(context.Context, T) error

type registeredHandler[T any] struct {
	id uint64
	fn Handler[T]
}

// Hook stores typed handlers and invokes them in registration order. Its zero value
// is ready to use and binding or unbinding is safe while another goroutine triggers it.
type Hook[T any] struct {
	mu       sync.RWMutex
	nextID   uint64
	handlers []registeredHandler[T]
}

// Bind appends fn and returns a function that removes it. Removal is idempotent and
// does not interrupt an invocation that already took its handler snapshot. It panics
// when fn is nil because a nil extension handler is a programming error.
func (h *Hook[T]) Bind(fn Handler[T]) func() {
	if fn == nil {
		panic("hooks: cannot bind a nil handler")
	}

	h.mu.Lock()
	h.nextID++
	id := h.nextID
	h.handlers = append(h.handlers, registeredHandler[T]{id: id, fn: fn})
	h.mu.Unlock()

	return sync.OnceFunc(func() { h.unbind(id) })
}

// Trigger invokes a snapshot of the registered handlers in order and stops at the
// first error. A handler bound during this call starts with the next invocation; one
// unbound during this call may finish the current invocation.
func (h *Hook[T]) Trigger(ctx context.Context, event T) error {
	for _, handler := range h.snapshot() {
		if err := handler.fn(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (h *Hook[T]) triggerAll(ctx context.Context, event T) error {
	errs := []error{}
	for _, handler := range h.snapshot() {
		if err := handler.fn(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *Hook[T]) snapshot() []registeredHandler[T] {
	h.mu.RLock()
	handlers := slices.Clone(h.handlers)
	h.mu.RUnlock()
	return handlers
}

func (h *Hook[T]) unbind(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, handler := range h.handlers {
		if handler.id == id {
			h.handlers = slices.Delete(h.handlers, i, i+1)
			return
		}
	}
}
