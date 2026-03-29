package core

import "runtime"

// hostKernelVersion returns a version string on Windows where uname is not available.
func hostKernelVersion() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
