package providers

import (
	"sync"
	"time"
)

type TokenCache struct {
	mu      sync.Mutex
	value   string
	expires time.Time
}

func (c *TokenCache) Get(load func() (string, time.Duration, error)) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value != "" && time.Until(c.expires) > 30*time.Second {
		return c.value, nil
	}
	value, ttl, err := load()
	if err != nil {
		return "", err
	}
	if ttl < time.Minute {
		ttl = time.Minute
	}
	c.value, c.expires = value, time.Now().Add(ttl)
	return value, nil
}
