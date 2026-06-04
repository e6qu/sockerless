package bleephub

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// PaginationParams holds parsed pagination query parameters.
type PaginationParams struct {
	Page    int
	PerPage int
}

// parsePagination extracts page/per_page from query string with GitHub defaults.
func parsePagination(r *http.Request) PaginationParams {
	p := PaginationParams{Page: 1, PerPage: 30}
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.PerPage = n
			if p.PerPage > 100 {
				p.PerPage = 100
			}
		}
	}
	return p
}

// paginateAndLink slices items to the current page and sets the Link header.
func paginateAndLink[T any](w http.ResponseWriter, r *http.Request, items []T) []T {
	pp := parsePagination(r)
	total := len(items)

	lastPage := 1
	if total > 0 {
		lastPage = (total + pp.PerPage - 1) / pp.PerPage
	}

	start := (pp.Page - 1) * pp.PerPage
	if start > total {
		start = total
	}
	end := start + pp.PerPage
	if end > total {
		end = total
	}
	page := items[start:end]

	if link := buildLinkHeader(r, pp.Page, pp.PerPage, lastPage); link != "" {
		w.Header().Set("Link", link)
	}

	return page
}

// paginateGQL implements Relay-style cursor pagination for GraphQL connections.
// toGQL converts each item into a map[string]interface{} that becomes the
// GraphQL node. Use a closure to thread extra state (e.g. *Store) into toGQL.
func paginateGQL[T any](items []T, first int, after string, toGQL func(T) map[string]interface{}) map[string]interface{} {
	total := len(items)

	startIdx := 0
	if after != "" {
		startIdx = decodeCursor(after) + 1
	}
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + first
	if endIdx > total {
		endIdx = total
	}

	page := items[startIdx:endIdx]
	nodes := make([]map[string]interface{}, 0, len(page))
	edges := make([]map[string]interface{}, 0, len(page))
	for i, item := range page {
		gql := toGQL(item)
		cursor := encodeCursor(startIdx + i)
		nodes = append(nodes, gql)
		edges = append(edges, map[string]interface{}{
			"node":   gql,
			"cursor": cursor,
		})
	}

	var startCursor, endCursor interface{}
	if len(edges) > 0 {
		startCursor = edges[0]["cursor"]
		endCursor = edges[len(edges)-1]["cursor"]
	}
	return map[string]interface{}{
		"nodes":      nodes,
		"edges":      edges,
		"totalCount": total,
		"pageInfo": map[string]interface{}{
			"hasNextPage":     endIdx < total,
			"hasPreviousPage": startIdx > 0,
			"startCursor":     startCursor,
			"endCursor":       endCursor,
		},
	}
}

// buildLinkHeader constructs an RFC 5988 Link header.
func buildLinkHeader(r *http.Request, page, perPage, lastPage int) string {
	if lastPage <= 1 {
		return ""
	}

	// Build base URL preserving existing query params except page
	base := r.URL.Path
	q := r.URL.Query()
	q.Del("page")

	linkURL := func(p int) string {
		qc := make(url.Values)
		for k, v := range q {
			qc[k] = v
		}
		qc.Set("page", strconv.Itoa(p))
		qc.Set("per_page", strconv.Itoa(perPage))
		return fmt.Sprintf("<%s?%s>", base, qc.Encode())
	}

	var parts []string
	if page < lastPage {
		parts = append(parts, linkURL(page+1)+`; rel="next"`)
		parts = append(parts, linkURL(lastPage)+`; rel="last"`)
	}
	if page > 1 {
		parts = append(parts, linkURL(1)+`; rel="first"`)
		parts = append(parts, linkURL(page-1)+`; rel="prev"`)
	}
	return strings.Join(parts, ", ")
}
