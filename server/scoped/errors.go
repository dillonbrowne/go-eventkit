package scoped

import (
	"errors"
	"fmt"
)

// ErrPolicyDenied is returned when an operation is forbidden by the
// configured [policy.Policy]. The HTTP layer maps this to 404 to avoid
// leaking the existence of denied resources.
var ErrPolicyDenied = errors.New("scoped: denied by policy")

// ErrPostFetchScopeViolation is the sentinel that *PostFetchScopeViolation
// errors wrap. The HTTP layer maps this to 422 — the write *did* succeed
// in EventKit but landed outside the writable scope, so the client
// learns about an orphaned artifact that needs manual cleanup.
//
// Auto-rollback is deliberately not performed: EventKit is not
// transactional and a failed rollback would compound the inconsistency.
var ErrPostFetchScopeViolation = errors.New("scoped: returned object falls outside writable scope")

// PostFetchScopeViolation is the typed error carrying enough context for
// the HTTP layer to render a structured 422 body. It wraps
// [ErrPostFetchScopeViolation] so callers can still match via errors.Is.
type PostFetchScopeViolation struct {
	// Kind is "event", "calendar", "reminder", or "list".
	Kind string
	// OrphanID is the identifier of the object EventKit returned.
	OrphanID string
	// OrphanContainerID is the calendar/list id the orphan lives in (so
	// the operator knows where to look).
	OrphanContainerID string
	// OrphanSource is the account source of the orphan's container.
	OrphanSource string
	// IntendedContainerID is the calendar/list id the caller asked for.
	IntendedContainerID string
}

// Error implements error.
func (v *PostFetchScopeViolation) Error() string {
	return fmt.Sprintf("scoped: post-fetch scope violation: %s %q lives in %s %q (source %q) but caller asked for %s %q",
		v.Kind, v.OrphanID, containerKind(v.Kind), v.OrphanContainerID, v.OrphanSource, containerKind(v.Kind), v.IntendedContainerID)
}

// Unwrap exposes [ErrPostFetchScopeViolation] so errors.Is matches.
func (v *PostFetchScopeViolation) Unwrap() error { return ErrPostFetchScopeViolation }

func containerKind(itemKind string) string {
	switch itemKind {
	case "event":
		return "calendar"
	case "reminder":
		return "list"
	default:
		return itemKind
	}
}

// ErrTargetRequired is returned when a write operation is missing the
// target calendar/list/source needed to evaluate the policy. Implicit
// "use the system default" is not permitted when a policy is in effect.
var ErrTargetRequired = errors.New("scoped: target calendar/list/source required for this operation")
