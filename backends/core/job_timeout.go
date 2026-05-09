// Runner job timeout helpers shared across backends.
//
// Sockerless backends inject SOCKERLESS_JOB_TIMEOUT_SECONDS on every
// workload container they materialize so the in-container bootstrap
// (`sockerless-{cloudrun,gcf}-bootstrap`) arms a timer. Default is
// 3600 seconds (1 h). Operators can override at three layers, in
// order of precedence (later wins):
//
//  1. **Per-job:** `docker run -e SOCKERLESS_JOB_TIMEOUT_SECONDS=120 …`.
//     The backend must respect a value the user already set in the
//     container's env list. JobTimeoutEnvIfUnset honours this.
//  2. **Per-backend default:** the operator sets
//     SOCKERLESS_JOB_TIMEOUT_SECONDS in the backend's own process env;
//     each new container gets that as its default.
//  3. **Sockerless default:** 3600 (DefaultJobTimeoutSeconds).
//
// Cloud-native caps (Cloud Run Jobs 24 h, ACA 7 d, Lambda 15 min, ECS
// effectively unlimited) are enforced separately by the backend at
// the cloud-API layer; this helper is the bootstrap-side timer's
// configuration.

package core

import (
	"os"
	"strconv"
	"strings"
)

const (
	// JobTimeoutEnvName is the env var name workload containers expose
	// for the bootstrap to read. Same name on every cloud.
	JobTimeoutEnvName = "SOCKERLESS_JOB_TIMEOUT_SECONDS"

	// DefaultJobTimeoutSeconds is the sockerless-wide default when
	// neither per-job nor per-backend overrides set the value.
	DefaultJobTimeoutSeconds = 3600
)

// JobTimeoutDefault returns the seconds the backend should default to
// when injecting SOCKERLESS_JOB_TIMEOUT_SECONDS on a container that
// doesn't already carry one. Reads the backend's process env first;
// falls back to DefaultJobTimeoutSeconds. Negative is clamped to 0
// (effectively disabled).
func JobTimeoutDefault() int {
	raw := strings.TrimSpace(os.Getenv(JobTimeoutEnvName))
	if raw == "" {
		return DefaultJobTimeoutSeconds
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return DefaultJobTimeoutSeconds
	}
	if n < 0 {
		return 0
	}
	return n
}

// JobTimeoutEnvIfUnset returns the SOCKERLESS_JOB_TIMEOUT_SECONDS
// env entry to append to the workload container's env. Returns ""
// when the user already set it via `docker run -e` — the user's value
// wins. The returned string is in `KEY=VALUE` form ready to append to
// a []string env list.
func JobTimeoutEnvIfUnset(userEnv []string) string {
	if hasEnv(userEnv, JobTimeoutEnvName) {
		return ""
	}
	return JobTimeoutEnvName + "=" + strconv.Itoa(JobTimeoutDefault())
}

// hasEnv reports whether `KEY=` appears as a prefix of any entry in env.
func hasEnv(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}
