package handlers

import (
	"testing"

	"github.com/dillonbrowne/go-eventkit/calendar"
)

func TestSpanValue_ToCalendar(t *testing.T) {
	tests := []struct {
		in   SpanValue
		want calendar.Span
	}{
		{SpanThis, calendar.SpanThisEvent},
		{SpanFuture, calendar.SpanFutureEvents},
		{"", calendar.SpanThisEvent},        // default
		{"unknown", calendar.SpanThisEvent}, // default
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			if got := tt.in.ToCalendar(); got != tt.want {
				t.Errorf("ToCalendar() = %v, want %v", got, tt.want)
			}
		})
	}
}
