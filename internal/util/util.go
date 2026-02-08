// Package util provides shared utility functions for the news-app.
package util

import (
	"os"
	"strconv"
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

// GetEnvDuration parses a duration from an environment variable.
// If the value is not set or invalid, the default is returned.
func GetEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}

// GetEnvInt parses an integer from an environment variable.
// If the value is not set or invalid, the default is returned.
func GetEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

// BoolToInt64 converts a boolean to an int64 (1 for true, 0 for false).
// This is useful for SQLite which stores booleans as integers.
func BoolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
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

