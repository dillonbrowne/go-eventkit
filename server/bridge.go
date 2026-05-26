package server

import (
	"context"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/reminders"
)

// CalendarBridge is the subset of [calendar.Client] the server uses.
// The interface exists so handlers and scoped wrappers can be unit-tested
// against fakes without going through cgo or EventKit.
//
// *calendar.Client satisfies this interface implicitly; the compile-time
// assertion in server.go enforces that the surfaces stay in sync.
type CalendarBridge interface {
	Calendars() ([]calendar.Calendar, error)
	Events(start, end time.Time, opts ...calendar.ListOption) ([]calendar.Event, error)
	Event(id string) (*calendar.Event, error)
	CreateEvent(input calendar.CreateEventInput) (*calendar.Event, error)
	UpdateEvent(id string, input calendar.UpdateEventInput, span calendar.Span) (*calendar.Event, error)
	DeleteEvent(id string, span calendar.Span) error
	DeleteEvents(ids []string, span calendar.Span) map[string]error
	CreateCalendar(input calendar.CreateCalendarInput) (*calendar.Calendar, error)
	UpdateCalendar(id string, input calendar.UpdateCalendarInput) (*calendar.Calendar, error)
	DeleteCalendar(id string) error
	WatchChanges(ctx context.Context) (<-chan struct{}, error)
	DefaultCalendar() (*calendar.Calendar, error)
}

// RemindersBridge is the subset of [reminders.Client] the server uses.
type RemindersBridge interface {
	Lists() ([]reminders.List, error)
	Reminders(opts ...reminders.ListOption) ([]reminders.Reminder, error)
	Reminder(id string) (*reminders.Reminder, error)
	CreateReminder(input reminders.CreateReminderInput) (*reminders.Reminder, error)
	UpdateReminder(id string, input reminders.UpdateReminderInput) (*reminders.Reminder, error)
	DeleteReminder(id string) error
	DeleteReminders(ids []string) map[string]error
	CompleteReminder(id string) (*reminders.Reminder, error)
	UncompleteReminder(id string) (*reminders.Reminder, error)
	CreateList(input reminders.CreateListInput) (*reminders.List, error)
	UpdateList(id string, input reminders.UpdateListInput) (*reminders.List, error)
	DeleteList(id string) error
	WatchChanges(ctx context.Context) (<-chan struct{}, error)
	DefaultList() (*reminders.List, error)
}

// Compile-time guard: if calendar.Client or reminders.Client drift away from
// the bridge interfaces, the build breaks at this site instead of at runtime.
var (
	_ CalendarBridge  = (*calendar.Client)(nil)
	_ RemindersBridge = (*reminders.Client)(nil)
)
