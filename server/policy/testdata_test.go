package policy

import (
	"os"
	"path/filepath"
	"testing"
)

// loadFixture is a small helper that reads a testdata YAML file and runs
// Load on it. Kept separate so the table tests in policy_test.go can stay
// inline-clean while the fixture-driven precedence cases live here.
func loadFixture(t *testing.T, name string) *Policy {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	defer f.Close()
	pol, err := Load(f)
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return pol
}

func TestPolicy_Fixture_ExplicitIDDemotesSource(t *testing.T) {
	pol := loadFixture(t, "explicit_id_demotes_source.yaml")
	// iCloud blanket allow is readwrite; CAL-LOCKED should resolve to read.
	if got := pol.ForCalendar("CAL-LOCKED", "iCloud"); got != ModeRead {
		t.Errorf("CAL-LOCKED in iCloud: got %v, want read (id demotes below source)", got)
	}
	if got := pol.ForCalendar("CAL-OTHER", "iCloud"); got != ModeReadWrite {
		t.Errorf("CAL-OTHER in iCloud: got %v, want readwrite (source-level allow)", got)
	}
}

func TestPolicy_Fixture_ExplicitIDPromotesSource(t *testing.T) {
	pol := loadFixture(t, "explicit_id_promotes_source.yaml")
	if got := pol.ForCalendar("CAL-WRITE", "iCloud"); got != ModeReadWrite {
		t.Errorf("CAL-WRITE in iCloud: got %v, want readwrite (id promotes above source)", got)
	}
	if got := pol.ForCalendar("CAL-OTHER", "iCloud"); got != ModeRead {
		t.Errorf("CAL-OTHER in iCloud: got %v, want read (source-level allow)", got)
	}
}

func TestPolicy_Fixture_DefaultAllow(t *testing.T) {
	pol := loadFixture(t, "default_allow.yaml")
	if got := pol.ForCalendar("ANY", "ANY"); got != ModeReadWrite {
		t.Errorf("calendar default-allow: got %v, want readwrite", got)
	}
	if got := pol.ForReminderList("ANY", "ANY"); got != ModeRead {
		t.Errorf("reminders default-allow: got %v, want read", got)
	}
}
