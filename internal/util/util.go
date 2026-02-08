// Package util provides shared utility functions for the news-app.
package util

import (
	"os"
	"strings"
	"time"
)

// Job frequency constants
const (
	FreqHourly  = "hourly"
	Freq6Hours  = "6hours"
	FreqDaily   = "daily"
	FreqWeekly  = "weekly"
)

// GetEnv returns the value of the environment variable, or the default if not set.
func GetEnv(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

// CalculateNextRun returns the next scheduled run time based on frequency.
// If isOneTime is true, returns a time 10 seconds in the future.
func CalculateNextRun(frequency string, isOneTime bool) time.Time {
	now := time.Now()
	if isOneTime {
		return now.Add(10 * time.Second)
	}
	switch frequency {
	case FreqHourly:
		return now.Add(1 * time.Hour)
	case Freq6Hours:
		return now.Add(6 * time.Hour)
	case FreqDaily:
		return now.Add(24 * time.Hour)
	case FreqWeekly:
		return now.Add(7 * 24 * time.Hour)
	default:
		return now.Add(24 * time.Hour)
	}
}

// FrequencyToCalendar converts a frequency string to a systemd calendar spec.
func FrequencyToCalendar(freq string) string {
	switch freq {
	case FreqHourly:
		return "*-*-* *:00:00"
	case Freq6Hours:
		return "*-*-* 00/6:00:00"
	case FreqDaily:
		return "*-*-* 06:00:00"
	case FreqWeekly:
		return "Mon *-*-* 06:00:00"
	default:
		return "*-*-* 06:00:00"
	}
}

