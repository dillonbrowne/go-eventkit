package tools

import (
	"context"
	"errors"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dillonbrowne/go-eventkit/mcp/client"
)

// ---- list_calendars ----

type ListCalendarsInput struct{}

type ListCalendarsOutput struct {
	Calendars []RedactedCalendar `json:"calendars"`
}

func registerListCalendars(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_calendars",
		Title:       "List calendars",
		Description: "List the macOS calendars visible under the REST policy (iCloud, Google, Exchange, local, etc.). " + UserDataNotice,
		Annotations: readOnly(),
		// Explicit no-arg schema so ChatGPT sees "properties":{}; see emptyObjectSchema.
		InputSchema: emptyObjectSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ ListCalendarsInput) (*mcp.CallToolResult, ListCalendarsOutput, error) {
		cals, err := c.ListCalendars(ctx)
		if err != nil {
			return nil, ListCalendarsOutput{}, err
		}
		return nil, ListCalendarsOutput{Calendars: RedactCalendars(cals)}, nil
	})
}

// ---- get_calendar ----

type GetCalendarInput struct {
	ID string `json:"id" jsonschema:"Calendar identifier (from list_calendars)"`
}

type GetCalendarOutput struct {
	Calendar RedactedCalendar `json:"calendar"`
}

func registerGetCalendar(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_calendar",
		Title:       "Get calendar",
		Description: "Fetch one calendar by ID. " + UserDataNotice,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetCalendarInput) (*mcp.CallToolResult, GetCalendarOutput, error) {
		if in.ID == "" {
			return nil, GetCalendarOutput{}, errors.New("id is required")
		}
		cal, err := c.GetCalendar(ctx, in.ID)
		if err != nil {
			return nil, GetCalendarOutput{}, err
		}
		return nil, GetCalendarOutput{Calendar: RedactCalendar(*cal)}, nil
	})
}

// ---- create_calendar ----

type CreateCalendarInput struct {
	Title  string `json:"title" jsonschema:"Display name for the new calendar"`
	Source string `json:"source" jsonschema:"Account source name (e.g. 'iCloud', 'Local'). Must be writable under the active REST policy."`
	Color  string `json:"color,omitempty" jsonschema:"Optional hex color, e.g. '#FF6961'"`
}

type CreateCalendarOutput struct {
	Calendar RedactedCalendar `json:"calendar"`
}

func registerCreateCalendar(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_calendar",
		Title:       "Create calendar",
		Description: "Create a new calendar in the named source.",
		Annotations: nonDestructiveWrite(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateCalendarInput) (*mcp.CallToolResult, CreateCalendarOutput, error) {
		cal, err := c.CreateCalendar(ctx, client.CreateCalendarInput{
			Title: in.Title, Source: in.Source, Color: in.Color,
		})
		if err != nil {
			return nil, CreateCalendarOutput{}, err
		}
		return nil, CreateCalendarOutput{Calendar: RedactCalendar(*cal)}, nil
	})
}

// ---- update_calendar ----

type UpdateCalendarInput struct {
	ID    string `json:"id" jsonschema:"Calendar identifier"`
	Title string `json:"title,omitempty" jsonschema:"New title (omit to leave unchanged)"`
	Color string `json:"color,omitempty" jsonschema:"New hex color (omit to leave unchanged)"`
}

type UpdateCalendarOutput struct {
	Calendar RedactedCalendar `json:"calendar"`
}

func registerUpdateCalendar(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_calendar",
		Title:       "Update calendar",
		Description: "Rename or recolor a calendar. Omit any field to leave it unchanged.",
		Annotations: idempotentWrite(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateCalendarInput) (*mcp.CallToolResult, UpdateCalendarOutput, error) {
		if in.ID == "" {
			return nil, UpdateCalendarOutput{}, errors.New("id is required")
		}
		var body client.UpdateCalendarInput
		if in.Title != "" {
			body.Title = &in.Title
		}
		if in.Color != "" {
			body.Color = &in.Color
		}
		cal, err := c.UpdateCalendar(ctx, in.ID, body)
		if err != nil {
			return nil, UpdateCalendarOutput{}, err
		}
		return nil, UpdateCalendarOutput{Calendar: RedactCalendar(*cal)}, nil
	})
}

// ---- delete_calendar ----

type DeleteCalendarInput struct {
	ID string `json:"id" jsonschema:"Calendar identifier"`
}

type DeleteCalendarOutput struct {
	Deleted bool `json:"deleted"`
}

func registerDeleteCalendar(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_calendar",
		Title:       "Delete calendar",
		Description: "Permanently delete a calendar and all its events. This is destructive — confirm with the user before calling.",
		Annotations: destructive(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DeleteCalendarInput) (*mcp.CallToolResult, DeleteCalendarOutput, error) {
		if in.ID == "" {
			return nil, DeleteCalendarOutput{}, errors.New("id is required")
		}
		if err := c.DeleteCalendar(ctx, in.ID); err != nil {
			return nil, DeleteCalendarOutput{}, err
		}
		return nil, DeleteCalendarOutput{Deleted: true}, nil
	})
}
