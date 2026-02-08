package util

import (
	"testing"
	"time"
)

func TestBoolToInt64(t *testing.T) {
	if BoolToInt64(true) != 1 {
		t.Error("expected 1 for true")
	}
	if BoolToInt64(false) != 0 {
		t.Error("expected 0 for false")
	}
}

func TestCalculateNextRunFrequencies(t *testing.T) {
	cases := []struct {
		freq     string
		expected time.Duration
	}{
		{"hourly", 1 * time.Hour},
		{"6hours", 6 * time.Hour},
		{"daily", 24 * time.Hour},
		{"weekly", 7 * 24 * time.Hour},
		{"unknown", 24 * time.Hour}, // default
	}

	for _, tc := range cases {
		now := time.Now()
		next := CalculateNextRun(tc.freq, false)
		diff := next.Sub(now)
		// Allow 1 second tolerance
		if diff < tc.expected-time.Second || diff > tc.expected+time.Second {
			t.Errorf("frequency %q: expected ~%v, got %v", tc.freq, tc.expected, diff)
		}
	}
}

func TestCalculateNextRun(t *testing.T) {
	// One-time should be ~10 seconds from now
	now := time.Now()
	next := CalculateNextRun("daily", true)
	diff := next.Sub(now)
	if diff < 9*time.Second || diff > 11*time.Second {
		t.Errorf("one-time: expected ~10s, got %v", diff)
	}

	// Recurring should use frequency
	now = time.Now()
	next = CalculateNextRun("hourly", false)
	diff = next.Sub(now)
	if diff < time.Hour-time.Second || diff > time.Hour+time.Second {
		t.Errorf("hourly: expected ~1h, got %v", diff)
	}
}

