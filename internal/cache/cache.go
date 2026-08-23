// Package cache provides a shared byte-oriented cache and typed JSON helpers.
package cache

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrMiss indicates that a key is not present in the cache.
var ErrMiss = errors.New("cache miss")

// Store is the storage contract implemented by cache backends.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte) error
}

// Get retrieves key from store and decodes its JSON representation into T. A miss
// or cache failure returns nil; failures are reported by the shared store.
func Get[T any](ctx context.Context, store Store, key string) *T {
	if store == nil {
		return nil
	}
	data, err := store.Get(ctx, key)
	if err != nil {
		if !errors.Is(err, ErrMiss) {
			reportError(ctx, store, "get", key, err)
		}
		return nil
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		reportError(ctx, store, "decode", key, err)
		return nil
	}
	return &value
}

// Set encodes value as JSON and stores it under key. The shared store owns the TTL
// and reports failures, so services do not need cache-specific error handling.
func Set[T any](ctx context.Context, store Store, key string, value T) {
	if store == nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		reportError(ctx, store, "encode", key, err)
		return
	}
	if err := store.Set(ctx, key, data); err != nil {
		reportError(ctx, store, "set", key, err)
	}
}

type errorReporter interface {
	reportError(ctx context.Context, operation, key string, err error)
}

func reportError(ctx context.Context, store Store, operation, key string, err error) {
	reporter, ok := store.(errorReporter)
	if ok {
		reporter.reportError(ctx, operation, key, err)
	}
}
