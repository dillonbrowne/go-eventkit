package scoped

import (
	"context"
	"sync/atomic"

	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server/policy"
)

// RemindersBridge mirrors server.RemindersBridge to keep this package
// importable without cycling.
type RemindersBridge interface {
	Lists() ([]reminders.List, error)
	Reminders(opts ...reminders.ListOption) ([]reminders.Reminder, error)
	Reminder(id string) (*reminders.Reminder, error)
	CreateReminder(input reminders.CreateReminderInput) (*reminders.Reminder, error)
	UpdateReminder(id string, input reminders.UpdateReminderInput) (*reminders.Reminder, error)
	DeleteReminder(id string) error
	DeleteReminders(ids []string) map[string]error
	CompleteReminder(id string) (*reminders.Reminder, error)
	UncompleteReminder(id string) (*reminders.Reminder, error)
	CreateList(input reminders.CreateListInput) (*reminders.List, error)
	UpdateList(id string, input reminders.UpdateListInput) (*reminders.List, error)
	DeleteList(id string) error
	WatchChanges(ctx context.Context) (<-chan struct{}, error)
}

// ScopedReminders is the reminders-side equivalent of [ScopedCalendar].
// The policy is held in an atomic.Pointer for lock-free reload via
// [ScopedReminders.ReplacePolicy].
type ScopedReminders struct {
	bridge RemindersBridge
	pol    atomic.Pointer[policy.Policy]
}

// NewReminders returns a ScopedReminders wrapping the given bridge.
func NewReminders(bridge RemindersBridge, pol *policy.Policy) *ScopedReminders {
	s := &ScopedReminders{bridge: bridge}
	s.pol.Store(pol)
	return s
}

// ReplacePolicy atomically swaps the active policy. See
// [ScopedCalendar.ReplacePolicy].
func (s *ScopedReminders) ReplacePolicy(p *policy.Policy) { s.pol.Store(p) }

func (s *ScopedReminders) policy() *policy.Policy { return s.pol.Load() }

type listIndex struct {
	byID   map[string]reminders.List
	byName map[string]reminders.List
}

func (s *ScopedReminders) index() (*listIndex, error) {
	lists, err := s.bridge.Lists()
	if err != nil {
		return nil, err
	}
	idx := &listIndex{
		byID:   make(map[string]reminders.List, len(lists)),
		byName: make(map[string]reminders.List, len(lists)),
	}
	for _, l := range lists {
		idx.byID[l.ID] = l
		idx.byName[l.Title] = l
	}
	return idx, nil
}

func (s *ScopedReminders) modeForListID(idx *listIndex, id string) policy.Mode {
	l, ok := idx.byID[id]
	if !ok {
		return policy.ModeDeny
	}
	return s.policy().ForReminderList(l.ID, l.Source)
}

func (s *ScopedReminders) modeForListName(idx *listIndex, name string) (reminders.List, policy.Mode, bool) {
	l, ok := idx.byName[name]
	if !ok {
		return reminders.List{}, policy.ModeDeny, false
	}
	return l, s.policy().ForReminderList(l.ID, l.Source), true
}

// Lists returns the lists that are readable under the policy.
func (s *ScopedReminders) Lists() ([]reminders.List, error) {
	lists, err := s.bridge.Lists()
	if err != nil {
		return nil, err
	}
	out := make([]reminders.List, 0, len(lists))
	for _, l := range lists {
		if s.policy().ForReminderList(l.ID, l.Source).CanRead() {
			out = append(out, l)
		}
	}
	return out, nil
}

// List returns a single readable list, or ErrPolicyDenied.
func (s *ScopedReminders) List(id string) (*reminders.List, error) {
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	l, ok := idx.byID[id]
	if !ok {
		return nil, reminders.ErrNotFound
	}
	if !s.policy().ForReminderList(l.ID, l.Source).CanRead() {
		return nil, ErrPolicyDenied
	}
	return &l, nil
}

// Reminders returns reminders whose list is readable under the policy.
func (s *ScopedReminders) Reminders(opts ...reminders.ListOption) ([]reminders.Reminder, error) {
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	rs, err := s.bridge.Reminders(opts...)
	if err != nil {
		return nil, err
	}
	out := make([]reminders.Reminder, 0, len(rs))
	for _, r := range rs {
		if s.modeForListID(idx, r.ListID).CanRead() {
			out = append(out, r)
		}
	}
	return out, nil
}

// Reminder returns a single reminder, or ErrPolicyDenied if its list is
// not readable.
func (s *ScopedReminders) Reminder(id string) (*reminders.Reminder, error) {
	r, err := s.bridge.Reminder(id)
	if err != nil {
		return nil, err
	}
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	if !s.modeForListID(idx, r.ListID).CanRead() {
		return nil, ErrPolicyDenied
	}
	return r, nil
}

// CreateReminder requires input.ListName.
func (s *ScopedReminders) CreateReminder(input reminders.CreateReminderInput) (*reminders.Reminder, error) {
	if input.ListName == "" {
		return nil, ErrTargetRequired
	}
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	target, mode, ok := s.modeForListName(idx, input.ListName)
	if !ok || !mode.CanWrite() {
		return nil, ErrPolicyDenied
	}
	r, err := s.bridge.CreateReminder(input)
	if err != nil {
		return nil, err
	}
	idx2, err := s.index()
	if err != nil {
		return nil, err
	}
	if r.ListID != target.ID || !s.modeForListID(idx2, r.ListID).CanWrite() {
		return nil, &PostFetchScopeViolation{
			Kind:                "reminder",
			OrphanID:            r.ID,
			OrphanContainerID:   r.ListID,
			OrphanSource:        idx2.byID[r.ListID].Source,
			IntendedContainerID: target.ID,
		}
	}
	return r, nil
}

// UpdateReminder enforces writable scope on the current list and, if the
// update moves the reminder, on the destination list.
func (s *ScopedReminders) UpdateReminder(id string, input reminders.UpdateReminderInput) (*reminders.Reminder, error) {
	idx, err := s.index()
	if err != nil {
		return nil, err
	}

	existing, err := s.bridge.Reminder(id)
	if err != nil {
		return nil, err
	}
	if !s.modeForListID(idx, existing.ListID).CanWrite() {
		return nil, ErrPolicyDenied
	}

	destID := existing.ListID
	if input.ListName != nil && *input.ListName != "" && *input.ListName != existing.List {
		target, mode, ok := s.modeForListName(idx, *input.ListName)
		if !ok || !mode.CanWrite() {
			return nil, ErrPolicyDenied
		}
		destID = target.ID
	}

	r, err := s.bridge.UpdateReminder(id, input)
	if err != nil {
		return nil, err
	}
	idx2, err := s.index()
	if err != nil {
		return nil, err
	}
	if r.ListID != destID || !s.modeForListID(idx2, r.ListID).CanWrite() {
		return nil, &PostFetchScopeViolation{
			Kind:                "reminder",
			OrphanID:            r.ID,
			OrphanContainerID:   r.ListID,
			OrphanSource:        idx2.byID[r.ListID].Source,
			IntendedContainerID: destID,
		}
	}
	return r, nil
}

// DeleteReminder enforces writable scope on the reminder's current list.
func (s *ScopedReminders) DeleteReminder(id string) error {
	idx, err := s.index()
	if err != nil {
		return err
	}
	existing, err := s.bridge.Reminder(id)
	if err != nil {
		return err
	}
	if !s.modeForListID(idx, existing.ListID).CanWrite() {
		return ErrPolicyDenied
	}
	return s.bridge.DeleteReminder(id)
}

// DeleteReminders partitions allowed/denied IDs and delegates the allowed
// batch to the bridge.
func (s *ScopedReminders) DeleteReminders(ids []string) map[string]error {
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
		r, err := s.bridge.Reminder(id)
		if err != nil {
			out[id] = err
			continue
		}
		if !s.modeForListID(idx, r.ListID).CanWrite() {
			out[id] = ErrPolicyDenied
			continue
		}
		pass = append(pass, id)
	}
	if len(pass) == 0 {
		return out
	}
	for id, err := range s.bridge.DeleteReminders(pass) {
		out[id] = err
	}
	return out
}

// CompleteReminder enforces writable scope on the reminder's current list.
func (s *ScopedReminders) CompleteReminder(id string) (*reminders.Reminder, error) {
	return s.statusChange(id, true)
}

// UncompleteReminder enforces writable scope on the reminder's current list.
func (s *ScopedReminders) UncompleteReminder(id string) (*reminders.Reminder, error) {
	return s.statusChange(id, false)
}

func (s *ScopedReminders) statusChange(id string, complete bool) (*reminders.Reminder, error) {
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	existing, err := s.bridge.Reminder(id)
	if err != nil {
		return nil, err
	}
	if !s.modeForListID(idx, existing.ListID).CanWrite() {
		return nil, ErrPolicyDenied
	}
	if complete {
		return s.bridge.CompleteReminder(id)
	}
	return s.bridge.UncompleteReminder(id)
}

// CreateList requires input.Source.
func (s *ScopedReminders) CreateList(input reminders.CreateListInput) (*reminders.List, error) {
	if input.Source == "" {
		return nil, ErrTargetRequired
	}
	if !s.policy().ForReminderList("", input.Source).CanWrite() {
		return nil, ErrPolicyDenied
	}
	l, err := s.bridge.CreateList(input)
	if err != nil {
		return nil, err
	}
	if !s.policy().ForReminderList(l.ID, l.Source).CanWrite() {
		return nil, &PostFetchScopeViolation{
			Kind:              "list",
			OrphanID:          l.ID,
			OrphanContainerID: l.ID,
			OrphanSource:      l.Source,
		}
	}
	return l, nil
}

// UpdateList enforces writable scope on the target list.
func (s *ScopedReminders) UpdateList(id string, input reminders.UpdateListInput) (*reminders.List, error) {
	idx, err := s.index()
	if err != nil {
		return nil, err
	}
	l, ok := idx.byID[id]
	if !ok {
		return nil, reminders.ErrNotFound
	}
	if !s.policy().ForReminderList(l.ID, l.Source).CanWrite() {
		return nil, ErrPolicyDenied
	}
	updated, err := s.bridge.UpdateList(id, input)
	if err != nil {
		return nil, err
	}
	if !s.policy().ForReminderList(updated.ID, updated.Source).CanWrite() {
		return nil, &PostFetchScopeViolation{
			Kind:                "list",
			OrphanID:            updated.ID,
			OrphanContainerID:   updated.ID,
			OrphanSource:        updated.Source,
			IntendedContainerID: id,
		}
	}
	return updated, nil
}

// DeleteList enforces writable scope on the target list.
func (s *ScopedReminders) DeleteList(id string) error {
	idx, err := s.index()
	if err != nil {
		return err
	}
	l, ok := idx.byID[id]
	if !ok {
		return reminders.ErrNotFound
	}
	if !s.policy().ForReminderList(l.ID, l.Source).CanWrite() {
		return ErrPolicyDenied
	}
	return s.bridge.DeleteList(id)
}
