package client

import (
	"context"
	"net/http"

	"github.com/dillonbrowne/go-eventkit/calendar"
)

// ListCalendars returns calendars visible under the active REST policy.
func (c *Client) ListCalendars(ctx context.Context) ([]calendar.Calendar, error) {
	var body struct {
		Calendars []calendar.Calendar `json:"calendars"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/calendars", nil, nil, &body); err != nil {
		return nil, err
	}
	return body.Calendars, nil
}

// GetCalendar fetches one calendar by ID.
func (c *Client) GetCalendar(ctx context.Context, id string) (*calendar.Calendar, error) {
	var out calendar.Calendar
	if err := c.do(ctx, http.MethodGet, "/v1/calendars/"+id, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCalendarInput mirrors the REST CreateCalendarRequest.Body shape.
type CreateCalendarInput struct {
	Title  string `json:"title"`
	Source string `json:"source"`
	Color  string `json:"color,omitempty"`
}

// CreateCalendar creates a calendar in the named source.
func (c *Client) CreateCalendar(ctx context.Context, in CreateCalendarInput) (*calendar.Calendar, error) {
	var out calendar.Calendar
	if err := c.do(ctx, http.MethodPost, "/v1/calendars", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateCalendarInput mirrors the REST UpdateCalendarRequest.Body shape.
// Pointer fields signal "leave unchanged if nil".
type UpdateCalendarInput struct {
	Title *string `json:"title,omitempty"`
	Color *string `json:"color,omitempty"`
}

// UpdateCalendar patches a calendar's title or color.
func (c *Client) UpdateCalendar(ctx context.Context, id string, in UpdateCalendarInput) (*calendar.Calendar, error) {
	var out calendar.Calendar
	if err := c.do(ctx, http.MethodPatch, "/v1/calendars/"+id, nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteCalendar removes a calendar by ID.
func (c *Client) DeleteCalendar(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/calendars/"+id, nil, nil, nil)
}
