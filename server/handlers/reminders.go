package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dillonbrowne/go-eventkit/reminders"
)

func registerReminderRoutes(api huma.API, h *ScopedRemindersHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "list-reminders",
		Method:      http.MethodGet,
		Path:        "/v1/reminders",
		Summary:     "List reminders, filtered to allowed lists",
		Tags:        []string{"reminders"},
	}, h.listReminders)

	huma.Register(api, huma.Operation{
		OperationID: "get-reminder",
		Method:      http.MethodGet,
		Path:        "/v1/reminders/{id}",
		Summary:     "Get a reminder by ID",
		Tags:        []string{"reminders"},
	}, h.getReminder)

	huma.Register(api, huma.Operation{
		OperationID:   "create-reminder",
		Method:        http.MethodPost,
		Path:          "/v1/reminders",
		Summary:       "Create a reminder in an allowed list",
		Tags:          []string{"reminders"},
		DefaultStatus: http.StatusCreated,
	}, h.createReminder)

	huma.Register(api, huma.Operation{
		OperationID: "update-reminder",
		Method:      http.MethodPatch,
		Path:        "/v1/reminders/{id}",
		Summary:     "Update a reminder",
		Tags:        []string{"reminders"},
	}, h.updateReminder)

	huma.Register(api, huma.Operation{
		OperationID:   "delete-reminder",
		Method:        http.MethodDelete,
		Path:          "/v1/reminders/{id}",
		Summary:       "Delete a reminder",
		Tags:          []string{"reminders"},
		DefaultStatus: http.StatusNoContent,
	}, h.deleteReminder)

	huma.Register(api, huma.Operation{
		OperationID: "complete-reminder",
		Method:      http.MethodPost,
		Path:        "/v1/reminders/{id}/complete",
		Summary:     "Mark a reminder as completed",
		Tags:        []string{"reminders"},
	}, h.completeReminder)

	huma.Register(api, huma.Operation{
		OperationID: "uncomplete-reminder",
		Method:      http.MethodPost,
		Path:        "/v1/reminders/{id}/uncomplete",
		Summary:     "Mark a reminder as not completed",
		Tags:        []string{"reminders"},
	}, h.uncompleteReminder)

	huma.Register(api, huma.Operation{
		OperationID: "batch-delete-reminders",
		Method:      http.MethodPost,
		Path:        "/v1/reminders/batch-delete",
		Summary:     "Delete multiple reminders in a single call",
		Tags:        []string{"reminders"},
	}, h.batchDeleteReminders)
}

func (h *ScopedRemindersHandler) listReminders(ctx context.Context, in *ListRemindersParams) (*ListRemindersResponse, error) {
	var opts []reminders.ListOption
	if in.List != "" {
		opts = append(opts, reminders.WithList(in.List))
	}
	if in.ListID != "" {
		opts = append(opts, reminders.WithListID(in.ListID))
	}
	switch in.Completed {
	case "true":
		opts = append(opts, reminders.WithCompleted(true))
	case "false":
		opts = append(opts, reminders.WithCompleted(false))
	}
	if in.Search != "" {
		opts = append(opts, reminders.WithSearch(in.Search))
	}
	if !in.DueBefore.IsZero() {
		opts = append(opts, reminders.WithDueBefore(in.DueBefore))
	}
	if !in.DueAfter.IsZero() {
		opts = append(opts, reminders.WithDueAfter(in.DueAfter))
	}
	rs, err := h.S.Reminders(opts...)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	out := &ListRemindersResponse{}
	out.Body.Reminders = rs
	return out, nil
}

func (h *ScopedRemindersHandler) getReminder(ctx context.Context, in *struct {
	ID string `path:"id"`
}) (*ReminderResponse, error) {
	r, err := h.S.Reminder(in.ID)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &ReminderResponse{Body: *r}, nil
}

func (h *ScopedRemindersHandler) createReminder(ctx context.Context, in *CreateReminderRequest) (*ReminderResponse, error) {
	r, err := h.S.CreateReminder(reminders.CreateReminderInput{
		Title:           in.Body.Title,
		Notes:           in.Body.Notes,
		ListName:        in.Body.ListName,
		DueDate:         in.Body.DueDate,
		RemindMeDate:    in.Body.RemindMeDate,
		Priority:        in.Body.Priority,
		URL:             in.Body.URL,
		Flagged:         in.Body.Flagged,
		Alarms:          in.Body.Alarms,
		RecurrenceRules: in.Body.RecurrenceRules,
	})
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &ReminderResponse{Body: *r}, nil
}

func (h *ScopedRemindersHandler) updateReminder(ctx context.Context, in *struct {
	ID   string `path:"id"`
	Body struct {
		Title           *string                    `json:"title,omitempty"`
		Notes           *string                    `json:"notes,omitempty"`
		ListName        *string                    `json:"list,omitempty"`
		DueDate         *time.Time                 `json:"due_date,omitempty"`
		ClearDueDate    bool                       `json:"clear_due_date,omitempty"`
		RemindMeDate    *time.Time                 `json:"remind_me_date,omitempty"`
		Priority        *reminders.Priority        `json:"priority,omitempty"`
		Completed       *bool                      `json:"completed,omitempty"`
		Flagged         *bool                      `json:"flagged,omitempty"`
		URL             *string                    `json:"url,omitempty"`
		Alarms          *[]reminders.Alarm         `json:"alarms,omitempty"`
		RecurrenceRules *[]reminderRecurrenceProxy `json:"recurrence_rules,omitempty"`
	}
}) (*ReminderResponse, error) {
	input := reminders.UpdateReminderInput{
		Title:        in.Body.Title,
		Notes:        in.Body.Notes,
		ListName:     in.Body.ListName,
		DueDate:      in.Body.DueDate,
		ClearDueDate: in.Body.ClearDueDate,
		RemindMeDate: in.Body.RemindMeDate,
		Priority:     in.Body.Priority,
		Completed:    in.Body.Completed,
		Flagged:      in.Body.Flagged,
		URL:          in.Body.URL,
		Alarms:       in.Body.Alarms,
	}
	if in.Body.RecurrenceRules != nil {
		converted := make([]reminderRecurrenceProxy, len(*in.Body.RecurrenceRules))
		copy(converted, *in.Body.RecurrenceRules)
		input.RecurrenceRules = &converted
	}
	r, err := h.S.UpdateReminder(in.ID, input)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &ReminderResponse{Body: *r}, nil
}

func (h *ScopedRemindersHandler) deleteReminder(ctx context.Context, in *struct {
	ID string `path:"id"`
}) (*struct{}, error) {
	if err := h.S.DeleteReminder(in.ID); err != nil {
		return nil, MapError(ctx, err)
	}
	return nil, nil
}

func (h *ScopedRemindersHandler) completeReminder(ctx context.Context, in *struct {
	ID string `path:"id"`
}) (*ReminderResponse, error) {
	r, err := h.S.CompleteReminder(in.ID)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &ReminderResponse{Body: *r}, nil
}

func (h *ScopedRemindersHandler) uncompleteReminder(ctx context.Context, in *struct {
	ID string `path:"id"`
}) (*ReminderResponse, error) {
	r, err := h.S.UncompleteReminder(in.ID)
	if err != nil {
		return nil, MapError(ctx, err)
	}
	return &ReminderResponse{Body: *r}, nil
}

func (h *ScopedRemindersHandler) batchDeleteReminders(ctx context.Context, in *BatchDeleteRemindersRequest) (*BatchDeleteResponse, error) {
	results := h.S.DeleteReminders(in.Body.IDs)
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
