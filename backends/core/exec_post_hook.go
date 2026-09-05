package core

import (
	"io"
	"sync"
)

// ExecPostHook wraps an exec stream so a follow-up action runs exactly
// once when the caller closes the stream. By then the exec has finished
// and, for a synced workspace volume, the bootstrap has uploaded its
// modifications, so this is where a backend pulls them back.
type ExecPostHook struct {
	io.ReadWriteCloser
	once sync.Once
	hook func()
}

// NewExecPostHook returns stream with hook attached to its Close.
func NewExecPostHook(stream io.ReadWriteCloser, hook func()) *ExecPostHook {
	return &ExecPostHook{ReadWriteCloser: stream, hook: hook}
}

// Close closes the underlying stream and then runs the hook once.
func (e *ExecPostHook) Close() error {
	err := e.ReadWriteCloser.Close()
	e.once.Do(e.hook)
	return err
}
