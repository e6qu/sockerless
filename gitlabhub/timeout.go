package gitlabhub

import (
	"fmt"
	"strconv"
	"strings"
)

// parseGitLabDuration parses GitLab CI duration strings.
// Supports: "10s" -> 10, "2m" -> 120, "1h" -> 3600, "1h 30m" -> 5400,
// "1 hour" -> 3600, "30 minutes" -> 1800, plain seconds "3600" -> 3600
// Returns duration in seconds. Default is 3600 (1 hour).
func parseGitLabDuration(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 3600
	}

	// Try plain integer (seconds)
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}

	total := 0
	parts := strings.Fields(s)
	for i := 0; i < len(parts); i++ {
		part := strings.ToLower(parts[i])

		// Check if the part itself contains unit suffix (e.g., "30m", "1h")
		if n, unit, ok := splitDurationPart(part); ok {
			total += n * unitToSeconds(unit)
			continue
		}

		// Check if next part is a unit word (e.g., "30 minutes")
		if n, err := strconv.Atoi(part); err == nil && i+1 < len(parts) {
			unit := strings.ToLower(parts[i+1])
			total += n * unitToSeconds(unit)
			i++ // skip unit word
			continue
		}
	}

	if total == 0 {
		return 3600
	}
	return total
}

func splitDurationPart(s string) (int, string, bool) {
	for i, c := range s {
		if c < '0' || c > '9' {
			n, err := strconv.Atoi(s[:i])
			if err != nil {
				return 0, "", false
			}
			return n, s[i:], true
		}
	}
	return 0, "", false
}

func unitToSeconds(unit string) int {
	switch {
	case strings.HasPrefix(unit, "h"):
		return 3600
	case strings.HasPrefix(unit, "m"):
		return 60
	case strings.HasPrefix(unit, "s"):
		return 1
	default:
		return 1
	}
}

// RetryDef represents retry configuration.
type RetryDef struct {
	Max int `json:"max"` // max retry count (0-2, default 0)
}

// parseRetry parses the retry: field from YAML.
func parseRetry(v interface{}) *RetryDef {
	switch val := v.(type) {
	case int:
		return &RetryDef{Max: clampRetry(val)}
	case float64:
		return &RetryDef{Max: clampRetry(int(val))}
	case map[string]interface{}:
		if maxRaw, ok := val["max"]; ok {
			switch m := maxRaw.(type) {
			case int:
				return &RetryDef{Max: clampRetry(m)}
			case float64:
				return &RetryDef{Max: clampRetry(int(m))}
			}
		}
	}
	return nil
}

func clampRetry(n int) int {
	if n < 0 {
		return 0
	}
	if n > 2 {
		return 2
	}
	return n
}

// formatTimeout formats seconds as a human-readable string.
func formatTimeout(seconds int) string {
	if seconds <= 0 {
		return "1h"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	var parts []string
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s > 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}
