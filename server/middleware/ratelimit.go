package middleware

import (
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// RateLimit applies a single token-bucket rate limit across all requests
// flowing through the handler. The bucket fills at rps tokens per second
// up to burst tokens. Requests that would overflow get 429 + Retry-After.
//
// The limit is intentionally global rather than per-IP — this server is
// loopback-only with one client (the MCP wrapper). The point is to keep
// an agent in a runaway retry loop from exhausting iCloud sync quota,
// not to enforce per-tenant fairness.
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	if rps <= 0 || burst <= 0 {
		// Disabled — pass-through.
		return func(next http.Handler) http.Handler { return next }
	}
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res := limiter.Reserve()
			if !res.OK() {
				// Should not happen unless rps is infinity. Treat as
				// silently allowed for safety.
				next.ServeHTTP(w, r)
				return
			}
			delay := res.Delay()
			if delay == 0 {
				next.ServeHTTP(w, r)
				return
			}
			// We don't want to actually wait — fail fast so a hung agent
			// doesn't accumulate goroutines. Cancel the reservation so
			// other requests can use the slot.
			res.Cancel()
			retrySeconds := int(delay / time.Second)
			if retrySeconds < 1 {
				retrySeconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		})
	}
}
