package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRejectMethodOverride_NoHeaderPasses(t *testing.T) {
	called := false
	h := RejectMethodOverride(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !called {
		t.Errorf("handler not called")
	}
}

func TestRejectMethodOverride_EachHeaderRejected(t *testing.T) {
	for _, hdr := range methodOverrideHeaders {
		t.Run(hdr, func(t *testing.T) {
			called := false
			h := RejectMethodOverride(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
			}))
			r := httptest.NewRequest(http.MethodPost, "/x", nil)
			r.Header.Set(hdr, "DELETE")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if called {
				t.Errorf("handler should NOT have been called for header %q", hdr)
			}
		})
	}
}
