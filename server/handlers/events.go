package handlers

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dillonbrowne/go-eventkit/calendar"
)

func registerEventRoutes(api huma.API, h *ScopedCalendarHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "list-events",
		Method:      http.MethodGet,
		Path:        "/v1/events",
		Summary:     "List events in a time range, filtered to allowed calendars",
		Tags:        []string{"events"},
	}, h.listEvents)

	huma.Register(api, huma.Operation{
		OperationID: "get-event",
		Method:      http.MethodGet,
		Path:        "/v1/events/{id}",
		Summary:     "Get an event by ID",
		Tags:        []string{"events"},
	}, h.getEvent)

	huma.Register(api, huma.Operation{
		OperationID:   "create-event",
		Method:        http.MethodPost,
		Path:          "/v1/events",
		Summary:       "Create an event in an allowed calendar",
		Tags:          []string{"events"},
		DefaultStatus: http.StatusCreated,
	}, h.createEvent)

	huma.Register(api, huma.Operation{
		OperationID: "update-event",
		Method:      http.MethodPatch,
		Path:        "/v1/events/{id}",
		Summary:     "Update an event",
		Tags:        []string{"events"},
	}, h.updateEvent)

	huma.Register(api, huma.Operation{
		OperationID:   "delete-event",
		Method:        http.MethodDelete,
		Path:          "/v1/events/{id}",
		Summary:       "Delete an event",
		Tags:          []string{"events"},
		DefaultStatus: http.StatusNoContent,
	}, h.deleteEvent)

	huma.Register(api, huma.Operation{
		OperationID: "batch-delete-events",
		Method:      http.MethodPost,
		Path:        "/v1/events/batch-delete",
		Summary:     "Delete multiple events in a single call",
		Tags:        []string{"events"},
	}, h.batchDeleteEvents)
}

func (h *ScopedCalendarHandler) listEvents(ctx context.Context, in *ListEventsParams) (*ListEventsResponse, error) {
	var opts []calendar.ListOption
	if in.Calendar != "" {
		opts = append(opts, calendar.WithCalendar(in.Calendar))
	}
	if in.CalendarID != "" {
		opts = append(opts, calendar.WithCalendarID(in.CalendarID))
	}
	if in.Search != "" {
		opts = append(opts, calendar.WithSearch(in.Search))
	}
	events, err := h.S.Events(in.Start, in.End, opts...)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	out := &ListEventsResponse{}
	out.Body.Events = events
	return out, nil
}

func (h *ScopedCalendarHandler) getEvent(ctx context.Context, in *struct {
	ID string `path:"id"`
}) (*EventResponse, error) {
	ev, err := h.S.Event(in.ID)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &EventResponse{Body: *ev}, nil
}

func (h *ScopedCalendarHandler) createEvent(ctx context.Context, in *CreateEventRequest) (*EventResponse, error) {
	input := calendar.CreateEventInput{
		Title:                 in.Body.Title,
		StartDate:             in.Body.StartDate,
		EndDate:               in.Body.EndDate,
		AllDay:                in.Body.AllDay,
		Location:              in.Body.Location,
		Notes:                 in.Body.Notes,
		URL:                   in.Body.URL,
		Calendar:              in.Body.Calendar,
		Alerts:                in.Body.Alerts,
		SuppressDefaultAlarms: in.Body.SuppressDefaultAlarms,
		TimeZone:              in.Body.TimeZone,
		RecurrenceRules:       in.Body.RecurrenceRules,
		StructuredLocation:    in.Body.StructuredLocation,
	}
	ev, err := h.S.CreateEvent(input)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &EventResponse{Body: *ev}, nil
}

func (h *ScopedCalendarHandler) updateEvent(ctx context.Context, in *UpdateEventRequest) (*EventResponse, error) {
	input := calendar.UpdateEventInput{
		Title:              in.Body.Title,
		StartDate:          in.Body.StartDate,
		EndDate:            in.Body.EndDate,
		AllDay:             in.Body.AllDay,
		Location:           in.Body.Location,
		Notes:              in.Body.Notes,
		URL:                in.Body.URL,
		Calendar:           in.Body.Calendar,
		Alerts:             in.Body.Alerts,
		TimeZone:           in.Body.TimeZone,
		StructuredLocation: in.Body.StructuredLocation,
	}
	if in.Body.RecurrenceRules != nil {
		converted := make([]reminderRecurrenceProxy, len(*in.Body.RecurrenceRules))
		copy(converted, *in.Body.RecurrenceRules)
		input.RecurrenceRules = &converted
	}
	ev, err := h.S.UpdateEvent(in.ID, input, in.Span.ToCalendar())
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &EventResponse{Body: *ev}, nil
}

func (h *ScopedCalendarHandler) deleteEvent(ctx context.Context, in *DeleteEventInput) (*struct{}, error) {
	if err := h.S.DeleteEvent(in.ID, in.Span.ToCalendar()); err != nil {
		return nil, MapError(ctx, err)
	}
	return nil, nil
}

func (h *ScopedCalendarHandler) batchDeleteEvents(ctx context.Context, in *BatchDeleteEventsRequest) (*BatchDeleteResponse, error) {
	results := h.S.DeleteEvents(in.Body.IDs, in.Body.Span.ToCalendar())
	out := &BatchDeleteResponse{}
	out.Body.Results = make(map[string]string, len(results))
	for id, err := range results {
		if err == nil {
			out.Body.Results[id] = "ok"
		} else {
			out.Body.Results[id] = err.Error()
		}
	}
	return out, nil
}
