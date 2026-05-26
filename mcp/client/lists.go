package client

import (
	"context"
	"net/http"

	"github.com/dillonbrowne/go-eventkit/reminders"
)

// ListReminderLists returns all reminder lists visible to the REST policy.
func (c *Client) ListReminderLists(ctx context.Context) ([]reminders.List, error) {
	var body struct {
		Lists []reminders.List `json:"lists"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/lists", nil, nil, &body); err != nil {
		return nil, err
	}
	return body.Lists, nil
}

// GetReminderList fetches one list by ID.
func (c *Client) GetReminderList(ctx context.Context, id string) (*reminders.List, error) {
	var out reminders.List
	if err := c.do(ctx, http.MethodGet, "/v1/lists/"+id, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateReminderListInput mirrors REST CreateListRequest.Body.
type CreateReminderListInput struct {
	Title  string `json:"title"`
	Source string `json:"source"`
	Color  string `json:"color,omitempty"`
}

// CreateReminderList creates a list.
func (c *Client) CreateReminderList(ctx context.Context, in CreateReminderListInput) (*reminders.List, error) {
	var out reminders.List
	if err := c.do(ctx, http.MethodPost, "/v1/lists", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateReminderListInput mirrors REST UpdateListRequest.Body.
type UpdateReminderListInput struct {
	Title *string `json:"title,omitempty"`
	Color *string `json:"color,omitempty"`
}

// UpdateReminderList patches a list.
func (c *Client) UpdateReminderList(ctx context.Context, id string, in UpdateReminderListInput) (*reminders.List, error) {
	var out reminders.List
	if err := c.do(ctx, http.MethodPatch, "/v1/lists/"+id, nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteReminderList removes a list.
func (c *Client) DeleteReminderList(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/lists/"+id, nil, nil, nil)
}
