package cache_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/momobasehq/momobase/internal/cache"
)

type cachedUser struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func newStore(t *testing.T, logger *slog.Logger) (*cache.RedisStore, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close Redis client: %v", err)
		}
	})
	return cache.NewRedisStore(client, time.Minute, logger), server
}

func TestTypedHelpersRoundTripValues(t *testing.T) {
	store, _ := newStore(t, nil)
	ctx := context.Background()
	want := []cachedUser{{ID: 7, Name: "Ada"}, {ID: 9, Name: "Grace"}}

	cache.Set(ctx, store, "users:v1:featured", want)
	got := cache.Get[[]cachedUser](ctx, store, "users:v1:featured")
	if got == nil {
		t.Fatal("Get() = nil, want cached users")
	}
	if len(*got) != len(want) || (*got)[0] != want[0] || (*got)[1] != want[1] {
		t.Errorf("Get() = %+v, want %+v", got, want)
	}
}

func TestRedisStoreMissAndSharedTTL(t *testing.T) {
	store, server := newStore(t, nil)
	ctx := context.Background()

	if _, err := store.Get(ctx, "user:v1:missing"); !errors.Is(err, cache.ErrMiss) {
		t.Fatalf("Get(missing) error = %v, want ErrMiss", err)
	}
	if err := store.Set(ctx, "user:v1:7", []byte(`{"id":7}`)); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	server.FastForward(time.Minute)
	if _, err := store.Get(ctx, "user:v1:7"); !errors.Is(err, cache.ErrMiss) {
		t.Fatalf("Get(expired) error = %v, want ErrMiss", err)
	}
}

func TestGetLogsInvalidJSONAndReturnsNil(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	store, _ := newStore(t, logger)
	ctx := context.Background()
	if err := store.Set(ctx, "user:v1:broken", []byte(`{"id":`)); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if got := cache.Get[cachedUser](ctx, store, "user:v1:broken"); got != nil {
		t.Fatalf("Get() = %+v, want nil", got)
	}
	if !strings.Contains(logs.String(), "operation=decode") {
		t.Fatalf("cache log = %q, want decode operation", logs.String())
	}
}

func TestSetLogsMarshalError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	store, _ := newStore(t, logger)

	cache.Set(context.Background(), store, "unsupported:v1", make(chan int))
	if !strings.Contains(logs.String(), "operation=encode") {
		t.Fatalf("cache log = %q, want encode operation", logs.String())
	}
}
