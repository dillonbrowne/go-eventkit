package policy

import (
	"strings"
	"testing"
)

// FuzzLoad asserts the YAML loader never panics and either returns an
// error or a policy that respects its own invariants (default mode is
// valid, entries have exactly one of id/source set).
func FuzzLoad(f *testing.F) {
	seeds := []string{
		"",
		"calendar:\n  default: deny\n",
		"calendar:\n  default: readwrite\nreminders:\n  default: read\n",
		"calendar:\n  default: deny\n  allow:\n    - id: X\n      mode: read\n",
		"calendar:\n  allow:\n    - source: iCloud\n      mode: rw\n",
		`calendar: [oops`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		// Guard against truly enormous inputs that would just slow the
		// fuzzer down without testing anything interesting.
		if len(in) > 64*1024 {
			return
		}
		pol, err := Load(strings.NewReader(in))
		if err != nil {
			// Any error is acceptable; the contract is "no panic".
			return
		}
		for _, e := range pol.Calendar.Entries {
			if (e.ID == "") == (e.Source == "") {
				t.Fatalf("calendar entry must have exactly one of id/source: %+v", e)
			}
			if e.Mode < ModeDeny || e.Mode > ModeReadWrite {
				t.Fatalf("calendar entry has invalid mode: %+v", e)
			}
		}
		for _, e := range pol.Reminders.Entries {
			if (e.ID == "") == (e.Source == "") {
				t.Fatalf("reminder entry must have exactly one of id/source: %+v", e)
			}
			if e.Mode < ModeDeny || e.Mode > ModeReadWrite {
				t.Fatalf("reminder entry has invalid mode: %+v", e)
			}
		}
		if pol.Calendar.Default < ModeDeny || pol.Calendar.Default > ModeReadWrite {
			t.Fatalf("invalid calendar.default: %v", pol.Calendar.Default)
		}
		if pol.Reminders.Default < ModeDeny || pol.Reminders.Default > ModeReadWrite {
			t.Fatalf("invalid reminders.default: %v", pol.Reminders.Default)
		}
	})
}
