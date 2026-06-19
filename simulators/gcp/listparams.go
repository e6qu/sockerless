package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// GCP list `filter` / `orderBy` query-parameter support.
//
// Real GCP list APIs accept a `filter` expression and an `orderBy` clause; the
// sim previously ignored both, returning the full name-sorted set. gcpApplyListParams
// evaluates them against each resource's JSON representation, so it works for any
// resource type. It supports the documented common forms — conjunctive
// `field (= | != | : | > | < | >= | <=) value` clauses (quoted or bare values,
// joined by AND) and `orderBy` of `field [desc]`. Richer expressions (OR, NOT,
// nested parens) fall through as "match everything", which is safe for the sim's
// small lists.

type gcpFilterClause struct {
	field string
	op    string
	value string
}

func gcpApplyListParams[T any](items []T, r *http.Request) []T {
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	orderBy := strings.TrimSpace(r.URL.Query().Get("orderBy"))
	if filter == "" && orderBy == "" {
		return items
	}
	type pair struct {
		v T
		m map[string]any
	}
	pairs := make([]pair, 0, len(items))
	for _, it := range items {
		var m map[string]any
		if b, err := json.Marshal(it); err == nil {
			_ = json.Unmarshal(b, &m)
		}
		pairs = append(pairs, pair{it, m})
	}

	if clauses := gcpParseFilter(filter); len(clauses) > 0 {
		kept := pairs[:0]
		for _, p := range pairs {
			if gcpFilterMatches(p.m, clauses) {
				kept = append(kept, p)
			}
		}
		pairs = kept
	}
	if orderBy != "" {
		field, desc := gcpParseOrderBy(orderBy)
		sort.SliceStable(pairs, func(i, j int) bool {
			a, b := gcpFieldString(pairs[i].m, field), gcpFieldString(pairs[j].m, field)
			if desc {
				return a > b
			}
			return a < b
		})
	}
	out := make([]T, len(pairs))
	for i, p := range pairs {
		out[i] = p.v
	}
	return out
}

// gcpApplyOrderBy applies only the `orderBy` query param (for handlers that
// already implement their own `filter`).
func gcpApplyOrderBy[T any](items []T, r *http.Request) []T {
	orderBy := strings.TrimSpace(r.URL.Query().Get("orderBy"))
	if orderBy == "" {
		return items
	}
	field, desc := gcpParseOrderBy(orderBy)
	maps := make([]map[string]any, len(items))
	for i, it := range items {
		if b, err := json.Marshal(it); err == nil {
			_ = json.Unmarshal(b, &maps[i])
		}
	}
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := gcpFieldString(maps[idx[a]], field), gcpFieldString(maps[idx[b]], field)
		if desc {
			return x > y
		}
		return x < y
	})
	out := make([]T, len(items))
	for i, j := range idx {
		out[i] = items[j]
	}
	return out
}

func gcpParseFilter(s string) []gcpFilterClause {
	if s == "" {
		return nil
	}
	// Split on case-insensitive " AND ".
	var parts []string
	rest := s
	for {
		lower := strings.ToLower(rest)
		idx := strings.Index(lower, " and ")
		if idx < 0 {
			parts = append(parts, rest)
			break
		}
		parts = append(parts, rest[:idx])
		rest = rest[idx+5:]
	}
	var clauses []gcpFilterClause
	for _, p := range parts {
		p = strings.TrimSpace(p)
		for _, op := range []string{"!=", ">=", "<=", "=", ">", "<", ":"} {
			if idx := strings.Index(p, op); idx > 0 {
				field := strings.TrimSpace(p[:idx])
				value := strings.TrimSpace(p[idx+len(op):])
				value = strings.Trim(value, `"'`)
				clauses = append(clauses, gcpFilterClause{field, op, value})
				break
			}
		}
	}
	return clauses
}

func gcpFilterMatches(m map[string]any, clauses []gcpFilterClause) bool {
	for _, c := range clauses {
		actual := gcpFieldString(m, c.field)
		switch c.op {
		case "=":
			if actual != c.value {
				return false
			}
		case "!=":
			if actual == c.value {
				return false
			}
		case ":":
			if !strings.Contains(actual, c.value) {
				return false
			}
		case ">", "<", ">=", "<=":
			if !gcpNumCompare(actual, c.op, c.value) {
				return false
			}
		}
	}
	return true
}

func gcpNumCompare(a, op, b string) bool {
	af, aerr := strconv.ParseFloat(a, 64)
	bf, berr := strconv.ParseFloat(b, 64)
	if aerr != nil || berr != nil {
		switch op { // lexicographic fallback
		case ">":
			return a > b
		case "<":
			return a < b
		case ">=":
			return a >= b
		case "<=":
			return a <= b
		}
		return false
	}
	switch op {
	case ">":
		return af > bf
	case "<":
		return af < bf
	case ">=":
		return af >= bf
	case "<=":
		return af <= bf
	}
	return false
}

func gcpParseOrderBy(s string) (field string, desc bool) {
	// "field desc" / "field asc" / "field"
	s = strings.TrimSpace(strings.Split(s, ",")[0])
	if strings.HasSuffix(strings.ToLower(s), " desc") {
		return strings.TrimSpace(s[:len(s)-5]), true
	}
	if strings.HasSuffix(strings.ToLower(s), " asc") {
		return strings.TrimSpace(s[:len(s)-4]), false
	}
	return s, false
}

func gcpFieldString(m map[string]any, path string) string {
	var cur any = m
	for _, seg := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[seg]
	}
	switch v := cur.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
