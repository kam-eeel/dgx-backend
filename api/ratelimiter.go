package api

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// rateLimiter is a per-client token bucket.
// each client holds up to `burst` tokens, refilled at `rate` tokens per second.
// one request costs one token.
type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*bucket
	rate    float64
	burst   float64
}

func newRateLimiter(ctx context.Context, rate float64, burst int) *rateLimiter {
	rl := &rateLimiter{
		clients: make(map[string]*bucket),
		rate:    rate,
		burst:   float64(burst),
	}
	go rl.cleanup(ctx)
	return rl
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.clients[key]
	if !ok {
		b = &bucket{tokens: rl.burst, last: now}
		rl.clients[key] = b
	}

	// refill based on time elapsed since last request, capped at burst
	b.tokens = min(rl.burst, b.tokens+float64(now.Sub(b.last).Seconds()*rl.rate))
	b.last = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *rateLimiter) cleanup(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rl.mu.Lock()
			for k, b := range rl.clients {
				if time.Since(b.last) > 3*time.Minute {
					delete(rl.clients, k)
				}
			}
			rl.mu.Unlock()
		}
	}
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
