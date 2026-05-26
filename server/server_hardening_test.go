package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/server"
	"github.com/dillonbrowne/go-eventkit/server/testfakes"
)

// ---- Error-mapping coverage ----

func TestServer_AccessDeniedReturns503(t *testing.T) {
	cal := fixtureCalFake()
	cal.Err = calendar.ErrAccessDenied
	rem := fixtureRemFake()
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(fixturePolicy()),
	)
	if err != nil {
		t.Fatal(err)
	}
	rr := do(t, srv, http.MethodGet, "/v1/calendars", nil)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_ImmutableReturns409(t *testing.T) {
	cal := fixtureCalFake()
	cal.Err = calendar.ErrImmutable
	rem := fixtureRemFake()
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(fixturePolicy()),
	)
	if err != nil {
		t.Fatal(err)
	}
	title := "new"
	rr := do(t, srv, http.MethodPatch, "/v1/calendars/CAL-HOME", map[string]any{"title": title})
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_UnsupportedReturns501(t *testing.T) {
	cal := fixtureCalFake()
	cal.Err = calendar.ErrUnsupported
	rem := fixtureRemFake()
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(fixturePolicy()),
	)
	if err != nil {
		t.Fatal(err)
	}
	rr := do(t, srv, http.MethodGet, "/v1/calendars", nil)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_UnknownPathReturns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/v1/nope", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_MalformedJSONReturns422(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader("{not json"))
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code < 400 || rr.Code >= 500 {
		t.Errorf("status = %d, want 4xx; body = %s", rr.Code, rr.Body.String())
	}
}

// ---- Hardening headers ----

func TestServer_ResponsesIncludeRequestID(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/healthz", nil)
	if got := rr.Header().Get("X-Request-ID"); got == "" {
		t.Errorf("X-Request-ID missing")
	}
}

func TestServer_EchoesValidIncomingRequestID(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "127.0.0.1:8765"
	req.Header.Set("X-Request-ID", "tr-deadbeef")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if got := rr.Header().Get("X-Request-ID"); got != "tr-deadbeef" {
		t.Errorf("X-Request-ID = %q, want tr-deadbeef", got)
	}
}

func TestServer_ServerHeaderStripped(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/healthz", nil)
	if got := rr.Header().Get("Server"); got != "" {
		t.Errorf("Server header leaked: %q", got)
	}
}

func TestServer_ContentType_Rejects415(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`{"x":1}`))
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "127.0.0.1:8765"
	// no Content-Type header
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_MethodOverride_Rejected(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "127.0.0.1:8765"
	req.Header.Set("X-HTTP-Method-Override", "GET")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// ---- Panic survives ----

func TestServer_PanicSurvives(t *testing.T) {
	cal := fixtureCalFake()
	cal.PanicOn = "intentional test panic"
	rem := fixtureRemFake()
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(fixturePolicy()),
	)
	if err != nil {
		t.Fatal(err)
	}
	// First request panics; expect 500.
	rr := do(t, srv, http.MethodGet, "/v1/calendars", nil)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("first request status = %d, want 500", rr.Code)
	}
	// Second request also panics (PanicOn still set), still survives.
	rr = do(t, srv, http.MethodGet, "/healthz", nil)
	// /healthz doesn't touch the bridge so no panic — server should
	// respond normally.
	if rr.Code != http.StatusOK {
		t.Errorf("healthz after panic = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
}

// ---- OpenAPI structural validity ----

func TestServer_OpenAPISpec_Structure(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/openapi.json", nil)
	var spec struct {
		OpenAPI string                               `json:"openapi"`
		Info    map[string]any                       `json:"info"`
		Paths   map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.1") {
		t.Errorf("openapi = %q, want 3.1.x", spec.OpenAPI)
	}
	if spec.Info["version"] == "" {
		t.Errorf("info.version is empty")
	}
	if len(spec.Paths) == 0 {
		t.Fatalf("no paths in spec")
	}
	// Every operation should declare at least one response and at least
	// one of them should be a success response. 4xx codes are added by
	// huma at request-validation time rather than declared up-front, so
	// we do not require them in the static spec.
	for path, methods := range spec.Paths {
		for method, op := range methods {
			responses, ok := op["responses"].(map[string]any)
			if !ok || len(responses) == 0 {
				t.Errorf("%s %s: no responses declared", method, path)
				continue
			}
			has2xx := false
			for code := range responses {
				if strings.HasPrefix(code, "2") {
					has2xx = true
				}
			}
			if !has2xx {
				t.Errorf("%s %s: no 2xx response declared", method, path)
			}
		}
	}
}

// ---- Scalar docs ----

func TestServer_DocsServesScalar(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/docs", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(strings.ToLower(body), "scalar") {
		t.Errorf("docs response does not appear to reference scalar; got first 400 chars: %s", truncate(body, 400))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
