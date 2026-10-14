package main

import (
	"testing"
	"time"
)

func TestHistoryRetentionDefaultsAndBounds(t *testing.T) {
	for environment, want := range map[string]time.Duration{
		"":      7 * 24 * time.Hour,
		"1h":    time.Hour,
		"720h":  720 * time.Hour,
		"8760h": 8760 * time.Hour,
	} {
		got, err := historyRetentionDefault(environment)
		if err != nil || got != want {
			t.Fatalf("CONSOLE_HISTORY_RETENTION=%q = %s, %v; want %s", environment, got, err, want)
		}
	}
	for _, environment := range []string{"seven days", "59m", "0", "-1h", "8761h"} {
		if _, err := historyRetentionDefault(environment); err == nil {
			t.Fatalf("CONSOLE_HISTORY_RETENTION=%q was accepted", environment)
		}
	}
}
