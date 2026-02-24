package gitlabhub

import "testing"

func TestExprVariableComparison(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"CI_PIPELINE_SOURCE": "push",
		"ENV":                "production",
	}}
	if !EvalGitLabExpr(`$CI_PIPELINE_SOURCE == "push"`, ctx) {
		t.Error(`$CI_PIPELINE_SOURCE == "push" should be true`)
	}
	if EvalGitLabExpr(`$CI_PIPELINE_SOURCE == "merge_request_event"`, ctx) {
		t.Error(`$CI_PIPELINE_SOURCE == "merge_request_event" should be false`)
	}
	if !EvalGitLabExpr(`$ENV != "staging"`, ctx) {
		t.Error(`$ENV != "staging" should be true`)
	}
	if EvalGitLabExpr(`$ENV != "production"`, ctx) {
		t.Error(`$ENV != "production" should be false`)
	}
}

func TestExprRegex(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"CI_COMMIT_BRANCH": "release-1.0",
	}}
	if !EvalGitLabExpr(`$CI_COMMIT_BRANCH =~ /^release-/`, ctx) {
		t.Error(`=~ /^release-/ should match "release-1.0"`)
	}
	if EvalGitLabExpr(`$CI_COMMIT_BRANCH !~ /^release-/`, ctx) {
		t.Error(`!~ /^release-/ should be false for "release-1.0"`)
	}
	if EvalGitLabExpr(`$CI_COMMIT_BRANCH =~ /^main$/`, ctx) {
		t.Error(`=~ /^main$/ should not match "release-1.0"`)
	}
	if !EvalGitLabExpr(`$CI_COMMIT_BRANCH !~ /^main$/`, ctx) {
		t.Error(`!~ /^main$/ should be true for "release-1.0"`)
	}
}

func TestExprBooleanOps(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"A": "1",
		"B": "2",
	}}
	if !EvalGitLabExpr(`$A == "1" && $B == "2"`, ctx) {
		t.Error("AND of two trues should be true")
	}
	if EvalGitLabExpr(`$A == "1" && $B == "wrong"`, ctx) {
		t.Error("AND with one false should be false")
	}
	if !EvalGitLabExpr(`$A == "wrong" || $B == "2"`, ctx) {
		t.Error("OR with one true should be true")
	}
	if EvalGitLabExpr(`$A == "wrong" || $B == "wrong"`, ctx) {
		t.Error("OR of two falses should be false")
	}
}

func TestExprNullVars(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"DEFINED": "hello",
	}}
	if !EvalGitLabExpr(`$UNDEFINED == null`, ctx) {
		t.Error("$UNDEFINED == null should be true")
	}
	if EvalGitLabExpr(`$DEFINED == null`, ctx) {
		t.Error("$DEFINED == null should be false")
	}
	if !EvalGitLabExpr(`$DEFINED != null`, ctx) {
		t.Error("$DEFINED != null should be true")
	}
	if EvalGitLabExpr(`$UNDEFINED != null`, ctx) {
		t.Error("$UNDEFINED != null should be false")
	}
}

func TestExprParens(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"A": "1",
		"B": "2",
		"C": "3",
	}}
	// Without parens: $A == "1" || $B == "2" && $C == "wrong" → true (AND binds tighter)
	// With parens: ($A == "1" || $B == "2") && $C == "3" → true
	if !EvalGitLabExpr(`($A == "1" || $B == "wrong") && $C == "3"`, ctx) {
		t.Error("grouped OR with AND should be true")
	}
	if EvalGitLabExpr(`($A == "wrong" || $B == "wrong") && $C == "3"`, ctx) {
		t.Error("grouped OR (both false) with AND should be false")
	}
}

func TestExprNegation(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"A": "yes",
	}}
	if EvalGitLabExpr(`!($A == "yes")`, ctx) {
		t.Error(`!($A == "yes") should be false when A is "yes"`)
	}
	if !EvalGitLabExpr(`!($A == "no")`, ctx) {
		t.Error(`!($A == "no") should be true when A is "yes"`)
	}
}

func TestExprEmpty(t *testing.T) {
	ctx := &ExprContext{}
	if !EvalGitLabExpr("", ctx) {
		t.Error("empty expression should be true (no condition = match)")
	}
}

func TestExprBraceVar(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"MY_VAR": "hello",
	}}
	if !EvalGitLabExpr(`${MY_VAR} == "hello"`, ctx) {
		t.Error(`${MY_VAR} == "hello" should be true`)
	}
	if EvalGitLabExpr(`${MY_VAR} == "world"`, ctx) {
		t.Error(`${MY_VAR} == "world" should be false`)
	}
}

func TestExprTruthy(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"FULL":  "value",
		"EMPTY": "",
		"ZERO":  "0",
		"FALSE": "false",
	}}
	// Defined non-empty variable is truthy
	if !EvalGitLabExpr(`$FULL`, ctx) {
		t.Error("$FULL should be truthy")
	}
	// Empty string is falsy
	if EvalGitLabExpr(`$EMPTY`, ctx) {
		t.Error("$EMPTY should be falsy")
	}
	// "0" is falsy
	if EvalGitLabExpr(`$ZERO`, ctx) {
		t.Error("$ZERO should be falsy")
	}
	// "false" is falsy
	if EvalGitLabExpr(`$FALSE`, ctx) {
		t.Error(`$FALSE should be falsy`)
	}
	// Undefined variable is null → falsy
	if EvalGitLabExpr(`$NOPE`, ctx) {
		t.Error("$NOPE (undefined) should be falsy")
	}
}

func TestExprNullRegex(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{}}
	// Regex on undefined var: =~ returns false, !~ returns true
	if EvalGitLabExpr(`$UNDEF =~ /anything/`, ctx) {
		t.Error("=~ on null should be false")
	}
	if !EvalGitLabExpr(`$UNDEF !~ /anything/`, ctx) {
		t.Error("!~ on null should be true")
	}
}

func TestExprComplexPipeline(t *testing.T) {
	// Simulates a real-world GitLab CI rule
	ctx := &ExprContext{Variables: map[string]string{
		"CI_PIPELINE_SOURCE": "push",
		"CI_COMMIT_BRANCH":   "main",
		"CI_COMMIT_TAG":      "",
	}}
	// Common pattern: run on push to main or tags
	if !EvalGitLabExpr(`$CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH == "main"`, ctx) {
		t.Error("should match push to main")
	}
	// Tag is empty, so this should not match
	if EvalGitLabExpr(`$CI_COMMIT_TAG != null && $CI_COMMIT_TAG =~ /^v/`, ctx) {
		t.Error("empty tag should be falsy in && chain")
	}
}

func TestExprNilContext(t *testing.T) {
	// Nil context should not panic
	ctx := &ExprContext{}
	if EvalGitLabExpr(`$VAR == "test"`, ctx) {
		t.Error("nil variables should make $VAR null, not equal to test")
	}
	if !EvalGitLabExpr(`$VAR == null`, ctx) {
		t.Error("nil variables should make $VAR null")
	}
}

func TestExprBooleanLiterals(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"FLAG": "true",
	}}
	if !EvalGitLabExpr(`$FLAG == "true"`, ctx) {
		t.Error(`$FLAG == "true" should be true`)
	}
}

func TestExprNotEqual(t *testing.T) {
	ctx := &ExprContext{Variables: map[string]string{
		"CI_PIPELINE_SOURCE": "merge_request_event",
	}}
	if !EvalGitLabExpr(`$CI_PIPELINE_SOURCE != "push"`, ctx) {
		t.Error("!= should be true for non-matching values")
	}
	if EvalGitLabExpr(`$CI_PIPELINE_SOURCE != "merge_request_event"`, ctx) {
		t.Error("!= should be false for matching values")
	}
}
