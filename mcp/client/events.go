package client

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
)

// ListEventsInput captures the query parameters of GET /v1/events.
type ListEventsInput struct {
	Start      time.Time
	End        time.Time
	Calendar   string
	CalendarID string
	Search     string
}

// ListEvents returns events visible under the active REST policy.
func (c *Client) ListEvents(ctx context.Context, in ListEventsInput) ([]calendar.Event, error) {
	q := url.Values{}
	q.Set("start", in.Start.UTC().Format(time.RFC3339))
	q.Set("end", in.End.UTC().Format(time.RFC3339))
	if in.Calendar != "" {
		q.Set("calendar", in.Calendar)
	}
	if in.CalendarID != "" {
		q.Set("calendar_id", in.CalendarID)
	}
	if in.Search != "" {
		q.Set("search", in.Search)
	}
	var body struct {
		Events []calendar.Event `json:"events"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/events", q, nil, &body); err != nil {
		return nil, err
	}
	return body.Events, nil
}

// GetEvent fetches one event by ID.
func (c *Client) GetEvent(ctx context.Context, id string) (*calendar.Event, error) {
	var out calendar.Event
	if err := c.do(ctx, http.MethodGet, "/v1/events/"+id, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateEvent mirrors the REST CreateEventBody shape. Each field maps
// 1:1 to the REST server's input; recurrence and structured location
// are accepted as Go types and forwarded.
type CreateEventInput struct {
	Title                 string           `json:"title"`
	StartDate             time.Time        `json:"startDate"`
	EndDate               time.Time        `json:"endDate"`
	Calendar              string           `json:"calendar"`
	AllDay                bool             `json:"allDay,omitempty"`
	Location              string           `json:"location,omitempty"`
	Notes                 string           `json:"notes,omitempty"`
	URL                   string           `json:"url,omitempty"`
	Alerts                []calendar.Alert `json:"alerts,omitempty"`
	SuppressDefaultAlarms bool             `json:"suppressDefaultAlarms,omitempty"`
	TimeZone              string           `json:"timeZone,omitempty"`
}

// CreateEvent creates a calendar event.
func (c *Client) CreateEvent(ctx context.Context, in CreateEventInput) (*calendar.Event, error) {
	var out calendar.Event
	if err := c.do(ctx, http.MethodPost, "/v1/events", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateEventInput mirrors REST UpdateEventBody.
type UpdateEventInput struct {
	Title     *string           `json:"title,omitempty"`
	StartDate *time.Time        `json:"startDate,omitempty"`
	EndDate   *time.Time        `json:"endDate,omitempty"`
	AllDay    *bool             `json:"allDay,omitempty"`
	Location  *string           `json:"location,omitempty"`
	Notes     *string           `json:"notes,omitempty"`
	URL       *string           `json:"url,omitempty"`
	Calendar  *string           `json:"calendar,omitempty"`
	Alerts    *[]calendar.Alert `json:"alerts,omitempty"`
	TimeZone  *string           `json:"timeZone,omitempty"`
}

// UpdateEvent patches an event. Span is "this" or "future" (default "this").
func (c *Client) UpdateEvent(ctx context.Context, id string, span string, in UpdateEventInput) (*calendar.Event, error) {
	q := url.Values{}
	if span != "" {
		q.Set("span", span)
	}
	var out calendar.Event
	if err := c.do(ctx, http.MethodPatch, "/v1/events/"+id, q, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteEvent removes an event.
func (c *Client) DeleteEvent(ctx context.Context, id string, span string) error {
	q := url.Values{}
	if span != "" {
		q.Set("span", span)
	}
	return c.do(ctx, http.MethodDelete, "/v1/events/"+id, q, nil, nil)
}

// BatchDeleteEvents removes multiple events in one REST call.
type BatchDeleteEventsInput struct {
	IDs  []string `json:"ids"`
	Span string   `json:"span,omitempty"`
}

// BatchDeleteResult is the per-id outcome map.
type BatchDeleteResult struct {
	Results map[string]string `json:"results"`
}

// BatchDeleteEvents posts to /v1/events/batch-delete.
func (c *Client) BatchDeleteEvents(ctx context.Context, in BatchDeleteEventsInput) (*BatchDeleteResult, error) {
	var out BatchDeleteResult
	if err := c.do(ctx, http.MethodPost, "/v1/events/batch-delete", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
