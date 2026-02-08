// Package util provides shared utility functions for the news-app.
package util

import (
	"os"
	"strings"
	"time"
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
	case "hourly":
		return now.Add(1 * time.Hour)
	case "6hours":
		return now.Add(6 * time.Hour)
	case "daily":
		return now.Add(24 * time.Hour)
	case "weekly":
		return now.Add(7 * 24 * time.Hour)
	default:
		return now.Add(24 * time.Hour)
	}
}

