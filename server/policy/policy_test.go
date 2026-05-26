package policy

import (
	"errors"
	"strings"
	"testing"
)

func TestModeCanRead(t *testing.T) {
	tests := []struct {
		mode      Mode
		canRead   bool
		canWrite  bool
		stringRep string
	}{
		{ModeDeny, false, false, "deny"},
		{ModeRead, true, false, "read"},
		{ModeReadWrite, true, true, "readwrite"},
	}
	for _, tt := range tests {
		t.Run(tt.stringRep, func(t *testing.T) {
			if got := tt.mode.CanRead(); got != tt.canRead {
				t.Errorf("CanRead() = %v, want %v", got, tt.canRead)
			}
			if got := tt.mode.CanWrite(); got != tt.canWrite {
				t.Errorf("CanWrite() = %v, want %v", got, tt.canWrite)
			}
			if got := tt.mode.String(); got != tt.stringRep {
				t.Errorf("String() = %q, want %q", got, tt.stringRep)
			}
		})
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"", ModeDeny, false},
		{"deny", ModeDeny, false},
		{"read", ModeRead, false},
		{"readwrite", ModeReadWrite, false},
		{"read_write", ModeReadWrite, false},
		{"rw", ModeReadWrite, false},
		{"write", ModeDeny, true},
		{"yes", ModeDeny, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseMode(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMode(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseMode(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestPackagePolicyResolve(t *testing.T) {
	pkg := PackagePolicy{
		Default: ModeDeny,
		Entries: []Entry{
			{Source: "iCloud", Mode: ModeRead},
			{Source: "iCloud", Mode: ModeReadWrite}, // most permissive wins among same-source matches
			{ID: "CAL-A", Mode: ModeRead},           // explicit id overrides source
			{ID: "CAL-B", Mode: ModeReadWrite},
			{ID: "CAL-C", Mode: ModeReadWrite},
			{ID: "CAL-C", Mode: ModeRead}, // most restrictive wins among same-id matches
		},
	}

	tests := []struct {
		name   string
		id     string
		source string
		want   Mode
	}{
		{"unknown_id_unknown_source_falls_to_default", "OTHER", "Local", ModeDeny},
		{"id_match_overrides_source", "CAL-A", "iCloud", ModeRead},
		{"id_match_for_writable_target", "CAL-B", "iCloud", ModeReadWrite},
		{"same_id_listed_twice_takes_restrictive", "CAL-C", "iCloud", ModeRead},
		{"source_match_only_takes_permissive", "CAL-OTHER", "iCloud", ModeReadWrite},
		{"unknown_source_falls_back_to_default", "CAL-OTHER", "Local", ModeDeny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pkg.Resolve(tt.id, tt.source); got != tt.want {
				t.Errorf("Resolve(%q, %q) = %v, want %v", tt.id, tt.source, got, tt.want)
			}
		})
	}
}

func TestPackagePolicyResolveAllowDefault(t *testing.T) {
	pkg := PackagePolicy{Default: ModeReadWrite}
	if got := pkg.Resolve("anything", "anysource"); got != ModeReadWrite {
		t.Errorf("default-allow policy: got %v, want %v", got, ModeReadWrite)
	}
}

func TestLoadValid(t *testing.T) {
	yamlSrc := `
calendar:
  default: deny
  allow:
    - id: CAL-WORK
      mode: readwrite
    - source: iCloud
      mode: read

reminders:
  default: deny
  allow:
    - list_id: LIST-TODO
      mode: readwrite
    - source: iCloud
      mode: read
`
	pol, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if pol.ForCalendar("CAL-WORK", "iCloud") != ModeReadWrite {
		t.Errorf("explicit-allow calendar should be readwrite")
	}
	if pol.ForCalendar("CAL-OTHER", "iCloud") != ModeRead {
		t.Errorf("source-allow calendar should be read")
	}
	if pol.ForCalendar("CAL-OTHER", "Local") != ModeDeny {
		t.Errorf("non-matching calendar should fall to default deny")
	}
	if pol.ForReminderList("LIST-TODO", "iCloud") != ModeReadWrite {
		t.Errorf("explicit-allow list should be readwrite")
	}
	if pol.ForReminderList("LIST-OTHER", "iCloud") != ModeRead {
		t.Errorf("source-allow list should be read")
	}
	if pol.ForReminderList("LIST-OTHER", "Local") != ModeDeny {
		t.Errorf("non-matching list should fall to default deny")
	}
}

func TestLoadDefaultAllow(t *testing.T) {
	yamlSrc := `
calendar:
  default: readwrite
reminders:
  default: read
`
	pol, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if pol.ForCalendar("ANY", "ANY") != ModeReadWrite {
		t.Errorf("calendar default-allow should grant readwrite")
	}
	if pol.ForReminderList("ANY", "ANY") != ModeRead {
		t.Errorf("reminder default-read should grant read")
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		errSub string
	}{
		{
			name: "unknown_field",
			src: `
calendar:
  default: deny
  allow:
    - id: X
      mode: read
      bogus: true
`,
			errSub: "bogus",
		},
		{
			name: "bad_mode",
			src: `
calendar:
  default: write
`,
			errSub: "calendar.default",
		},
		{
			name: "calendar_entry_uses_list_id",
			src: `
calendar:
  allow:
    - list_id: X
      mode: read
`,
			errSub: "use 'id', not 'list_id'",
		},
		{
			name: "reminders_entry_uses_id",
			src: `
reminders:
  allow:
    - id: X
      mode: read
`,
			errSub: "use 'list_id', not 'id'",
		},
		{
			name: "entry_missing_id_and_source",
			src: `
calendar:
  allow:
    - mode: read
`,
			errSub: "must specify id or source",
		},
		{
			name: "entry_has_both_id_and_source",
			src: `
calendar:
  allow:
    - id: X
      source: iCloud
      mode: read
`,
			errSub: "id OR source",
		},
		{
			name:   "malformed_yaml",
			src:    "calendar: [oops",
			errSub: "parse yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tt.src))
			if err == nil {
				t.Fatalf("Load() error = nil, want error containing %q", tt.errSub)
			}
			if !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("Load() error = %v, want substring %q", err, tt.errSub)
			}
		})
	}
}

func TestLoadEmpty(t *testing.T) {
	// Empty input → both packages default to deny, no entries.
	pol, err := Load(strings.NewReader(""))
	// An empty YAML stream returns io.EOF from the decoder.
	if err == nil {
		// If the implementation accepts empty input, the result must deny all.
		if pol.ForCalendar("X", "Y") != ModeDeny {
			t.Errorf("empty policy must deny all calendar access")
		}
		if pol.ForReminderList("X", "Y") != ModeDeny {
			t.Errorf("empty policy must deny all reminder access")
		}
	} else if !errors.Is(err, errEOF) && !strings.Contains(err.Error(), "EOF") {
		t.Errorf("empty input: unexpected error %v", err)
	}
}

// sentinel so the empty-input test compiles without importing io.
var errEOF = errors.New("EOF")
