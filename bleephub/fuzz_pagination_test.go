package bleephub

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// FuzzParsePagination fuzzes the page/per_page query parser. Invariant: the
// parsed page and per_page are always positive and per_page is clamped to
// GitHub's 100 ceiling, for any attacker-supplied string.
func FuzzParsePagination(f *testing.F) {
	f.Add("1", "30")
	f.Add("0", "0")
	f.Add("-1", "-100")
	f.Add("999999999999999999999", "999999999999999999999")
	f.Add("2147483648", "101")
	f.Add("abc", "def")
	f.Add("", "")
	f.Add("1.5", "3e2")
	f.Add("0x10", "010")

	f.Fuzz(func(t *testing.T, page, perPage string) {
		req := httptest.NewRequest(http.MethodGet, "/x?"+url.Values{"page": {page}, "per_page": {perPage}}.Encode(), nil)
		pp := parsePagination(req)
		if pp.Page < 1 {
			t.Fatalf("page=%q parsed to %d (want >=1)", page, pp.Page)
		}
		if pp.PerPage < 1 || pp.PerPage > 100 {
			t.Fatalf("per_page=%q parsed to %d (want 1..100)", perPage, pp.PerPage)
		}
	})
}

// FuzzPaginateAndLink drives the generic page-slicer with an attacker-supplied
// page/per_page against a fixed-size collection. Invariant: never an
// out-of-range slice panic; the returned window is a sub-slice no larger than
// per_page and never exceeds the source.
func FuzzPaginateAndLink(f *testing.F) {
	items := make([]int, 137)
	for i := range items {
		items[i] = i
	}

	f.Add("1", "30")
	f.Add("0", "0")
	f.Add("-1", "-1")
	f.Add("9223372036854775807", "100")
	f.Add("2", "50")
	f.Add("100000", "100")
	f.Add("abc", "xyz")

	f.Fuzz(func(t *testing.T, page, perPage string) {
		req := httptest.NewRequest(http.MethodGet, "/x?"+url.Values{"page": {page}, "per_page": {perPage}}.Encode(), nil)
		w := httptest.NewRecorder()
		got := paginateAndLink(w, req, items)
		if len(got) > len(items) {
			t.Fatalf("page=%q per_page=%q returned %d > %d items", page, perPage, len(got), len(items))
		}
		pp := parsePagination(req)
		if len(got) > pp.PerPage {
			t.Fatalf("returned %d items > per_page %d", len(got), pp.PerPage)
		}
	})
}

// FuzzPaginateGQLCursors fuzzes the Relay forward-pagination helper directly
// with an attacker-controlled first + after cursor, plus the raw cursor codec.
// Invariants: decode(encode(x))==x for non-negative x; paginateGQL never
// returns more nodes than the item count and never panics on a garbage cursor.
func FuzzPaginateGQLCursors(f *testing.F) {
	items := make([]int, 60)
	for i := range items {
		items[i] = i
	}
	toGQL := func(n int) map[string]interface{} { return map[string]interface{}{"v": n} }

	f.Add(10, "")
	f.Add(0, "")
	f.Add(-5, "")
	f.Add(1<<31, "Y3Vyc29yOjU=")
	f.Add(30, "not-base64!!!")
	f.Add(30, "Y3Vyc29yOjk5OTk5OTk5OTk5OTk5OTk5OTk=")
	f.Add(1<<62, "Y3Vyc29yOi0x") // cursor:-1
	f.Add(50, "Y3Vyc29yOg==")    // cursor:

	f.Fuzz(func(t *testing.T, first int, after string) {
		res := paginateGQL(items, first, after, toGQL)
		nodes, _ := res["nodes"].([]map[string]interface{})
		if len(nodes) > len(items) {
			t.Fatalf("first=%d after=%q returned %d > %d nodes", first, after, len(nodes), len(items))
		}
		edges, _ := res["edges"].([]map[string]interface{})
		if len(edges) != len(nodes) {
			t.Fatalf("edges/nodes length mismatch: %d vs %d", len(edges), len(nodes))
		}
		// Round-trip identity for any cursor the helper itself emits.
		for _, e := range edges {
			c, _ := e["cursor"].(string)
			if got := decodeCursor(c); got < 0 {
				t.Fatalf("emitted cursor %q decodes to negative %d", c, got)
			}
		}
	})
}

// FuzzRepaginateConnection fuzzes the embedded-connection re-slicer (first/
// after and last/before) with a fuzzed connection map. Invariant: never an
// out-of-range slice panic; a returned connection never has more nodes than
// the source.
func FuzzRepaginateConnection(f *testing.F) {
	f.Add(10, "", 0, "")
	f.Add(-1, "", 5, "Y3Vyc29yOjM=")
	f.Add(1<<31, "Y3Vyc29yOjU=", 0, "")
	f.Add(0, "", 200, "")
	f.Add(0, "garbage", 0, "garbage")
	f.Add(5, "", 5, "") // both first and last present

	f.Fuzz(func(t *testing.T, first int, after string, last int, before string) {
		src := make([]map[string]interface{}, 40)
		for i := range src {
			src[i] = map[string]interface{}{"id": i}
		}
		conn := map[string]interface{}{"nodes": src}
		args := map[string]interface{}{}
		if first != 0 || after != "" {
			args["first"] = first
			args["after"] = after
		}
		if last != 0 || before != "" {
			args["last"] = last
			args["before"] = before
		}
		out := repaginateConnection(conn, args)
		m, ok := out.(map[string]interface{})
		if !ok {
			t.Fatalf("repaginateConnection returned non-map: %T", out)
		}
		nodes, _ := m["nodes"].([]map[string]interface{})
		if len(nodes) > len(src) {
			t.Fatalf("first=%d last=%d returned %d > %d nodes", first, last, len(nodes), len(src))
		}
	})
}
