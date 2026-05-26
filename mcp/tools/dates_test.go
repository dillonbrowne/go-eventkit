package tools

import (
	"strings"
	"testing"
)

func TestParseDate_Empty(t *testing.T) {
	got, err := ParseDate("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for empty input, got %v", got)
	}
}

func TestParseDate_ISO8601(t *testing.T) {
	got, err := ParseDate("2026-05-26T15:00:00Z")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.IsZero() {
		t.Errorf("expected non-zero")
	}
}

func TestParseDate_Natural(t *testing.T) {
	tests := []string{"tomorrow 2pm", "next friday", "in 3 hours", "eod"}
	for _, s := range tests {
		t.Run(s, func(t *testing.T) {
			got, err := ParseDate(s)
			if err != nil {
				t.Errorf("ParseDate(%q): %v", s, err)
			}
			if got.IsZero() {
				t.Errorf("ParseDate(%q): zero time", s)
			}
		})
	}
}

func TestParseDate_Garbage(t *testing.T) {
	_, err := ParseDate("not a real date at all")
	if err == nil {
		t.Errorf("expected error on garbage input")
	}
	if !strings.Contains(err.Error(), "could not parse") {
		t.Errorf("error message should mention parse failure: %v", err)
	}
}

func TestParseDateRequired_EmptyErrors(t *testing.T) {
	_, err := ParseDateRequired("startDate", "")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("expected required-field error, got %v", err)
	}
}

func TestParseDatePtr_EmptyNil(t *testing.T) {
	got, err := ParseDatePtr("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil pointer for empty input, got %v", got)
	}
}

func TestParseDatePtr_NonEmpty(t *testing.T) {
	got, err := ParseDatePtr("tomorrow")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil || got.IsZero() {
		t.Errorf("expected non-nil non-zero pointer")
	}
}
