package main

import "testing"

// FuzzDDBEvalExpr fuzzes the DynamoDB condition/update expression parser+evaluator.
// A malformed or hostile expression must degrade to a non-match, never panic.
func FuzzDDBEvalExpr(f *testing.F) {
	seeds := []string{
		"",
		"a = :v",
		"attribute_exists(a)",
		"attribute_not_exists(#n)",
		"begins_with(a, :p)",
		"contains(a, :v) AND size(b) > :n",
		"a BETWEEN :lo AND :hi",
		"a IN (:x, :y, :z)",
		"NOT a = :v OR (b < :c AND c >= :d)",
		"size(",
		"begins_with(",
		"a BETWEEN",
		"a IN (",
		"(((((((((((",
		"a.b[0].c = :v",
		"a[",
		"a[999999999999999999999999] = :v",
		"\\",
		"é = :v",
		"\xff\xfe = :v",
		"a <",
		"<=",
		",",
		"NOT NOT NOT",
		"size(a) = size(b)",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	item := map[string]any{
		"a": map[string]any{"S": "hello"},
		"b": map[string]any{"L": []any{map[string]any{"N": "1"}}},
	}
	names := map[string]string{"#n": "a"}
	values := map[string]any{":v": map[string]any{"S": "hello"}, ":p": map[string]any{"S": "he"}, ":n": map[string]any{"N": "0"}}
	f.Fuzz(func(t *testing.T, expr string) {
		_ = ddbEvalExpr(item, true, expr, names, values)
		_ = ddbEvalExpr(item, false, expr, names, values)
	})
}
