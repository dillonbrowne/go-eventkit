package scoped

import (
	"errors"
	"testing"

	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server/testfakes"
)

func fixtureReminderBridge() *testfakes.RemFake {
	return &testfakes.RemFake{
		Lists: []reminders.List{
			{ID: "LIST-TODO", Title: "Todo", Source: "iCloud"},
			{ID: "LIST-ARCHIVE", Title: "Archive", Source: "iCloud"}, // demoted to read by ID
			{ID: "LIST-OUT", Title: "Outside", Source: "Personal"},   // denied
		},
		Items: []reminders.Reminder{
			{ID: "R-1", Title: "1", List: "Todo", ListID: "LIST-TODO"},
			{ID: "R-2", Title: "2", List: "Archive", ListID: "LIST-ARCHIVE"},
			{ID: "R-3", Title: "3", List: "Outside", ListID: "LIST-OUT"},
		},
	}
}

func TestScopedReminders_Lists_Filters(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	lists, err := sr.Lists()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, l := range lists {
		got[l.ID] = true
	}
	want := map[string]bool{"LIST-TODO": true, "LIST-ARCHIVE": true}
	if len(got) != len(want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestScopedReminders_List_DeniedReturnsPolicyError(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	if _, err := sr.List("LIST-OUT"); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("denied list: err = %v, want ErrPolicyDenied", err)
	}
	if l, err := sr.List("LIST-ARCHIVE"); err != nil || l.ID != "LIST-ARCHIVE" {
		t.Errorf("read-only list: l=%v err=%v", l, err)
	}
}

func TestScopedReminders_Reminders_Filters(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	rs, err := sr.Reminders()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range rs {
		got[r.ID] = true
	}
	want := map[string]bool{"R-1": true, "R-2": true}
	if len(got) != len(want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestScopedReminders_Reminder_DeniedReturnsPolicyError(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	if _, err := sr.Reminder("R-3"); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("denied reminder fetch: err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedReminders_CreateReminder_RequiresList(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	_, err := sr.CreateReminder(reminders.CreateReminderInput{Title: "X"})
	if !errors.Is(err, ErrTargetRequired) {
		t.Errorf("err = %v, want ErrTargetRequired", err)
	}
}

func TestScopedReminders_CreateReminder_RejectsDenied(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	tests := []struct {
		name string
		list string
	}{
		{"unknown_list", "Nope"},
		{"readonly_by_id", "Archive"},
		{"denied_list", "Outside"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sr.CreateReminder(reminders.CreateReminderInput{Title: "X", ListName: tt.list})
			if !errors.Is(err, ErrPolicyDenied) {
				t.Errorf("err = %v, want ErrPolicyDenied", err)
			}
		})
	}
}

func TestScopedReminders_CreateReminder_SucceedsOnWritable(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	r, err := sr.CreateReminder(reminders.CreateReminderInput{Title: "Pay bill", ListName: "Todo"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if r.ListID != "LIST-TODO" {
		t.Errorf("ListID = %q, want LIST-TODO", r.ListID)
	}
}

func TestScopedReminders_CreateReminder_PostFetchViolation(t *testing.T) {
	br := fixtureReminderBridge()
	br.CreateOverrideListID = "LIST-OUT"
	sr := NewReminders(testfakes.NewReminders(br), fixturePolicy())
	_, err := sr.CreateReminder(reminders.CreateReminderInput{Title: "X", ListName: "Todo"})
	if !errors.Is(err, ErrPostFetchScopeViolation) {
		t.Errorf("err = %v, want ErrPostFetchScopeViolation", err)
	}
}

func TestScopedReminders_UpdateReminder_RejectsReadOnly(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	title := "Renamed"
	_, err := sr.UpdateReminder("R-2", reminders.UpdateReminderInput{Title: &title})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied (LIST-ARCHIVE is read-only)", err)
	}
}

func TestScopedReminders_UpdateReminder_RejectsMoveToDenied(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	dest := "Outside"
	_, err := sr.UpdateReminder("R-1", reminders.UpdateReminderInput{ListName: &dest})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedReminders_DeleteReminder_RejectsReadOnly(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	if err := sr.DeleteReminder("R-2"); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("read-only: err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedReminders_DeleteReminders_Partitions(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	results := sr.DeleteReminders([]string{"R-1", "R-2", "R-3", "R-MISSING"})

	if results["R-1"] != nil {
		t.Errorf("R-1 (writable): err = %v, want nil", results["R-1"])
	}
	if !errors.Is(results["R-2"], ErrPolicyDenied) {
		t.Errorf("R-2 (read-only): err = %v, want ErrPolicyDenied", results["R-2"])
	}
	if !errors.Is(results["R-3"], ErrPolicyDenied) {
		t.Errorf("R-3 (denied): err = %v, want ErrPolicyDenied", results["R-3"])
	}
	if !errors.Is(results["R-MISSING"], reminders.ErrNotFound) {
		t.Errorf("R-MISSING: err = %v, want ErrNotFound", results["R-MISSING"])
	}
}

func TestScopedReminders_Complete_RejectsReadOnly(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	if _, err := sr.CompleteReminder("R-2"); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied", err)
	}
	if _, err := sr.UncompleteReminder("R-2"); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedReminders_Complete_SucceedsOnWritable(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	r, err := sr.CompleteReminder("R-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !r.Completed {
		t.Errorf("Completed = false, want true")
	}
}

func TestScopedReminders_CreateList_RequiresSource(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	_, err := sr.CreateList(reminders.CreateListInput{Title: "X"})
	if !errors.Is(err, ErrTargetRequired) {
		t.Errorf("err = %v, want ErrTargetRequired", err)
	}
}

func TestScopedReminders_CreateList_RejectsDeniedSource(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	_, err := sr.CreateList(reminders.CreateListInput{Title: "X", Source: "Personal"})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedReminders_CreateList_AllowsWritableSource(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	l, err := sr.CreateList(reminders.CreateListInput{Title: "Trips", Source: "iCloud"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if l.Source != "iCloud" {
		t.Errorf("Source = %q, want iCloud", l.Source)
	}
}

func TestScopedReminders_UpdateList_RejectsReadOnly(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	title := "X"
	_, err := sr.UpdateList("LIST-ARCHIVE", reminders.UpdateListInput{Title: &title})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied", err)
	}
}

func TestScopedReminders_DeleteList_RejectsDenied(t *testing.T) {
	sr := NewReminders(testfakes.NewReminders(fixtureReminderBridge()), fixturePolicy())
	if err := sr.DeleteList("LIST-OUT"); !errors.Is(err, ErrPolicyDenied) {
		t.Errorf("err = %v, want ErrPolicyDenied", err)
	}
}
