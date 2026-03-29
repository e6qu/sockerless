//go:build !windows

package core

import (
	"os/exec"
	"strings"
)

// hostKernelVersion returns the kernel version of the host OS via uname -r.
func hostKernelVersion() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
