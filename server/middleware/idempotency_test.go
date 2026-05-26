package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIdempotency_ReplaysSameKey(t *testing.T) {
	calls := 0
	h := Idempotency(time.Hour, 16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"x":1}`))
		req.Header.Set(IdempotencyHeader, "same-key")
		h.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("iteration %d: got %d, want 201", i, w.Code)
		}
		if i > 0 && w.Header().Get(IdempotencyReplayedHeader) != "true" {
			t.Errorf("iteration %d: expected %s=true header", i, IdempotencyReplayedHeader)
		}
	}
	if calls != 1 {
		t.Errorf("handler invoked %d times, want 1", calls)
	}
}

func TestIdempotency_DifferentKeysIndependent(t *testing.T) {
	calls := 0
	h := Idempotency(time.Hour, 16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusCreated)
	}))
	for _, key := range []string{"a", "b", "c"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events", nil)
		req.Header.Set(IdempotencyHeader, key)
		h.ServeHTTP(w, req)
	}
	if calls != 3 {
		t.Errorf("handler invoked %d times, want 3", calls)
	}
}

func TestIdempotency_NoKeyBypasses(t *testing.T) {
	calls := 0
	h := Idempotency(time.Hour, 16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusCreated)
	}))
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/events", nil)
		// no Idempotency-Key
		h.ServeHTTP(w, req)
	}
	if calls != 3 {
		t.Errorf("handler invoked %d times, want 3 (no idempotency for keyless requests)", calls)
	}
}

func TestIdempotency_GetUnaffected(t *testing.T) {
	calls := 0
	h := Idempotency(time.Hour, 16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/events", nil)
		req.Header.Set(IdempotencyHeader, "same-key")
		h.ServeHTTP(w, req)
	}
	if calls != 3 {
		t.Errorf("handler invoked %d times, want 3 (GET shouldn't be cached)", calls)
	}
}

func TestIdempotency_ErrorsNotCached(t *testing.T) {
	calls := 0
	h := Idempotency(time.Hour, 16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set(IdempotencyHeader, "k")
		h.ServeHTTP(w, req)
	}
	if calls != 3 {
		t.Errorf("handler invoked %d times, want 3 (5xx must not be cached)", calls)
	}
}

func TestIdempotency_Expiry(t *testing.T) {
	calls := 0
	h := Idempotency(10*time.Millisecond, 16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(IdempotencyHeader, "k")
	h.ServeHTTP(httptest.NewRecorder(), req)
	time.Sleep(15 * time.Millisecond)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if calls != 2 {
		t.Errorf("handler invoked %d times, want 2 (expired entry should re-execute)", calls)
	}
}

func TestIdempotency_DisabledWhenCapZero(t *testing.T) {
	calls := 0
	h := Idempotency(time.Hour, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusCreated)
	}))
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set(IdempotencyHeader, "k")
		h.ServeHTTP(w, req)
	}
	if calls != 3 {
		t.Errorf("disabled middleware should not cache; calls = %d", calls)
	}
}
