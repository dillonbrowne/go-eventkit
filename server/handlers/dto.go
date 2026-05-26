package handlers

import (
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/reminders"
)

// SpanValue is the string-typed representation of [calendar.Span] used in
// REST query parameters. Internally maps to SpanThisEvent / SpanFutureEvents.
type SpanValue string

const (
	SpanThis   SpanValue = "this"
	SpanFuture SpanValue = "future"
)

func (s SpanValue) ToCalendar() calendar.Span {
	if s == SpanFuture {
		return calendar.SpanFutureEvents
	}
	return calendar.SpanThisEvent
}

// ListCalendarsResponse wraps the calendars list returned to clients.
type ListCalendarsResponse struct {
	Body struct {
		Calendars []calendar.Calendar `json:"calendars"`
	}
}

// CalendarResponse wraps a single calendar.
type CalendarResponse struct {
	Body calendar.Calendar
}

// CreateCalendarRequest wraps the input body.
type CreateCalendarRequest struct {
	Body struct {
		Title  string `json:"title" required:"true" doc:"Display name for the new calendar"`
		Source string `json:"source" required:"true" doc:"Account source name (e.g. 'iCloud'). Must be writable under the loaded policy."`
		Color  string `json:"color,omitempty" doc:"Hex color (e.g. '#FF6961')"`
	}
}

// UpdateCalendarRequest wraps the PATCH body for a calendar. Pointer fields
// signal "only update if provided".
type UpdateCalendarRequest struct {
	Body struct {
		Title *string `json:"title,omitempty" doc:"New display name"`
		Color *string `json:"color,omitempty" doc:"New hex color"`
	}
}

// ListEventsParams captures the query parameters for GET /v1/events.
type ListEventsParams struct {
	Start      time.Time `query:"start" required:"true" doc:"Inclusive lower bound (RFC 3339)"`
	End        time.Time `query:"end" required:"true" doc:"Inclusive upper bound (RFC 3339)"`
	Calendar   string    `query:"calendar" doc:"Filter by calendar name"`
	CalendarID string    `query:"calendar_id" doc:"Filter by calendar identifier"`
	Search     string    `query:"search" doc:"Filter by substring in title/location/notes"`
}

// ListEventsResponse wraps the events list.
type ListEventsResponse struct {
	Body struct {
		Events []calendar.Event `json:"events"`
	}
}

// EventResponse wraps a single event.
type EventResponse struct {
	Body calendar.Event
}

// CreateEventBody is the JSON body for POST /v1/events. Defined here
// (rather than reusing calendar.CreateEventInput directly) so the omitempty
// tags match huma's "required" inference and the OpenAPI spec lists only
// genuinely required fields.
type CreateEventBody struct {
	Title                 string                    `json:"title" required:"true"`
	StartDate             time.Time                 `json:"startDate" required:"true"`
	EndDate               time.Time                 `json:"endDate" required:"true"`
	Calendar              string                    `json:"calendar" required:"true" doc:"Target calendar name; must be writable under the loaded policy"`
	AllDay                bool                      `json:"allDay,omitempty"`
	Location              string                    `json:"location,omitempty"`
	Notes                 string                    `json:"notes,omitempty"`
	URL                   string                    `json:"url,omitempty"`
	Alerts                []calendar.Alert          `json:"alerts,omitempty"`
	SuppressDefaultAlarms bool                      `json:"suppressDefaultAlarms,omitempty"`
	TimeZone              string                    `json:"timeZone,omitempty"`
	RecurrenceRules       []reminderRecurrenceProxy `json:"recurrenceRules,omitempty"`
	StructuredLocation    *structuredLocationProxy  `json:"structuredLocation,omitempty"`
}

// CreateEventRequest wraps the create-event body.
type CreateEventRequest struct {
	Body CreateEventBody
}

// UpdateEventBody is the JSON body for PATCH /v1/events/{id}. All fields
// are optional; pointer fields signal "leave unchanged if nil".
type UpdateEventBody struct {
	Title              *string                    `json:"title,omitempty"`
	StartDate          *time.Time                 `json:"startDate,omitempty"`
	EndDate            *time.Time                 `json:"endDate,omitempty"`
	AllDay             *bool                      `json:"allDay,omitempty"`
	Location           *string                    `json:"location,omitempty"`
	Notes              *string                    `json:"notes,omitempty"`
	URL                *string                    `json:"url,omitempty"`
	Calendar           *string                    `json:"calendar,omitempty"`
	Alerts             *[]calendar.Alert          `json:"alerts,omitempty"`
	TimeZone           *string                    `json:"timeZone,omitempty"`
	RecurrenceRules    *[]reminderRecurrenceProxy `json:"recurrenceRules,omitempty"`
	StructuredLocation *structuredLocationProxy   `json:"structuredLocation,omitempty"`
}

// UpdateEventRequest wraps the update-event body + span query.
type UpdateEventRequest struct {
	ID   string    `path:"id"`
	Span SpanValue `query:"span" enum:"this,future" default:"this" doc:"Whether the update affects this occurrence only or this and all future occurrences"`
	Body UpdateEventBody
}

// DeleteEventInput captures the path id and span query.
type DeleteEventInput struct {
	ID   string    `path:"id"`
	Span SpanValue `query:"span" enum:"this,future" default:"this"`
}

// BatchDeleteEventsRequest wraps the body.
type BatchDeleteEventsRequest struct {
	Body struct {
		IDs  []string  `json:"ids" required:"true" minItems:"1" doc:"Event identifiers to delete"`
		Span SpanValue `json:"span,omitempty" enum:"this,future" default:"this"`
	}
}

// BatchDeleteResponse reports per-ID outcomes.
type BatchDeleteResponse struct {
	Body struct {
		Results map[string]string `json:"results" doc:"Maps target id → 'ok' or an error message"`
	}
}

// ListsResponse wraps the reminder lists.
type ListsResponse struct {
	Body struct {
		Lists []reminders.List `json:"lists"`
	}
}

// ListResponse wraps a single list.
type ListResponse struct {
	Body reminders.List
}

// CreateListRequest wraps the create-list body.
type CreateListRequest struct {
	Body struct {
		Title  string `json:"title" required:"true"`
		Source string `json:"source" required:"true"`
		Color  string `json:"color,omitempty"`
	}
}

// UpdateListRequest wraps the PATCH body for a list.
type UpdateListRequest struct {
	Body struct {
		Title *string `json:"title,omitempty"`
		Color *string `json:"color,omitempty"`
	}
}

// ListRemindersParams captures the query parameters for GET /v1/reminders.
// huma does not permit pointer-typed query parameters, so the optional
// completion filter is a string enum with the empty value meaning "no
// filter". Date-range parameters use the time.Time zero value as "no filter".
type ListRemindersParams struct {
	List      string    `query:"list" doc:"Filter by list name"`
	ListID    string    `query:"list_id" doc:"Filter by list identifier"`
	Completed string    `query:"completed" enum:",true,false" doc:"'true' for completed only, 'false' for incomplete only, omit for no filter"`
	Search    string    `query:"search" doc:"Substring match against title/notes"`
	DueBefore time.Time `query:"due_before" doc:"RFC 3339; omit for no filter"`
	DueAfter  time.Time `query:"due_after" doc:"RFC 3339; omit for no filter"`
}

// ListRemindersResponse wraps the reminders list.
type ListRemindersResponse struct {
	Body struct {
		Reminders []reminders.Reminder `json:"reminders"`
	}
}

// ReminderResponse wraps a single reminder.
type ReminderResponse struct {
	Body reminders.Reminder
}

// CreateReminderRequest wraps the create-reminder body. The Go input type
// uses non-tagged fields, so a small mirror is defined here with JSON tags
// to control the wire shape.
type CreateReminderRequest struct {
	Body struct {
		Title           string                    `json:"title" required:"true"`
		Notes           string                    `json:"notes,omitempty"`
		ListName        string                    `json:"list" required:"true" doc:"Target list name. Must be writable under the loaded policy."`
		DueDate         *time.Time                `json:"due_date,omitempty"`
		RemindMeDate    *time.Time                `json:"remind_me_date,omitempty"`
		Priority        reminders.Priority        `json:"priority,omitempty"`
		URL             string                    `json:"url,omitempty"`
		Flagged         bool                      `json:"flagged,omitempty"`
		Alarms          []reminders.Alarm         `json:"alarms,omitempty"`
		RecurrenceRules []reminderRecurrenceProxy `json:"recurrence_rules,omitempty"`
	}
}

// UpdateReminderRequest wraps the PATCH body for a reminder.
type UpdateReminderRequest struct {
	Body struct {
		Title           *string                    `json:"title,omitempty"`
		Notes           *string                    `json:"notes,omitempty"`
		ListName        *string                    `json:"list,omitempty"`
		DueDate         *time.Time                 `json:"due_date,omitempty"`
		ClearDueDate    bool                       `json:"clear_due_date,omitempty"`
		RemindMeDate    *time.Time                 `json:"remind_me_date,omitempty"`
		Priority        *reminders.Priority        `json:"priority,omitempty"`
		Completed       *bool                      `json:"completed,omitempty"`
		Flagged         *bool                      `json:"flagged,omitempty"`
		URL             *string                    `json:"url,omitempty"`
		Alarms          *[]reminders.Alarm         `json:"alarms,omitempty"`
		RecurrenceRules *[]reminderRecurrenceProxy `json:"recurrence_rules,omitempty"`
	}
}

// BatchDeleteRemindersRequest wraps the body.
type BatchDeleteRemindersRequest struct {
	Body struct {
		IDs []string `json:"ids" required:"true" minItems:"1"`
	}
}
