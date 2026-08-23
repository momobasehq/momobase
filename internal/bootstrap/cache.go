package bootstrap

import (
	"crypto/tls"

	"github.com/redis/go-redis/v9"
)

func newRedisClient(cfg Config) *redis.Client {
	addr := cfg.Cache.Addr
	if addr == "" {
		addr = "localhost:6379"
	}
	options := &redis.Options{
		Addr:       addr,
		Username:   cfg.Cache.Username,
		Password:   cfg.Cache.Password,
		DB:         cfg.Cache.DB,
		ClientName: cfg.App.Name,
	}
	if cfg.Cache.TLS {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(options)
}
