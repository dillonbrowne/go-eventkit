package tools

import "time"

// rfc3339 returns t.Format(RFC3339) or "" if t is zero.
func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// rfc3339Ptr is the *time.Time variant.
func rfc3339Ptr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
