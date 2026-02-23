package bleephub

import "testing"

func TestExprSuccess(t *testing.T) {
	ctx := &ExprContext{
		DepResults: map[string]string{"build": "success"},
	}
	if !EvalExpr("success()", ctx) {
		t.Error("success() should be true when all deps succeed")
	}
}

func TestExprSuccessWithFailedDep(t *testing.T) {
	ctx := &ExprContext{
		DepResults: map[string]string{"build": "failure"},
	}
	if EvalExpr("success()", ctx) {
		t.Error("success() should be false when a dep failed")
	}
}

func TestExprFailure(t *testing.T) {
	ctx := &ExprContext{
		DepResults: map[string]string{"build": "failure"},
	}
	if !EvalExpr("failure()", ctx) {
		t.Error("failure() should be true when a dep failed")
	}
}

func TestExprFailureNone(t *testing.T) {
	ctx := &ExprContext{
		DepResults: map[string]string{"build": "success"},
	}
	if EvalExpr("failure()", ctx) {
		t.Error("failure() should be false when no dep failed")
	}
}

func TestExprAlways(t *testing.T) {
	ctx := &ExprContext{
		DepResults: map[string]string{"build": "failure"},
	}
	if !EvalExpr("always()", ctx) {
		t.Error("always() should always be true")
	}
}

func TestExprCancelled(t *testing.T) {
	ctx := &ExprContext{WorkflowCancelled: true}
	if !EvalExpr("cancelled()", ctx) {
		t.Error("cancelled() should be true when workflow is cancelled")
	}

	ctx2 := &ExprContext{WorkflowCancelled: false}
	if EvalExpr("cancelled()", ctx2) {
		t.Error("cancelled() should be false when workflow is not cancelled")
	}
}

func TestExprStringComparison(t *testing.T) {
	ctx := &ExprContext{
		Values: map[string]string{"github.event_name": "push"},
	}
	if !EvalExpr("github.event_name == 'push'", ctx) {
		t.Error("should match push")
	}
	if EvalExpr("github.event_name == 'pull_request'", ctx) {
		t.Error("should not match pull_request")
	}
	if !EvalExpr("github.event_name != 'pull_request'", ctx) {
		t.Error("!= should be true for non-matching")
	}
}

func TestExprBooleanOps(t *testing.T) {
	ctx := &ExprContext{
		DepResults: map[string]string{"build": "success"},
		Values:     map[string]string{"github.ref": "refs/heads/main"},
	}

	if !EvalExpr("success() && github.ref == 'refs/heads/main'", ctx) {
		t.Error("AND of two trues should be true")
	}
	if EvalExpr("failure() && github.ref == 'refs/heads/main'", ctx) {
		t.Error("AND with failure should be false")
	}
	if !EvalExpr("failure() || success()", ctx) {
		t.Error("OR with one true should be true")
	}
}

func TestExprNegation(t *testing.T) {
	ctx := &ExprContext{
		DepResults: map[string]string{"build": "success"},
	}
	if EvalExpr("!success()", ctx) {
		t.Error("!success() should be false when deps succeed")
	}
	if !EvalExpr("!failure()", ctx) {
		t.Error("!failure() should be true when no deps failed")
	}
}

func TestExprContextAccess(t *testing.T) {
	ctx := &ExprContext{
		Values: map[string]string{
			"github.event_name": "workflow_dispatch",
			"github.ref":        "refs/tags/v1.0",
		},
	}
	if !EvalExpr("github.event_name == 'workflow_dispatch'", ctx) {
		t.Error("should match workflow_dispatch")
	}
	if !EvalExpr("github.ref == 'refs/tags/v1.0'", ctx) {
		t.Error("should match ref")
	}
}

func TestExprParentheses(t *testing.T) {
	ctx := &ExprContext{
		DepResults: map[string]string{"build": "failure"},
		Values:     map[string]string{"github.ref": "refs/heads/main"},
	}
	// (failure() || success()) && github.ref == 'refs/heads/main'
	if !EvalExpr("(failure() || success()) && github.ref == 'refs/heads/main'", ctx) {
		t.Error("grouped OR with AND should be true")
	}
}

func TestExprContainsStatusFunction(t *testing.T) {
	hasAlways, hasFailure := ExprContainsStatusFunction("always()")
	if !hasAlways {
		t.Error("should detect always()")
	}
	if hasFailure {
		t.Error("should not detect failure() in always()")
	}

	hasAlways, hasFailure = ExprContainsStatusFunction("failure() || always()")
	if !hasAlways || !hasFailure {
		t.Error("should detect both")
	}

	hasAlways, hasFailure = ExprContainsStatusFunction("success()")
	if hasAlways || hasFailure {
		t.Error("should not detect either")
	}
}

func TestExprDollarBracketWrapper(t *testing.T) {
	ctx := &ExprContext{
		Values: map[string]string{"github.event_name": "push"},
	}
	if !EvalExpr("${{ github.event_name == 'push' }}", ctx) {
		t.Error("should strip ${{ }} wrapper")
	}
}

func TestExprEmpty(t *testing.T) {
	ctx := &ExprContext{}
	if !EvalExpr("", ctx) {
		t.Error("empty expression should be true (default)")
	}
}
