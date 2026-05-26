// Package testfakes provides in-memory bridge implementations for tests.
// Exported (not _test.go) so handler tests and integration tests can share
// the same fakes.
package testfakes

import (
	"context"
	"sync"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/reminders"
)

// CalFake is a deterministic in-memory CalendarBridge. The type name avoids
// colliding with the calendar package import.
type CalFake struct {
	Cals   []calendar.Calendar
	Events []calendar.Event

	// CreateOverrideCalID, if non-empty, makes CreateEvent return an event
	// stamped with this calendar id regardless of the input — useful for
	// exercising post-fetch scope verification.
	CreateOverrideCalID string

	// Err, if non-nil, is returned from every fake method. Useful for
	// asserting the HTTP layer's error-mapping behavior on a global
	// "bridge is dead" scenario.
	Err error

	// Per-method error injection. When set, the named method returns the
	// configured error instead of its happy-path result. Use these to
	// drive specific scoped-layer test paths without taking down the
	// whole bridge.
	ErrCalendars      error
	ErrEvents         error
	ErrEvent          error
	ErrCreateEvent    error
	ErrUpdateEvent    error
	ErrDeleteEvent    error
	ErrCreateCalendar error
	ErrUpdateCalendar error
	ErrDeleteCalendar error

	// PanicOn, if non-nil, makes a fake method panic with this value to
	// exercise the recover middleware.
	PanicOn any

	mu    sync.Mutex
	Calls []string
}

func (f *CalFake) record(op string) {
	f.mu.Lock()
	f.Calls = append(f.Calls, op)
	f.mu.Unlock()
	if f.PanicOn != nil {
		panic(f.PanicOn)
	}
}

// firstErr returns the per-method error if set, otherwise the global Err.
// Centralizes the error-injection precedence.
func firstErr(specific, global error) error {
	if specific != nil {
		return specific
	}
	return global
}

func (f *CalFake) Calendars() ([]calendar.Calendar, error) {
	f.record("Calendars")
	if err := firstErr(f.ErrCalendars, f.Err); err != nil {
		return nil, err
	}
	return append([]calendar.Calendar(nil), f.Cals...), nil
}

func (f *CalFake) GetEvents(start, end time.Time, opts ...calendar.ListOption) ([]calendar.Event, error) {
	f.record("Events")
	if err := firstErr(f.ErrEvents, f.Err); err != nil {
		return nil, err
	}
	return append([]calendar.Event(nil), f.Events...), nil
}

func (f *CalFake) Event(id string) (*calendar.Event, error) {
	f.record("Event:" + id)
	if err := firstErr(f.ErrEvent, f.Err); err != nil {
		return nil, err
	}
	for i := range f.Events {
		if f.Events[i].ID == id {
			ev := f.Events[i]
			return &ev, nil
		}
	}
	return nil, calendar.ErrNotFound
}

func (f *CalFake) CreateEvent(input calendar.CreateEventInput) (*calendar.Event, error) {
	f.record("CreateEvent:" + input.Calendar)
	if err := firstErr(f.ErrCreateEvent, f.Err); err != nil {
		return nil, err
	}
	calID := ""
	for _, c := range f.Cals {
		if c.Title == input.Calendar {
			calID = c.ID
		}
	}
	if f.CreateOverrideCalID != "" {
		calID = f.CreateOverrideCalID
	}
	ev := calendar.Event{
		ID:         "EV-NEW-" + input.Title,
		Title:      input.Title,
		StartDate:  input.StartDate,
		EndDate:    input.EndDate,
		Calendar:   input.Calendar,
		CalendarID: calID,
	}
	f.Events = append(f.Events, ev)
	return &ev, nil
}

func (f *CalFake) UpdateEvent(id string, input calendar.UpdateEventInput, span calendar.Span) (*calendar.Event, error) {
	f.record("UpdateEvent:" + id)
	if err := firstErr(f.ErrUpdateEvent, f.Err); err != nil {
		return nil, err
	}
	for i := range f.Events {
		if f.Events[i].ID != id {
			continue
		}
		ev := &f.Events[i]
		if input.Title != nil {
			ev.Title = *input.Title
		}
		if input.Calendar != nil && *input.Calendar != "" {
			for _, c := range f.Cals {
				if c.Title == *input.Calendar {
					ev.Calendar = c.Title
					ev.CalendarID = c.ID
				}
			}
		}
		out := *ev
		return &out, nil
	}
	return nil, calendar.ErrNotFound
}

func (f *CalFake) DeleteEvent(id string, span calendar.Span) error {
	f.record("DeleteEvent:" + id)
	if err := firstErr(f.ErrDeleteEvent, f.Err); err != nil {
		return err
	}
	for i := range f.Events {
		if f.Events[i].ID == id {
			f.Events = append(f.Events[:i], f.Events[i+1:]...)
			return nil
		}
	}
	return calendar.ErrNotFound
}

func (f *CalFake) DeleteEvents(ids []string, span calendar.Span) map[string]error {
	out := make(map[string]error, len(ids))
	for _, id := range ids {
		out[id] = f.DeleteEvent(id, span)
	}
	return out
}

func (f *CalFake) CreateCalendar(input calendar.CreateCalendarInput) (*calendar.Calendar, error) {
	f.record("CreateCalendar:" + input.Source)
	if err := firstErr(f.ErrCreateCalendar, f.Err); err != nil {
		return nil, err
	}
	c := calendar.Calendar{
		ID:     "CAL-NEW-" + input.Title,
		Title:  input.Title,
		Source: input.Source,
		Color:  input.Color,
	}
	f.Cals = append(f.Cals, c)
	return &c, nil
}

func (f *CalFake) UpdateCalendar(id string, input calendar.UpdateCalendarInput) (*calendar.Calendar, error) {
	f.record("UpdateCalendar:" + id)
	if err := firstErr(f.ErrUpdateCalendar, f.Err); err != nil {
		return nil, err
	}
	for i := range f.Cals {
		if f.Cals[i].ID != id {
			continue
		}
		c := &f.Cals[i]
		if input.Title != nil {
			c.Title = *input.Title
		}
		if input.Color != nil {
			c.Color = *input.Color
		}
		out := *c
		return &out, nil
	}
	return nil, calendar.ErrNotFound
}

func (f *CalFake) DeleteCalendar(id string) error {
	f.record("DeleteCalendar:" + id)
	if err := firstErr(f.ErrDeleteCalendar, f.Err); err != nil {
		return err
	}
	for i := range f.Cals {
		if f.Cals[i].ID == id {
			f.Cals = append(f.Cals[:i], f.Cals[i+1:]...)
			return nil
		}
	}
	return calendar.ErrNotFound
}

func (f *CalFake) WatchChanges(ctx context.Context) (<-chan struct{}, error) { return nil, nil }

// DefaultCalendar returns the first non-readonly calendar in the fake's
// list as the "default", or nil if no writable cal exists.
func (f *CalFake) DefaultCalendar() (*calendar.Calendar, error) {
	f.record("DefaultCalendar")
	if f.Err != nil {
		return nil, f.Err
	}
	for i := range f.Cals {
		if !f.Cals[i].ReadOnly {
			c := f.Cals[i]
			return &c, nil
		}
	}
	return nil, nil
}

// CalAdapter exposes a *CalFake under the method names required by
// server.CalendarBridge. The "Events" name collides with the field on
// CalFake, so the adapter renames it.
type CalAdapter struct{ *CalFake }

// NewCalendar wraps a *CalFake into a server.CalendarBridge.
func NewCalendar(f *CalFake) *CalAdapter { return &CalAdapter{f} }

func (a *CalAdapter) Events(start, end time.Time, opts ...calendar.ListOption) ([]calendar.Event, error) {
	return a.CalFake.GetEvents(start, end, opts...)
}

// ---- Reminders fake ----

// RemFake is a deterministic in-memory RemindersBridge.
type RemFake struct {
	Lists []reminders.List
	Items []reminders.Reminder

	CreateOverrideListID string

	// Global + per-method error injection (see CalFake docs).
	Err                   error
	ErrLists              error
	ErrReminders          error
	ErrReminder           error
	ErrCreateReminder     error
	ErrUpdateReminder     error
	ErrDeleteReminder     error
	ErrCompleteReminder   error
	ErrUncompleteReminder error
	ErrCreateList         error
	ErrUpdateList         error
	ErrDeleteList         error

	PanicOn any

	mu    sync.Mutex
	Calls []string
}

func (f *RemFake) record(op string) {
	f.mu.Lock()
	f.Calls = append(f.Calls, op)
	f.mu.Unlock()
	if f.PanicOn != nil {
		panic(f.PanicOn)
	}
}

func (f *RemFake) GetLists() ([]reminders.List, error) {
	f.record("Lists")
	if err := firstErr(f.ErrLists, f.Err); err != nil {
		return nil, err
	}
	return append([]reminders.List(nil), f.Lists...), nil
}

func (f *RemFake) GetReminders(opts ...reminders.ListOption) ([]reminders.Reminder, error) {
	f.record("Reminders")
	if err := firstErr(f.ErrReminders, f.Err); err != nil {
		return nil, err
	}
	return append([]reminders.Reminder(nil), f.Items...), nil
}

func (f *RemFake) Reminder(id string) (*reminders.Reminder, error) {
	f.record("Reminder:" + id)
	if err := firstErr(f.ErrReminder, f.Err); err != nil {
		return nil, err
	}
	for i := range f.Items {
		if f.Items[i].ID == id {
			r := f.Items[i]
			return &r, nil
		}
	}
	return nil, reminders.ErrNotFound
}

func (f *RemFake) CreateReminder(input reminders.CreateReminderInput) (*reminders.Reminder, error) {
	f.record("CreateReminder:" + input.ListName)
	if err := firstErr(f.ErrCreateReminder, f.Err); err != nil {
		return nil, err
	}
	listID := ""
	for _, l := range f.Lists {
		if l.Title == input.ListName {
			listID = l.ID
		}
	}
	if f.CreateOverrideListID != "" {
		listID = f.CreateOverrideListID
	}
	r := reminders.Reminder{
		ID:     "REM-NEW-" + input.Title,
		Title:  input.Title,
		List:   input.ListName,
		ListID: listID,
	}
	f.Items = append(f.Items, r)
	return &r, nil
}

func (f *RemFake) UpdateReminder(id string, input reminders.UpdateReminderInput) (*reminders.Reminder, error) {
	f.record("UpdateReminder:" + id)
	if err := firstErr(f.ErrUpdateReminder, f.Err); err != nil {
		return nil, err
	}
	for i := range f.Items {
		if f.Items[i].ID != id {
			continue
		}
		r := &f.Items[i]
		if input.Title != nil {
			r.Title = *input.Title
		}
		if input.ListName != nil && *input.ListName != "" {
			for _, l := range f.Lists {
				if l.Title == *input.ListName {
					r.List = l.Title
					r.ListID = l.ID
				}
			}
		}
		out := *r
		return &out, nil
	}
	return nil, reminders.ErrNotFound
}

func (f *RemFake) DeleteReminder(id string) error {
	f.record("DeleteReminder:" + id)
	if err := firstErr(f.ErrDeleteReminder, f.Err); err != nil {
		return err
	}
	for i := range f.Items {
		if f.Items[i].ID == id {
			f.Items = append(f.Items[:i], f.Items[i+1:]...)
			return nil
		}
	}
	return reminders.ErrNotFound
}

func (f *RemFake) DeleteReminders(ids []string) map[string]error {
	out := make(map[string]error, len(ids))
	for _, id := range ids {
		out[id] = f.DeleteReminder(id)
	}
	return out
}

func (f *RemFake) CompleteReminder(id string) (*reminders.Reminder, error) {
	f.record("CompleteReminder:" + id)
	if err := firstErr(f.ErrCompleteReminder, f.Err); err != nil {
		return nil, err
	}
	for i := range f.Items {
		if f.Items[i].ID == id {
			f.Items[i].Completed = true
			r := f.Items[i]
			return &r, nil
		}
	}
	return nil, reminders.ErrNotFound
}

func (f *RemFake) UncompleteReminder(id string) (*reminders.Reminder, error) {
	f.record("UncompleteReminder:" + id)
	if err := firstErr(f.ErrUncompleteReminder, f.Err); err != nil {
		return nil, err
	}
	for i := range f.Items {
		if f.Items[i].ID == id {
			f.Items[i].Completed = false
			r := f.Items[i]
			return &r, nil
		}
	}
	return nil, reminders.ErrNotFound
}

func (f *RemFake) CreateList(input reminders.CreateListInput) (*reminders.List, error) {
	f.record("CreateList:" + input.Source)
	if err := firstErr(f.ErrCreateList, f.Err); err != nil {
		return nil, err
	}
	l := reminders.List{
		ID:     "LIST-NEW-" + input.Title,
		Title:  input.Title,
		Source: input.Source,
	}
	f.Lists = append(f.Lists, l)
	return &l, nil
}

func (f *RemFake) UpdateList(id string, input reminders.UpdateListInput) (*reminders.List, error) {
	f.record("UpdateList:" + id)
	if err := firstErr(f.ErrUpdateList, f.Err); err != nil {
		return nil, err
	}
	for i := range f.Lists {
		if f.Lists[i].ID != id {
			continue
		}
		l := &f.Lists[i]
		if input.Title != nil {
			l.Title = *input.Title
		}
		if input.Color != nil {
			l.Color = *input.Color
		}
		out := *l
		return &out, nil
	}
	return nil, reminders.ErrNotFound
}

func (f *RemFake) DeleteList(id string) error {
	f.record("DeleteList:" + id)
	if err := firstErr(f.ErrDeleteList, f.Err); err != nil {
		return err
	}
	for i := range f.Lists {
		if f.Lists[i].ID == id {
			f.Lists = append(f.Lists[:i], f.Lists[i+1:]...)
			return nil
		}
	}
	return reminders.ErrNotFound
}

func (f *RemFake) WatchChanges(ctx context.Context) (<-chan struct{}, error) { return nil, nil }

// DefaultList returns the first non-readonly list as the "default".
func (f *RemFake) DefaultList() (*reminders.List, error) {
	f.record("DefaultList")
	for i := range f.Lists {
		if !f.Lists[i].ReadOnly {
			l := f.Lists[i]
			return &l, nil
		}
	}
	return nil, nil
}

// RemAdapter exposes a *RemFake under the method names required by
// server.RemindersBridge. "Lists" and "Reminders" collide with the
// struct fields and are renamed on the adapter.
type RemAdapter struct{ *RemFake }

// NewReminders wraps a *RemFake into a server.RemindersBridge.
func NewReminders(f *RemFake) *RemAdapter { return &RemAdapter{f} }

func (a *RemAdapter) Lists() ([]reminders.List, error) {
	return a.RemFake.GetLists()
}

func (a *RemAdapter) Reminders(opts ...reminders.ListOption) ([]reminders.Reminder, error) {
	return a.RemFake.GetReminders(opts...)
}
