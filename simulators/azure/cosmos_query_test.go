package main

import "testing"

// TestCosmosParseEqualityQuery_UnicodeWhere guards against the slice-bounds
// panic that arises when "where" is located in a strings.ToLower copy and the
// resulting byte offset is applied to the original query. Unicode case-folding
// can change byte length (e.g. the Kelvin sign 'K' U+212A lowercases to ASCII
// 'k', shrinking the string by 2 bytes), so the lowercased offset can point
// past the end of the original. The parser must locate "where" against the
// original bytes and never panic.
func TestCosmosParseEqualityQuery_UnicodeWhere(t *testing.T) {
	params := []map[string]any{{"name": "@id", "value": "x"}}
	cases := []string{
		"00©0000000000000WHERE", // original fuzz crash seed
		"K WHERE c.id = 'v'",    // Kelvin sign before WHERE
		"İWHERE c.id = 'v'",     // dotted capital I (expands on fold)
		"WHERE",
		"where c.id =",
	}
	for _, q := range cases {
		// Must not panic; result correctness is asserted separately below.
		_, _, _ = cosmosParseEqualityQuery(q, params)
	}
}

// TestCosmosParseEqualityQuery_Extract asserts the parser still extracts the
// field/value correctly for well-formed queries after the case-insensitive
// rewrite — case folding of the WHERE keyword must not break extraction.
func TestCosmosParseEqualityQuery_Extract(t *testing.T) {
	params := []map[string]any{{"name": "@id", "value": "abc"}}
	cases := []struct {
		query, wantField, wantValue string
		wantOK                      bool
	}{
		{"SELECT * FROM c WHERE c.id = 'val'", "id", "val", true},
		{"SELECT * FROM c where c.name = 'bob'", "name", "bob", true},
		{"SELECT * FROM c WhErE c.id = @id", "id", "abc", true},
		{"SELECT * FROM c", "", "", false},
	}
	for _, c := range cases {
		field, value, ok := cosmosParseEqualityQuery(c.query, params)
		if ok != c.wantOK || field != c.wantField || value != c.wantValue {
			t.Errorf("cosmosParseEqualityQuery(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.query, field, value, ok, c.wantField, c.wantValue, c.wantOK)
		}
	}
}
