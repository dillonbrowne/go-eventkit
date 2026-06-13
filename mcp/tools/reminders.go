package tools

import (
	"context"
	"errors"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dillonbrowne/go-eventkit/mcp/client"
)

// ---- list_reminders ----

type ListRemindersInput struct {
	List      string `json:"list,omitempty" jsonschema:"Filter by list name"`
	ListID    string `json:"listID,omitempty" jsonschema:"Filter by list identifier"`
	Completed string `json:"completed,omitempty" jsonschema:"Filter by completion: 'true' (completed only) or 'false' (incomplete only); omit for both"`
	Search    string `json:"search,omitempty"`
	DueBefore string `json:"dueBefore,omitempty" jsonschema:"Only reminders due before this. Accepts ISO 8601 or natural language."`
	DueAfter  string `json:"dueAfter,omitempty" jsonschema:"Only reminders due after this. Accepts ISO 8601 or natural language."`
}

type ListRemindersOutput struct {
	Reminders []RedactedReminder `json:"reminders"`
}

func registerListReminders(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "list_reminders",
		Title:       "List reminders",
		Description: "List reminders, optionally filtered by list / completion / search / due date. " + DateDoc + " " + UserDataNotice,
		Annotations: readOnly(),
		InputSchema: withEnum(strictInput[ListRemindersInput](), "completed", "true", "false"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListRemindersInput) (*mcp.CallToolResult, ListRemindersOutput, error) {
		due1, err := ParseDate(in.DueBefore)
		if err != nil {
			return nil, ListRemindersOutput{}, err
		}
		due2, err := ParseDate(in.DueAfter)
		if err != nil {
			return nil, ListRemindersOutput{}, err
		}
		rs, err := c.ListReminders(ctx, client.ListRemindersInput{
			List: in.List, ListID: in.ListID, Completed: in.Completed,
			Search: in.Search, DueBefore: due1, DueAfter: due2,
		})
		if err != nil {
			return nil, ListRemindersOutput{}, err
		}
		return nil, ListRemindersOutput{Reminders: RedactReminders(rs)}, nil
	})
}

// ---- get_reminder ----

type GetReminderInput struct {
	ID string `json:"id" jsonschema:"Reminder identifier"`
}
type GetReminderOutput struct {
	Reminder RedactedReminder `json:"reminder"`
}

func registerGetReminder(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "get_reminder",
		Title:       "Get reminder",
		Description: "Fetch one reminder by ID. " + UserDataNotice,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetReminderInput) (*mcp.CallToolResult, GetReminderOutput, error) {
		if in.ID == "" {
			return nil, GetReminderOutput{}, errors.New("id is required")
		}
		r, err := c.GetReminder(ctx, in.ID)
		if err != nil {
			return nil, GetReminderOutput{}, err
		}
		return nil, GetReminderOutput{Reminder: RedactReminder(*r)}, nil
	})
}

// ---- create_reminder ----

type CreateReminderInput struct {
	Title        string `json:"title" jsonschema:"Reminder title"`
	List         string `json:"list" jsonschema:"Target list name (from list_reminder_lists)"`
	Notes        string `json:"notes,omitempty"`
	DueDate      string `json:"dueDate,omitempty" jsonschema:"When the reminder is due. Accepts ISO 8601 or natural language (e.g. tomorrow 5pm)."`
	RemindMeDate string `json:"remindMeDate,omitempty" jsonschema:"When to fire the notification alarm (independent of due date)."`
	Priority     int    `json:"priority,omitempty" jsonschema:"Priority: 0=none, 1=high, 5=medium, 9=low (output renders these as none/high/medium/low)"`
	URL          string `json:"url,omitempty"`
	Flagged      bool   `json:"flagged,omitempty"`
}
type CreateReminderOutput struct {
	Reminder RedactedReminder `json:"reminder"`
}

func registerCreateReminder(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "create_reminder",
		Title:       "Create reminder",
		Description: "Create a reminder in the named list. " + DateDoc,
		Annotations: nonDestructiveWrite(),
		InputSchema: withEnum(strictInput[CreateReminderInput](), "priority", 0, 1, 5, 9),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateReminderInput) (*mcp.CallToolResult, CreateReminderOutput, error) {
		due, err := ParseDatePtr(in.DueDate)
		if err != nil {
			return nil, CreateReminderOutput{}, err
		}
		remind, err := ParseDatePtr(in.RemindMeDate)
		if err != nil {
			return nil, CreateReminderOutput{}, err
		}
		r, err := c.CreateReminder(ctx, client.CreateReminderInput{
			Title: in.Title, List: in.List, Notes: in.Notes,
			DueDate: due, RemindMeDate: remind,
			Priority: in.Priority, URL: in.URL, Flagged: in.Flagged,
		})
		if err != nil {
			return nil, CreateReminderOutput{}, err
		}
		return nil, CreateReminderOutput{Reminder: RedactReminder(*r)}, nil
	})
}

// ---- update_reminder ----

type UpdateReminderInput struct {
	ID           string `json:"id" jsonschema:"Reminder identifier"`
	Title        string `json:"title,omitempty"`
	Notes        string `json:"notes,omitempty"`
	List         string `json:"list,omitempty" jsonschema:"Move to a different list"`
	DueDate      string `json:"dueDate,omitempty" jsonschema:"New due date. Accepts ISO 8601 or natural language."`
	ClearDueDate bool   `json:"clearDueDate,omitempty" jsonschema:"Set true to remove the due date entirely (overrides dueDate)"`
	RemindMeDate string `json:"remindMeDate,omitempty"`
	Priority     *int   `json:"priority,omitempty" jsonschema:"Priority. Use 0 (none), 1 (high), 5 (medium), or 9 (low)."`
	Completed    *bool  `json:"completed,omitempty"`
	Flagged      *bool  `json:"flagged,omitempty"`
	URL          string `json:"url,omitempty"`
}
type UpdateReminderOutput struct {
	Reminder RedactedReminder `json:"reminder"`
}

func registerUpdateReminder(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "update_reminder",
		Title:       "Update reminder",
		Description: "Patch a reminder. Omit a field to leave it unchanged. " + DateDoc,
		Annotations: idempotentWrite(),
		InputSchema: withEnum(strictInput[UpdateReminderInput](), "priority", 0, 1, 5, 9),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateReminderInput) (*mcp.CallToolResult, UpdateReminderOutput, error) {
		if in.ID == "" {
			return nil, UpdateReminderOutput{}, errors.New("id is required")
		}
		body := client.UpdateReminderInput{
			ClearDueDate: in.ClearDueDate, Priority: in.Priority, Completed: in.Completed, Flagged: in.Flagged,
		}
		if in.Title != "" {
			body.Title = &in.Title
		}
		if in.Notes != "" {
			body.Notes = &in.Notes
		}
		if in.List != "" {
			body.List = &in.List
		}
		if in.URL != "" {
			body.URL = &in.URL
		}
		if in.DueDate != "" {
			t, err := ParseDate(in.DueDate)
			if err != nil {
				return nil, UpdateReminderOutput{}, err
			}
			body.DueDate = &t
		}
		if in.RemindMeDate != "" {
			t, err := ParseDate(in.RemindMeDate)
			if err != nil {
				return nil, UpdateReminderOutput{}, err
			}
			body.RemindMeDate = &t
		}
		r, err := c.UpdateReminder(ctx, in.ID, body)
		if err != nil {
			return nil, UpdateReminderOutput{}, err
		}
		return nil, UpdateReminderOutput{Reminder: RedactReminder(*r)}, nil
	})
}

// ---- delete_reminder ----

type DeleteReminderInput struct {
	ID string `json:"id" jsonschema:"Reminder identifier"`
}
type DeleteReminderOutput struct {
	Deleted bool `json:"deleted"`
}

func registerDeleteReminder(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "delete_reminder",
		Title:       "Delete reminder",
		Description: "Permanently delete a reminder. Destructive — confirm with the user before calling.",
		Annotations: destructive(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DeleteReminderInput) (*mcp.CallToolResult, DeleteReminderOutput, error) {
		if in.ID == "" {
			return nil, DeleteReminderOutput{}, errors.New("id is required")
		}
		if err := c.DeleteReminder(ctx, in.ID); err != nil {
			return nil, DeleteReminderOutput{}, err
		}
		return nil, DeleteReminderOutput{Deleted: true}, nil
	})
}

// ---- complete_reminder ----

type CompleteReminderInput struct {
	ID string `json:"id" jsonschema:"Reminder identifier"`
}
type CompleteReminderOutput struct {
	Reminder RedactedReminder `json:"reminder"`
}

func registerCompleteReminder(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "complete_reminder",
		Title:       "Complete reminder",
		Description: "Mark a reminder complete. Idempotent.",
		Annotations: idempotentWrite(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CompleteReminderInput) (*mcp.CallToolResult, CompleteReminderOutput, error) {
		if in.ID == "" {
			return nil, CompleteReminderOutput{}, errors.New("id is required")
		}
		r, err := c.CompleteReminder(ctx, in.ID)
		if err != nil {
			return nil, CompleteReminderOutput{}, err
		}
		return nil, CompleteReminderOutput{Reminder: RedactReminder(*r)}, nil
	})
}

// ---- uncomplete_reminder ----

type UncompleteReminderInput struct {
	ID string `json:"id" jsonschema:"Reminder identifier"`
}
type UncompleteReminderOutput struct {
	Reminder RedactedReminder `json:"reminder"`
}

func registerUncompleteReminder(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "uncomplete_reminder",
		Title:       "Uncomplete reminder",
		Description: "Mark a reminder incomplete. Idempotent.",
		Annotations: idempotentWrite(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UncompleteReminderInput) (*mcp.CallToolResult, UncompleteReminderOutput, error) {
		if in.ID == "" {
			return nil, UncompleteReminderOutput{}, errors.New("id is required")
		}
		r, err := c.UncompleteReminder(ctx, in.ID)
		if err != nil {
			return nil, UncompleteReminderOutput{}, err
		}
		return nil, UncompleteReminderOutput{Reminder: RedactReminder(*r)}, nil
	})
}

// ---- batch_delete_reminders ----

type BatchDeleteRemindersInput struct {
	IDs []string `json:"ids" jsonschema:"Reminder identifiers to delete"`
}
type BatchDeleteRemindersOutput struct {
	Results map[string]string `json:"results" jsonschema:"Per-ID outcome ('ok' or an error message)"`
}

func registerBatchDeleteReminders(s *mcp.Server, c *client.Client) {
	addTool(s, &mcp.Tool{
		Name:        "batch_delete_reminders",
		Title:       "Batch delete reminders",
		Description: "Delete multiple reminders in one call. Per-ID results are returned. Destructive — confirm with the user.",
		Annotations: destructive(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in BatchDeleteRemindersInput) (*mcp.CallToolResult, BatchDeleteRemindersOutput, error) {
		if len(in.IDs) == 0 {
			return nil, BatchDeleteRemindersOutput{}, errors.New("ids must not be empty")
		}
		res, err := c.BatchDeleteReminders(ctx, client.BatchDeleteRemindersInput{IDs: in.IDs})
		if err != nil {
			return nil, BatchDeleteRemindersOutput{}, err
		}
		return nil, BatchDeleteRemindersOutput{Results: res.Results}, nil
	})
}
