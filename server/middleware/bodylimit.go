package middleware

import "net/http"

// DefaultBodyLimit is the per-request cap on the request body.
const DefaultBodyLimit int64 = 1 << 20 // 1 MiB

// BodyLimit wraps r.Body in [http.MaxBytesReader] so handlers cannot be
// forced to allocate unbounded memory by an over-sized request body.
func BodyLimit(limit int64) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = DefaultBodyLimit
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
