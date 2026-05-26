package client

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/dillonbrowne/go-eventkit/reminders"
)

// ListRemindersInput captures the query params of GET /v1/reminders.
type ListRemindersInput struct {
	List      string
	ListID    string
	Completed string // "true" | "false" | ""
	Search    string
	DueBefore time.Time
	DueAfter  time.Time
}

// ListReminders returns reminders visible under the active REST policy.
func (c *Client) ListReminders(ctx context.Context, in ListRemindersInput) ([]reminders.Reminder, error) {
	q := url.Values{}
	if in.List != "" {
		q.Set("list", in.List)
	}
	if in.ListID != "" {
		q.Set("list_id", in.ListID)
	}
	if in.Completed != "" {
		q.Set("completed", in.Completed)
	}
	if in.Search != "" {
		q.Set("search", in.Search)
	}
	if !in.DueBefore.IsZero() {
		q.Set("due_before", in.DueBefore.UTC().Format(time.RFC3339))
	}
	if !in.DueAfter.IsZero() {
		q.Set("due_after", in.DueAfter.UTC().Format(time.RFC3339))
	}
	var body struct {
		Reminders []reminders.Reminder `json:"reminders"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/reminders", q, nil, &body); err != nil {
		return nil, err
	}
	return body.Reminders, nil
}

// GetReminder fetches one reminder.
func (c *Client) GetReminder(ctx context.Context, id string) (*reminders.Reminder, error) {
	var out reminders.Reminder
	if err := c.do(ctx, http.MethodGet, "/v1/reminders/"+id, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateReminderInput mirrors REST CreateReminderRequest.Body.
type CreateReminderInput struct {
	Title        string     `json:"title"`
	List         string     `json:"list"`
	Notes        string     `json:"notes,omitempty"`
	DueDate      *time.Time `json:"due_date,omitempty"`
	RemindMeDate *time.Time `json:"remind_me_date,omitempty"`
	Priority     int        `json:"priority,omitempty"`
	URL          string     `json:"url,omitempty"`
	Flagged      bool       `json:"flagged,omitempty"`
}

// CreateReminder creates a reminder.
func (c *Client) CreateReminder(ctx context.Context, in CreateReminderInput) (*reminders.Reminder, error) {
	var out reminders.Reminder
	if err := c.do(ctx, http.MethodPost, "/v1/reminders", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateReminderInput mirrors REST UpdateReminderRequest.Body.
type UpdateReminderInput struct {
	Title        *string    `json:"title,omitempty"`
	Notes        *string    `json:"notes,omitempty"`
	List         *string    `json:"list,omitempty"`
	DueDate      *time.Time `json:"due_date,omitempty"`
	ClearDueDate bool       `json:"clear_due_date,omitempty"`
	RemindMeDate *time.Time `json:"remind_me_date,omitempty"`
	Priority     *int       `json:"priority,omitempty"`
	Completed    *bool      `json:"completed,omitempty"`
	Flagged      *bool      `json:"flagged,omitempty"`
	URL          *string    `json:"url,omitempty"`
}

// UpdateReminder patches a reminder.
func (c *Client) UpdateReminder(ctx context.Context, id string, in UpdateReminderInput) (*reminders.Reminder, error) {
	var out reminders.Reminder
	if err := c.do(ctx, http.MethodPatch, "/v1/reminders/"+id, nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteReminder removes a reminder.
func (c *Client) DeleteReminder(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/reminders/"+id, nil, nil, nil)
}

// CompleteReminder marks a reminder complete.
func (c *Client) CompleteReminder(ctx context.Context, id string) (*reminders.Reminder, error) {
	var out reminders.Reminder
	if err := c.do(ctx, http.MethodPost, "/v1/reminders/"+id+"/complete", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UncompleteReminder marks a reminder incomplete.
func (c *Client) UncompleteReminder(ctx context.Context, id string) (*reminders.Reminder, error) {
	var out reminders.Reminder
	if err := c.do(ctx, http.MethodPost, "/v1/reminders/"+id+"/uncomplete", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// BatchDeleteRemindersInput is the body for /v1/reminders/batch-delete.
type BatchDeleteRemindersInput struct {
	IDs []string `json:"ids"`
}

// BatchDeleteReminders removes multiple reminders in one REST call.
func (c *Client) BatchDeleteReminders(ctx context.Context, in BatchDeleteRemindersInput) (*BatchDeleteResult, error) {
	var out BatchDeleteResult
	if err := c.do(ctx, http.MethodPost, "/v1/reminders/batch-delete", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
