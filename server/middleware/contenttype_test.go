package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newCTTestHandler() http.Handler {
	return RequireContentType("application/json")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestRequireContentType_GetUnaffected(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	newCTTestHandler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRequireContentType_EmptyBodyPostAllowed(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	w := httptest.NewRecorder()
	newCTTestHandler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no body so CT not required); body = %s", w.Code, w.Body.String())
	}
}

func TestRequireContentType_PostWithBodyMissingHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"x":1}`))
	w := httptest.NewRecorder()
	newCTTestHandler().ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", w.Code)
	}
}

func TestRequireContentType_PostWithJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"x":1}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newCTTestHandler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRequireContentType_PostWithJSONCharset(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"x":1}`))
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	w := httptest.NewRecorder()
	newCTTestHandler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRequireContentType_PostWithWrongType(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("hello"))
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	newCTTestHandler().ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", w.Code)
	}
}

func TestRequireContentType_PatchWithJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"x":1}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newCTTestHandler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
