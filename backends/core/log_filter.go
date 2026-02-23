package core

import (
	"strconv"
	"strings"
	"time"
)

// FilterLogTail returns the last n lines. If n <= 0, returns empty.
// If n >= len(lines), returns all lines.
func FilterLogTail(lines []string, n int) []string {
	if n <= 0 {
		return nil
	}
	if n >= len(lines) {
		return lines
	}
	return lines[len(lines)-n:]
}

// FilterLogSince keeps lines with timestamp >= since.
// Lines without a parseable timestamp prefix are kept.
func FilterLogSince(lines []string, since time.Time) []string {
	var result []string
	for _, line := range lines {
		ts, ok := parseLineTimestamp(line)
		if !ok || !ts.Before(since) {
			result = append(result, line)
		}
	}
	return result
}

// FilterLogUntil keeps lines with timestamp < until.
// Lines without a parseable timestamp prefix are kept.
func FilterLogUntil(lines []string, until time.Time) []string {
	var result []string
	for _, line := range lines {
		ts, ok := parseLineTimestamp(line)
		if !ok || ts.Before(until) {
			result = append(result, line)
		}
	}
	return result
}

// ParseDockerTimestamp parses a timestamp string from Docker's since/until
// query parameters. Accepts RFC3339Nano or Unix epoch (integer or float).
func ParseDockerTimestamp(s string) (time.Time, error) {
	// Try RFC3339Nano first
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	// Try Unix epoch (integer)
	if epoch, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(epoch, 0), nil
	}
	// Try Unix epoch (float)
	if epoch, err := strconv.ParseFloat(s, 64); err == nil {
		sec := int64(epoch)
		nsec := int64((epoch - float64(sec)) * 1e9)
		return time.Unix(sec, nsec), nil
	}
	return time.Time{}, &time.ParseError{Value: s, Message: "unrecognized timestamp format"}
}

// parseLineTimestamp extracts the RFC3339Nano timestamp from the beginning
// of a log line (format: "2006-01-02T15:04:05.999999999Z07:00 message").
func parseLineTimestamp(line string) (time.Time, bool) {
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, line[:idx])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
