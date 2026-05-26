package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server/scoped"
)

func TestMapError_Nil(t *testing.T) {
	if err := MapError(context.Background(), nil); err != nil {
		t.Errorf("MapError(context.Background(), nil) = %v, want nil", err)
	}
}

func TestMapError_Matrix(t *testing.T) {
	tests := []struct {
		name       string
		in         error
		wantStatus int
		wantSubstr string
	}{
		{"calendar_not_found", calendar.ErrNotFound, http.StatusNotFound, "not found"},
		{"reminders_not_found", reminders.ErrNotFound, http.StatusNotFound, "not found"},
		{"policy_denied", scoped.ErrPolicyDenied, http.StatusNotFound, "not found"},
		{"target_required", scoped.ErrTargetRequired, http.StatusUnprocessableEntity, "target"},
		{"calendar_immutable", calendar.ErrImmutable, http.StatusConflict, "immutable"},
		{"reminders_immutable", reminders.ErrImmutable, http.StatusConflict, "immutable"},
		{"calendar_access_denied", calendar.ErrAccessDenied, http.StatusServiceUnavailable, "EventKit"},
		{"reminders_access_denied", reminders.ErrAccessDenied, http.StatusServiceUnavailable, "EventKit"},
		{"calendar_unsupported", calendar.ErrUnsupported, http.StatusNotImplemented, "not supported"},
		{"reminders_unsupported", reminders.ErrUnsupported, http.StatusNotImplemented, "not supported"},
		{"generic", errors.New("something broke"), http.StatusInternalServerError, "something broke"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MapError(context.Background(), tt.in)
			if err == nil {
				t.Fatalf("err = nil, want non-nil")
			}
			var he huma.StatusError
			if !errors.As(err, &he) {
				t.Fatalf("err %v is not huma.StatusError", err)
			}
			if he.GetStatus() != tt.wantStatus {
				t.Errorf("status = %d, want %d", he.GetStatus(), tt.wantStatus)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("err %q missing substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestMapError_PostFetchViolation_422WithDetails(t *testing.T) {
	pfv := &scoped.PostFetchScopeViolation{
		Kind:                "event",
		OrphanID:            "EV-ORPHAN",
		OrphanContainerID:   "CAL-FALLBACK",
		OrphanSource:        "Personal",
		IntendedContainerID: "CAL-HOME",
	}
	err := MapError(context.Background(), fmt.Errorf("scoped: oops: %w", pfv))
	if err == nil {
		t.Fatalf("err = nil")
	}
	em, ok := err.(*huma.ErrorModel)
	if !ok {
		t.Fatalf("err is not *huma.ErrorModel: %T", err)
	}
	if em.Status != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", em.Status)
	}
	// Build a "location:value" lookup from ErrorDetails for assertions.
	got := map[string]string{}
	for _, d := range em.Errors {
		got[d.Location] = fmt.Sprintf("%v", d.Value)
	}
	want := map[string]string{
		"orphan.kind":           "event",
		"orphan.id":             "EV-ORPHAN",
		"orphan.container_id":   "CAL-FALLBACK",
		"orphan.source":         "Personal",
		"intended.container_id": "CAL-HOME",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("detail %q: got %q, want %q (full: %+v)", k, got[k], v, got)
		}
	}
}
