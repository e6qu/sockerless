package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// kqlQuery represents a parsed KQL query with table, filters, and limit.
type kqlQuery struct {
	Table   string
	Filters []kqlFilter
	Limit   int
}

// kqlFilter represents a single where-clause filter.
type kqlFilter struct {
	Field    string
	Operator string // "==", ">", ">="
	Value    string
	IsTime   bool // true if value was wrapped in datetime()
}

// parseKQL parses a simplified KQL query into its components.
// Supports: Table | where Field == 'value' | where Field > datetime(ts) | take N
func parseKQL(query string) kqlQuery {
	q := kqlQuery{Limit: -1}
	parts := strings.Split(query, "|")

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if i == 0 {
			// First segment is the table name (may contain trailing spaces)
			q.Table = strings.TrimSpace(part)
			continue
		}

		if strings.HasPrefix(part, "where ") {
			clause := strings.TrimPrefix(part, "where ")
			f := parseKQLWhere(clause)
			if f.Field != "" {
				q.Filters = append(q.Filters, f)
			}
		} else if strings.HasPrefix(part, "take ") {
			fmt.Sscanf(strings.TrimPrefix(part, "take "), "%d", &q.Limit)
		} else if strings.HasPrefix(part, "limit ") {
			fmt.Sscanf(strings.TrimPrefix(part, "limit "), "%d", &q.Limit)
		}
		// Ignore order by, project, and other clauses — they don't affect filtering
	}

	return q
}

// parseKQLWhere parses a single where clause.
func parseKQLWhere(clause string) kqlFilter {
	// Try operators in order of length (>= before >)
	for _, op := range []string{">=", ">", "=="} {
		idx := strings.Index(clause, op)
		if idx < 0 {
			continue
		}
		field := strings.TrimSpace(clause[:idx])
		rawVal := strings.TrimSpace(clause[idx+len(op):])

		f := kqlFilter{
			Field:    field,
			Operator: op,
		}

		// Check for datetime() wrapper
		if strings.HasPrefix(rawVal, "datetime(") && strings.HasSuffix(rawVal, ")") {
			inner := rawVal[len("datetime(") : len(rawVal)-1]
			f.Value = strings.Trim(inner, "'\"")
			f.IsTime = true
		} else {
			f.Value = strings.Trim(rawVal, "'\"")
		}

		return f
	}
	return kqlFilter{}
}

// Table schemas — maps table name to column definitions.
var kqlTableSchemas = map[string][]Column{
	"ContainerAppConsoleLogs_CL": {
		{Name: "TimeGenerated", Type: "datetime"},
		{Name: "ContainerGroupName_s", Type: "string"},
		{Name: "Log_s", Type: "string"},
		{Name: "Stream_s", Type: "string"},
	},
	"AppTraces": {
		{Name: "TimeGenerated", Type: "datetime"},
		{Name: "Message", Type: "string"},
		{Name: "AppRoleName", Type: "string"},
	},
}

// monitorLogRow is a generic log row stored as field→value pairs.
type monitorLogRow map[string]string

// matchesFilters returns true if the row matches all the given KQL filters.
func (row monitorLogRow) matchesFilters(filters []kqlFilter) bool {
	for _, f := range filters {
		val, exists := row[f.Field]
		if !exists {
			return false
		}

		switch f.Operator {
		case "==":
			if val != f.Value {
				return false
			}
		case ">", ">=":
			if f.IsTime {
				rowTime, err1 := parseTimeFlexible(val)
				filterTime, err2 := parseTimeFlexible(f.Value)
				if err1 != nil || err2 != nil {
					return false
				}
				if f.Operator == ">" && !rowTime.After(filterTime) {
					return false
				}
				if f.Operator == ">=" && rowTime.Before(filterTime) {
					return false
				}
			} else {
				// Numeric comparison fallback
				rv, err1 := strconv.ParseFloat(val, 64)
				fv, err2 := strconv.ParseFloat(f.Value, 64)
				if err1 != nil || err2 != nil {
					return false
				}
				if f.Operator == ">" && rv <= fv {
					return false
				}
				if f.Operator == ">=" && rv < fv {
					return false
				}
			}
		}
	}
	return true
}

// toRow converts a monitorLogRow to an ordered slice matching the given columns.
func (row monitorLogRow) toRow(columns []Column) []any {
	result := make([]any, len(columns))
	for i, col := range columns {
		result[i] = row[col.Name]
	}
	return result
}

func parseTimeFlexible(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
	}
	return t, err
}
