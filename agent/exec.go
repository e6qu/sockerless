package agent

import (
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// ExecSession handles an exec request: fork+exec a child process.
type ExecSession struct {
	id       string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser             // pipe read end (nil in TTY mode); closed on reaper path
	stderr   io.ReadCloser             // pipe read end (nil in TTY mode); closed on reaper path
	ptmx     *os.File                  // non-nil when TTY mode
	tty      bool                      // requested TTY mode (drives Start)
	reaper   *childReaper              // non-nil when running under PID-1 init; owns child waiting
	statusCh <-chan syscall.WaitStatus // delivers the child's exit status on the reaper path
	conn     *websocket.Conn
	connMu   *sync.Mutex
	logger   zerolog.Logger
	done     chan struct{}
	streamWg sync.WaitGroup // tracks readStream/readPTY goroutines
	postExec func()         // optional per-exec teardown (e.g. gcs-sync save), run after the process exits, before the exit code is sent
	onExit   func()         // optional hook run after the exit frame is sent (e.g. forget this session from the registry); set by the router
}

// NewExecSession creates an exec session; the caller invokes Start to launch
// the process (after registering the session). postExec (may be nil) runs
// after the process exits, before the exit code is sent to the backend —
// used by the cloudrun bootstrap to save the gcs-sync workspace so the
// backend's PostExec download sees the step's modifications. onExit (may be
// nil) runs after the exit frame is sent — the router uses it to forget the
// finished session from the registry.
func NewExecSession(id string, msg *Message, conn *websocket.Conn, connMu *sync.Mutex, logger zerolog.Logger, postExec, onExit func(), reaper *childReaper) (*ExecSession, error) {
	if len(msg.Cmd) == 0 {
		return nil, &sessionError{"exec requires cmd"}
	}

	cmd := exec.Command(msg.Cmd[0], msg.Cmd[1:]...)

	if msg.WorkDir != "" {
		cmd.Dir = msg.WorkDir
	}

	// Inherit environment and add extras
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, msg.Env...)

	// Set process group for signal delivery
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	s := &ExecSession{
		id:       id,
		cmd:      cmd,
		conn:     conn,
		connMu:   connMu,
		logger:   logger.With().Str("session", id).Logger(),
		done:     make(chan struct{}),
		postExec: postExec,
		onExit:   onExit,
		tty:      msg.Tty,
		reaper:   reaper,
	}

	return s, nil
}

// Start launches the session's child process and its stream/wait
// goroutines. The router calls Start AFTER registering the session, so the
// onExit hook (which forgets the session from the registry) can never race
// ahead of the Register that added it. A start failure is returned without
// any goroutine having run, so the caller removes the session it registered.
func (s *ExecSession) Start() error {
	if s.tty {
		return s.startWithPTY()
	}
	return s.startWithPipes()
}

func (s *ExecSession) startWithPTY() error {
	var ptmx *os.File
	if s.reaper != nil {
		ch, err := s.reaper.startAndRegister(func() (int, error) {
			p, perr := pty.Start(s.cmd)
			if perr != nil {
				return 0, perr
			}
			ptmx = p
			return s.cmd.Process.Pid, nil
		})
		if err != nil {
			return err
		}
		s.statusCh = ch
	} else {
		p, err := pty.Start(s.cmd)
		if err != nil {
			return err
		}
		ptmx = p
	}
	s.ptmx = ptmx
	s.stdin = ptmx

	s.streamWg.Add(1)
	go s.readPTY(ptmx)
	go s.waitAndNotify()
	return nil
}

func (s *ExecSession) startWithPipes() error {
	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		return err
	}
	s.stdin = stdin

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return err
	}
	// Retain the pipe read ends so the reaper path can close them itself
	// (it skips cmd.Wait(), which is what normally closes them).
	s.stdout = stdout
	s.stderr = stderr

	if s.reaper != nil {
		ch, serr := s.reaper.startAndRegister(func() (int, error) {
			if cerr := s.cmd.Start(); cerr != nil {
				return 0, cerr
			}
			return s.cmd.Process.Pid, nil
		})
		if serr != nil {
			return serr
		}
		s.statusCh = ch
	} else if err := s.cmd.Start(); err != nil {
		return err
	}

	s.streamWg.Add(2)
	go s.readStream(stdout, TypeStdout)
	go s.readStream(stderr, TypeStderr)
	go s.waitAndNotify()
	return nil
}

func (s *ExecSession) readPTY(r io.Reader) {
	defer s.streamWg.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			s.sendOutput(TypeStdout, buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (s *ExecSession) readStream(r io.Reader, streamType string) {
	defer s.streamWg.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			s.sendOutput(streamType, buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (s *ExecSession) sendOutput(streamType string, data []byte) {
	msg := Message{
		Type: streamType,
		ID:   s.id,
		Data: base64.StdEncoding.EncodeToString(data),
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if err := s.conn.WriteJSON(msg); err != nil {
		s.logger.Debug().Err(err).Msg("failed to send output")
	}
}

// waitCode returns the child's exit code. On the reaper path it reads the
// status the childReaper collected (cmd.Wait() must NOT be called there — it
// would race the reaper's Wait4 for the same PID and lose the status) and
// closes the parent pipe fds itself, the cleanup cmd.Wait() would otherwise do.
// On the non-reaper path it uses cmd.Wait() as the host's init reaps orphans.
func (s *ExecSession) waitCode() int {
	if s.reaper != nil {
		status := <-s.statusCh
		if s.stdout != nil {
			_ = s.stdout.Close()
		}
		if s.stderr != nil {
			_ = s.stderr.Close()
		}
		switch {
		case status.Exited():
			return status.ExitStatus()
		case status.Signaled():
			return 128 + int(status.Signal())
		default:
			return 1
		}
	}

	err := s.cmd.Wait()
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func (s *ExecSession) waitAndNotify() {
	defer close(s.done)

	// Wait for stream readers to finish before cmd.Wait(), because
	// cmd.Wait() closes the pipe descriptors. When the process exits,
	// it closes its stdout/stderr write ends, causing readStream to
	// get EOF and return — independent of cmd.Wait().
	s.streamWg.Wait()

	code := s.waitCode()

	s.logger.Debug().Int("code", code).Msg("process exited")

	// Per-exec teardown (e.g. gcs-sync workspace save) before the exit
	// code is sent, so the backend's PostExec sync-back — which it runs on
	// the exec-stream close that follows this exit — sees the saved state.
	if s.postExec != nil {
		s.postExec()
	}

	msg := Message{
		Type: TypeExit,
		ID:   s.id,
		Code: intPtr(code),
	}
	s.connMu.Lock()
	// If the exit frame fails to send, the backend's bridge blocks until
	// the whole WebSocket tears down and the caller sees an opaque exit
	// rather than this code — so log it loudly on the agent side.
	if err := s.conn.WriteJSON(msg); err != nil {
		s.logger.Error().Err(err).Str("id", s.id).Int("code", code).Msg("failed to send exit frame to backend — exec exit code may be lost")
	}
	s.connMu.Unlock()

	// Forget this finished session from the registry so a keep-alive
	// connection serving many execs doesn't leak a stale *ExecSession per
	// exec. Done after the exit frame (and after releasing connMu) so the
	// registry teardown never re-SIGKILLs the already-exited process.
	if s.onExit != nil {
		s.onExit()
	}
}

// ID returns the session identifier.
func (s *ExecSession) ID() string { return s.id }

// WriteStdin writes data to the process stdin.
func (s *ExecSession) WriteStdin(data []byte) error {
	if s.stdin == nil {
		return &sessionError{"stdin not available"}
	}
	_, err := s.stdin.Write(data)
	return err
}

// CloseStdin closes the process stdin.
func (s *ExecSession) CloseStdin() error {
	if s.stdin == nil {
		return nil
	}
	return s.stdin.Close()
}

// Signal sends a signal to the process.
func (s *ExecSession) Signal(sig string) error {
	osSignal := parseSignal(sig)
	if osSignal == nil {
		return &sessionError{"unknown signal: " + sig}
	}
	if s.cmd.Process == nil {
		return &sessionError{"process not started"}
	}
	return s.cmd.Process.Signal(osSignal)
}

// Resize resizes the PTY.
func (s *ExecSession) Resize(width, height int) error {
	if s.ptmx == nil {
		return nil
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{
		Cols: uint16(width),
		Rows: uint16(height),
	})
}

// Close cleans up the session.
func (s *ExecSession) Close() {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGKILL)
	}
	if s.ptmx != nil {
		_ = s.ptmx.Close()
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
	}
}

func parseSignal(sig string) os.Signal {
	sig = strings.ToUpper(strings.TrimPrefix(strings.ToUpper(sig), "SIG"))
	switch sig {
	case "TERM":
		return syscall.SIGTERM
	case "KILL":
		return syscall.SIGKILL
	case "INT":
		return syscall.SIGINT
	case "HUP":
		return syscall.SIGHUP
	case "QUIT":
		return syscall.SIGQUIT
	case "USR1":
		return syscall.SIGUSR1
	case "USR2":
		return syscall.SIGUSR2
	default:
		return nil
	}
}

type sessionError struct {
	msg string
}

func (e *sessionError) Error() string { return e.msg }
