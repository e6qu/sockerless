package agent

import "os"

// EnsureWorkdir returns the working directory a workload starts in, created
// when it does not exist: Docker creates a container's configured working
// directory at start, and a client that sets one it has not created (act
// sets its job container's to the checkout path) expects the same of the
// bootstrap that stands in for the engine.
func EnsureWorkdir(dir string) string {
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return dir
}
