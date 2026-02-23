package core

import (
	"context"
	"io"
	"net"
	"os"

	"github.com/sockerless/api"
)

// --- Driver Interfaces ---
//
// These interfaces formalize the execution patterns that were previously
// scattered across handler code as if/else chains. Each driver represents
// a capability that can be backed by different implementations:
//   - Synthetic (fallback, no real execution)
//   - WASM (memory backend, mvdan.cc/sh + busybox)
//   - Agent (cloud backends, forward or reverse agent bridge)
//
// The DriverSet composes drivers into a dispatch chain. Handlers call
// through drivers instead of branching on container state directly.

// ExecDriver runs commands inside containers and captures output + exit code.
type ExecDriver interface {
	// Exec runs a command in the container and streams I/O over conn.
	// When tty is false, output must be wrapped with Docker multiplexed
	// stream headers (8-byte prefix per chunk).
	Exec(ctx context.Context, containerID string, execID string,
		cmd []string, env []string, workDir string, tty bool,
		conn net.Conn) (exitCode int)
}

// FilesystemDriver manages the container's root filesystem for archive ops.
type FilesystemDriver interface {
	// PutArchive extracts a tar archive into the container at the given path.
	PutArchive(containerID string, path string, tarStream io.Reader) error

	// GetArchive writes a tar archive of the container path to w.
	// Returns file info for the stat header, or error if path not found.
	GetArchive(containerID string, path string, w io.Writer) (os.FileInfo, error)

	// StatPath returns file info for a path inside the container.
	StatPath(containerID string, path string) (os.FileInfo, error)

	// RootPath returns the host filesystem root for the container, or ""
	// if no real filesystem exists (synthetic mode).
	RootPath(containerID string) (string, error)
}

// StreamDriver handles bidirectional streaming between client and container.
type StreamDriver interface {
	// Attach establishes a bidirectional stream to the container.
	// The conn is the hijacked HTTP connection.
	Attach(ctx context.Context, containerID string, tty bool, conn net.Conn) error

	// LogBytes returns the buffered log output for a container.
	LogBytes(containerID string) []byte

	// LogSubscribe returns a channel that receives live log chunks for follow mode.
	// The channel is closed when the container exits. Returns nil if not supported.
	LogSubscribe(containerID, subID string) chan []byte

	// LogUnsubscribe removes a log subscription.
	LogUnsubscribe(containerID, subID string)
}

// ProcessLifecycleDriver manages container process start/stop/wait.
type ProcessLifecycleDriver interface {
	// Start spawns the container's main process.
	// Returns true if a real process was started, false for synthetic.
	Start(containerID string, cmd []string, env []string,
		binds map[string]string) (started bool, err error)

	// Stop signals the container process to stop.
	Stop(containerID string)

	// Kill sends a termination signal to the container process.
	Kill(containerID string)

	// Cleanup releases resources for a container process.
	Cleanup(containerID string)

	// WaitCh returns a channel that closes when the container process exits.
	// For synthetic containers, returns a closed channel (immediate).
	WaitCh(containerID string) <-chan struct{}

	// Top returns the list of running processes inside the container.
	Top(containerID string) ([]ProcessTopEntry, error)

	// Stats returns resource usage statistics for the container.
	Stats(containerID string) (*ProcessStats, error)

	// IsSynthetic returns true if the container has no real process
	// (i.e., running in synthetic/no-op mode).
	IsSynthetic(containerID string) bool
}

// DriverSet holds the complete set of drivers used by a backend.
// Handlers dispatch through these interfaces instead of using if/else chains.
// A nil driver means the handler uses its built-in default behavior.
type DriverSet struct {
	Exec             ExecDriver
	Filesystem       FilesystemDriver
	Stream           StreamDriver
	ProcessLifecycle ProcessLifecycleDriver
	Network          api.NetworkDriver
}
