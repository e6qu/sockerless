package main

import (
	"sort"
	"strconv"
)

// awsPage applies numeric-offset pagination to a sorted slice of any type.
// token is the incoming page token (empty = first page).
// maxResults is the caller-requested page size (0 = use defaultMax).
// Returns the page slice and the next token (empty = last page).
func awsPage[T any](all []T, token string, maxResults, defaultMax int) ([]T, string) {
	start := 0
	if token != "" {
		offset, err := strconv.Atoi(token)
		if err != nil || offset < 0 {
			offset = 0
		}
		start = offset
	}
	if start >= len(all) {
		return []T{}, ""
	}
	page := defaultMax
	if maxResults > 0 && maxResults < page {
		page = maxResults
	}
	end := start + page
	if end >= len(all) {
		return all[start:], ""
	}
	return all[start:end], strconv.Itoa(end)
}

// sortBy is a convenience wrapper that sorts a slice in-place by a string key
// and returns it (for chaining).
func sortBy[T any](s []T, key func(T) string) []T {
	sort.Slice(s, func(i, j int) bool { return key(s[i]) < key(s[j]) })
	return s
}
