package main

import "testing"

func TestCWLogPatternMatches(t *testing.T) {
	cases := []struct {
		msg, pat string
		want     bool
	}{
		// Unstructured: AND, exclude, optional(OR), quoted phrase.
		{"ERROR disk full", "ERROR", true},
		{"INFO ok", "ERROR", false},
		{"ERROR disk full", "ERROR full", true},     // all terms (AND)
		{"ERROR disk full", "ERROR missing", false}, // a required term absent
		{"ERROR disk full", "ERROR -full", false},   // exclusion
		{"ERROR disk ok", "ERROR -full", true},
		{"a WARN b", "?ERROR ?WARN", true}, // optional OR group
		{"a INFO b", "?ERROR ?WARN", false},
		{"got status 500 now", `"status 500"`, true}, // quoted phrase substring
		// Structured JSON.
		{`{"level":"ERROR","code":500}`, `{ $.level = "ERROR" }`, true},
		{`{"level":"INFO","code":200}`, `{ $.level = "ERROR" }`, false},
		{`{"code":500}`, `{ $.code >= 500 }`, true},
		{`{"code":499}`, `{ $.code >= 500 }`, false},
		{`{"level":"ERROR","code":500}`, `{ $.level = "ERROR" && $.code = 500 }`, true},
		{`{"level":"ERROR","code":499}`, `{ $.level = "ERROR" && $.code = 500 }`, false},
		{`{"level":"WARN"}`, `{ $.level = "ERROR" || $.level = "WARN" }`, true},
		{`{"svc":"api-gw"}`, `{ $.svc = "api-*" }`, true}, // wildcard
		{`{"a":{"b":7}}`, `{ $.a.b > 5 }`, true},          // nested
		{`{"xs":[1,2,3]}`, `{ $.xs[2] = 3 }`, true},       // array index
		{`not json`, `{ $.x = "y" }`, false},              // structured on non-JSON
	}
	for _, tc := range cases {
		if got := cwLogPatternMatches(tc.msg, tc.pat); got != tc.want {
			t.Errorf("cwLogPatternMatches(%q, %q) = %v, want %v", tc.msg, tc.pat, got, tc.want)
		}
	}
}
