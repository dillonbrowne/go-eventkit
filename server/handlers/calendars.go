package handlers

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/server/scoped"
)

func registerCalendarRoutes(api huma.API, sc *ScopedCalendarHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "list-calendars",
		Method:      http.MethodGet,
		Path:        "/v1/calendars",
		Summary:     "List calendars allowed by the policy",
		Tags:        []string{"calendars"},
	}, sc.list)

	huma.Register(api, huma.Operation{
		OperationID: "get-calendar",
		Method:      http.MethodGet,
		Path:        "/v1/calendars/{id}",
		Summary:     "Get a single calendar",
		Tags:        []string{"calendars"},
	}, sc.get)

	huma.Register(api, huma.Operation{
		OperationID:   "create-calendar",
		Method:        http.MethodPost,
		Path:          "/v1/calendars",
		Summary:       "Create a new calendar in an allowed source",
		Tags:          []string{"calendars"},
		DefaultStatus: http.StatusCreated,
	}, sc.create)

	huma.Register(api, huma.Operation{
		OperationID: "update-calendar",
		Method:      http.MethodPatch,
		Path:        "/v1/calendars/{id}",
		Summary:     "Update a calendar's title or color",
		Tags:        []string{"calendars"},
	}, sc.update)

	huma.Register(api, huma.Operation{
		OperationID:   "delete-calendar",
		Method:        http.MethodDelete,
		Path:          "/v1/calendars/{id}",
		Summary:       "Delete a calendar",
		Tags:          []string{"calendars"},
		DefaultStatus: http.StatusNoContent,
	}, sc.delete)
}

// ScopedCalendarHandler binds the calendar scoped wrapper to handler methods.
type ScopedCalendarHandler struct {
	S *scoped.ScopedCalendar
}

func (h *ScopedCalendarHandler) list(ctx context.Context, _ *struct{}) (*ListCalendarsResponse, error) {
	cals, err := h.S.Calendars()
	if err != nil {
		return nil, MapError(ctx, err)
	}
	out := &ListCalendarsResponse{}
	out.Body.Calendars = cals
	return out, nil
}

func (h *ScopedCalendarHandler) get(ctx context.Context, in *struct {
	ID string `path:"id"`
}) (*CalendarResponse, error) {
	c, err := h.S.Calendar(in.ID)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &CalendarResponse{Body: *c}, nil
}

func (h *ScopedCalendarHandler) create(ctx context.Context, in *CreateCalendarRequest) (*CalendarResponse, error) {
	c, err := h.S.CreateCalendar(calendar.CreateCalendarInput{
		Title:  in.Body.Title,
		Source: in.Body.Source,
		Color:  in.Body.Color,
	})
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &CalendarResponse{Body: *c}, nil
}

func (h *ScopedCalendarHandler) update(ctx context.Context, in *struct {
	ID   string `path:"id"`
	Body struct {
		Title *string `json:"title,omitempty"`
		Color *string `json:"color,omitempty"`
	}
}) (*CalendarResponse, error) {
	c, err := h.S.UpdateCalendar(in.ID, calendar.UpdateCalendarInput{
		Title: in.Body.Title,
		Color: in.Body.Color,
	})
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &CalendarResponse{Body: *c}, nil
}

func (h *ScopedCalendarHandler) delete(ctx context.Context, in *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	if err := h.S.DeleteCalendar(in.ID); err != nil {
		return nil, MapError(ctx, err)
	}
	return nil, nil
}
