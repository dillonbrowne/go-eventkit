package server_test

// REST-API e2e: full lifecycle round-trips over a real http.Client +
// httptest.Server. Where server_test.go uses httptest.NewRecorder
// (in-process), this file exercises the actual HTTP wire format so
// regressions in encoding, headers, status codes, etc. surface.
//
// No TCC, no cgo, no real EventKit — all bridges are testfakes. Runs
// anywhere `go test` runs.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server"
	"github.com/dillonbrowne/go-eventkit/server/policy"
	"github.com/dillonbrowne/go-eventkit/server/testfakes"
)

// restE2E wires the REST server with testfakes + a permissive iCloud
// policy and returns an httptest.Server URL ready for http.Client.
func restE2E(t *testing.T) (string, *testfakes.CalFake, *testfakes.RemFake) {
	t.Helper()

	cal := &testfakes.CalFake{
		Cals: []calendar.Calendar{
			{ID: "CAL-TEST", Title: "testing", Source: "iCloud"},
			{ID: "CAL-OUT", Title: "Outside", Source: "Personal"}, // denied
		},
	}
	rem := &testfakes.RemFake{
		Lists: []reminders.List{
			{ID: "LIST-TEST", Title: "testing", Source: "iCloud"},
			{ID: "LIST-OUT", Title: "Outside", Source: "Personal"}, // denied
		},
	}
	pol := &policy.Policy{
		Calendar: policy.PackagePolicy{
			Default: policy.ModeDeny,
			Entries: []policy.Entry{{Source: "iCloud", Mode: policy.ModeReadWrite}},
		},
		Reminders: policy.PackagePolicy{
			Default: policy.ModeDeny,
			Entries: []policy.Entry{{Source: "iCloud", Mode: policy.ModeReadWrite}},
		},
	}
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(pol),
	)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL, cal, rem
}

// req issues an HTTP request with optional JSON body and returns the
// response status + decoded body. Body decode is best-effort — empty
// bodies (204) decode into nil.
func req(t *testing.T, method, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(context.Background(), method, url, rdr)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode == http.StatusNoContent {
		return resp, nil
	}
	var decoded map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &decoded)
	return resp, decoded
}

// TestRESTE2E_HealthAndReady covers liveness + readiness over real HTTP.
func TestRESTE2E_HealthAndReady(t *testing.T) {
	base, _, _ := restE2E(t)
	for _, p := range []string{"/healthz", "/readyz"} {
		resp, body := req(t, http.MethodGet, base+p, nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d, want 200; body=%v", p, resp.StatusCode, body)
		}
	}
}

// TestRESTE2E_OpenAPISpec asserts /openapi.json is present and well-formed.
func TestRESTE2E_OpenAPISpec(t *testing.T) {
	base, _, _ := restE2E(t)
	resp, body := req(t, http.MethodGet, base+"/openapi.json", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	paths, ok := body["paths"].(map[string]any)
	if !ok {
		t.Fatalf("missing paths block; got %v", body)
	}
	for _, want := range []string{"/v1/calendars", "/v1/events", "/v1/lists", "/v1/reminders"} {
		if _, ok := paths[want]; !ok {
			t.Errorf("missing OpenAPI path %s", want)
		}
	}
}

// TestRESTE2E_HardeningHeaders confirms the middleware chain emits the
// right headers over the wire.
func TestRESTE2E_HardeningHeaders(t *testing.T) {
	base, _, _ := restE2E(t)
	resp, _ := req(t, http.MethodGet, base+"/healthz", nil)
	if resp.Header.Get("X-Request-Id") == "" {
		t.Errorf("missing X-Request-Id")
	}
	if got := resp.Header.Get("Server"); got != "" {
		t.Errorf("Server header leaked: %q", got)
	}
}

// TestRESTE2E_EventLifecycle: full create→get→patch→delete over HTTP.
func TestRESTE2E_EventLifecycle(t *testing.T) {
	base, _, _ := restE2E(t)

	// Create
	resp, body := req(t, http.MethodPost, base+"/v1/events", map[string]any{
		"title":     "stand-up",
		"startDate": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"endDate":   time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		"calendar":  "testing",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d; body=%v", resp.StatusCode, body)
	}
	id, ok := body["id"].(string)
	if !ok || id == "" {
		t.Fatalf("no id in create response: %v", body)
	}

	// Get
	resp, _ = req(t, http.MethodGet, base+"/v1/events/"+id, nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("get status %d", resp.StatusCode)
	}

	// Patch — title rename
	newTitle := "renamed"
	resp, body = req(t, http.MethodPatch, base+"/v1/events/"+id, map[string]any{"title": newTitle})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("patch status %d; body=%v", resp.StatusCode, body)
	}
	if body["title"] != newTitle {
		t.Errorf("title not updated: got %v", body["title"])
	}

	// Delete
	resp, _ = req(t, http.MethodDelete, base+"/v1/events/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete status %d, want 204", resp.StatusCode)
	}

	// Get after delete → 404
	resp, _ = req(t, http.MethodGet, base+"/v1/events/"+id, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get-after-delete status %d, want 404", resp.StatusCode)
	}
}

// TestRESTE2E_ReminderLifecycle: full create→complete→uncomplete→delete.
func TestRESTE2E_ReminderLifecycle(t *testing.T) {
	base, _, _ := restE2E(t)

	resp, body := req(t, http.MethodPost, base+"/v1/reminders", map[string]any{
		"title": "Buy milk", "list": "testing",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create %d; body=%v", resp.StatusCode, body)
	}
	id := body["id"].(string)

	resp, body = req(t, http.MethodPost, base+"/v1/reminders/"+id+"/complete", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("complete status %d; body=%v", resp.StatusCode, body)
	}
	if body["completed"] != true {
		t.Errorf("completed=%v", body["completed"])
	}

	resp, body = req(t, http.MethodPost, base+"/v1/reminders/"+id+"/uncomplete", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("uncomplete status %d", resp.StatusCode)
	}
	if body["completed"] != false {
		t.Errorf("completed=%v after uncomplete", body["completed"])
	}

	resp, _ = req(t, http.MethodDelete, base+"/v1/reminders/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete status %d", resp.StatusCode)
	}
}

// TestRESTE2E_PolicyDenials hits the same 404 path our scoped layer
// produces. The body deliberately doesn't differentiate from genuine
// not-found.
func TestRESTE2E_PolicyDenials(t *testing.T) {
	base, _, _ := restE2E(t)
	tests := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/v1/calendars/CAL-OUT", nil},
		{http.MethodGet, "/v1/lists/LIST-OUT", nil},
		{http.MethodPost, "/v1/events", map[string]any{
			"title":     "x",
			"startDate": time.Now().Format(time.RFC3339),
			"endDate":   time.Now().Add(time.Hour).Format(time.RFC3339),
			"calendar":  "Outside",
		}},
		{http.MethodPost, "/v1/reminders", map[string]any{"title": "x", "list": "Outside"}},
	}
	for _, tt := range tests {
		resp, _ := req(t, tt.method, base+tt.path, tt.body)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404", tt.method, tt.path, resp.StatusCode)
		}
	}
}

// TestRESTE2E_RateLimit confirms 429 + Retry-After on overflow.
func TestRESTE2E_RateLimit(t *testing.T) {
	// Standalone REST server with tight rate limit.
	cal := &testfakes.CalFake{Cals: []calendar.Calendar{{ID: "C", Title: "t", Source: "iCloud"}}}
	rem := &testfakes.RemFake{}
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(&policy.Policy{
			Calendar:  policy.PackagePolicy{Default: policy.ModeReadWrite},
			Reminders: policy.PackagePolicy{Default: policy.ModeReadWrite},
		}),
		server.WithRateLimit(0.01, 1), // ~1 request every 100s, burst 1
	)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, _ := req(t, http.MethodGet, ts.URL+"/healthz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: %d", resp.StatusCode)
	}
	resp2, _ := req(t, http.MethodGet, ts.URL+"/healthz", nil)
	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Errorf("second request: status %d, want 429", resp2.StatusCode)
	}
	if resp2.Header.Get("Retry-After") == "" {
		t.Errorf("missing Retry-After on 429")
	}
}

// TestRESTE2E_IdempotencyKey: same key → replay header.
func TestRESTE2E_IdempotencyKey(t *testing.T) {
	cal := &testfakes.CalFake{Cals: []calendar.Calendar{{ID: "C", Title: "testing", Source: "iCloud"}}}
	rem := &testfakes.RemFake{}
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(&policy.Policy{
			Calendar:  policy.PackagePolicy{Default: policy.ModeReadWrite},
			Reminders: policy.PackagePolicy{Default: policy.ModeReadWrite},
		}),
		server.WithIdempotency(time.Hour, 16),
	)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := map[string]any{
		"title":     "x",
		"startDate": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"endDate":   time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		"calendar":  "testing",
	}
	makeReq := func() (*http.Response, []byte) {
		b, _ := json.Marshal(body)
		r, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/events", bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "abc-123")
		resp, _ := http.DefaultClient.Do(r)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, raw
	}
	resp1, _ := makeReq()
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first: %d", resp1.StatusCode)
	}
	if resp1.Header.Get("Idempotency-Replayed") != "" {
		t.Errorf("first request unexpectedly marked replayed")
	}
	resp2, _ := makeReq()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("replay: %d", resp2.StatusCode)
	}
	if resp2.Header.Get("Idempotency-Replayed") != "true" {
		t.Errorf("replay missing Idempotency-Replayed=true header")
	}
	// Fake should have been called exactly once.
	creates := 0
	for _, c := range cal.Calls {
		if c == "CreateEvent:testing" {
			creates++
		}
	}
	if creates != 1 {
		t.Errorf("CreateEvent called %d times, want 1 (second call should be served from cache)", creates)
	}
}
