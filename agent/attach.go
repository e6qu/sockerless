package agent

import (
	"encoding/base64"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// AttachSession pipes a WebSocket connection to the main process stdio.
type AttachSession struct {
	id     string
	mp     *MainProcess
	conn   *websocket.Conn
	connMu *sync.Mutex
	logger zerolog.Logger
	done   chan struct{}
}

// NewAttachSession creates and starts an attach session to the main process.
func NewAttachSession(id string, mp *MainProcess, conn *websocket.Conn, connMu *sync.Mutex, logger zerolog.Logger) *AttachSession {
	s := &AttachSession{
		id:     id,
		mp:     mp,
		conn:   conn,
		connMu: connMu,
		logger: logger.With().Str("session", id).Logger(),
		done:   make(chan struct{}),
	}

	go s.stream()
	return s
}

func (s *AttachSession) stream() {
	defer close(s.done)

	// Subscribe to main process output
	stdoutBuf, stderrBuf, ch := s.mp.Subscribe(s.id)

	// Replay buffered output
	if len(stdoutBuf) > 0 {
		s.sendOutput(TypeStdout, stdoutBuf)
	}
	if len(stderrBuf) > 0 {
		s.sendOutput(TypeStderr, stderrBuf)
	}

	// Check if process already exited
	if code := s.mp.ExitCode(); code != nil {
		s.sendExit(*code)
		return
	}

	// Stream live output
	for evt := range ch {
		s.sendOutput(evt.Stream, evt.Data)
	}

	// Process exited — send exit message
	if code := s.mp.ExitCode(); code != nil {
		s.sendExit(*code)
	}
}

func (s *AttachSession) sendOutput(streamType string, data []byte) {
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

func (s *AttachSession) sendExit(code int) {
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
func (s *AttachSession) ID() string { return s.id }

// WriteStdin writes data to the main process stdin.
func (s *AttachSession) WriteStdin(data []byte) error {
	return s.mp.WriteStdin(data)
}

// CloseStdin closes the main process stdin.
func (s *AttachSession) CloseStdin() error {
	return s.mp.CloseStdin()
}

// Signal sends a signal to the main process.
func (s *AttachSession) Signal(sig string) error {
	osSignal := parseSignal(sig)
	if osSignal == nil {
		return &sessionError{"unknown signal: " + sig}
	}
	return s.mp.Signal(osSignal)
}

// Resize is a no-op for attach sessions (main process has no PTY).
func (s *AttachSession) Resize(width, height int) error {
	return nil
}

// Close cleans up the session.
func (s *AttachSession) Close() {
	s.mp.Unsubscribe(s.id)
}
