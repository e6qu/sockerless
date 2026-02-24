package agent

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/rs/zerolog"
)

const ringBufferSize = 1024 * 1024 // 1MB

// RingBuffer is a fixed-size circular buffer for capturing pre-attach output.
type RingBuffer struct {
	mu   sync.Mutex
	buf  []byte
	pos  int
	full bool
}

// NewRingBuffer creates a new ring buffer.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{buf: make([]byte, size)}
}

// Write writes data to the ring buffer.
func (rb *RingBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	n := len(p)
	for i := 0; i < n; i++ {
		rb.buf[rb.pos] = p[i]
		rb.pos++
		if rb.pos >= len(rb.buf) {
			rb.pos = 0
			rb.full = true
		}
	}
	return n, nil
}

// Bytes returns the buffered data in order.
func (rb *RingBuffer) Bytes() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if !rb.full {
		return append([]byte(nil), rb.buf[:rb.pos]...)
	}
	result := make([]byte, len(rb.buf))
	copy(result, rb.buf[rb.pos:])
	copy(result[len(rb.buf)-rb.pos:], rb.buf[:rb.pos])
	return result
}

// MainProcess manages the primary process lifecycle in keep-alive mode.
type MainProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	// Ring buffers capture output before any attach session connects.
	stdoutBuf *RingBuffer
	stderrBuf *RingBuffer

	// Fan-out to attached sessions.
	mu        sync.RWMutex
	listeners map[string]chan OutputEvent
	exitCode  *int
	done      chan struct{}
	logger    zerolog.Logger
}

// OutputEvent represents a chunk of output from the main process.
type OutputEvent struct {
	Stream string // "stdout" or "stderr"
	Data   []byte
}

// NewMainProcess creates and starts the main process.
func NewMainProcess(logger zerolog.Logger, args []string, env []string) (*MainProcess, error) {
	if len(args) == 0 {
		args = []string{"/bin/sh"}
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	// Set process group so signals propagate
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	mp := &MainProcess{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		stdoutBuf: NewRingBuffer(ringBufferSize),
		stderrBuf: NewRingBuffer(ringBufferSize),
		listeners: make(map[string]chan OutputEvent),
		done:      make(chan struct{}),
		logger:    logger,
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Fan-out stdout and stderr
	go mp.fanOut(stdout, "stdout", mp.stdoutBuf)
	go mp.fanOut(stderr, "stderr", mp.stderrBuf)

	// Wait for process to exit
	go mp.wait()

	return mp, nil
}

func (mp *MainProcess) fanOut(r io.Reader, stream string, buf *RingBuffer) {
	data := make([]byte, 32*1024)
	for {
		n, err := r.Read(data)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, data[:n])
			_, _ = buf.Write(chunk) // bytes.Buffer.Write never fails

			mp.mu.RLock()
			for _, ch := range mp.listeners {
				select {
				case ch <- OutputEvent{Stream: stream, Data: chunk}:
				default:
					// Drop if listener is slow
				}
			}
			mp.mu.RUnlock()
		}
		if err != nil {
			return
		}
	}
}

func (mp *MainProcess) wait() {
	err := mp.cmd.Wait()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = 1
		}
	}

	mp.mu.Lock()
	mp.exitCode = &code
	for _, ch := range mp.listeners {
		close(ch)
	}
	mp.listeners = nil
	mp.mu.Unlock()

	close(mp.done)
}

// Subscribe registers a listener for output events. Returns buffered output and a channel.
func (mp *MainProcess) Subscribe(id string) (stdoutBuf, stderrBuf []byte, ch chan OutputEvent) {
	ch = make(chan OutputEvent, 256)

	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Check if process already exited
	if mp.exitCode != nil {
		close(ch)
		return mp.stdoutBuf.Bytes(), mp.stderrBuf.Bytes(), ch
	}

	mp.listeners[id] = ch
	return mp.stdoutBuf.Bytes(), mp.stderrBuf.Bytes(), ch
}

// Unsubscribe removes a listener.
func (mp *MainProcess) Unsubscribe(id string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	delete(mp.listeners, id)
}

// WriteStdin writes data to the main process stdin.
func (mp *MainProcess) WriteStdin(data []byte) error {
	_, err := mp.stdin.Write(data)
	return err
}

// CloseStdin closes the main process stdin.
func (mp *MainProcess) CloseStdin() error {
	return mp.stdin.Close()
}

// Signal sends a signal to the main process.
func (mp *MainProcess) Signal(sig os.Signal) error {
	return mp.cmd.Process.Signal(sig)
}

// ExitCode returns the exit code if the process has exited.
func (mp *MainProcess) ExitCode() *int {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	return mp.exitCode
}

// Done returns a channel that is closed when the process exits.
func (mp *MainProcess) Done() <-chan struct{} {
	return mp.done
}

// Pid returns the process ID.
func (mp *MainProcess) Pid() int {
	if mp.cmd.Process != nil {
		return mp.cmd.Process.Pid
	}
	return 0
}
