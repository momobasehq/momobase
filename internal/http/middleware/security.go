package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/momobasehq/momobase/internal/platform"
)

func MaxBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
func JSONOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			platform.Error(w, 415, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func NoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if value := recover(); value != nil {
					if log != nil {
						log.Error("http panic", "error", value)
					}
					platform.Error(w, 500, "SERVER_ERROR", "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type bucket struct {
	count int
	reset time.Time
}

func RateLimitByIP(limit int, window time.Duration) func(http.Handler) http.Handler {
	var mu sync.Mutex
	buckets := map[string]bucket{}
	var checks uint64
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now, ip := time.Now(), clientIP(r)
			mu.Lock()
			current := buckets[ip]
			if now.After(current.reset) {
				current = bucket{reset: now.Add(window)}
			}
			current.count++
			buckets[ip] = current
			checks++
			if checks%256 == 0 {
				for key, item := range buckets {
					if now.After(item.reset) {
						delete(buckets, key)
					}
				}
			}
			over := current.count > limit
			mu.Unlock()
			if over {
				platform.Error(w, 429, "RATE_LIMITED", "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
