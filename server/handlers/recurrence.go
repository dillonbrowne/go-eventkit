package handlers

import (
	"github.com/dillonbrowne/go-eventkit"
)

// reminderRecurrenceProxy is a thin alias for eventkit.RecurrenceRule —
// kept locally so DTOs do not force consumers to reach into the eventkit
// root package.
type reminderRecurrenceProxy = eventkit.RecurrenceRule

// structuredLocationProxy is a thin alias for eventkit.StructuredLocation.
type structuredLocationProxy = eventkit.StructuredLocation
