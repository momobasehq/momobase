package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore stores cache values in Redis.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
	logger *slog.Logger
}

// NewRedisStore creates a byte-oriented store backed by client. ttl applies to
// every value written through the store, and logger receives all cache failures.
func NewRedisStore(client *redis.Client, ttl time.Duration, logger *slog.Logger) *RedisStore {
	return &RedisStore{client: client, ttl: ttl, logger: logger}
}

// Get retrieves the bytes stored at key.
func (r *RedisStore) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrMiss
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Set stores value at key using the shared TTL.
func (r *RedisStore) Set(ctx context.Context, key string, value []byte) error {
	return r.client.Set(ctx, key, value, r.ttl).Err()
}

func (r *RedisStore) reportError(ctx context.Context, operation, key string, err error) {
	if r.logger == nil {
		return
	}
	r.logger.WarnContext(
		ctx,
		"cache operation failed",
		slog.String("operation", operation),
		slog.String("key", key),
		slog.String("error", err.Error()),
	)
}

var _ Store = (*RedisStore)(nil)
