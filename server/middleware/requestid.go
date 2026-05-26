package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
)

// RequestIDHeader is the HTTP header carrying the per-request correlation
// id. RequestID middleware honors an incoming value if it looks safe;
// otherwise it generates a fresh one.
const RequestIDHeader = "X-Request-ID"

type ctxKey struct{ name string }

// requestIDKey is the context key under which the request ID is stored.
// Use [RequestIDFromContext] to retrieve.
var requestIDKey = ctxKey{name: "request-id"}

// validIncomingID restricts accepted incoming IDs to safe characters.
// Anything that does not match is replaced with a freshly generated id.
var validIncomingID = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,64}$`)

// RequestID assigns each request a stable correlation id, exposed as the
// X-Request-ID response header and stored in the request context.
// An incoming X-Request-ID is preserved when it matches a conservative
// regex; otherwise a fresh 128-bit hex id is generated.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ""
		if got := r.Header.Get(RequestIDHeader); got != "" && validIncomingID.MatchString(got) {
			id = got
		}
		if id == "" {
			id = newRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the per-request id, or empty if none was
// set (e.g. when the middleware is not installed).
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should not fail on darwin/linux. If it does, fall
		// back to a deterministic-but-distinct value rather than panicking.
		return "0000000000000000-rand-error"
	}
	return hex.EncodeToString(b[:])
}
