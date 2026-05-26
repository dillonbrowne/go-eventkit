package middleware

import "net/http"

// methodOverrideHeaders are the headers some frameworks honor to remap
// the HTTP method (e.g. POST → DELETE). We do not honor them; this
// middleware rejects any request carrying one. Silently ignoring would
// mislead clients into thinking the request was processed under the
// override.
var methodOverrideHeaders = []string{
	"X-HTTP-Method-Override",
	"X-HTTP-Method",
	"X-Method-Override",
}

// RejectMethodOverride returns 400 if the request includes any of the
// method-override headers.
func RejectMethodOverride(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, h := range methodOverrideHeaders {
			if r.Header.Get(h) != "" {
				http.Error(w, "method override headers are not supported", http.StatusBadRequest)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
