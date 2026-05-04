package gcf

import (
	"github.com/sockerless/api"
)

// isRunnerPatternGCF mirrors cloudrun.isRunnerPattern. REAL signals
// only (no hardcoded image lists, no cmd-pattern heuristics):
//   - explicit label `sockerless.runner-pattern=true`, OR
//   - image declares ExposedPorts (binds a service port).
//
// Used by gcf to set MinInstanceCount=1 so the underlying Cloud Run
// Service stays warm between chained HTTP invocations.
func isRunnerPatternGCF(c *api.Container) bool {
	if c == nil {
		return false
	}
	if c.Config.Labels["sockerless.runner-pattern"] == "true" {
		return true
	}
	for portKey := range c.Config.ExposedPorts {
		_ = portKey
		return true
	}
	return false
}
