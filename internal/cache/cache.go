// Package cache provides a shared byte-oriented cache and typed JSON helpers.
package cache

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrMiss indicates that a key is not present in the cache.
var ErrMiss = errors.New("cache miss")

// Get retrieves key from store and decodes its JSON representation into T. A miss
// or cache failure returns nil; failures are reported by the shared store.
func Get[T any](ctx context.Context, store *RedisStore, key string) *T {
	if store == nil {
		return nil
	}
	data, err := store.Get(ctx, key)
	if err != nil {
		if !errors.Is(err, ErrMiss) {
			store.reportError(ctx, "get", key, err)
		}
		return nil
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		store.reportError(ctx, "decode", key, err)
		return nil
	}
	return &value
}

// Set encodes value as JSON and stores it under key. The shared store owns the TTL
// and reports failures, so services do not need cache-specific error handling.
func Set[T any](ctx context.Context, store *RedisStore, key string, value T) {
	if store == nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		store.reportError(ctx, "encode", key, err)
		return
	}
	if err := store.Set(ctx, key, data); err != nil {
		store.reportError(ctx, "set", key, err)
	}
}
