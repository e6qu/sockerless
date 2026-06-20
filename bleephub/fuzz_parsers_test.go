package bleephub

import (
	"testing"

	"github.com/graphql-go/graphql"
)

// FuzzGraphQLQuery drives arbitrary query strings through the real schema's
// executor. A malformed or pathological query must surface as a GraphQL error,
// never a panic.
func FuzzGraphQLQuery(f *testing.F) {
	s := newTestServer()
	s.initGraphQLSchema()
	f.Add("{viewer{login}}")
	f.Add("")
	f.Add("{")
	f.Add("query{repository(owner:\"a\",name:\"b\"){name}}")
	f.Add("{__schema{types{name}}}")
	f.Add("mutation{")
	f.Fuzz(func(t *testing.T, q string) {
		_ = graphql.Do(graphql.Params{Schema: s.graphqlSchema, RequestString: q})
	})
}

// FuzzPaginateGQL exercises the shared Relay cursor-pagination chokepoint with
// attacker-controlled `first` and `after` values. A malformed cursor or an
// out-of-range `first` must never panic on the internal slice expressions.
func FuzzPaginateGQL(f *testing.F) {
	f.Add(10, "")
	f.Add(0, "")
	f.Add(-1, "")
	f.Add(1<<31-1, "")
	f.Add(5, "Y3Vyc29yOjA=") // cursor:0
	f.Add(5, "not-base64!!")
	f.Add(5, "Y3Vyc29yOjk5OTk5OTk5OTk5OQ==") // cursor:999999999999
	f.Add(1<<31-1, "Y3Vyc29yOjU=")           // first=MaxInt32, cursor:5

	items := make([]int, 50)
	for i := range items {
		items[i] = i
	}
	toGQL := func(n int) map[string]interface{} {
		return map[string]interface{}{"v": n}
	}

	f.Fuzz(func(t *testing.T, first int, after string) {
		// Must not panic regardless of input.
		res := paginateGQL(items, first, after, toGQL)
		if res == nil {
			t.Fatal("nil result")
		}
		nodes, _ := res["nodes"].([]map[string]interface{})
		if len(nodes) > len(items) {
			t.Fatalf("returned more nodes (%d) than items (%d)", len(nodes), len(items))
		}
	})
}

// FuzzDecodeCursor checks the base64 cursor decoder never panics.
func FuzzDecodeCursor(f *testing.F) {
	f.Add("Y3Vyc29yOjA=")
	f.Add("")
	f.Add("!!!!")
	f.Add("Y3Vyc29yOg==")
	f.Fuzz(func(t *testing.T, s string) {
		_ = decodeCursor(s)
	})
}

// FuzzParseContentRange checks the cache chunk Content-Range parser.
func FuzzParseContentRange(f *testing.F) {
	f.Add("bytes 0-1023/*")
	f.Add("bytes -")
	f.Add("bytes 9999999999999999999999-0/*")
	f.Add("")
	f.Add("bytes 0-/*")
	f.Fuzz(func(t *testing.T, h string) {
		_, _, _ = parseContentRange(h)
	})
}

// FuzzParseAndVerifyAppJWT feeds arbitrary token strings to the app-JWT parser.
func FuzzParseAndVerifyAppJWT(f *testing.F) {
	st := NewStore()
	f.Add("a.b.c")
	f.Add("")
	f.Add("....")
	f.Add("eyJ.eyJ.")
	f.Fuzz(func(t *testing.T, tok string) {
		_, _ = st.parseAndVerifyAppJWT(tok)
	})
}

// FuzzAgentRSAPublicKey feeds arbitrary modulus/exponent strings.
func FuzzAgentRSAPublicKey(f *testing.F) {
	f.Add("AQAB", "AQAB")
	f.Add("", "")
	f.Add("////", "!!!!")
	f.Fuzz(func(t *testing.T, mod, exp string) {
		_, _ = agentRSAPublicKey(&AgentPublicKey{Modulus: mod, Exponent: exp})
	})
}
