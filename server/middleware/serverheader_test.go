package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripServerHeader_RemovesServerOnWriteHeader(t *testing.T) {
	h := StripServerHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Go-evil-server/1.0")
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("Server"); got != "" {
		t.Errorf("Server = %q, want empty", got)
	}
}

func TestStripServerHeader_RemovesServerOnImplicitWrite(t *testing.T) {
	h := StripServerHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Go-evil-server/1.0")
		_, _ = w.Write([]byte("hi"))
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("Server"); got != "" {
		t.Errorf("Server = %q, want empty", got)
	}
}
