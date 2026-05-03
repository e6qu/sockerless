package cloudrun

import (
	"github.com/sockerless/api"
)

// isRunnerPattern reports whether the container should use the Cloud
// Run Service path (UseService=1) rather than Cloud Run Job. Phase 122f.
//
// Detection — REAL signals only, no hardcoded image lists or cmd
// patterns (per project no-defaults / no-fallbacks rule):
//   - Container has explicit label `sockerless.runner-pattern=true`,
//     OR the image declares ExposedPorts (it's an HTTP/TCP service
//     that binds a port and Cloud Run Service can health-probe it).
//
// Everything else (one-shot commands, hold-open `tail -f /dev/null`
// containers without ExposedPorts, etc.) routes to Cloud Run Job.
// Cloud Run Job is itself long-lived for the cmd's duration, so
// `tail -f` containers stay alive there too — no Service needed.
func isRunnerPattern(c *api.Container) bool {
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
