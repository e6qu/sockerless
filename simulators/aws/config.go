package main

import (
	"os"
	"time"
)

// execTimeout returns the configured execution auto-stop timeout.
// Reads SIM_EXEC_TIMEOUT env var (Go duration string); defaults to 30s.
func execTimeout() time.Duration {
	if v := os.Getenv("SIM_EXEC_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 30 * time.Second
}
