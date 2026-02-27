package main

import (
	"strings"
	"time"
)

// parseFilter splits a GCP logging filter string into individual clauses
// joined by AND. Supports: field="value", field>"value", field>="value".
func parseFilter(filter string) []filterClause {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil
	}

	// Split on " AND " (case-sensitive, as GCP requires uppercase AND)
	parts := strings.Split(filter, " AND ")
	var clauses []filterClause
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		c := parseClause(part)
		clauses = append(clauses, c)
	}
	return clauses
}

type filterOp int

const (
	opEq filterOp = iota // =
	opGt                 // >
	opGe                 // >=
)

type filterClause struct {
	field string
	op    filterOp
	value string
}

// parseClause parses a single clause like `field="value"`, `field >= "value"`,
// or `field > "value"`. Handles optional whitespace around the operator.
func parseClause(s string) filterClause {
	// Try >= first (before > to avoid partial match)
	if idx := strings.Index(s, ">="); idx > 0 {
		field := strings.TrimSpace(s[:idx])
		value := unquote(strings.TrimSpace(s[idx+2:]))
		return filterClause{field: field, op: opGe, value: value}
	}
	if idx := strings.Index(s, ">"); idx > 0 {
		field := strings.TrimSpace(s[:idx])
		value := unquote(strings.TrimSpace(s[idx+1:]))
		return filterClause{field: field, op: opGt, value: value}
	}
	if idx := strings.Index(s, "="); idx > 0 {
		field := strings.TrimSpace(s[:idx])
		value := unquote(strings.TrimSpace(s[idx+1:]))
		return filterClause{field: field, op: opEq, value: value}
	}
	// Fallback: treat entire string as an equality match on textPayload
	return filterClause{field: "textPayload", op: opEq, value: s}
}

// unquote removes surrounding double quotes from a string.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// resolveField extracts the value of a dot-notation field path from a LogEntry.
func resolveField(entry LogEntry, field string) (string, bool) {
	switch field {
	case "resource.type":
		if entry.Resource != nil {
			return entry.Resource.Type, true
		}
		return "", false
	case "logName":
		return entry.LogName, true
	case "severity":
		return entry.Severity, true
	case "textPayload":
		return entry.TextPayload, true
	case "timestamp":
		return entry.Timestamp, true
	default:
		// Handle resource.labels.X
		if strings.HasPrefix(field, "resource.labels.") {
			labelKey := field[len("resource.labels."):]
			if entry.Resource != nil && entry.Resource.Labels != nil {
				v, ok := entry.Resource.Labels[labelKey]
				return v, ok
			}
			return "", false
		}
		// Handle labels.X
		if strings.HasPrefix(field, "labels.") {
			labelKey := field[len("labels."):]
			if entry.Labels != nil {
				v, ok := entry.Labels[labelKey]
				return v, ok
			}
			return "", false
		}
		return "", false
	}
}

// parseTimestamp parses a timestamp string in RFC3339 or RFC3339Nano format.
func parseTimestamp(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
	}
	return t, err
}

// matchesFilter checks whether a LogEntry matches a structured filter string.
// Supports: field="value" AND field>"value" AND field>="value"
// with dot-notation paths (resource.type, resource.labels.X, timestamp, etc.)
func matchesFilter(entry LogEntry, filter string) bool {
	clauses := parseFilter(filter)
	if len(clauses) == 0 {
		return true // empty filter matches all
	}
	for _, c := range clauses {
		val, ok := resolveField(entry, c.field)
		if !ok {
			return false
		}
		switch c.op {
		case opEq:
			if val != c.value {
				return false
			}
		case opGt:
			if val <= c.value {
				return false
			}
		case opGe:
			if val < c.value {
				return false
			}
		}
	}
	return true
}
