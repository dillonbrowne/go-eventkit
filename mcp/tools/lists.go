package tools

import (
	"context"
	"errors"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dillonbrowne/go-eventkit/mcp/client"
)

// ---- list_reminder_lists ----

type ListReminderListsInput struct{}

type ListReminderListsOutput struct {
	Lists []RedactedList `json:"lists"`
}

func registerListReminderLists(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "list_reminder_lists",
		Title:       "List reminder lists",
		Description: "List the reminder lists visible under the active REST policy. " + UserDataNotice,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ ListReminderListsInput) (*mcp.CallToolResult, ListReminderListsOutput, error) {
		ls, err := c.ListReminderLists(ctx)
		if err != nil {
			return nil, ListReminderListsOutput{}, err
		}
		return nil, ListReminderListsOutput{Lists: RedactLists(ls)}, nil
	})
}

// ---- get_reminder_list ----

type GetReminderListInput struct {
	ID string `json:"id" jsonschema:"List identifier"`
}
type GetReminderListOutput struct {
	List RedactedList `json:"list"`
}

func registerGetReminderList(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "get_reminder_list",
		Title:       "Get reminder list",
		Description: "Fetch one reminder list by ID. " + UserDataNotice,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetReminderListInput) (*mcp.CallToolResult, GetReminderListOutput, error) {
		if in.ID == "" {
			return nil, GetReminderListOutput{}, errors.New("id is required")
		}
		l, err := c.GetReminderList(ctx, in.ID)
		if err != nil {
			return nil, GetReminderListOutput{}, err
		}
		return nil, GetReminderListOutput{List: RedactList(*l)}, nil
	})
}

// ---- create_reminder_list ----

type CreateReminderListInput struct {
	Title  string `json:"title" jsonschema:"Display name for the new list"`
	Source string `json:"source" jsonschema:"Account source name (e.g. 'iCloud')"`
	Color  string `json:"color,omitempty" jsonschema:"Optional hex color"`
}
type CreateReminderListOutput struct {
	List RedactedList `json:"list"`
}

func registerCreateReminderList(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "create_reminder_list",
		Title:       "Create reminder list",
		Description: "Create a new reminder list in the named source.",
		Annotations: nonDestructiveWrite(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateReminderListInput) (*mcp.CallToolResult, CreateReminderListOutput, error) {
		l, err := c.CreateReminderList(ctx, client.CreateReminderListInput{
			Title: in.Title, Source: in.Source, Color: in.Color,
		})
		if err != nil {
			return nil, CreateReminderListOutput{}, err
		}
		return nil, CreateReminderListOutput{List: RedactList(*l)}, nil
	})
}

// ---- update_reminder_list ----

type UpdateReminderListInput struct {
	ID    string `json:"id" jsonschema:"List identifier"`
	Title string `json:"title,omitempty" jsonschema:"New title (omit to leave unchanged)"`
	Color string `json:"color,omitempty" jsonschema:"New hex color, e.g. #FF0000 (omit to leave unchanged)"`
}
type UpdateReminderListOutput struct {
	List RedactedList `json:"list"`
}

func registerUpdateReminderList(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "update_reminder_list",
		Title:       "Update reminder list",
		Description: "Rename or recolor a reminder list. Omit any field to leave it unchanged.",
		Annotations: idempotentWrite(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateReminderListInput) (*mcp.CallToolResult, UpdateReminderListOutput, error) {
		if in.ID == "" {
			return nil, UpdateReminderListOutput{}, errors.New("id is required")
		}
		var body client.UpdateReminderListInput
		if in.Title != "" {
			body.Title = &in.Title
		}
		if in.Color != "" {
			body.Color = &in.Color
		}
		l, err := c.UpdateReminderList(ctx, in.ID, body)
		if err != nil {
			return nil, UpdateReminderListOutput{}, err
		}
		return nil, UpdateReminderListOutput{List: RedactList(*l)}, nil
	})
}

// ---- delete_reminder_list ----

type DeleteReminderListInput struct {
	ID string `json:"id" jsonschema:"List identifier"`
}
type DeleteReminderListOutput struct {
	Deleted bool `json:"deleted"`
}

func registerDeleteReminderList(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "delete_reminder_list",
		Title:       "Delete reminder list",
		Description: "Permanently delete a reminder list and all its reminders. Destructive — confirm with the user before calling.",
		Annotations: destructive(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DeleteReminderListInput) (*mcp.CallToolResult, DeleteReminderListOutput, error) {
		if in.ID == "" {
			return nil, DeleteReminderListOutput{}, errors.New("id is required")
		}
		if err := c.DeleteReminderList(ctx, in.ID); err != nil {
			return nil, DeleteReminderListOutput{}, err
		}
		return nil, DeleteReminderListOutput{Deleted: true}, nil
	})
}
