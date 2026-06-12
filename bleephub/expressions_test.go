package bleephub

import (
	"testing"
)

// evalBool is the test-side convenience: expression errors count as false.
func evalBool(expr string, ctx *ExprContext) bool {
	ok, err := EvalExprErr(expr, ctx)
	return err == nil && ok
}

func exprTestCtx() *ExprContext {
	return &ExprContext{
		DepResults: map[string]string{"build": "success"},
		Contexts: map[string]interface{}{
			"github": map[string]interface{}{
				"event_name": "push",
				"ref":        "refs/heads/main",
				"ref_name":   "main",
				"repository": "octo/hello",
				"event": map[string]interface{}{
					"action": "opened",
					"pull_request": map[string]interface{}{
						"number": float64(7),
						"draft":  false,
						"base":   map[string]interface{}{"ref": "main"},
						"labels": []interface{}{
							map[string]interface{}{"name": "bug"},
							map[string]interface{}{"name": "urgent"},
						},
					},
					"commits": []interface{}{
						map[string]interface{}{"message": "fix: thing"},
					},
				},
			},
			"needs": map[string]interface{}{
				"build": map[string]interface{}{
					"result":  "success",
					"outputs": map[string]interface{}{"version": "1.2.3"},
				},
			},
			"inputs": map[string]interface{}{"deploy-env": "staging", "dry_run": "true"},
			"vars":   map[string]interface{}{"REGION": "eu-west-1"},
		},
	}
}

func TestExprTruthTable(t *testing.T) {
	ctx := exprTestCtx()
	cases := []struct {
		expr string
		want bool
	}{
		// Status functions
		{"success()", true},
		{"failure()", false},
		{"always()", true},
		{"cancelled()", false},
		// Literals + truthiness
		{"true", true},
		{"false", false},
		{"null", false},
		{"0", false},
		{"1", true},
		{"''", false},
		{"'x'", true},
		// Boolean operators
		{"true && false", false},
		{"true || false", true},
		{"!true", false},
		{"!(false || false)", true},
		// Equality: case-insensitive strings, numeric coercion
		{"'Main' == 'main'", true},
		{"'main' != 'dev'", true},
		{"1 == '1'", true},
		{"true == 1", true},
		{"null == 0", true},
		{"null == ''", true},
		{"'abc' == 0", false}, // NaN never equals
		// Relational (numeric coercion)
		{"2 < 10", true},
		{"'2' < '10'", true},
		{"5 >= 5", true},
		{"'abc' < 1", false}, // NaN comparison is false
		// Context access (dot, case-insensitive, bracket)
		{"github.event_name == 'push'", true},
		{"GitHub.Event_Name == 'PUSH'", true},
		{"github.ref == 'refs/heads/main'", true},
		{"github['ref_name'] == 'main'", true},
		{"github.event.action == 'opened'", true},
		{"github.event.pull_request.number == 7", true},
		{"github.event.pull_request.draft", false},
		{"github.event.pull_request.base.ref == 'main'", true},
		{"github.event.pull_request.labels[1].name == 'urgent'", true},
		{"github.event.commits[0].message == 'fix: thing'", true},
		// Missing properties are null, not errors
		{"github.event.no_such_thing == null", true},
		{"github.event.no.such.thing == null", true},
		// needs / inputs / vars
		{"needs.build.result == 'success'", true},
		{"needs.build.outputs.version == '1.2.3'", true},
		{"inputs.deploy-env == 'staging'", true},
		{"inputs.dry_run == 'true'", true},
		{"vars.REGION == 'eu-west-1'", true},
		// Functions
		{"contains('Hello World', 'world')", true},
		{"contains('Hello', 'xyz')", false},
		{"contains(fromJSON('[\"a\",\"b\"]'), 'b')", true},
		{"contains(fromJSON('[1,2,3]'), 2)", true},
		{"startsWith('refs/heads/main', 'refs/heads/')", true},
		{"startsWith('REFS/heads/x', 'refs/')", true},
		{"endsWith('file.yml', '.YML')", true},
		{"format('{0}-{1}', 'a', 'b') == 'a-b'", true},
		{"format('{{literal}}') == '{literal}'", true},
		{"join(fromJSON('[\"x\",\"y\"]'), '+') == 'x+y'", true},
		{"join(fromJSON('[\"x\",\"y\"]')) == 'x,y'", true},
		{"fromJSON('{\"a\":{\"b\":2}}').a.b == 2", true},
		{"fromJSON('true')", true},
		{"toJSON(github.event.action) == '\"opened\"'", true},
		// Value-preserving && / ||
		{"github.event_name == 'push' && github.ref_name || 'fallback'", true},
		{"false || ''", false},
		// ${{ }} wrapper accepted
		{"${{ github.event_name == 'push' }}", true},
		// Whole-expression with mixed precedence
		{"github.event_name == 'pull_request' || github.event_name == 'push' && contains(github.ref, 'main')", true},
		{"!startsWith(github.ref, 'refs/tags/') && success()", true},
	}
	for _, tc := range cases {
		got, err := EvalExprErr(tc.expr, ctx)
		if err != nil {
			t.Errorf("EvalExprErr(%q) error: %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("EvalExprErr(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestExprErrors(t *testing.T) {
	ctx := exprTestCtx()
	for _, expr := range []string{
		"github.event_name =",     // single =
		"'unterminated",           // bad string
		"no_such_context.x",       // unknown root
		"contains('a')",           // arity
		"format('{9}', 'x')",      // bad placeholder
		"fromJSON('not json')",    // parse failure
		"hashFiles('**/*.lock')",  // runner-only function
		"github.event_name == ()", // empty parens
		"foo bar",                 // trailing garbage
	} {
		if _, err := EvalExprErr(expr, ctx); err == nil {
			t.Errorf("EvalExprErr(%q) expected error, got none", expr)
		}
	}
}

func TestExprStatusFunctions(t *testing.T) {
	failedDep := &ExprContext{DepResults: map[string]string{"build": "failure"}}
	if evalBool("success()", failedDep) {
		t.Error("success() should be false when a dep failed")
	}
	if !evalBool("failure()", failedDep) {
		t.Error("failure() should be true when a dep failed")
	}
	skippedDep := &ExprContext{DepResults: map[string]string{"build": "skipped"}}
	if !evalBool("success()", skippedDep) {
		t.Error("success() should treat skipped deps as non-failures")
	}
	cancelled := &ExprContext{WorkflowCancelled: true}
	if !evalBool("cancelled()", cancelled) {
		t.Error("cancelled() should be true when the workflow is cancelled")
	}
	if evalBool("success()", cancelled) {
		t.Error("success() should be false when the workflow is cancelled")
	}
	if !evalBool("always()", cancelled) {
		t.Error("always() must be true even when cancelled")
	}
}

func TestExprEmptyIsTrue(t *testing.T) {
	if !evalBool("", nil) {
		t.Error("empty if: must evaluate true")
	}
}

func TestEvalTemplate(t *testing.T) {
	ctx := exprTestCtx()
	got, err := EvalTemplate("ci-${{ github.ref_name }}-${{ inputs.deploy-env }}", ctx)
	if err != nil {
		t.Fatalf("EvalTemplate error: %v", err)
	}
	if got != "ci-main-staging" {
		t.Errorf("EvalTemplate = %q, want %q", got, "ci-main-staging")
	}

	// No templates → unchanged
	got, err = EvalTemplate("plain string", ctx)
	if err != nil || got != "plain string" {
		t.Errorf("EvalTemplate(plain) = %q, %v", got, err)
	}

	// Numbers render in shortest form, null renders empty
	got, err = EvalTemplate("n=${{ github.event.pull_request.number }} x=${{ github.event.missing }}", ctx)
	if err != nil || got != "n=7 x=" {
		t.Errorf("EvalTemplate = %q, %v", got, err)
	}

	// Unterminated template is an error
	if _, err := EvalTemplate("${{ github.ref", ctx); err == nil {
		t.Error("unterminated template should error")
	}

	// Expression errors propagate
	if _, err := EvalTemplate("${{ nope.x }}", ctx); err == nil {
		t.Error("unknown context in template should error")
	}
}

func TestExprContainsStatusFunction(t *testing.T) {
	hasAlways, hasFailure := ExprContainsStatusFunction("always() && failure()")
	if !hasAlways || !hasFailure {
		t.Error("expected both status functions detected")
	}
	hasAlways, hasFailure = ExprContainsStatusFunction("success()")
	if hasAlways || hasFailure {
		t.Error("expected neither status function detected")
	}
}
