package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoopback_AcceptsLoopback(t *testing.T) {
	called := false
	h := Loopback(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		remoteAddr string
		host       string
	}{
		{"ipv4_loopback", "127.0.0.1:54321", "127.0.0.1:8765"},
		{"ipv6_loopback", "[::1]:54321", "[::1]:8765"},
		{"localhost_host", "127.0.0.1:54321", "localhost:8765"},
		{"localhost_no_port", "127.0.0.1:54321", "localhost"},
		{"httptest_empty_remote", "", "localhost:8765"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			r.Host = tt.host
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200; body = %s", w.Code, w.Body.String())
			}
			if !called {
				t.Errorf("next handler not invoked")
			}
		})
	}
}

func TestLoopback_RejectsRemote(t *testing.T) {
	called := false
	h := Loopback(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	tests := []struct {
		name       string
		remoteAddr string
		host       string
	}{
		{"public_ipv4_remote", "8.8.8.8:54321", "localhost:8765"},
		{"public_ipv6_remote", "[2001:db8::1]:54321", "localhost:8765"},
		{"non_loopback_host", "127.0.0.1:54321", "example.com:8765"},
		{"empty_host", "127.0.0.1:54321", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			r.Host = tt.host
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", w.Code)
			}
			if called {
				t.Errorf("next handler should NOT have been invoked")
			}
		})
	}
}
