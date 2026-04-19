// sockerless-lambda-bootstrap is the entrypoint binary injected into
// Lambda container images by the sockerless Lambda backend. It wraps a
// user's entrypoint so the Lambda function can serve both (a) the
// user's declared workload and (b) a reverse-agent WebSocket that
// sockerless uses to proxy "docker exec" into the running invocation.
//
// See docs/LAMBDA_EXEC_DESIGN.md for the full architecture. This file
// is currently a compile-clean skeleton: the Lambda Runtime API loop
// and the co-process supervisor are stubbed behind TODOs and will be
// filled in during the Phase-86 AWS track, since they require a real
// $AWS_LAMBDA_RUNTIME_API endpoint to exercise.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Env vars the bootstrap consults. The Runtime-API loop (TODO) will
// read envCallbackURL and envContainerID to wire the reverse agent.
const (
	envUserEntrypoint = "SOCKERLESS_USER_ENTRYPOINT" // colon-separated list
	envUserCmd        = "SOCKERLESS_USER_CMD"        // colon-separated list
	_                 = "SOCKERLESS_CALLBACK_URL"    // envCallbackURL; unused until Runtime-API loop lands
	_                 = "SOCKERLESS_CONTAINER_ID"    // envContainerID; unused until Runtime-API loop lands
)

func main() {
	// Fast-fail if the env is missing the basics. When the bootstrap is
	// invoked outside a Lambda (e.g. local smoke test), exec the user's
	// entrypoint directly with no agent supervision.
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") == "" {
		runUserProcessStandalone()
		return
	}

	// TODO(P86-005 AWS track): implement the Lambda Runtime API loop.
	// Poll $AWS_LAMBDA_RUNTIME_API/2018-06-01/runtime/invocation/next,
	// spawn (a) sockerless-agent in reverse mode and (b) the user's
	// entrypoint as a subprocess; block until one exits; POST the
	// result to /response or /error; loop. See design doc.
	fmt.Fprintln(os.Stderr, "sockerless-lambda-bootstrap: Runtime API loop not yet implemented (Phase 86 AWS track)")
	os.Exit(1)
}

// runUserProcessStandalone execs the user's declared entrypoint + cmd
// with no Lambda framing. Used when the binary is run outside Lambda
// (local container smoke tests, image-inject integration tests).
func runUserProcessStandalone() {
	argv := append(splitColon(os.Getenv(envUserEntrypoint)), splitColon(os.Getenv(envUserCmd))...)
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "sockerless-lambda-bootstrap: no user entrypoint/cmd configured")
		os.Exit(0)
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-lambda-bootstrap: exec %q: %v\n", argv[0], err)
		os.Exit(127)
	}
	if err := syscall.Exec(bin, argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-lambda-bootstrap: exec %q: %v\n", bin, err)
		os.Exit(126)
	}
}

func splitColon(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ":")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
