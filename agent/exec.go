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
	ptmx     *os.File // non-nil when TTY mode
	conn     *websocket.Conn
	connMu   *sync.Mutex
	logger   zerolog.Logger
	done     chan struct{}
	streamWg sync.WaitGroup // tracks readStream/readPTY goroutines
}

// NewExecSession creates and starts an exec session.
func NewExecSession(id string, msg *Message, conn *websocket.Conn, connMu *sync.Mutex, logger zerolog.Logger) (*ExecSession, error) {
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
		id:     id,
		cmd:    cmd,
		conn:   conn,
		connMu: connMu,
		logger: logger.With().Str("session", id).Logger(),
		done:   make(chan struct{}),
	}

	if msg.Tty {
		if err := s.startWithPTY(); err != nil {
			return nil, err
		}
	} else {
		if err := s.startWithPipes(); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *ExecSession) startWithPTY() error {
	ptmx, err := pty.Start(s.cmd)
	if err != nil {
		return err
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

	if err := s.cmd.Start(); err != nil {
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

func (s *ExecSession) waitAndNotify() {
	defer close(s.done)
	err := s.cmd.Wait()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = 1
		}
	}

	// Wait for stream readers to finish sending all output before sending exit.
	s.streamWg.Wait()

	s.logger.Debug().Int("code", code).Msg("process exited")

	msg := Message{
		Type: TypeExit,
		ID:   s.id,
		Code: intPtr(code),
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	s.conn.WriteJSON(msg)
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
