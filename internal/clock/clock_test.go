package clock

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"15s": 15 * time.Second,
		"30m": 30 * time.Minute,
		"2h":  2 * time.Hour,
		"1d":  24 * time.Hour,
	}
	for input, want := range tests {
		got, err := ParseDuration(input)
		if err != nil {
			t.Fatalf("ParseDuration(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseDuration(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	got := FormatDuration(26*time.Hour + 3*time.Minute + 4*time.Second)
	want := "1d 2h 3m 4s"
	if got != want {
		t.Fatalf("FormatDuration returned %q, want %q", got, want)
	}
}

func TestRFC3339RoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 19, 6, 30, 0, 123, time.UTC)
	text := FormatRFC3339(now)
	got, err := ParseRFC3339(text)
	if err != nil {
		t.Fatalf("ParseRFC3339 returned error: %v", err)
	}
	if !got.Equal(now) {
		t.Fatalf("round trip = %s, want %s", got, now)
	}
}
