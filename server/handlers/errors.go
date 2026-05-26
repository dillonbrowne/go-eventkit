// Package handlers contains the huma operation registrations that make up
// the REST surface. Every handler in this package follows the same shape:
// (1) parse the request, (2) delegate to a scoped wrapper, (3) translate
// errors via [MapError].
package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server/middleware"
	"github.com/dillonbrowne/go-eventkit/server/scoped"
)

// MapError translates errors from the calendar, reminders, and scoped
// packages into huma HTTP errors. Policy denials are mapped to 404 to
// avoid leaking the existence of denied resources. When ctx is provided
// and the error is a policy denial, the request context is stamped so
// the audit middleware logs an explicit policy_denied=true field.
func MapError(ctx context.Context, err error) error {
	if errors.Is(err, scoped.ErrPolicyDenied) && ctx != nil {
		middleware.MarkPolicyDenied(ctx)
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, calendar.ErrNotFound), errors.Is(err, reminders.ErrNotFound):
		return huma.Error404NotFound("not found")
	case errors.Is(err, scoped.ErrPolicyDenied):
		return huma.Error404NotFound("not found")
	case errors.Is(err, scoped.ErrTargetRequired):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.Is(err, calendar.ErrImmutable), errors.Is(err, reminders.ErrImmutable):
		return huma.Error409Conflict("target is immutable")
	case errors.Is(err, calendar.ErrAccessDenied), errors.Is(err, reminders.ErrAccessDenied):
		return huma.NewError(http.StatusServiceUnavailable, "EventKit access not granted")
	case errors.Is(err, calendar.ErrUnsupported), errors.Is(err, reminders.ErrUnsupported):
		return huma.NewError(http.StatusNotImplemented, "not supported on this platform")
	}

	// Post-fetch scope violation: the write succeeded in EventKit but the
	// returned object lives outside writable scope. Surface the orphan
	// identifiers so the operator can investigate.
	var pfv *scoped.PostFetchScopeViolation
	if errors.As(err, &pfv) {
		return huma.Error422UnprocessableEntity(
			"post-fetch scope violation: write succeeded but object lies outside writable scope; manual cleanup required",
			&huma.ErrorDetail{Location: "orphan.kind", Value: pfv.Kind},
			&huma.ErrorDetail{Location: "orphan.id", Value: pfv.OrphanID},
			&huma.ErrorDetail{Location: "orphan.container_id", Value: pfv.OrphanContainerID},
			&huma.ErrorDetail{Location: "orphan.source", Value: pfv.OrphanSource},
			&huma.ErrorDetail{Location: "intended.container_id", Value: pfv.IntendedContainerID},
		)
	}

	return huma.NewError(http.StatusInternalServerError, err.Error())
}
