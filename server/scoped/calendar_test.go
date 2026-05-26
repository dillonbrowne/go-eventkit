package scoped

import (
	"errors"
	"testing"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/server/policy"
	"github.com/dillonbrowne/go-eventkit/server/testfakes"
)

// fixturePolicy: iCloud is readwrite by source, Local is read by source,
// CAL-WORK is explicitly read (overrides iCloud readwrite),
// default is deny.
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

func fixtureBridge() *testfakes.CalFake {
	return &testfakes.CalFake{
		Cals: []calendar.Calendar{
			{ID: "CAL-HOME", Title: "Home", Source: "iCloud"},
			{ID: "CAL-WORK", Title: "Work", Source: "iCloud"},     // demoted to read by ID
			{ID: "CAL-FAMILY", Title: "Family", Source: "Local"},  // read by source
			{ID: "CAL-OUT", Title: "Outside", Source: "Personal"}, // denied
		},
		Events: []calendar.Event{
			{ID: "EV-1", Title: "1", Calendar: "Home", CalendarID: "CAL-HOME"},
			{ID: "EV-2", Title: "2", Calendar: "Work", CalendarID: "CAL-WORK"},
			{ID: "EV-3", Title: "3", Calendar: "Family", CalendarID: "CAL-FAMILY"},
			{ID: "EV-4", Title: "4", Calendar: "Outside", CalendarID: "CAL-OUT"},
		},
	}
}

func TestScopedCalendar_Calendars_Filters(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	cals, err := sc.Calendars()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(cals))
	for _, c := range cals {
		got = append(got, c.ID)
	}
	want := map[string]bool{"CAL-HOME": true, "CAL-WORK": true, "CAL-FAMILY": true}
	if len(got) != len(want) {
		t.Fatalf("got %d cals (%v), want %d", len(got), got, len(want))
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected calendar %q in output", id)
		}
	}
}

func TestScopedCalendar_Calendar_DeniedReturnsPolicyError(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())

	if _, err := sc.Calendar("CAL-OUT"); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("denied calendar fetch: err = %v, want ErrPolicyDenied", err)
	}
	if c, err := sc.Calendar("CAL-HOME"); err != nil || c.ID != "CAL-HOME" {
		t.Errorf("allowed calendar fetch: c=%v err=%v", c, err)
	}
}

func TestScopedCalendar_Events_FiltersByCalendarMode(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	evs, err := sc.Events(time.Now(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range evs {
		got[e.ID] = true
	}
	want := map[string]bool{"EV-1": true, "EV-2": true, "EV-3": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("missing event %q", id)
		}
	}
}

func TestScopedCalendar_Event_DeniedReturnsPolicyError(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())

	if _, err := sc.Event("EV-4"); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("denied event fetch: err = %v, want ErrPolicyDenied", err)
	}
	// Work is read-only (id override) — read should still succeed.
	if ev, err := sc.Event("EV-2"); err != nil || ev.ID != "EV-2" {
		t.Errorf("read-only allowed event: ev=%v err=%v", ev, err)
	}
}

func TestScopedCalendar_CreateEvent_RequiresCalendarName(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	_, err := sc.CreateEvent(calendar.CreateEventInput{Title: "X"})
	if !errors.Is(err, ErrTargetRequired) {
		t.Errorf("missing calendar: err = %v, want ErrTargetRequired", err)
	}
}

func TestScopedCalendar_CreateEvent_RejectsDenied(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())

	tests := []struct {
		name string
		cal  string
		want error
	}{
		{"unknown_calendar", "Nope", ErrPolicyDenied},
		{"readonly_calendar_by_id", "Work", ErrPolicyDenied},
		{"readonly_calendar_by_source", "Family", ErrPolicyDenied},
		{"denied_calendar", "Outside", ErrPolicyDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sc.CreateEvent(calendar.CreateEventInput{Title: "X", Calendar: tt.cal})
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestScopedCalendar_CreateEvent_SucceedsOnWritable(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	ev, err := sc.CreateEvent(calendar.CreateEventInput{Title: "Standup", Calendar: "Home"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ev.CalendarID != "CAL-HOME" {
		t.Errorf("CalendarID = %q, want CAL-HOME", ev.CalendarID)
	}
}

func TestScopedCalendar_CreateEvent_PostFetchViolation(t *testing.T) {
	// Bridge silently writes to CAL-OUT (denied) instead of the named cal.
	br := fixtureBridge()
	br.CreateOverrideCalID = "CAL-OUT"
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())

	_, err := sc.CreateEvent(calendar.CreateEventInput{Title: "X", Calendar: "Home"})
	if !errors.Is(err, ErrPostFetchScopeViolation) {
		t.Errorf("err = %v, want ErrPostFetchScopeViolation", err)
	}
}

func TestScopedCalendar_UpdateEvent_RejectsReadOnlyExisting(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	title := "Renamed"
	_, err := sc.UpdateEvent("EV-2", calendar.UpdateEventInput{Title: &title}, calendar.SpanThisEvent)
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied (CAL-WORK is read-only)", err)
	}
}

func TestScopedCalendar_UpdateEvent_RejectsMoveToDenied(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	target := "Outside"
	_, err := sc.UpdateEvent("EV-1", calendar.UpdateEventInput{Calendar: &target}, calendar.SpanThisEvent)
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied (CAL-OUT is denied)", err)
	}
}

func TestScopedCalendar_UpdateEvent_AllowsRenameWithinWritable(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	title := "Renamed"
	ev, err := sc.UpdateEvent("EV-1", calendar.UpdateEventInput{Title: &title}, calendar.SpanThisEvent)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ev.Title != "Renamed" {
		t.Errorf("Title = %q, want Renamed", ev.Title)
	}
}

func TestScopedCalendar_DeleteEvent_RejectsReadOnly(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	if err := sc.DeleteEvent("EV-2", calendar.SpanThisEvent); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("delete read-only: err = %v, want ErrPolicyDenied", err)
	}
	if err := sc.DeleteEvent("EV-4", calendar.SpanThisEvent); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("delete denied: err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedCalendar_DeleteEvent_AllowsWritable(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	if err := sc.DeleteEvent("EV-1", calendar.SpanThisEvent); err != nil {
		t.Errorf("delete writable: err = %v", err)
	}
}

func TestScopedCalendar_DeleteEvents_Partitions(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	results := sc.DeleteEvents([]string{"EV-1", "EV-2", "EV-3", "EV-4", "EV-MISSING"}, calendar.SpanThisEvent)

	if results["EV-1"] != nil {
		t.Errorf("EV-1 (writable): err = %v, want nil", results["EV-1"])
	}
	if !errors.Is(results["EV-2"], ErrPolicyDenied) {
		t.Errorf("EV-2 (read-only by id): err = %v, want ErrPolicyDenied", results["EV-2"])
	}
	if !errors.Is(results["EV-3"], ErrPolicyDenied) {
		t.Errorf("EV-3 (read-only by source): err = %v, want ErrPolicyDenied", results["EV-3"])
	}
	if !errors.Is(results["EV-4"], ErrPolicyDenied) {
		t.Errorf("EV-4 (denied): err = %v, want ErrPolicyDenied", results["EV-4"])
	}
	if !errors.Is(results["EV-MISSING"], calendar.ErrNotFound) {
		t.Errorf("EV-MISSING: err = %v, want ErrNotFound", results["EV-MISSING"])
	}
}

func TestScopedCalendar_CreateCalendar_RequiresSource(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	_, err := sc.CreateCalendar(calendar.CreateCalendarInput{Title: "X"})
	if !errors.Is(err, ErrTargetRequired) {
		t.Errorf("err = %v, want ErrTargetRequired", err)
	}
}

func TestScopedCalendar_CreateCalendar_RejectsDeniedSource(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	_, err := sc.CreateCalendar(calendar.CreateCalendarInput{Title: "X", Source: "Personal"})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("denied source: err = %v, want ErrPolicyDenied", err)
	}
	_, err = sc.CreateCalendar(calendar.CreateCalendarInput{Title: "X", Source: "Local"})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("read-only source: err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedCalendar_CreateCalendar_AllowsWritableSource(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	c, err := sc.CreateCalendar(calendar.CreateCalendarInput{Title: "Trips", Source: "iCloud"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if c.Source != "iCloud" || c.Title != "Trips" {
		t.Errorf("returned calendar = %+v", c)
	}
}

func TestScopedCalendar_UpdateCalendar_RejectsReadOnly(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	title := "X"
	_, err := sc.UpdateCalendar("CAL-WORK", calendar.UpdateCalendarInput{Title: &title})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("update read-only-by-id: err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedCalendar_DeleteCalendar_RejectsDenied(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	if err := sc.DeleteCalendar("CAL-OUT"); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("delete denied: err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedCalendar_DeleteCalendar_NotFoundPasses(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	if err := sc.DeleteCalendar("CAL-NOPE"); !errors.Is(err, calendar.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
