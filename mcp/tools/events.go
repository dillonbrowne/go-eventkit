package tools

import (
	"context"
	"errors"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dillonbrowne/go-eventkit/mcp/client"
)

// ---- list_events ----

type ListEventsInput struct {
	Start      string `json:"start" jsonschema:"Inclusive start of the time range. Accepts ISO 8601 or natural language (e.g. tomorrow 2pm, next friday)"`
	End        string `json:"end" jsonschema:"Inclusive end of the time range."`
	Calendar   string `json:"calendar,omitempty" jsonschema:"Filter by calendar name (case-insensitive)"`
	CalendarID string `json:"calendarID,omitempty" jsonschema:"Filter by calendar identifier"`
	Search     string `json:"search,omitempty" jsonschema:"Substring match against title / location / notes"`
}

type ListEventsOutput struct {
	Events []RedactedEvent `json:"events"`
}

func registerListEvents(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "list_events",
		Title:       "List events",
		Description: "List calendar events in a time range. " + DateDoc + " " + UserDataNotice,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListEventsInput) (*mcp.CallToolResult, ListEventsOutput, error) {
		start, err := ParseDateRequired("start", in.Start)
		if err != nil {
			return nil, ListEventsOutput{}, err
		}
		end, err := ParseDateRequired("end", in.End)
		if err != nil {
			return nil, ListEventsOutput{}, err
		}
		events, err := c.ListEvents(ctx, client.ListEventsInput{
			Start: start, End: end, Calendar: in.Calendar, CalendarID: in.CalendarID, Search: in.Search,
		})
		if err != nil {
			return nil, ListEventsOutput{}, err
		}
		return nil, ListEventsOutput{Events: RedactEvents(events)}, nil
	})
}

// ---- get_event ----

type GetEventInput struct {
	ID string `json:"id" jsonschema:"Event identifier"`
}

type GetEventOutput struct {
	Event RedactedEvent `json:"event"`
}

func registerGetEvent(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "get_event",
		Title:       "Get event",
		Description: "Fetch one event by ID. " + UserDataNotice,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetEventInput) (*mcp.CallToolResult, GetEventOutput, error) {
		if in.ID == "" {
			return nil, GetEventOutput{}, errors.New("id is required")
		}
		ev, err := c.GetEvent(ctx, in.ID)
		if err != nil {
			return nil, GetEventOutput{}, err
		}
		return nil, GetEventOutput{Event: RedactEvent(*ev)}, nil
	})
}

// ---- create_event ----

type CreateEventInput struct {
	Title     string `json:"title" jsonschema:"Event title"`
	StartDate string `json:"startDate" jsonschema:"When the event begins. Accepts ISO 8601 or natural language."`
	EndDate   string `json:"endDate" jsonschema:"When the event ends."`
	Calendar  string `json:"calendar" jsonschema:"Target calendar name (from list_calendars). Must be writable under the active policy."`
	AllDay    bool   `json:"allDay,omitempty" jsonschema:"All-day event (use date-only for start/end)"`
	Location  string `json:"location,omitempty"`
	Notes     string `json:"notes,omitempty"`
	URL       string `json:"url,omitempty"`
	TimeZone  string `json:"timeZone,omitempty" jsonschema:"IANA timezone, e.g. America/New_York. Omit for the user's default."`
}

type CreateEventOutput struct {
	Event RedactedEvent `json:"event"`
}

func registerCreateEvent(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "create_event",
		Title:       "Create event",
		Description: "Create a calendar event. " + DateDoc,
		Annotations: nonDestructiveWrite(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateEventInput) (*mcp.CallToolResult, CreateEventOutput, error) {
		start, err := ParseDateRequired("startDate", in.StartDate)
		if err != nil {
			return nil, CreateEventOutput{}, err
		}
		end, err := ParseDateRequired("endDate", in.EndDate)
		if err != nil {
			return nil, CreateEventOutput{}, err
		}
		ev, err := c.CreateEvent(ctx, client.CreateEventInput{
			Title: in.Title, StartDate: start, EndDate: end, Calendar: in.Calendar,
			AllDay: in.AllDay, Location: in.Location, Notes: in.Notes, URL: in.URL, TimeZone: in.TimeZone,
		})
		if err != nil {
			return nil, CreateEventOutput{}, err
		}
		return nil, CreateEventOutput{Event: RedactEvent(*ev)}, nil
	})
}

// ---- update_event ----

type UpdateEventInput struct {
	ID        string `json:"id" jsonschema:"Event identifier"`
	Span      string `json:"span,omitempty" jsonschema:"For recurring events: 'this' (default) or 'future'"`
	Title     string `json:"title,omitempty"`
	StartDate string `json:"startDate,omitempty" jsonschema:"New start. Accepts ISO 8601 or natural language."`
	EndDate   string `json:"endDate,omitempty"`
	Calendar  string `json:"calendar,omitempty" jsonschema:"Move to a different calendar"`
	Location  string `json:"location,omitempty"`
	Notes     string `json:"notes,omitempty"`
	URL       string `json:"url,omitempty"`
	TimeZone  string `json:"timeZone,omitempty"`
}

type UpdateEventOutput struct {
	Event RedactedEvent `json:"event"`
}

func registerUpdateEvent(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "update_event",
		Title:       "Update event",
		Description: "Patch an event. Only fields you set are changed. " + DateDoc,
		Annotations: idempotentWrite(),
		InputSchema: withEnum(strictInput[UpdateEventInput](), "span", "this", "future"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateEventInput) (*mcp.CallToolResult, UpdateEventOutput, error) {
		if in.ID == "" {
			return nil, UpdateEventOutput{}, errors.New("id is required")
		}
		body := client.UpdateEventInput{}
		if in.Title != "" {
			body.Title = &in.Title
		}
		if in.StartDate != "" {
			t, err := ParseDate(in.StartDate)
			if err != nil {
				return nil, UpdateEventOutput{}, err
			}
			body.StartDate = &t
		}
		if in.EndDate != "" {
			t, err := ParseDate(in.EndDate)
			if err != nil {
				return nil, UpdateEventOutput{}, err
			}
			body.EndDate = &t
		}
		if in.Calendar != "" {
			body.Calendar = &in.Calendar
		}
		if in.Location != "" {
			body.Location = &in.Location
		}
		if in.Notes != "" {
			body.Notes = &in.Notes
		}
		if in.URL != "" {
			body.URL = &in.URL
		}
		if in.TimeZone != "" {
			body.TimeZone = &in.TimeZone
		}
		ev, err := c.UpdateEvent(ctx, in.ID, in.Span, body)
		if err != nil {
			return nil, UpdateEventOutput{}, err
		}
		return nil, UpdateEventOutput{Event: RedactEvent(*ev)}, nil
	})
}

// ---- delete_event ----

type DeleteEventInput struct {
	ID   string `json:"id" jsonschema:"Event identifier"`
	Span string `json:"span,omitempty" jsonschema:"For recurring events: 'this' (default) or 'future'"`
}

type DeleteEventOutput struct {
	Deleted bool `json:"deleted"`
}

func registerDeleteEvent(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "delete_event",
		Title:       "Delete event",
		Description: "Permanently delete an event. Destructive — confirm with the user before calling.",
		Annotations: destructive(),
		InputSchema: withEnum(strictInput[DeleteEventInput](), "span", "this", "future"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DeleteEventInput) (*mcp.CallToolResult, DeleteEventOutput, error) {
		if in.ID == "" {
			return nil, DeleteEventOutput{}, errors.New("id is required")
		}
		if err := c.DeleteEvent(ctx, in.ID, in.Span); err != nil {
			return nil, DeleteEventOutput{}, err
		}
		return nil, DeleteEventOutput{Deleted: true}, nil
	})
}

// ---- batch_delete_events ----

type BatchDeleteEventsInput struct {
	IDs  []string `json:"ids" jsonschema:"Event identifiers to delete"`
	Span string   `json:"span,omitempty" jsonschema:"For recurring events: 'this' (default) or 'future'"`
}

type BatchDeleteEventsOutput struct {
	Results map[string]string `json:"results" jsonschema:"Per-ID outcome ('ok' or an error message)"`
}

func registerBatchDeleteEvents(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "batch_delete_events",
		Title:       "Batch delete events",
		Description: "Delete multiple events in one call. Per-ID results are returned. Destructive — confirm with the user.",
		Annotations: destructive(),
		InputSchema: withEnum(strictInput[BatchDeleteEventsInput](), "span", "this", "future"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in BatchDeleteEventsInput) (*mcp.CallToolResult, BatchDeleteEventsOutput, error) {
		if len(in.IDs) == 0 {
			return nil, BatchDeleteEventsOutput{}, errors.New("ids must not be empty")
		}
		res, err := c.BatchDeleteEvents(ctx, client.BatchDeleteEventsInput{IDs: in.IDs, Span: in.Span})
		if err != nil {
			return nil, BatchDeleteEventsOutput{}, err
		}
		return nil, BatchDeleteEventsOutput{Results: res.Results}, nil
	})
}
