package middleware

import (
	"context"
	"net/http"
	"time"
)

// DefaultRequestTimeout caps a single request's total handler time.
const DefaultRequestTimeout = 30 * time.Second

// RequestTimeout attaches a context.WithTimeout to each request. Handlers
// observe cancellation via r.Context(); bridges that honor the context
// (including EventKit calls that thread one through) abort early. If the
// handler does not return before the deadline, the client gets a 504.
//
// This is per-request and distinct from the coarse http.Server R/W
// timeouts set in the binary main: it bounds *handler logic*, not the
// transport.
func RequestTimeout(d time.Duration) func(http.Handler) http.Handler {
	if d <= 0 {
		d = DefaultRequestTimeout
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			done := make(chan struct{})
			panicCh := make(chan any, 1)
			go func() {
				defer func() {
					if v := recover(); v != nil {
						panicCh <- v
					}
					close(done)
				}()
				next.ServeHTTP(w, r.WithContext(ctx))
			}()

			select {
			case <-done:
				select {
				case v := <-panicCh:
					// Re-raise on the original goroutine so an outer
					// Recover middleware can handle it.
					panic(v)
				default:
				}
				return
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					// Handler is still running on the goroutine; we cannot
					// preempt it. Best we can do is return 504 to the
					// client; the handler's eventual write will be a no-op
					// because http.ResponseWriter is not safe after
					// timeout but we've already committed a status.
					tryWriteStatus(w, http.StatusGatewayTimeout)
				}
				// We deliberately do not wait for the handler — it might
				// hang forever. The goroutine will leak until the bridge
				// honors the context, which is a known trade-off documented
				// in the PRD.
			}
		})
	}
}
