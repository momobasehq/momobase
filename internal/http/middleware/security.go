package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/momobasehq/momobase/internal/platform"
)

// MaxBodyBytes limits the number of bytes that downstream handlers can read
// from a request body.
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

// JSONOnly rejects requests whose Content-Type is not application/json.
func JSONOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			platform.Error(w, 415, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NoCache adds a Cache-Control no-store header to responses.
func NoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// Recover converts panics from downstream handlers into internal-server-error
// responses and logs the recovered value when a logger is supplied.
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

// RateLimitByIP permits at most limit requests from each client during each window
// and responds with too many requests after the limit is exceeded.
//
// The resolver is a parameter rather than package state so the untrusted default is
// explicit at every call site: pass RemoteClientIP to key on the immediate peer, or a
// resolver from NewForwardedClientIP where a configured proxy sits in front.
func RateLimitByIP(limit int, window time.Duration, ip ClientIP) func(http.Handler) http.Handler {
	if ip == nil {
		ip = RemoteClientIP
	}
	var mu sync.Mutex
	buckets := map[string]bucket{}
	var checks uint64
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now, client := time.Now(), ip(r)
			mu.Lock()
			current := buckets[client]
			if now.After(current.reset) {
				current = bucket{reset: now.Add(window)}
			}
			current.count++
			buckets[client] = current
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
