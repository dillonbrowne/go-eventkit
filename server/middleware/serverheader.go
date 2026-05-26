package middleware

import "net/http"

// StripServerHeader removes the `Server` response header that Go's
// net/http sets by default. The header advertises the runtime/version,
// which is information the server has no reason to disclose.
func StripServerHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(stripWriter{w}, r)
	})
}

// stripWriter wraps http.ResponseWriter to delete the Server header right
// before headers are flushed.
type stripWriter struct{ http.ResponseWriter }

func (s stripWriter) WriteHeader(code int) {
	s.ResponseWriter.Header().Del("Server")
	s.ResponseWriter.WriteHeader(code)
}

func (s stripWriter) Write(b []byte) (int, error) {
	// If WriteHeader was never called, Write triggers an implicit 200
	// flush; delete Server before that happens.
	s.ResponseWriter.Header().Del("Server")
	return s.ResponseWriter.Write(b)
}
