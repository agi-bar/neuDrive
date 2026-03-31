package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiter implements a per-key token-bucket rate limiter without external
// dependencies. Each unique key (API key or client IP) gets its own bucket.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    int           // max requests per window
	window  time.Duration // refill window
	cleanup time.Duration // how often to purge stale entries
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

// NewRateLimiter creates a RateLimiter that allows rate requests per window.
// A background goroutine cleans up stale entries every 2*window.
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		window:  window,
		cleanup: 2 * window,
	}
	go rl.cleanupLoop()
	return rl
}

// cleanupLoop periodically removes buckets that haven't been used recently.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.cleanup)
		for key, b := range rl.buckets {
			if b.lastReset.Before(cutoff) {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

// allow checks whether the given key has remaining tokens. It refills the
// bucket if the current window has elapsed. Returns (remaining, resetTime, allowed).
func (rl *RateLimiter) allow(key string) (remaining int, resetTime time.Time, ok bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]
	if !exists || now.Sub(b.lastReset) >= rl.window {
		b = &bucket{
			tokens:    rl.rate,
			lastReset: now,
		}
		rl.buckets[key] = b
	}

	resetTime = b.lastReset.Add(rl.window)

	if b.tokens <= 0 {
		return 0, resetTime, false
	}

	b.tokens--
	return b.tokens, resetTime, true
}

// Middleware returns an http.Handler that enforces the rate limit. Requests are
// keyed by X-API-Key if present, otherwise by the remote IP address.
// On success it sets X-RateLimit-Limit, X-RateLimit-Remaining, and
// X-RateLimit-Reset headers. When the limit is exceeded it responds with 429
// and a Retry-After header.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" {
			key = r.RemoteAddr
		}

		remaining, resetTime, allowed := rl.allow(key)
		retryAfter := int(time.Until(resetTime).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.rate))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "rate limit exceeded",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
