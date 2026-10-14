package main

import (
	"errors"
	"time"
)

// History retention bounds. Pruning runs hourly, so a shorter retention is not
// enforced; the upper bound limits database growth.
const (
	defaultHistoryRetention = 7 * 24 * time.Hour
	minHistoryRetention     = time.Hour
	maxHistoryRetention     = 365 * 24 * time.Hour
)

// historyRetentionDefault returns the flag default: CONSOLE_HISTORY_RETENTION when set,
// otherwise seven days.
func historyRetentionDefault(environment string) (time.Duration, error) {
	if environment == "" {
		return defaultHistoryRetention, nil
	}
	value, err := time.ParseDuration(environment)
	if err != nil {
		return 0, errors.New("CONSOLE_HISTORY_RETENTION must be a duration such as 168h")
	}
	return value, validHistoryRetention(value)
}

func validHistoryRetention(value time.Duration) error {
	if value < minHistoryRetention || value > maxHistoryRetention {
		return errors.New("history retention must be between 1h and 8760h")
	}
	return nil
}
