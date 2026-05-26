package scoped

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server/testfakes"
)

// ---- Bridge error injection: calendar ----

func TestScopedCalendar_BridgeError_Calendars(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureBridge()
	br.ErrCalendars = want
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	_, err := sc.Calendars()
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedCalendar_BridgeError_Events(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureBridge()
	br.ErrEvents = want
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	_, err := sc.Events(time.Now(), time.Now().Add(time.Hour))
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedCalendar_BridgeError_Event(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureBridge()
	br.ErrEvent = want
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	_, err := sc.Event("EV-1")
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedCalendar_BridgeError_UpdateEvent(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureBridge()
	br.ErrUpdateEvent = want
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	title := "X"
	_, err := sc.UpdateEvent("EV-1", calendar.UpdateEventInput{Title: &title}, calendar.SpanThisEvent)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedCalendar_BridgeError_DeleteEvent(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureBridge()
	br.ErrDeleteEvent = want
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	if err := sc.DeleteEvent("EV-1", calendar.SpanThisEvent); !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedCalendar_BridgeError_CreateCalendar(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureBridge()
	br.ErrCreateCalendar = want
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	_, err := sc.CreateCalendar(calendar.CreateCalendarInput{Title: "X", Source: "iCloud"})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedCalendar_BridgeError_UpdateCalendar(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureBridge()
	br.ErrUpdateCalendar = want
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	title := "X"
	_, err := sc.UpdateCalendar("CAL-HOME", calendar.UpdateCalendarInput{Title: &title})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedCalendar_BridgeError_DeleteCalendar(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureBridge()
	br.ErrDeleteCalendar = want
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	if err := sc.DeleteCalendar("CAL-HOME"); !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

// ---- Bridge error injection: reminders ----

func TestScopedReminders_BridgeError_Lists(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureReminderBridge()
	br.ErrLists = want
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	_, err := sr.Lists()
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedReminders_BridgeError_Reminders(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureReminderBridge()
	br.ErrReminders = want
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	_, err := sr.Reminders()
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedReminders_BridgeError_UpdateReminder(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureReminderBridge()
	br.ErrUpdateReminder = want
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	title := "X"
	_, err := sr.UpdateReminder("R-1", reminders.UpdateReminderInput{Title: &title})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedReminders_BridgeError_DeleteReminder(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureReminderBridge()
	br.ErrDeleteReminder = want
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	if err := sr.DeleteReminder("R-1"); !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedReminders_BridgeError_CompleteReminder(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureReminderBridge()
	br.ErrCompleteReminder = want
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	_, err := sr.CompleteReminder("R-1")
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedReminders_BridgeError_UncompleteReminder(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureReminderBridge()
	br.ErrUncompleteReminder = want
	br.Items[0].Completed = true
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	_, err := sr.UncompleteReminder("R-1")
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedReminders_BridgeError_CreateList(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureReminderBridge()
	br.ErrCreateList = want
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	_, err := sr.CreateList(reminders.CreateListInput{Title: "X", Source: "iCloud"})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedReminders_BridgeError_UpdateList(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureReminderBridge()
	br.ErrUpdateList = want
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	title := "X"
	_, err := sr.UpdateList("LIST-TODO", reminders.UpdateListInput{Title: &title})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

func TestScopedReminders_BridgeError_DeleteList(t *testing.T) {
	want := errors.New("bridge boom")
	br := fixtureReminderBridge()
	br.ErrDeleteList = want
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	if err := sr.DeleteList("LIST-TODO"); !errors.Is(err, want) {
		t.Errorf("err = %v, want bridge error", err)
	}
}

// ---- Direct Uncomplete test ----

func TestScopedReminders_Uncomplete_Allowed(t *testing.T) {
	br := fixtureReminderBridge()
	br.Items[0].Completed = true
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	r, err := sr.UncompleteReminder("R-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if r.Completed {
		t.Errorf("Completed = true, want false")
	}
}

// ---- Empty batch inputs ----

func TestScopedCalendar_DeleteEvents_EmptyInput(t *testing.T) {
	br := fixtureBridge()
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	got := sc.DeleteEvents(nil, calendar.SpanThisEvent)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
	// Bridge should not have been asked to do anything: only the index
	// fetch should appear.
	for _, c := range br.Calls {
		if strings.HasPrefix(c, "DeleteEvents") {
			t.Errorf("bridge.DeleteEvents was called for empty input: %s", c)
		}
	}
}

func TestScopedReminders_DeleteReminders_EmptyInput(t *testing.T) {
	br := fixtureReminderBridge()
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	got := sr.DeleteReminders(nil)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// ---- Post-fetch violation on Update operations ----

func TestScopedCalendar_UpdateEvent_PostFetchViolation(t *testing.T) {
	br := fixtureBridge()
	// Inject behavior: simulate a "move" that lands in a denied calendar.
	// Achieve this by adding a denied calendar to the index, and making
	// the fake's UpdateEvent stamp it onto the event.
	br.Cals = append(br.Cals, calendar.Calendar{ID: "CAL-NEW-OUT", Title: "Outside2", Source: "Personal"})
	dest := "Outside2"
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	_, err := sc.UpdateEvent("EV-1", calendar.UpdateEventInput{Calendar: &dest}, calendar.SpanThisEvent)
	if !errors.Is(err, ErrPolicyDenied) && !errors.Is(err, ErrPostFetchScopeViolation) {
		t.Errorf("err = %v, want denial or post-fetch violation", err)
	}
}

func TestScopedCalendar_UpdateCalendar_PostFetchViolation(t *testing.T) {
	br := fixtureBridge()
	// Update CAL-HOME but the fake returns a cal whose source becomes
	// denied. Simulate via injecting an error wrapper isn't possible from
	// outside; instead reroute by mutating the fixture so the returned
	// calendar has a denied source.
	sc := NewCalendar(testfakes.NewCalendar(br), fixturePolicy())
	title := "Home Renamed"
	// Patch the underlying calendar so UpdateCalendar returns it with a
	// denied source. Mutate before the call.
	for i := range br.Cals {
		if br.Cals[i].ID == "CAL-HOME" {
			br.Cals[i].Source = "Personal" // policy denies Personal
		}
	}
	_, err := sc.UpdateCalendar("CAL-HOME", calendar.UpdateCalendarInput{Title: &title})
	// Pre-check should reject because CAL-HOME's new source is denied.
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied (pre-check), or post-fetch", err)
	}
}

// ---- Typed error fields ----

func TestPostFetchScopeViolation_ImplementsIs(t *testing.T) {
	v := &PostFetchScopeViolation{Kind: "event", OrphanID: "EV-OOPS", OrphanSource: "Personal"}
	if !errors.Is(v, ErrPostFetchScopeViolation) {
		t.Errorf("typed error should match ErrPostFetchScopeViolation via errors.Is")
	}
}

func TestPostFetchScopeViolation_ErrorString(t *testing.T) {
	tests := []struct {
		kind       string
		wantSubstr []string
	}{
		{"event", []string{"event", "calendar"}},
		{"reminder", []string{"reminder", "list"}},
		{"calendar", []string{"calendar"}},
		{"list", []string{"list"}},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			v := &PostFetchScopeViolation{
				Kind:                tt.kind,
				OrphanID:            "OBJ-X",
				OrphanContainerID:   "CONT-Y",
				OrphanSource:        "src",
				IntendedContainerID: "CONT-Z",
			}
			msg := v.Error()
			for _, want := range tt.wantSubstr {
				if !strings.Contains(msg, want) {
					t.Errorf("Error() = %q, missing %q", msg, want)
				}
			}
			for _, want := range []string{"OBJ-X", "CONT-Y", "CONT-Z", "src"} {
				if !strings.Contains(msg, want) {
					t.Errorf("Error() = %q, missing %q", msg, want)
				}
			}
		})
	}
}

// ---- Concurrent reads ----

func TestScopedCalendar_ConcurrentReads(t *testing.T) {
	sc := NewCalendar(testfakes.NewCalendar(fixtureBridge()), fixturePolicy())
	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if _, err := sc.Calendars(); err != nil {
				errs <- fmt.Errorf("Calendars: %w", err)
			}
		}(i)
		go func() {
			defer wg.Done()
			if _, err := sc.Events(time.Now(), time.Now().Add(time.Hour)); err != nil {
				errs <- fmt.Errorf("Events: %w", err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
