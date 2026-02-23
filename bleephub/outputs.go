package bleephub

import (
	"regexp"
	"strings"
)

// resolveJobOutputs extracts output variables from the runner's FinishJob body
// and resolves them against the job's declared outputs.
//
// The runner sends outputVariables as a flat map with keys like "stepId.outputName" → "value".
// JobDef.Outputs maps declared output names to expressions like
// "${{ steps.<id>.outputs.<name> }}".
//
// Returns the resolved output map (outputName → value).
func resolveJobOutputs(outputVars map[string]string, declaredOutputs map[string]string) map[string]string {
	if len(declaredOutputs) == 0 || len(outputVars) == 0 {
		return nil
	}

	result := make(map[string]string)
	for name, expr := range declaredOutputs {
		stepID, outputName := parseStepsExpression(expr)
		if stepID == "" {
			continue
		}
		// Look up "stepId.outputName" in the runner's output variables
		key := stepID + "." + outputName
		if val, ok := outputVars[key]; ok {
			result[name] = val
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// stepsExprRegexp matches "${{ steps.<id>.outputs.<name> }}"
var stepsExprRegexp = regexp.MustCompile(`\$\{\{\s*steps\.([^.]+)\.outputs\.([^}\s]+)\s*\}\}`)

// parseStepsExpression extracts stepID and outputName from an expression like
// "${{ steps.build.outputs.version }}".
func parseStepsExpression(expr string) (stepID, outputName string) {
	expr = strings.TrimSpace(expr)
	matches := stepsExprRegexp.FindStringSubmatch(expr)
	if len(matches) != 3 {
		return "", ""
	}
	return matches[1], matches[2]
}

// extractOutputVariables extracts the outputVariables map from a FinishJob body.
// The runner sends output variables in the body as:
//
//	{"outputVariables": {"stepId.outputName": {"value": "val"}, ...}}
//
// or as a flat map: {"outputVariables": {"stepId.outputName": "val"}}
func extractOutputVariables(body map[string]interface{}) map[string]string {
	raw, ok := body["outputVariables"]
	if !ok || raw == nil {
		return nil
	}

	rawMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}

	result := make(map[string]string, len(rawMap))
	for k, v := range rawMap {
		switch val := v.(type) {
		case string:
			result[k] = val
		case map[string]interface{}:
			// Runner may send {"value": "..."} objects
			if s, ok := val["value"].(string); ok {
				result[k] = s
			}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}
