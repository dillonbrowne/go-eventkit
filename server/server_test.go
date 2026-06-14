package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server"
	"github.com/dillonbrowne/go-eventkit/server/policy"
	"github.com/dillonbrowne/go-eventkit/server/testfakes"
)

// fixturePolicy: iCloud is readwrite by source; CAL-WORK is explicitly
// read-only (overrides iCloud); Local source is read-only; default is
// deny. Same pattern for reminders.
func fixturePolicy() *policy.Policy {
	return &policy.Policy{
		Calendar: policy.PackagePolicy{
			Default: policy.ModeDeny,
			Entries: []policy.Entry{
				{Source: "iCloud", Mode: policy.ModeReadWrite},
				{Source: "Local", Mode: policy.ModeRead},
				{ID: "CAL-WORK", Mode: policy.ModeRead},
			},
		},
		Reminders: policy.PackagePolicy{
			Default: policy.ModeDeny,
			Entries: []policy.Entry{
				{Source: "iCloud", Mode: policy.ModeReadWrite},
				{ID: "LIST-ARCHIVE", Mode: policy.ModeRead},
			},
		},
	}
}

func fixtureCalFake() *testfakes.CalFake {
	return &testfakes.CalFake{
		Cals: []calendar.Calendar{
			{ID: "CAL-HOME", Title: "Home", Source: "iCloud"},
			{ID: "CAL-WORK", Title: "Work", Source: "iCloud"},
			{ID: "CAL-FAMILY", Title: "Family", Source: "Local"},
			{ID: "CAL-OUT", Title: "Outside", Source: "Personal"},
		},
		Events: []calendar.Event{
			{ID: "EV-1", Title: "Home event", Calendar: "Home", CalendarID: "CAL-HOME"},
			{ID: "EV-2", Title: "Work event", Calendar: "Work", CalendarID: "CAL-WORK"},
			{ID: "EV-OUT", Title: "Outside event", Calendar: "Outside", CalendarID: "CAL-OUT"},
		},
	}
}

func fixtureRemFake() *testfakes.RemFake {
	return &testfakes.RemFake{
		Lists: []reminders.List{
			{ID: "LIST-TODO", Title: "Todo", Source: "iCloud"},
			{ID: "LIST-ARCHIVE", Title: "Archive", Source: "iCloud"},
			{ID: "LIST-OUT", Title: "Outside", Source: "Personal"},
		},
		Items: []reminders.Reminder{
			{ID: "R-1", Title: "Buy milk", List: "Todo", ListID: "LIST-TODO"},
			{ID: "R-ARC", Title: "Done thing", List: "Archive", ListID: "LIST-ARCHIVE", Completed: true},
			{ID: "R-OUT", Title: "Personal", List: "Outside", ListID: "LIST-OUT"},
		},
	}
}

func newTestServer(t *testing.T) (*server.Server, *testfakes.CalFake, *testfakes.RemFake) {
	t.Helper()
	cal := fixtureCalFake()
	rem := fixtureRemFake()
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(fixturePolicy()),
	)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv, cal, rem
}

// do is a small helper: dispatches a request through the server's handler
// with loopback addressing and returns the recorder.
func do(t *testing.T, srv *server.Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "127.0.0.1:8765"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func TestServer_NoPolicyAllowsOnlyDefaults(t *testing.T) {
	cal := fixtureCalFake()
	rem := fixtureRemFake()
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
	)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	// With no policy, the server should scope access to only the bridge's
	// reported default calendar / list (CalFake reports the first
	// non-readonly cal: CAL-HOME).
	rr := do(t, srv, http.MethodGet, "/v1/calendars", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Calendars []calendar.Calendar `json:"calendars"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range body.Calendars {
		got[c.ID] = true
	}
	if len(got) != 1 || !got["CAL-HOME"] {
		t.Errorf("no-policy default should expose only CAL-HOME; got %v", got)
	}

	// Other calendars must be invisible.
	rr = do(t, srv, http.MethodGet, "/v1/calendars/CAL-WORK", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("CAL-WORK should be 404 under no-policy default; got %d", rr.Code)
	}
}

func TestServer_RefusesWithoutBridges(t *testing.T) {
	_, err := server.New(server.WithPolicy(fixturePolicy()))
	if err == nil {
		t.Fatalf("expected error when bridges are missing")
	}
}

func TestServer_Healthz(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/healthz", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_OpenAPISpecGenerated(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/openapi.json", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var spec map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("spec.paths missing or wrong type")
	}
	want := []string{
		"/healthz",
		"/v1/calendars",
		"/v1/calendars/{id}",
		"/v1/events",
		"/v1/events/{id}",
		"/v1/events/batch-delete",
		"/v1/lists",
		"/v1/lists/{id}",
		"/v1/reminders",
		"/v1/reminders/{id}",
		"/v1/reminders/{id}/complete",
		"/v1/reminders/{id}/uncomplete",
		"/v1/reminders/batch-delete",
	}
	for _, p := range want {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing OpenAPI path %q", p)
		}
	}
}

func TestServer_PublicURL_SetsServersBlock(t *testing.T) {
	// Default (no WithPublicURL): spec must carry no servers block, which is
	// correct for loopback-only use.
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/openapi.json", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var spec map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := spec["servers"]; ok {
		t.Errorf("default spec should have no servers block; got %v", spec["servers"])
	}

	// With WithPublicURL: spec advertises that URL as servers[0].url so the
	// GPT Actions builder accepts it.
	cal := fixtureCalFake()
	rem := fixtureRemFake()
	pub, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(fixturePolicy()),
		server.WithPublicURL("https://apple-gpt.dbee.me"),
	)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	for _, path := range []string{"/openapi.json", "/openapi-3.0.json"} {
		rr := do(t, pub, http.MethodGet, path, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rr.Code)
		}
		var spec struct {
			Servers []struct {
				URL string `json:"url"`
			} `json:"servers"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
			t.Fatalf("%s decode: %v", path, err)
		}
		if len(spec.Servers) != 1 || spec.Servers[0].URL != "https://apple-gpt.dbee.me" {
			t.Errorf("%s servers = %+v, want one entry with url https://apple-gpt.dbee.me", path, spec.Servers)
		}
	}
}

func TestServer_ListCalendars_FilteredByPolicy(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/v1/calendars", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Calendars []calendar.Calendar `json:"calendars"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]bool{}
	for _, c := range body.Calendars {
		got[c.ID] = true
	}
	want := map[string]bool{"CAL-HOME": true, "CAL-WORK": true, "CAL-FAMILY": true}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got["CAL-OUT"] {
		t.Errorf("denied calendar leaked in list response")
	}
}

func TestServer_GetCalendar_AllowedReturns200(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/v1/calendars/CAL-HOME", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_GetCalendar_DeniedReturns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/v1/calendars/CAL-OUT", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (policy denial); body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_CreateCalendar_DeniedSourceReturns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/calendars", map[string]any{
		"title":  "Trip",
		"source": "Personal",
	})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_CreateCalendar_AllowedSourceReturns201(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/calendars", map[string]any{
		"title":  "Trip",
		"source": "iCloud",
		"color":  "#FF0000",
	})
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_ListEvents_FilteredByPolicy(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/v1/events?start=2026-01-01T00:00:00Z&end=2026-12-31T23:59:59Z", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Events []calendar.Event `json:"events"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]bool{}
	for _, e := range body.Events {
		got[e.ID] = true
	}
	if got["EV-OUT"] {
		t.Errorf("denied event leaked in list response")
	}
	if !got["EV-1"] || !got["EV-2"] {
		t.Errorf("expected EV-1 and EV-2 in response, got %v", got)
	}
}

func TestServer_CreateEvent_ReadOnlyCalReturns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/events", map[string]any{
		"title":     "X",
		"startDate": time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		"endDate":   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		"calendar":  "Work", // CAL-WORK is read-only by explicit ID
	})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_CreateEvent_DeniedCalReturns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/events", map[string]any{
		"title":     "X",
		"startDate": time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		"endDate":   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		"calendar":  "Outside",
	})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_CreateEvent_WritableCalReturns201(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/events", map[string]any{
		"title":     "Standup",
		"startDate": time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		"endDate":   time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC),
		"calendar":  "Home",
	})
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_CreateEvent_MissingCalendarReturns422(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/events", map[string]any{
		"title":     "X",
		"startDate": time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		"endDate":   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_PatchEvent_ReadOnlyCalReturns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	title := "Renamed"
	rr := do(t, srv, http.MethodPatch, "/v1/events/EV-2", map[string]any{
		"title": title,
	})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_DeleteEvent_ReadOnlyCalReturns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodDelete, "/v1/events/EV-2", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_DeleteEvent_WritableCalReturns204(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodDelete, "/v1/events/EV-1", nil)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_BatchDeleteEvents_Partitions(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/events/batch-delete", map[string]any{
		"ids": []string{"EV-1", "EV-2", "EV-OUT", "EV-MISSING"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Results map[string]string `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Results["EV-1"] != "ok" {
		t.Errorf("EV-1 (writable): result = %q, want 'ok'", body.Results["EV-1"])
	}
	for _, id := range []string{"EV-2", "EV-OUT", "EV-MISSING"} {
		if body.Results[id] == "" || body.Results[id] == "ok" {
			t.Errorf("%s: result = %q, want error", id, body.Results[id])
		}
	}
}

func TestServer_BatchDeleteReminders_Partitions(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/reminders/batch-delete", map[string]any{
		"ids": []string{"R-1", "R-ARC", "R-OUT", "R-MISSING"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Results map[string]string `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// R-1 is writable and succeeds: it must be reported "ok", not silently
	// omitted (the real bridge returns only failures — the scoped layer marks
	// successes).
	if body.Results["R-1"] != "ok" {
		t.Errorf("R-1 (writable): result = %q, want 'ok'", body.Results["R-1"])
	}
	for _, id := range []string{"R-ARC", "R-OUT", "R-MISSING"} {
		if body.Results[id] == "" || body.Results[id] == "ok" {
			t.Errorf("%s: result = %q, want error", id, body.Results[id])
		}
	}
}

func TestServer_ListReminders_FilteredByPolicy(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/v1/reminders", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Reminders []reminders.Reminder `json:"reminders"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]bool{}
	for _, r := range body.Reminders {
		got[r.ID] = true
	}
	if got["R-OUT"] {
		t.Errorf("denied reminder leaked in list response")
	}
}

func TestServer_CreateReminder_ReadOnlyListReturns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/reminders", map[string]any{
		"title": "X",
		"list":  "Archive",
	})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_CreateReminder_WritableListReturns201(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/reminders", map[string]any{
		"title": "Pay bill",
		"list":  "Todo",
	})
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_CompleteReminder_ReadOnlyListReturns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/reminders/R-ARC/complete", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_CompleteReminder_WritableListReturns200(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/v1/reminders/R-1/complete", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
}

func TestServer_Loopback_NonLoopbackHostRejected(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/calendars", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "example.com:8765"
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestServer_Loopback_NonLoopbackRemoteRejected(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/calendars", nil)
	req.RemoteAddr = "8.8.8.8:54321"
	req.Host = "127.0.0.1:8765"
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestServer_BodyLimit_RejectsOversize(t *testing.T) {
	cal := fixtureCalFake()
	rem := fixtureRemFake()
	srv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(fixturePolicy()),
		server.WithBodyLimit(10),
	)
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("A", 200)
	req := httptest.NewRequest(http.MethodPost, "/v1/calendars", strings.NewReader(`{"title":"`+big+`","source":"iCloud"}`))
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code == http.StatusCreated {
		t.Errorf("body cap not enforced; got 201")
	}
}
