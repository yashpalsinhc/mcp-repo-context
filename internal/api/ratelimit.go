package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
	lastAccess time.Time
}

type rateLimiter struct {
	buckets sync.Map
	rate    float64 // tokens per second
	burst   float64 // max tokens
}

func newRateLimiter(perMinute int) *rateLimiter {
	return &rateLimiter{
		rate:  float64(perMinute) / 60.0,
		burst: float64(perMinute),
	}
}

func (rl *rateLimiter) allow(ip string) (bool, time.Duration) {
	now := time.Now()

	val, _ := rl.buckets.LoadOrStore(ip, &tokenBucket{
		tokens:     rl.burst,
		lastRefill: now,
		lastAccess: now,
	})
	bucket := val.(*tokenBucket)

	// Refill tokens based on elapsed time
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	bucket.tokens += elapsed * rl.rate
	if bucket.tokens > rl.burst {
		bucket.tokens = rl.burst
	}
	bucket.lastRefill = now
	bucket.lastAccess = now

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true, 0
	}

	// Calculate retry-after
	deficit := 1.0 - bucket.tokens
	retryAfter := time.Duration(deficit/rl.rate*1000) * time.Millisecond
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter
}

func (rl *rateLimiter) cleanup(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-10 * time.Minute)
			rl.buckets.Range(func(key, value interface{}) bool {
				bucket := value.(*tokenBucket)
				if bucket.lastAccess.Before(cutoff) {
					rl.buckets.Delete(key)
				}
				return true
			})
		}
	}
}

// rateLimitMiddleware is applied to API routes.
func (s *APIServer) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		allowed, retryAfter := s.rateLimiter.allow(ip)
		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
			writeError(w, r, http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
