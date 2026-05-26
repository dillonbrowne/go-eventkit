// Package policy declares the allowlist that scopes the REST API's access
// to calendars and reminder lists.
//
// A policy is loaded from a YAML file at server startup. For each managed
// package (calendar, reminders) the policy lists explicit allow entries —
// matched by calendar/list ID or by account source — and a default mode
// applied when nothing matches. The resolved [Mode] for a given target
// answers two questions:
//
//   - CanRead reports whether the API is allowed to list or fetch the target.
//   - CanWrite reports whether the API is allowed to create, modify, or
//     delete the target.
//
// Resolution rules: if any entry matches the target's ID, the most
// restrictive of those wins (safer when entries disagree). Otherwise,
// among entries matching by source, the most permissive wins. If
// nothing matches, the package's Default mode applies.
package policy

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Mode is a policy decision for a single target. Higher values are more
// permissive: ModeDeny < ModeRead < ModeReadWrite.
type Mode int

const (
	// ModeDeny forbids all access.
	ModeDeny Mode = iota
	// ModeRead allows list and fetch operations.
	ModeRead
	// ModeReadWrite allows list, fetch, create, update, and delete.
	ModeReadWrite
)

// CanRead reports whether the mode permits read operations.
func (m Mode) CanRead() bool { return m >= ModeRead }

// CanWrite reports whether the mode permits write operations.
func (m Mode) CanWrite() bool { return m >= ModeReadWrite }

// String returns the canonical string form of the mode.
func (m Mode) String() string {
	switch m {
	case ModeDeny:
		return "deny"
	case ModeRead:
		return "read"
	case ModeReadWrite:
		return "readwrite"
	default:
		return fmt.Sprintf("mode(%d)", int(m))
	}
}

func parseMode(s string) (Mode, error) {
	switch s {
	case "deny", "":
		return ModeDeny, nil
	case "read":
		return ModeRead, nil
	case "readwrite", "read_write", "rw":
		return ModeReadWrite, nil
	default:
		return ModeDeny, fmt.Errorf("unknown mode %q (want deny, read, or readwrite)", s)
	}
}

// Entry is a single allow rule. Exactly one of ID or Source is set.
type Entry struct {
	ID     string
	Source string
	Mode   Mode
}

// PackagePolicy holds the entries and default for one managed package.
type PackagePolicy struct {
	Default Mode
	Entries []Entry
}

// Resolve returns the mode that applies to a target identified by its
// stable ID and account source. See the package documentation for the
// precedence rules.
func (p PackagePolicy) Resolve(id, source string) Mode {
	// ID-explicit entries always override source-level entries. If multiple
	// entries name the same ID with different modes, take the most
	// restrictive — disagreement is a config error and a safer default.
	var idMode *Mode
	for _, e := range p.Entries {
		if e.ID == "" || e.ID != id {
			continue
		}
		if idMode == nil || e.Mode < *idMode {
			m := e.Mode
			idMode = &m
		}
	}
	if idMode != nil {
		return *idMode
	}

	// Among source matches, take the most permissive.
	bestSource := ModeDeny
	hasSource := false
	for _, e := range p.Entries {
		if e.ID != "" || e.Source == "" || e.Source != source {
			continue
		}
		if !hasSource || e.Mode > bestSource {
			bestSource = e.Mode
			hasSource = true
		}
	}
	if hasSource {
		return bestSource
	}
	return p.Default
}

// Policy is the loaded allowlist for the server.
type Policy struct {
	Calendar  PackagePolicy
	Reminders PackagePolicy
}

// Permissive returns a policy that grants readwrite access to every
// calendar and reminder list. The server uses this as the implicit
// default when no policy is supplied — it removes the start-up friction
// for local development and demos but should not be used when exposing
// the API to an AI agent or MCP client. Pass a real allowlist via
// [Load] (and `WithPolicy`) for those cases.
func Permissive() *Policy {
	return &Policy{
		Calendar:  PackagePolicy{Default: ModeReadWrite},
		Reminders: PackagePolicy{Default: ModeReadWrite},
	}
}

// DenyAll returns a policy that refuses every read and write. Useful as
// an explicit "deny" baseline in tests, or for callers that want to
// construct a policy programmatically (entry by entry on top of
// [PackagePolicy.Default]).
func DenyAll() *Policy {
	return &Policy{
		Calendar:  PackagePolicy{Default: ModeDeny},
		Reminders: PackagePolicy{Default: ModeDeny},
	}
}

// ForCalendar returns the mode for the given calendar identifier and source.
func (p *Policy) ForCalendar(id, source string) Mode {
	return p.Calendar.Resolve(id, source)
}

// ForReminderList returns the mode for the given reminder list identifier
// and source.
func (p *Policy) ForReminderList(id, source string) Mode {
	return p.Reminders.Resolve(id, source)
}

type yamlEntry struct {
	ID     string `yaml:"id,omitempty"`
	ListID string `yaml:"list_id,omitempty"`
	Source string `yaml:"source,omitempty"`
	Mode   string `yaml:"mode"`
}

type yamlPackage struct {
	Default string      `yaml:"default"`
	Allow   []yamlEntry `yaml:"allow"`
}

type yamlPolicy struct {
	Calendar  yamlPackage `yaml:"calendar"`
	Reminders yamlPackage `yaml:"reminders"`
}

// Load parses a YAML policy from r and returns a validated [*Policy].
// Unknown keys are rejected to catch typos in user configs.
func Load(r io.Reader) (*Policy, error) {
	var raw yamlPolicy
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("policy: parse yaml: %w", err)
	}

	pol := &Policy{}
	cal, err := buildPackage("calendar", raw.Calendar, false)
	if err != nil {
		return nil, err
	}
	pol.Calendar = cal

	rem, err := buildPackage("reminders", raw.Reminders, true)
	if err != nil {
		return nil, err
	}
	pol.Reminders = rem

	return pol, nil
}

func buildPackage(name string, p yamlPackage, useListID bool) (PackagePolicy, error) {
	defMode, err := parseMode(p.Default)
	if err != nil {
		return PackagePolicy{}, fmt.Errorf("policy: %s.default: %w", name, err)
	}

	out := PackagePolicy{Default: defMode}
	for i, e := range p.Allow {
		mode, err := parseMode(e.Mode)
		if err != nil {
			return PackagePolicy{}, fmt.Errorf("policy: %s.allow[%d].mode: %w", name, i, err)
		}

		var id string
		if useListID {
			if e.ID != "" {
				return PackagePolicy{}, fmt.Errorf("policy: %s.allow[%d]: use 'list_id', not 'id'", name, i)
			}
			id = e.ListID
		} else {
			if e.ListID != "" {
				return PackagePolicy{}, fmt.Errorf("policy: %s.allow[%d]: use 'id', not 'list_id'", name, i)
			}
			id = e.ID
		}

		if id == "" && e.Source == "" {
			return PackagePolicy{}, fmt.Errorf("policy: %s.allow[%d]: entry must specify id or source", name, i)
		}
		if id != "" && e.Source != "" {
			return PackagePolicy{}, fmt.Errorf("policy: %s.allow[%d]: entry must specify id OR source, not both", name, i)
		}
		out.Entries = append(out.Entries, Entry{ID: id, Source: e.Source, Mode: mode})
	}
	return out, nil
}
