package middleware

import (
	"mime"
	"net/http"
	"strings"
)

// RequireContentType rejects state-changing requests (POST, PUT, PATCH)
// whose Content-Type does not match the expected media type. huma also
// validates Content-Type during body unmarshal, but that check fires only
// after the body has been read — failing earlier here makes the contract
// clearer and avoids partial-body reads.
//
// Requests without a body (Content-Length 0 and no Transfer-Encoding) are
// allowed through regardless: a POST that takes no body (e.g. the
// reminder complete endpoint) shouldn't be required to set Content-Type.
func RequireContentType(expected string) func(http.Handler) http.Handler {
	expected = strings.ToLower(strings.TrimSpace(expected))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
			default:
				next.ServeHTTP(w, r)
				return
			}
			if !hasRequestBody(r) {
				next.ServeHTTP(w, r)
				return
			}
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || strings.ToLower(mediaType) != expected {
				http.Error(w, "expected Content-Type: "+expected, http.StatusUnsupportedMediaType)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func hasRequestBody(r *http.Request) bool {
	if r.ContentLength > 0 {
		return true
	}
	if len(r.TransferEncoding) > 0 {
		return true
	}
	return false
}
