package tools

import (
	"fmt"
	"strings"
	"time"

	"github.com/dillonbrowne/go-eventkit/dateparser"
)

// DateDoc is the standard hint appended to tool descriptions whose
// inputs include a date string. Keeps the wording consistent across the
// 24 tools and easy to update in one place.
const DateDoc = "Dates accept ISO 8601 (e.g. `2026-05-26T15:00:00Z`) or natural language (`tomorrow 2pm`, `next friday`, `eod`, `in 2 hours`)."

// dateParseOpts is the canonical parser configuration used by all MCP
// tools. WithSmartTimeRollover ensures "9am" rolls to tomorrow if it's
// already past today; WithDefaultHour(9) makes bare dates ("tomorrow")
// land at 09:00 local time rather than midnight.
var dateParseOpts = []dateparser.Option{
	dateparser.WithSmartTimeRollover(),
	dateparser.WithDefaultHour(9),
}

// ParseDate converts an LLM-supplied date string into time.Time. Empty
// input returns the zero value with no error so the caller can use it
// as an "omit" sentinel. Anything else is run through dateparser.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	t, err := dateparser.ParseDate(s, dateParseOpts...)
	if err != nil {
		return time.Time{}, fmt.Errorf("could not parse date %q: %w", s, err)
	}
	return t, nil
}

// ParseDateRequired is like ParseDate but treats the empty string as an
// error. Use it for required date fields.
func ParseDateRequired(field, s string) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}
	return ParseDate(s)
}

// ParseDatePtr returns a *time.Time pointer that is nil for empty input
// and a parsed time otherwise. Used for optional pointer fields on
// REST update inputs.
func ParseDatePtr(s string) (*time.Time, error) {
	t, err := ParseDate(s)
	if err != nil {
		return nil, err
	}
	if t.IsZero() {
		return nil, nil
	}
	return &t, nil
}
