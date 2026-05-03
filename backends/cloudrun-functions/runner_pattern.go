package gcf

import (
	"strings"

	"github.com/sockerless/api"
)

// isRunnerPatternGCF mirrors cloudrun.isRunnerPattern. Detects whether
// a container is "runner-pattern" (long-lived) — used by gcf to set
// min_instance_count=1 on the underlying Cloud Run Service so the
// Function instance stays warm between chained HTTP invocations
// (Phase 122f). One-shot containers stay at min_instance_count=0
// (default) so they cold-start per invocation.
func isRunnerPatternGCF(c *api.Container) bool {
	if c == nil {
		return false
	}
	if c.Config.Labels["sockerless.runner-pattern"] == "true" {
		return true
	}
	cmd := strings.Join(append(append([]string{c.Path}, c.Args...), c.Config.Cmd...), " ")
	cmd = strings.TrimSpace(cmd)
	holdOpen := []string{
		"tail -f /dev/null",
		"sleep infinity",
		"while true",
		"while :",
		"sleep 9223372036854775807",
	}
	for _, p := range holdOpen {
		if cmd != "" && strings.Contains(cmd, p) {
			return true
		}
	}
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
