package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ProcessStats holds resource usage data for a container process.
type ProcessStats struct {
	MemoryUsage int64 // bytes used by rootDir on disk
	CPUNanos    int64 // cumulative wall-clock nanoseconds of WASM command executions
	PIDs        int   // number of active processes (1 main + active execs)
}

// ProcessTopEntry represents a single process in the top listing.
type ProcessTopEntry struct {
	PID     int
	Command string
}

// Process manages the lifecycle of a container's main process.
// It owns the container's temp directory (virtual filesystem),
// captures output for docker logs, and supports fan-out to
// docker attach sessions.
type Process struct {
	runtime *Runtime
	RootDir string // exported for archive handlers
	mounts  []DirMount
	mainCmd string // entrypoint command string for top listing

	cancel context.CancelFunc
	ctx    context.Context

	// Output capture
	logBuf bytes.Buffer
	logMu  sync.Mutex

	// Fan-out to attach listeners
	listeners map[string]chan []byte
	listMu    sync.Mutex

	// Stdin pipe for attach-based input (e.g. gitlab-runner)
	stdinReader *io.PipeReader
	stdinWriter *io.PipeWriter

	// Exec tracking
	nextPID   atomic.Int32   // next PID to assign for exec sessions
	activeExecs sync.Map     // pid (int32) → command string
	cpuNanos  atomic.Int64   // cumulative wall-clock nanoseconds of all command executions

	startTime time.Time
	exitCode  int
	done      chan struct{}
}

// NewProcess creates a new container process. It creates a temp directory,
// populates it with an Alpine-like rootfs, and starts the main command
// in a background goroutine. The binds map specifies containerPath → hostPath
// directory mounts for volumes.
func NewProcess(rt *Runtime, cmd []string, env []string, binds map[string]string) (*Process, error) {
	// Check if the primary binary exists in the sandbox.
	// Unknown commands (e.g. "gitlab-runner-helper") can't run in WASM;
	// returning an error lets the backend fall back to synthetic behavior.
	if len(cmd) > 0 && !IsKnownCommand(cmd[0]) {
		return nil, fmt.Errorf("command not found in sandbox: %s", cmd[0])
	}

	rootDir, err := os.MkdirTemp("", "sandbox-*")
	if err != nil {
		return nil, err
	}
	if err := PopulateRootfs(rootDir); err != nil {
		os.RemoveAll(rootDir)
		return nil, err
	}

	// Build dir mounts from binds + create symlinks in rootDir for shell.
	// Sort container paths shortest-first so parent mounts are created before
	// children. Skip creating symlinks for paths under an already-mounted parent
	// (overlapping mounts like /__w and /__w/_temp cause symlink loops otherwise).
	var containerPaths []string
	for cp := range binds {
		containerPaths = append(containerPaths, cp)
	}
	sort.Strings(containerPaths)

	var mounts []DirMount
	var mountedPrefixes []string
	for _, containerPath := range containerPaths {
		hostPath := binds[containerPath]
		mounts = append(mounts, DirMount{
			HostPath:      hostPath,
			ContainerPath: containerPath,
		})

		// Skip symlink creation if this path is under an already-mounted parent
		cleanCP := filepath.Clean(containerPath)
		underParent := false
		for _, prefix := range mountedPrefixes {
			if strings.HasPrefix(cleanCP, prefix+"/") {
				underParent = true
				break
			}
		}
		if underParent {
			continue
		}
		mountedPrefixes = append(mountedPrefixes, cleanCP)

		// Create symlink in rootDir so shell redirects follow the link
		target := filepath.Join(rootDir, cleanCP)
		_ = os.MkdirAll(filepath.Dir(target), 0755)
		os.RemoveAll(target)
		_ = os.Symlink(hostPath, target)
	}

	ctx, cancel := context.WithCancel(context.Background())

	stdinR, stdinW := io.Pipe()

	p := &Process{
		runtime:     rt,
		RootDir:     rootDir,
		mounts:      mounts,
		mainCmd:     strings.Join(cmd, " "),
		cancel:      cancel,
		ctx:         ctx,
		stdinReader: stdinR,
		stdinWriter: stdinW,
		listeners:   make(map[string]chan []byte),
		startTime:   time.Now(),
		done:        make(chan struct{}),
	}
	p.nextPID.Store(2) // PID 1 is the main process

	// Start the main command in a goroutine
	go p.run(cmd, env)

	return p, nil
}

func (p *Process) run(cmd []string, env []string) {
	// Detect "tail -f /dev/null" keepalive pattern — WASI has no inotify,
	// so busybox tail -f would exit immediately. Instead, block until canceled.
	if isTailDevNull(cmd) {
		<-p.ctx.Done()
		p.exitCode = 0
		p.listMu.Lock()
		for _, ch := range p.listeners {
			close(ch)
		}
		p.listMu.Unlock()
		close(p.done)
		return
	}

	w := &fanOutWriter{p: p}

	start := time.Now()
	exitCode, _ := p.runtime.RunSimpleCommand(p.ctx, cmd, env, p.RootDir, p.mounts, "", p.stdinReader, w, w)
	p.cpuNanos.Add(time.Since(start).Nanoseconds())
	p.exitCode = exitCode

	// Close all listener channels
	p.listMu.Lock()
	for _, ch := range p.listeners {
		close(ch)
	}
	p.listMu.Unlock()

	close(p.done)
}

// isTailDevNull returns true for the standard Docker keepalive pattern:
// tail -f /dev/null (blocks forever, used to keep containers alive)
func isTailDevNull(cmd []string) bool {
	if len(cmd) < 3 {
		return false
	}
	return cmd[0] == "tail" && cmd[1] == "-f" && cmd[2] == "/dev/null"
}

// Wait blocks until the main process exits and returns its exit code.
func (p *Process) Wait() int {
	<-p.done
	return p.exitCode
}

// Signal cancels the process context, which kills the WASM module
// via WithCloseOnContextDone.
func (p *Process) Signal() {
	p.cancel()
}

// LogBytes returns the accumulated stdout/stderr output.
func (p *Process) LogBytes() []byte {
	p.logMu.Lock()
	defer p.logMu.Unlock()
	return append([]byte(nil), p.logBuf.Bytes()...)
}

// Subscribe registers a listener for live output. Returns a channel
// that receives output chunks. The channel is closed when the process exits.
func (p *Process) Subscribe(id string) chan []byte {
	ch := make(chan []byte, 256)
	p.listMu.Lock()
	defer p.listMu.Unlock()

	// If already done, close immediately
	select {
	case <-p.done:
		close(ch)
		return ch
	default:
	}

	p.listeners[id] = ch
	return ch
}

// Unsubscribe removes a listener.
func (p *Process) Unsubscribe(id string) {
	p.listMu.Lock()
	defer p.listMu.Unlock()
	delete(p.listeners, id)
}

// Done returns a channel that is closed when the process exits.
func (p *Process) Done() <-chan struct{} {
	return p.done
}

// StdinWriter returns a writer that feeds into the process's stdin pipe.
func (p *Process) StdinWriter() io.WriteCloser {
	return p.stdinWriter
}

// Close cancels the process, waits for it to finish, and removes the
// temp directory.
func (p *Process) Close() {
	_ = p.stdinWriter.Close()
	p.cancel()
	<-p.done
	os.RemoveAll(p.RootDir)
}

// RunExec runs a command in the same container filesystem. This is used
// by docker exec to run additional commands in a running container.
// If workDir is non-empty, the command executes in that directory
// (relative to the container root).
func (p *Process) RunExec(ctx context.Context, cmd []string, env []string,
	workDir string, stdin io.Reader, stdout, stderr io.Writer) int {

	pid := p.nextPID.Add(1) - 1
	cmdStr := strings.Join(cmd, " ")
	p.activeExecs.Store(pid, cmdStr)
	defer p.activeExecs.Delete(pid)

	// Ensure workdir exists in rootDir (like real Docker does)
	if workDir != "" {
		_ = os.MkdirAll(filepath.Join(p.RootDir, filepath.Clean(workDir)), 0755)
	}

	start := time.Now()
	exitCode, _ := p.runtime.RunSimpleCommand(ctx, cmd, env, p.RootDir, p.mounts, workDir, stdin, stdout, stderr)
	p.cpuNanos.Add(time.Since(start).Nanoseconds())
	return exitCode
}

// RunInteractiveShell starts an interactive shell in the container.
func (p *Process) RunInteractiveShell(ctx context.Context, env []string,
	stdin io.Reader, stdout, stderr io.Writer) int {

	pid := p.nextPID.Add(1) - 1
	p.activeExecs.Store(pid, "sh")
	defer p.activeExecs.Delete(pid)

	start := time.Now()
	err := p.runtime.RunInteractiveShell(ctx, env, p.RootDir, p.mounts, stdin, stdout, stderr)
	p.cpuNanos.Add(time.Since(start).Nanoseconds())
	if err != nil {
		return 1
	}
	return 0
}

// Stats returns resource usage data for this container process.
func (p *Process) Stats() ProcessStats {
	memUsage := dirSize(p.RootDir)

	// Count active PIDs: 1 (main process) + active execs
	pids := 1
	p.activeExecs.Range(func(_, _ any) bool {
		pids++
		return true
	})

	return ProcessStats{
		MemoryUsage: memUsage,
		CPUNanos:    p.cpuNanos.Load(),
		PIDs:        pids,
	}
}

// Top returns the list of processes running in this container.
func (p *Process) Top() []ProcessTopEntry {
	entries := []ProcessTopEntry{
		{PID: 1, Command: p.mainCmd},
	}
	p.activeExecs.Range(func(key, value any) bool {
		entries = append(entries, ProcessTopEntry{
			PID:     int(key.(int32)),
			Command: value.(string),
		})
		return true
	})
	return entries
}

// StartTime returns when the process was started.
func (p *Process) StartTime() time.Time {
	return p.startTime
}

// dirSize walks a directory and returns the total size of all files in bytes.
func dirSize(path string) int64 {
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

// DirSize is exported for use by the backend handlers (e.g. system df).
func DirSize(path string) int64 {
	return dirSize(path)
}

// RootPath returns the container's root directory path.
func (p *Process) RootPath() string { return p.RootDir }

// FormatStartTime formats the process start time as HH:MM for top output.
func (p *Process) FormatStartTime() string {
	return fmt.Sprintf("%02d:%02d", p.startTime.Hour(), p.startTime.Minute())
}

// fanOutWriter writes to the process log buffer and broadcasts to listeners.
type fanOutWriter struct {
	p *Process
}

func (w *fanOutWriter) Write(data []byte) (int, error) {
	n := len(data)
	chunk := make([]byte, n)
	copy(chunk, data)

	// Append to log buffer
	w.p.logMu.Lock()
	w.p.logBuf.Write(chunk)
	w.p.logMu.Unlock()

	// Broadcast to listeners
	w.p.listMu.Lock()
	for _, ch := range w.p.listeners {
		select {
		case ch <- chunk:
		default:
			// Drop if listener is slow
		}
	}
	w.p.listMu.Unlock()

	return n, nil
}
