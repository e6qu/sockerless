package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ContainerProcess represents a running container's process.
// Implemented by the memory backend's WASM sandbox adapter.
type ContainerProcess interface {
	// Lifecycle
	Wait() int
	Signal()
	Close()
	Done() <-chan struct{}

	// Logs & streaming
	LogBytes() []byte
	Subscribe(id string) chan []byte
	Unsubscribe(id string)

	// Stdin
	StdinWriter() io.WriteCloser

	// Exec
	RunExec(ctx context.Context, cmd []string, env []string,
		workDir string, stdin io.Reader, stdout, stderr io.Writer) int
	RunInteractiveShell(ctx context.Context, env []string,
		stdin io.Reader, stdout, stderr io.Writer) int

	// Monitoring
	Stats() ProcessStats
	Top() []ProcessTopEntry
	StartTime() time.Time
	FormatStartTime() string

	// Filesystem
	RootPath() string
}

// ProcessStats holds resource usage data for a container process.
type ProcessStats struct {
	MemoryUsage int64
	CPUNanos    int64
	PIDs        int
}

// ProcessTopEntry represents a single process in the top listing.
type ProcessTopEntry struct {
	PID     int
	Command string
}

// ProcessFactory creates container processes. Injected by backends
// that support real command execution (e.g., memory backend with WASM).
type ProcessFactory interface {
	NewProcess(cmd []string, env []string, binds map[string]string) (ContainerProcess, error)
	IsShellCommand(cmd []string) bool
	Close(ctx context.Context) error
}

// DirSize walks a directory and returns the total size of all files in bytes.
func DirSize(path string) int64 {
	var size int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}
