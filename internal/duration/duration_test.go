package duration

import (
	"testing"
	"time"
)

// Cover the human-spelling normalization -- the reason this package wraps xhit at all. Forms like "1day",
// "1 hour 30 minutes", "1sec" are exercised nowhere else; util fixtures only use compact/xhit-native forms.
func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"5s", 5 * time.Second},
		{"500ms", 500 * time.Millisecond},
		{"1h30m", 90 * time.Minute},
		{"1h30min", 90 * time.Minute},
		{"1 hour 30 minutes", 90 * time.Minute},
		{"1day", 24 * time.Hour},
		{"1days", 24 * time.Hour},
		{"1d", 24 * time.Hour},
		{"1w", 7 * 24 * time.Hour},
		{"1week", 7 * 24 * time.Hour},
		{"1sec", time.Second},
		{"5 seconds", 5 * time.Second},
		{"-5s", 5 * time.Second}, // absolute magnitude
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Parse(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// Pin the refusal of variable-length calendar units. Flip this test if year/month ever gets added.
func TestParseRejectsCalendarUnits(t *testing.T) {
	for _, s := range []string{"1y", "1year", "1mo", "1month"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", s)
		}
	}
}
