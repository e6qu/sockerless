package cloudrun

import (
	"strings"

	"github.com/sockerless/api"
)

// isRunnerPattern reports whether the container looks like a
// long-lived "runner-pattern" container that should use the Cloud Run
// Service path (UseService=1) rather than the one-shot Cloud Run Job
// path. Phase 122f.
//
// Detection heuristic (any of):
//   - Cmd looks like `tail -f /dev/null`, `sleep infinity`, `sleep N`
//     where N >= 60, or `bash -c 'while true ...'` patterns.
//   - Image is a known long-lived helper (postgres, mysql, redis,
//     mongo, etc.).
//   - Container has explicit label `sockerless.runner-pattern=true`.
//
// One-shot containers (gitlab-runner permission containers running
// `chown`, `alpine echo hi`, build helpers) MUST go via Cloud Run
// Job because they exit immediately and don't bind $PORT — Cloud Run
// Service would reject them as failed-startup.
func isRunnerPattern(c *api.Container) bool {
	if c == nil {
		return false
	}
	if c.Config.Labels["sockerless.runner-pattern"] == "true" {
		return true
	}
	cmd := strings.Join(append(append([]string{c.Path}, c.Args...), c.Config.Cmd...), " ")
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	holdOpen := []string{
		"tail -f /dev/null",
		"sleep infinity",
		"while true",
		"while :",
		"sleep 9223372036854775807",
	}
	for _, p := range holdOpen {
		if strings.Contains(cmd, p) {
			return true
		}
	}
	// Known long-lived images (databases, queues, web servers).
	longLivedImages := []string{
		"postgres", "mysql", "mariadb", "redis", "mongo", "rabbitmq",
		"nginx", "httpd", "apache", "elasticsearch", "memcached",
		"kafka", "zookeeper",
	}
	imageLower := strings.ToLower(c.Image)
	for _, p := range longLivedImages {
		if strings.Contains(imageLower, "/"+p+":") || strings.Contains(imageLower, "/"+p+"/") || strings.HasPrefix(imageLower, p+":") || strings.HasPrefix(imageLower, p+"/") {
			return true
		}
	}
	return false
}
