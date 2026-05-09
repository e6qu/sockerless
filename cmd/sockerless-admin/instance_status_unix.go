//go:build !windows

package main

import "syscall"

// syscall0 returns the no-op signal used by readPidStatus to probe a
// PID for life. Pulled into a per-OS file because Windows doesn't have
// signal-0 semantics; admin doesn't target Windows today but this
// keeps the door open without leaking syscall constants into the main
// instance_status.go.
func syscall0() syscall.Signal { return syscall.Signal(0) }
