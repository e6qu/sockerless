package bleephub

import "testing"

func TestResolveJobOutputsBasic(t *testing.T) {
	outputVars := map[string]string{
		"build_step.version": "1.0.0",
	}
	declared := map[string]string{
		"version": "${{ steps.build_step.outputs.version }}",
	}
	result := resolveJobOutputs(outputVars, declared)
	if result["version"] != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", result["version"])
	}
}

func TestResolveJobOutputsMultiple(t *testing.T) {
	outputVars := map[string]string{
		"build.version": "2.0.0",
		"build.sha":     "abc123",
	}
	declared := map[string]string{
		"version": "${{ steps.build.outputs.version }}",
		"sha":     "${{ steps.build.outputs.sha }}",
	}
	result := resolveJobOutputs(outputVars, declared)
	if result["version"] != "2.0.0" {
		t.Errorf("version = %q, want 2.0.0", result["version"])
	}
	if result["sha"] != "abc123" {
		t.Errorf("sha = %q, want abc123", result["sha"])
	}
}

func TestResolveJobOutputsMissingStep(t *testing.T) {
	outputVars := map[string]string{
		"other.value": "x",
	}
	declared := map[string]string{
		"result": "${{ steps.missing.outputs.value }}",
	}
	result := resolveJobOutputs(outputVars, declared)
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
}

func TestResolveJobOutputsNoOutputsDefined(t *testing.T) {
	outputVars := map[string]string{
		"step.out": "val",
	}
	result := resolveJobOutputs(outputVars, nil)
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
}

func TestParseStepsExpression(t *testing.T) {
	tests := []struct {
		expr       string
		wantStep   string
		wantOutput string
	}{
		{"${{ steps.build.outputs.version }}", "build", "version"},
		{"${{steps.s1.outputs.out}}", "s1", "out"},
		{"  ${{  steps.x.outputs.y  }}  ", "x", "y"},
		{"not an expression", "", ""},
		{"${{ github.ref }}", "", ""},
	}
	for _, tt := range tests {
		step, out := parseStepsExpression(tt.expr)
		if step != tt.wantStep || out != tt.wantOutput {
			t.Errorf("parseStepsExpression(%q) = (%q, %q), want (%q, %q)",
				tt.expr, step, out, tt.wantStep, tt.wantOutput)
		}
	}
}

func TestExtractOutputVariables(t *testing.T) {
	// Flat string values
	body := map[string]interface{}{
		"outputVariables": map[string]interface{}{
			"step1.out1": "val1",
			"step1.out2": "val2",
		},
	}
	result := extractOutputVariables(body)
	if result["step1.out1"] != "val1" || result["step1.out2"] != "val2" {
		t.Errorf("flat: got %v", result)
	}

	// Nested {"value": "..."} format
	body2 := map[string]interface{}{
		"outputVariables": map[string]interface{}{
			"build.ver": map[string]interface{}{"value": "3.0"},
		},
	}
	result2 := extractOutputVariables(body2)
	if result2["build.ver"] != "3.0" {
		t.Errorf("nested: got %v", result2)
	}

	// No outputVariables
	result3 := extractOutputVariables(map[string]interface{}{})
	if result3 != nil {
		t.Errorf("empty: got %v", result3)
	}
}
