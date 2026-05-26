package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if seen == "" {
		t.Errorf("request id missing from context")
	}
	if w.Header().Get(RequestIDHeader) != seen {
		t.Errorf("response header %q ≠ context %q", w.Header().Get(RequestIDHeader), seen)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(seen) {
		t.Errorf("generated id %q is not a 32-char hex string", seen)
	}
}

func TestRequestID_EchoesValidIncoming(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set(RequestIDHeader, "trace-abc-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get(RequestIDHeader); got != "trace-abc-123" {
		t.Errorf("got %q, want trace-abc-123", got)
	}
}

func TestRequestID_RejectsUnsafeIncoming(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"contains_space", "abc def"},
		{"contains_lf", "abc\ndef"},
		{"too_long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, // 65 chars
		{"contains_slash", "abc/def"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			r.Header.Set(RequestIDHeader, tt.in)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			got := w.Header().Get(RequestIDHeader)
			if got == tt.in && tt.in != "" {
				t.Errorf("unsafe input %q was echoed", tt.in)
			}
			if got == "" {
				t.Errorf("no id assigned at all")
			}
		})
	}
}

func TestRequestIDFromContext_EmptyWhenAbsent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := RequestIDFromContext(r.Context()); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
