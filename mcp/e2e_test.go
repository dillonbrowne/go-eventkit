package mcp_test

// End-to-end regression suite for the MCP server. Wires the *real*
// REST server (server.New) with in-memory testfakes bridges, fronts it
// with httptest, then connects an MCP client to the MCP server via the
// SDK's in-memory transport. Every one of the 24 registered tools is
// exercised at least once. No TCC, no cgo, no real EventKit — runs
// anywhere `go test` runs.
//
// The point: keep this green on every commit so we catch breakage in
// any of the layers (MCP tool registration, REST client, REST server
// middleware, scoped wrappers, policy enforcement).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/mcp/client"
	"github.com/dillonbrowne/go-eventkit/mcp/tools"
	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server"
	"github.com/dillonbrowne/go-eventkit/server/policy"
	"github.com/dillonbrowne/go-eventkit/server/testfakes"
)

// e2eHarness wires the full stack and returns:
//   - a connected MCP ClientSession ready for tool calls
//   - the underlying CalFake / RemFake so tests can inspect or seed state
//
// Cleanup happens automatically via t.Cleanup.
func e2eHarness(t *testing.T) (*sdkmcp.ClientSession, *testfakes.CalFake, *testfakes.RemFake) {
	t.Helper()

	cal := &testfakes.CalFake{
		Cals: []calendar.Calendar{
			{ID: "CAL-TEST", Title: "testing", Source: "iCloud"},
			{ID: "CAL-HOME", Title: "Home", Source: "iCloud"},
			{ID: "CAL-OUT", Title: "Outside", Source: "Personal"}, // denied by policy
		},
	}
	rem := &testfakes.RemFake{
		Lists: []reminders.List{
			{ID: "LIST-TEST", Title: "testing", Source: "iCloud"},
			{ID: "LIST-TODO", Title: "Todo", Source: "iCloud"},
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

	restSrv, err := server.New(
		server.WithCalendarBridge(testfakes.NewCalendar(cal)),
		server.WithRemindersBridge(testfakes.NewReminders(rem)),
		server.WithPolicy(pol),
	)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	// httptest binds 127.0.0.1, so the REST server's loopback middleware
	// allows requests going through ts.URL.
	rest := httptest.NewServer(restSrv.Handler())
	t.Cleanup(rest.Close)

	// Build a real MCP server and register every tool — no shortcuts.
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "eventkit-mcp-e2e", Version: "test"},
		&sdkmcp.ServerOptions{Instructions: tools.UserDataNotice},
	)
	tools.Register(mcpServer, client.New(rest.URL, 5*time.Second))

	t1, t2 := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	if _, err := mcpServer.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("mcp server connect: %v", err)
	}
	cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := cli.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("mcp client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs, cal, rem
}

// callTool is a tiny helper to drive a tool by name + args, fail-fast
// on transport errors, and return the parsed structured content.
func callTool(t *testing.T, cs *sdkmcp.ClientSession, name string, args map[string]any) *sdkmcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

// structured unmarshals the result's structured content into the target.
func structured(t *testing.T, res *sdkmcp.CallToolResult, target any) {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	if err := json.Unmarshal(b, target); err != nil {
		t.Fatalf("unmarshal structured: %v (raw: %s)", err, b)
	}
}

// ---- Tool surface contract ----

// TestE2E_ToolSurface pins the number of tools and the annotation
// distribution. Bump these counters intentionally when adding/removing
// tools.
func TestE2E_ToolSurface(t *testing.T) {
	cs, _, _ := e2eHarness(t)
	got, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	// 24 CRUD tools + search + fetch (the two ChatGPT Deep Research tools).
	if len(got.Tools) != 26 {
		t.Fatalf("got %d tools, want 26", len(got.Tools))
	}
	var ro, destructive, idempotent int
	for _, tt := range got.Tools {
		if tt.Annotations == nil {
			t.Errorf("tool %q has nil annotations", tt.Name)
			continue
		}
		if tt.Annotations.ReadOnlyHint {
			ro++
		}
		if tt.Annotations.DestructiveHint != nil && *tt.Annotations.DestructiveHint {
			destructive++
		}
		if tt.Annotations.IdempotentHint {
			idempotent++
		}
	}
	// 8 list/get + search + fetch = 10 read-only.
	if ro != 10 {
		t.Errorf("read-only tool count = %d, want 10", ro)
	}
	if destructive != 6 {
		t.Errorf("destructive tool count = %d, want 6", destructive)
	}
	// 6 update/complete/uncomplete + 6 delete (delete is idempotent because
	// a second delete of the same id is a no-op) = 12.
	if idempotent != 12 {
		t.Errorf("idempotent tool count = %d, want 12", idempotent)
	}
}

// ---- Read tools ----

func TestE2E_ListCalendars(t *testing.T) {
	cs, _, _ := e2eHarness(t)
	res := callTool(t, cs, "list_calendars", nil)
	if res.IsError {
		t.Fatalf("IsError: %+v", res.Content)
	}
	var out struct {
		Calendars []tools.RedactedCalendar `json:"calendars"`
	}
	structured(t, res, &out)
	// Two iCloud cals; the Personal one is denied by policy.
	if len(out.Calendars) != 2 {
		t.Fatalf("got %d calendars, want 2; got=%+v", len(out.Calendars), out.Calendars)
	}
	for _, c := range out.Calendars {
		if !strings.HasPrefix(c.Title, "<USER_DATA>") {
			t.Errorf("calendar %q title not wrapped: %q", c.ID, c.Title)
		}
		if c.Source != "iCloud" {
			t.Errorf("denied calendar leaked: %q", c.Source)
		}
	}
}

func TestE2E_ListReminderLists(t *testing.T) {
	cs, _, _ := e2eHarness(t)
	res := callTool(t, cs, "list_reminder_lists", nil)
	if res.IsError {
		t.Fatalf("IsError: %+v", res.Content)
	}
	var out struct {
		Lists []tools.RedactedList `json:"lists"`
	}
	structured(t, res, &out)
	if len(out.Lists) != 2 {
		t.Fatalf("got %d lists, want 2", len(out.Lists))
	}
}

func TestE2E_GetCalendar(t *testing.T) {
	cs, _, _ := e2eHarness(t)
	res := callTool(t, cs, "get_calendar", map[string]any{"id": "CAL-TEST"})
	if res.IsError {
		t.Fatalf("IsError: %+v", res.Content)
	}
	var out struct {
		Calendar tools.RedactedCalendar `json:"calendar"`
	}
	structured(t, res, &out)
	if out.Calendar.ID != "CAL-TEST" {
		t.Errorf("got id %q, want CAL-TEST", out.Calendar.ID)
	}
}

func TestE2E_GetReminderList(t *testing.T) {
	cs, _, _ := e2eHarness(t)
	res := callTool(t, cs, "get_reminder_list", map[string]any{"id": "LIST-TEST"})
	if res.IsError {
		t.Fatalf("IsError: %+v", res.Content)
	}
}

func TestE2E_ListEvents_NaturalDates(t *testing.T) {
	cs, _, rem := e2eHarness(t)
	_ = rem
	res := callTool(t, cs, "list_events", map[string]any{
		"start": "today",
		"end":   "in 7 days",
	})
	if res.IsError {
		t.Fatalf("IsError: %+v", res.Content)
	}
}

func TestE2E_ListReminders(t *testing.T) {
	cs, _, _ := e2eHarness(t)
	res := callTool(t, cs, "list_reminders", map[string]any{"list": "testing"})
	if res.IsError {
		t.Fatalf("IsError: %+v", res.Content)
	}
}

// ---- Calendar lifecycle ----

func TestE2E_CalendarLifecycle(t *testing.T) {
	cs, cal, _ := e2eHarness(t)

	// Create
	res := callTool(t, cs, "create_calendar", map[string]any{
		"title":  "Trip",
		"source": "iCloud",
		"color":  "#FF6961",
	})
	if res.IsError {
		t.Fatalf("create: IsError: %+v", res.Content)
	}
	var created struct {
		Calendar tools.RedactedCalendar `json:"calendar"`
	}
	structured(t, res, &created)

	// Update
	res = callTool(t, cs, "update_calendar", map[string]any{
		"id":    created.Calendar.ID,
		"title": "Trip (renamed)",
	})
	if res.IsError {
		t.Fatalf("update: IsError: %+v", res.Content)
	}

	// Delete
	res = callTool(t, cs, "delete_calendar", map[string]any{"id": created.Calendar.ID})
	if res.IsError {
		t.Fatalf("delete: IsError: %+v", res.Content)
	}
	// Verify gone from the fake.
	for _, c := range cal.Cals {
		if c.ID == created.Calendar.ID {
			t.Errorf("calendar still in fake after delete: %v", c)
		}
	}
}

// ---- Event lifecycle ----

func TestE2E_EventLifecycle(t *testing.T) {
	cs, _, _ := e2eHarness(t)

	// Create with natural-language dates.
	res := callTool(t, cs, "create_event", map[string]any{
		"title":     "stand-up",
		"startDate": "tomorrow 9am",
		"endDate":   "tomorrow 9:30am",
		"calendar":  "testing",
		"notes":     "via e2e test",
	})
	if res.IsError {
		t.Fatalf("create: IsError: %+v", res.Content)
	}
	var created struct {
		Event tools.RedactedEvent `json:"event"`
	}
	structured(t, res, &created)
	if created.Event.ID == "" {
		t.Fatal("created event has no id")
	}
	// Date parsing: result must be RFC 3339 with a real timestamp.
	if _, err := time.Parse(time.RFC3339, created.Event.StartDate); err != nil {
		t.Errorf("startDate not RFC3339: %q", created.Event.StartDate)
	}
	// Redaction.
	if !strings.HasPrefix(created.Event.Title, "<USER_DATA>") {
		t.Errorf("title not wrapped: %q", created.Event.Title)
	}

	// Get
	res = callTool(t, cs, "get_event", map[string]any{"id": created.Event.ID})
	if res.IsError {
		t.Fatalf("get: IsError: %+v", res.Content)
	}

	// Update
	res = callTool(t, cs, "update_event", map[string]any{
		"id":    created.Event.ID,
		"title": "renamed",
	})
	if res.IsError {
		t.Fatalf("update: IsError: %+v", res.Content)
	}

	// Delete
	res = callTool(t, cs, "delete_event", map[string]any{"id": created.Event.ID})
	if res.IsError {
		t.Fatalf("delete: IsError: %+v", res.Content)
	}

	// Get after delete → IsError.
	res = callTool(t, cs, "get_event", map[string]any{"id": created.Event.ID})
	if !res.IsError {
		t.Errorf("get_event after delete: expected IsError")
	}
}

// ---- Reminder list lifecycle ----

func TestE2E_ReminderListLifecycle(t *testing.T) {
	cs, _, _ := e2eHarness(t)

	res := callTool(t, cs, "create_reminder_list", map[string]any{
		"title":  "Trips",
		"source": "iCloud",
	})
	if res.IsError {
		t.Fatalf("create: IsError: %+v", res.Content)
	}
	var created struct {
		List tools.RedactedList `json:"list"`
	}
	structured(t, res, &created)

	res = callTool(t, cs, "update_reminder_list", map[string]any{
		"id":    created.List.ID,
		"title": "Trips (renamed)",
	})
	if res.IsError {
		t.Fatalf("update: IsError: %+v", res.Content)
	}

	res = callTool(t, cs, "delete_reminder_list", map[string]any{"id": created.List.ID})
	if res.IsError {
		t.Fatalf("delete: IsError: %+v", res.Content)
	}
}

// ---- Reminder lifecycle ----

func TestE2E_ReminderLifecycle(t *testing.T) {
	cs, _, _ := e2eHarness(t)

	res := callTool(t, cs, "create_reminder", map[string]any{
		"title":    "Buy milk",
		"list":     "testing",
		"dueDate":  "tomorrow 5pm",
		"priority": float64(1),
	})
	if res.IsError {
		t.Fatalf("create: IsError: %+v", res.Content)
	}
	var created struct {
		Reminder tools.RedactedReminder `json:"reminder"`
	}
	structured(t, res, &created)

	// Get
	res = callTool(t, cs, "get_reminder", map[string]any{"id": created.Reminder.ID})
	if res.IsError {
		t.Fatalf("get: IsError: %+v", res.Content)
	}

	// Update
	res = callTool(t, cs, "update_reminder", map[string]any{
		"id":    created.Reminder.ID,
		"notes": "remember to use cash",
	})
	if res.IsError {
		t.Fatalf("update: IsError: %+v", res.Content)
	}

	// Complete
	res = callTool(t, cs, "complete_reminder", map[string]any{"id": created.Reminder.ID})
	if res.IsError {
		t.Fatalf("complete: IsError: %+v", res.Content)
	}
	var completed struct {
		Reminder tools.RedactedReminder `json:"reminder"`
	}
	structured(t, res, &completed)
	if !completed.Reminder.Completed {
		t.Errorf("expected completed=true; got %+v", completed.Reminder)
	}

	// Uncomplete
	res = callTool(t, cs, "uncomplete_reminder", map[string]any{"id": created.Reminder.ID})
	if res.IsError {
		t.Fatalf("uncomplete: IsError: %+v", res.Content)
	}

	// Delete
	res = callTool(t, cs, "delete_reminder", map[string]any{"id": created.Reminder.ID})
	if res.IsError {
		t.Fatalf("delete: IsError: %+v", res.Content)
	}
}

// ---- Batch operations ----

func TestE2E_BatchDeleteEvents(t *testing.T) {
	cs, _, _ := e2eHarness(t)

	// Seed three events.
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		res := callTool(t, cs, "create_event", map[string]any{
			"title":     "batch test",
			"calendar":  "testing",
			"startDate": "tomorrow 9am",
			"endDate":   "tomorrow 10am",
		})
		if res.IsError {
			t.Fatalf("create %d: IsError: %+v", i, res.Content)
		}
		var ev struct {
			Event tools.RedactedEvent `json:"event"`
		}
		structured(t, res, &ev)
		ids = append(ids, ev.Event.ID)
	}

	res := callTool(t, cs, "batch_delete_events", map[string]any{"ids": ids})
	if res.IsError {
		t.Fatalf("batch_delete: IsError: %+v", res.Content)
	}
	var out struct {
		Results map[string]string `json:"results"`
	}
	structured(t, res, &out)
	for _, id := range ids {
		if got := out.Results[id]; got != "ok" {
			t.Errorf("id %s: result %q, want ok", id, got)
		}
	}
}

func TestE2E_BatchDeleteReminders(t *testing.T) {
	cs, _, _ := e2eHarness(t)
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		res := callTool(t, cs, "create_reminder", map[string]any{
			"title": "batch test",
			"list":  "testing",
		})
		if res.IsError {
			t.Fatalf("create %d: IsError: %+v", i, res.Content)
		}
		var r struct {
			Reminder tools.RedactedReminder `json:"reminder"`
		}
		structured(t, res, &r)
		ids = append(ids, r.Reminder.ID)
	}
	res := callTool(t, cs, "batch_delete_reminders", map[string]any{"ids": ids})
	if res.IsError {
		t.Fatalf("batch_delete: IsError: %+v", res.Content)
	}
}

// ---- Policy denial ----

func TestE2E_PolicyDenial(t *testing.T) {
	cs, _, _ := e2eHarness(t)

	// Try to create in the "Outside" calendar/list, denied by policy.
	tests := []struct {
		tool string
		args map[string]any
	}{
		{"create_event", map[string]any{"title": "x", "calendar": "Outside", "startDate": "tomorrow", "endDate": "tomorrow 1pm"}},
		{"create_reminder", map[string]any{"title": "x", "list": "Outside"}},
		{"get_calendar", map[string]any{"id": "CAL-OUT"}},
		{"get_reminder_list", map[string]any{"id": "LIST-OUT"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
				Name: tt.tool, Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if !res.IsError {
				t.Errorf("expected IsError on policy denial for %s; got %+v", tt.tool, res.StructuredContent)
			}
		})
	}
}

// ---- Input validation ----

func TestE2E_BadInput(t *testing.T) {
	cs, _, _ := e2eHarness(t)

	// Garbage natural-language date → MCP error.
	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name: "create_event",
		Arguments: map[string]any{
			"title": "x", "calendar": "testing",
			"startDate": "not a real date", "endDate": "tomorrow",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError on bad date input")
	}

	// Missing required field.
	res, err = cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "get_event",
		Arguments: map[string]any{}, // missing id
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError on missing required id")
	}
}

// ---- ChatGPT search / fetch ----

func TestE2E_SearchAndFetch(t *testing.T) {
	cs, cal, rem := e2eHarness(t)

	// Seed an event (id deliberately contains a colon, like real EventKit
	// occurrence identifiers) and a reminder, both in allowed iCloud
	// containers.
	cal.Events = append(cal.Events, calendar.Event{
		ID:         "EVT-1:OCC-9",
		Title:      "Dentist appointment",
		Calendar:   "testing",
		CalendarID: "CAL-TEST",
		Location:   "123 Main St",
		StartDate:  time.Now().Add(48 * time.Hour),
		EndDate:    time.Now().Add(49 * time.Hour),
	})
	rem.Items = append(rem.Items, reminders.Reminder{
		ID: "REM-1", Title: "Buy milk", List: "testing", ListID: "LIST-TEST",
	})

	// --- search returns both, with namespaced ids + urls ---
	res := callTool(t, cs, "search", map[string]any{"query": "anything"})
	if res.IsError {
		t.Fatalf("search: IsError: %+v", res.Content)
	}
	var sout struct {
		Results []struct {
			ID, Title, Text, URL string
		} `json:"results"`
	}
	structured(t, res, &sout)

	byID := map[string]string{} // id -> title
	for _, r := range sout.Results {
		byID[r.ID] = r.Title
		if r.URL == "" {
			t.Errorf("result %q has empty url (OpenAI requires url)", r.ID)
		}
	}
	if byID["event:EVT-1:OCC-9"] != "Dentist appointment" {
		t.Errorf("event hit missing/wrong: %v", byID)
	}
	if byID["reminder:REM-1"] != "Buy milk" {
		t.Errorf("reminder hit missing/wrong: %v", byID)
	}

	// --- fetch an event whose id itself contains a colon ---
	res = callTool(t, cs, "fetch", map[string]any{"id": "event:EVT-1:OCC-9"})
	if res.IsError {
		t.Fatalf("fetch event: IsError: %+v", res.Content)
	}
	var fevent struct {
		ID, Title, Text, URL string
		Metadata             map[string]any `json:"metadata"`
	}
	structured(t, res, &fevent)
	if fevent.ID != "event:EVT-1:OCC-9" {
		t.Errorf("fetch echoed id = %q, want event:EVT-1:OCC-9", fevent.ID)
	}
	if fevent.Title != "Dentist appointment" {
		t.Errorf("fetch event title = %q", fevent.Title)
	}
	if fevent.Metadata["kind"] != "event" {
		t.Errorf("fetch event metadata.kind = %v, want event", fevent.Metadata["kind"])
	}
	if !strings.Contains(fevent.Text, "Dentist appointment") {
		t.Errorf("fetch event text missing title: %q", fevent.Text)
	}

	// --- fetch a reminder ---
	res = callTool(t, cs, "fetch", map[string]any{"id": "reminder:REM-1"})
	if res.IsError {
		t.Fatalf("fetch reminder: IsError: %+v", res.Content)
	}
	var frem struct {
		ID, Title string
		Metadata  map[string]any `json:"metadata"`
	}
	structured(t, res, &frem)
	if frem.Title != "Buy milk" || frem.Metadata["kind"] != "reminder" {
		t.Errorf("fetch reminder wrong: %+v", frem)
	}

	// --- fetch with a malformed id → IsError ---
	bad := callTool(t, cs, "fetch", map[string]any{"id": "no-prefix"})
	if !bad.IsError {
		t.Errorf("fetch with malformed id should be IsError")
	}
}

// silence unused-import lint
var _ = errors.New
