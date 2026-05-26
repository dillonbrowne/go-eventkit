package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover wraps each request in defer/recover. A panicking handler turns
// into a 500 with no body; the panic value and stack trace go to the
// provided logger. Without this middleware a panic crashes the request
// goroutine and the connection is terminated abruptly.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					// http.ErrAbortHandler is the documented way for a
					// handler to abort without logging — honor it.
					if v == http.ErrAbortHandler {
						panic(v)
					}
					logger.Error("panic recovered",
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.Any("panic", v),
						slog.String("stack", string(debug.Stack())),
					)
					// Best-effort 500. If headers were already written we
					// cannot do anything more; the client sees a partial
					// response.
					tryWriteStatus(w, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func tryWriteStatus(w http.ResponseWriter, code int) {
	defer func() {
		// Writing headers after another write will panic; swallow.
		_ = recover()
	}()
	w.WriteHeader(code)
}
