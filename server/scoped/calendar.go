package scoped

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/server/policy"
)

// CalendarBridge is the subset of [calendar.Client] that ScopedCalendar
// uses. It mirrors server.CalendarBridge but is redeclared here so the
// scoped package can be imported without cycling through server.
type CalendarBridge interface {
	Calendars() ([]calendar.Calendar, error)
	Events(start, end time.Time, opts ...calendar.ListOption) ([]calendar.Event, error)
	Event(id string) (*calendar.Event, error)
	CreateEvent(input calendar.CreateEventInput) (*calendar.Event, error)
	UpdateEvent(id string, input calendar.UpdateEventInput, span calendar.Span) (*calendar.Event, error)
	DeleteEvent(id string, span calendar.Span) error
	DeleteEvents(ids []string, span calendar.Span) map[string]error
	CreateCalendar(input calendar.CreateCalendarInput) (*calendar.Calendar, error)
	UpdateCalendar(id string, input calendar.UpdateCalendarInput) (*calendar.Calendar, error)
	DeleteCalendar(id string) error
	WatchChanges(ctx context.Context) (<-chan struct{}, error)
}

// ScopedCalendar enforces the policy on every read and write that flows
// through it. The wrapper is the central defense-in-depth layer of the
// server — it consults the policy on entry and re-verifies the result
// on exit, so a bug in any single layer cannot grant access by itself.
//
// The policy reference is held in an atomic.Pointer so an in-flight
// reload (SIGHUP) can swap the policy without locking out concurrent
// readers. All callers go through [ScopedCalendar.policy] rather than
// reading the field directly.
type ScopedCalendar struct {
	bridge CalendarBridge
	pol    atomic.Pointer[policy.Policy]
}

// NewCalendar returns a ScopedCalendar wrapping the given bridge.
func NewCalendar(bridge CalendarBridge, pol *policy.Policy) *ScopedCalendar {
	s := &ScopedCalendar{bridge: bridge}
	s.pol.Store(pol)
	return s
}

// ReplacePolicy atomically swaps the active policy. Used by SIGHUP
// hot-reload — readers see the new policy on their next call without
// taking any lock.
func (s *ScopedCalendar) ReplacePolicy(p *policy.Policy) { s.pol.Store(p) }

// policy returns the current policy. Lock-free read.
func (s *ScopedCalendar) policy() *policy.Policy { return s.pol.Load() }

// calendarIndex is a snapshot of bridge.Calendars() used to map a calendar
// identifier to its source account name. Rebuilt per top-level call rather
// than long-lived so policy changes and freshly-synced calendars take
// effect on the next request.
type calendarIndex struct {
	byID   map[string]calendar.Calendar
	byName map[string]calendar.Calendar
}

func (s *ScopedCalendar) index() (*calendarIndex, error) {
	cals, err := s.bridge.Calendars()
	if err != nil {
		return nil, err
	}
	idx := &calendarIndex{
		byID:   make(map[string]calendar.Calendar, len(cals)),
		byName: make(map[string]calendar.Calendar, len(cals)),
	}
	for _, c := range cals {
		idx.byID[c.ID] = c
		idx.byName[c.Title] = c
	}
	return idx, nil
}

func (s *ScopedCalendar) modeForCalID(idx *calendarIndex, id string) policy.Mode {
	c, ok := idx.byID[id]
	if !ok {
		return policy.ModeDeny
	}
	return s.policy().ForCalendar(c.ID, c.Source)
}

func (s *ScopedCalendar) modeForCalName(idx *calendarIndex, name string) (calendar.Calendar, policy.Mode, bool) {
	c, ok := idx.byName[name]
	if !ok {
		return calendar.Calendar{}, policy.ModeDeny, false
	}
	return c, s.policy().ForCalendar(c.ID, c.Source), true
}

// Calendars returns the calendars that are readable under the policy.
func (s *ScopedCalendar) Calendars() ([]calendar.Calendar, error) {
	cals, err := s.bridge.Calendars()
	if err != nil {
		return nil, err
	}
	out := make([]calendar.Calendar, 0, len(cals))
	for _, c := range cals {
		if s.policy().ForCalendar(c.ID, c.Source).CanRead() {
			out = append(out, c)
		}
	}
	return out, nil
}

// Calendar returns a single readable calendar, or ErrPolicyDenied (→ 404).
func (s *ScopedCalendar) Calendar(id string) (*calendar.Calendar, error) {
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	c, ok := idx.byID[id]
	if !ok {
		return nil, calendar.ErrNotFound
	}
	if !s.policy().ForCalendar(c.ID, c.Source).CanRead() {
		return nil, ErrPolicyDenied
	}
	return &c, nil
}

// Events returns events whose calendar is readable under the policy.
func (s *ScopedCalendar) Events(start, end time.Time, opts ...calendar.ListOption) ([]calendar.Event, error) {
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	events, err := s.bridge.Events(start, end, opts...)
	if err != nil {
		return nil, err
	}
	out := make([]calendar.Event, 0, len(events))
	for _, e := range events {
		if s.modeForCalID(idx, e.CalendarID).CanRead() {
			out = append(out, e)
		}
	}
	return out, nil
}

// Event returns a single event, or ErrPolicyDenied if its calendar is not
// readable.
func (s *ScopedCalendar) Event(id string) (*calendar.Event, error) {
	ev, err := s.bridge.Event(id)
	if err != nil {
		return nil, err
	}
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	if !s.modeForCalID(idx, ev.CalendarID).CanRead() {
		return nil, ErrPolicyDenied
	}
	return ev, nil
}

// CreateEvent requires input.Calendar to be set so the policy can be
// evaluated up-front; "use the system default" is refused.
func (s *ScopedCalendar) CreateEvent(input calendar.CreateEventInput) (*calendar.Event, error) {
	if input.Calendar == "" {
		return nil, ErrTargetRequired
	}
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	target, mode, ok := s.modeForCalName(idx, input.Calendar)
	if !ok || !mode.CanWrite() {
		return nil, ErrPolicyDenied
	}
	ev, err := s.bridge.CreateEvent(input)
	if err != nil {
		return nil, err
	}
	idx2, err := s.index()
	if err != nil {
		return nil, err
	}
	if ev.CalendarID != target.ID || !s.modeForCalID(idx2, ev.CalendarID).CanWrite() {
		return nil, &PostFetchScopeViolation{
			Kind:                "event",
			OrphanID:            ev.ID,
			OrphanContainerID:   ev.CalendarID,
			OrphanSource:        idx2.byID[ev.CalendarID].Source,
			IntendedContainerID: target.ID,
		}
	}
	return ev, nil
}

// UpdateEvent enforces writable scope on the event's current calendar and,
// if the update moves it, on the destination calendar.
func (s *ScopedCalendar) UpdateEvent(id string, input calendar.UpdateEventInput, span calendar.Span) (*calendar.Event, error) {
	idx, err := s.index()
	if err != nil {
		return nil, err
	}

	existing, err := s.bridge.Event(id)
	if err != nil {
		return nil, err
	}
	if !s.modeForCalID(idx, existing.CalendarID).CanWrite() {
		return nil, ErrPolicyDenied
	}

	destID := existing.CalendarID
	if input.Calendar != nil && *input.Calendar != "" && *input.Calendar != existing.Calendar {
		target, mode, ok := s.modeForCalName(idx, *input.Calendar)
		if !ok || !mode.CanWrite() {
			return nil, ErrPolicyDenied
		}
		destID = target.ID
	}

	ev, err := s.bridge.UpdateEvent(id, input, span)
	if err != nil {
		return nil, err
	}
	idx2, err := s.index()
	if err != nil {
		return nil, err
	}
	if ev.CalendarID != destID || !s.modeForCalID(idx2, ev.CalendarID).CanWrite() {
		return nil, &PostFetchScopeViolation{
			Kind:                "event",
			OrphanID:            ev.ID,
			OrphanContainerID:   ev.CalendarID,
			OrphanSource:        idx2.byID[ev.CalendarID].Source,
			IntendedContainerID: destID,
		}
	}
	return ev, nil
}

// DeleteEvent enforces writable scope on the event's current calendar.
func (s *ScopedCalendar) DeleteEvent(id string, span calendar.Span) error {
	idx, err := s.index()
	if err != nil {
		return err
	}
	existing, err := s.bridge.Event(id)
	if err != nil {
		return err
	}
	if !s.modeForCalID(idx, existing.CalendarID).CanWrite() {
		return ErrPolicyDenied
	}
	return s.bridge.DeleteEvent(id, span)
}

// DeleteEvents partitions the input into allowed and denied IDs, records
// ErrPolicyDenied (or the underlying lookup error) for the denied set, and
// delegates the allowed set to the bridge in a single batch call.
func (s *ScopedCalendar) DeleteEvents(ids []string, span calendar.Span) map[string]error {
	out := make(map[string]error, len(ids))
	idx, err := s.index()
	if err != nil {
		for _, id := range ids {
			out[id] = err
		}
		return out
	}
	var pass []string
	for _, id := range ids {
		ev, err := s.bridge.Event(id)
		if err != nil {
			out[id] = err
			continue
		}
		if !s.modeForCalID(idx, ev.CalendarID).CanWrite() {
			out[id] = ErrPolicyDenied
			continue
		}
		pass = append(pass, id)
	}
	if len(pass) == 0 {
		return out
	}
	// The bridge reports only failures, so pre-mark every attempted id as a
	// success and overlay any failures it returns. Without this, a fully
	// successful batch yields an empty results map and the caller cannot tell
	// which ids were actually deleted.
	for _, id := range pass {
		out[id] = nil
	}
	for id, err := range s.bridge.DeleteEvents(pass, span) {
		out[id] = err
	}
	return out
}

// CreateCalendar requires input.Source to be set and that source to have
// write permission at the policy level.
func (s *ScopedCalendar) CreateCalendar(input calendar.CreateCalendarInput) (*calendar.Calendar, error) {
	if input.Source == "" {
		return nil, ErrTargetRequired
	}
	if !s.policy().ForCalendar("", input.Source).CanWrite() {
		return nil, ErrPolicyDenied
	}
	cal, err := s.bridge.CreateCalendar(input)
	if err != nil {
		return nil, err
	}
	if !s.policy().ForCalendar(cal.ID, cal.Source).CanWrite() {
		return nil, &PostFetchScopeViolation{
			Kind:              "calendar",
			OrphanID:          cal.ID,
			OrphanContainerID: cal.ID,
			OrphanSource:      cal.Source,
		}
	}
	return cal, nil
}

// UpdateCalendar enforces writable scope on the target calendar.
func (s *ScopedCalendar) UpdateCalendar(id string, input calendar.UpdateCalendarInput) (*calendar.Calendar, error) {
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	c, ok := idx.byID[id]
	if !ok {
		return nil, calendar.ErrNotFound
	}
	if !s.policy().ForCalendar(c.ID, c.Source).CanWrite() {
		return nil, ErrPolicyDenied
	}
	cal, err := s.bridge.UpdateCalendar(id, input)
	if err != nil {
		return nil, err
	}
	if !s.policy().ForCalendar(cal.ID, cal.Source).CanWrite() {
		return nil, &PostFetchScopeViolation{
			Kind:                "calendar",
			OrphanID:            cal.ID,
			OrphanContainerID:   cal.ID,
			OrphanSource:        cal.Source,
			IntendedContainerID: id,
		}
	}
	return cal, nil
}

// DeleteCalendar enforces writable scope on the target calendar.
func (s *ScopedCalendar) DeleteCalendar(id string) error {
	idx, err := s.index()
	if err != nil {
		return err
	}
	c, ok := idx.byID[id]
	if !ok {
		return calendar.ErrNotFound
	}
	if !s.policy().ForCalendar(c.ID, c.Source).CanWrite() {
		return ErrPolicyDenied
	}
	return s.bridge.DeleteCalendar(id)
}
