package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// policyDeniedKey is the context key set by the scoped layer when a
// request is rejected by the allowlist. The audit middleware reads it
// to emit a structured `policy_denied=true` field for SIEM correlation.
type policyDeniedKey struct{}

// MarkPolicyDenied stamps the request context so [Audit] emits an
// explicit policy_denied=true field. Call this from the scoped layer
// when returning ErrPolicyDenied — the handler never sees the context,
// but the middleware does (it wraps the whole chain).
func MarkPolicyDenied(ctx context.Context) {
	if v := ctx.Value(policyDeniedKey{}); v != nil {
		if p, ok := v.(*bool); ok {
			*p = true
		}
	}
}

// PolicyDeniedContext returns a context that the scoped layer can
// stamp via [MarkPolicyDenied]. The boolean pointer flips to true on
// denial; the audit middleware reads it after the handler returns.
func PolicyDeniedContext(parent context.Context) (context.Context, *bool) {
	denied := new(bool)
	return context.WithValue(parent, policyDeniedKey{}, denied), denied
}

// Audit logs every non-GET, non-HEAD request with method, path, query,
// status, elapsed, response size, target id (from chi URL params),
// request id, and an explicit policy_denied flag. Reads (GET, HEAD) are
// skipped to keep the log focused on state-changing operations.
func Audit(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}
			started := time.Now()
			ctx, denied := PolicyDeniedContext(r.Context())
			r = r.WithContext(ctx)
			ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)

			attrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", trimQuery(r.URL.RawQuery)),
				slog.Int("status", ww.status),
				slog.Int("response_bytes", ww.written),
				slog.Duration("elapsed", time.Since(started)),
				slog.String("request_id", RequestIDFromContext(r.Context())),
			}
			if id := chi.URLParam(r, "id"); id != "" {
				attrs = append(attrs, slog.String("target_id", id))
			}
			if *denied {
				attrs = append(attrs, slog.Bool("policy_denied", true))
			}
			logger.Info("audit", attrs...)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status  int
	written int
	wrote   bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	n, err := s.ResponseWriter.Write(b)
	s.written += n
	return n, err
}

func trimQuery(q string) string {
	const max = 256
	if len(q) <= max {
		return q
	}
	return q[:max] + strings.Repeat("…", 1)
}
