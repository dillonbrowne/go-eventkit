package handlers

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server/scoped"
)

func registerListRoutes(api huma.API, sr *ScopedRemindersHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "list-reminder-lists",
		Method:      http.MethodGet,
		Path:        "/v1/lists",
		Summary:     "List reminder lists allowed by the policy",
		Tags:        []string{"lists"},
	}, sr.listLists)

	huma.Register(api, huma.Operation{
		OperationID: "get-reminder-list",
		Method:      http.MethodGet,
		Path:        "/v1/lists/{id}",
		Summary:     "Get a single reminder list",
		Tags:        []string{"lists"},
	}, sr.getList)

	huma.Register(api, huma.Operation{
		OperationID:   "create-reminder-list",
		Method:        http.MethodPost,
		Path:          "/v1/lists",
		Summary:       "Create a reminder list in an allowed source",
		Tags:          []string{"lists"},
		DefaultStatus: http.StatusCreated,
	}, sr.createList)

	huma.Register(api, huma.Operation{
		OperationID: "update-reminder-list",
		Method:      http.MethodPatch,
		Path:        "/v1/lists/{id}",
		Summary:     "Update a reminder list",
		Tags:        []string{"lists"},
	}, sr.updateList)

	huma.Register(api, huma.Operation{
		OperationID:   "delete-reminder-list",
		Method:        http.MethodDelete,
		Path:          "/v1/lists/{id}",
		Summary:       "Delete a reminder list",
		Tags:          []string{"lists"},
		DefaultStatus: http.StatusNoContent,
	}, sr.deleteList)
}

// ScopedRemindersHandler binds the reminders scoped wrapper to handler methods.
type ScopedRemindersHandler struct {
	S *scoped.ScopedReminders
}

func (h *ScopedRemindersHandler) listLists(ctx context.Context, _ *struct{}) (*ListsResponse, error) {
	lists, err := h.S.Lists()
	if err != nil {
		return nil, MapError(ctx, err)
	}
	out := &ListsResponse{}
	out.Body.Lists = lists
	return out, nil
}

func (h *ScopedRemindersHandler) getList(ctx context.Context, in *struct {
	ID string `path:"id"`
}) (*ListResponse, error) {
	l, err := h.S.List(in.ID)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &ListResponse{Body: *l}, nil
}

func (h *ScopedRemindersHandler) createList(ctx context.Context, in *CreateListRequest) (*ListResponse, error) {
	l, err := h.S.CreateList(reminders.CreateListInput{
		Title:  in.Body.Title,
		Source: in.Body.Source,
		Color:  in.Body.Color,
	})
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &ListResponse{Body: *l}, nil
}

func (h *ScopedRemindersHandler) updateList(ctx context.Context, in *struct {
	ID   string `path:"id"`
	Body struct {
		Title *string `json:"title,omitempty"`
		Color *string `json:"color,omitempty"`
	}
}) (*ListResponse, error) {
	l, err := h.S.UpdateList(in.ID, reminders.UpdateListInput{
		Title: in.Body.Title,
		Color: in.Body.Color,
	})
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &ListResponse{Body: *l}, nil
}

func (h *ScopedRemindersHandler) deleteList(ctx context.Context, in *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	if err := h.S.DeleteList(in.ID); err != nil {
		return nil, MapError(ctx, err)
	}
	return nil, nil
}
