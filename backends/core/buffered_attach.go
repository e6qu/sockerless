package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// BufferedAttachStream is the hijacked stream a backend returns to an
// attach caller when the workload runs as one buffered invocation rather
// than a live process. Writes go into the container's StdinPipe, to be
// replayed as the command's stdin when the invocation fires. Reads block
// until the invocation's output is published, then return it framed in
// Docker's stdcopy multiplexing (stream 1 = stdout, stream 2 = stderr).
//
// A read never blocks past the deadline: if the invocation stalls or the
// workload's lifetime ends before anything is published, the reader gets
// EOF instead of hanging forever. The deadline starts at the first Read,
// which for an attach-then-start client is before the workload has even
// bootstrapped, so callers size it as bootstrap budget plus run budget.
type BufferedAttachStream struct {
	pipe     *StdinPipe
	deadline time.Duration
	onClose  func()

	respMu    sync.Mutex
	respBuf   bytes.Buffer
	respDone  bool
	respReady chan struct{}
}

// NewBufferedAttachStream wires a stream to the container's stdin pipe.
// onClose, when non-nil, runs once when the caller closes the stream —
// backends use it to drop the stream from their per-container registry.
func NewBufferedAttachStream(pipe *StdinPipe, deadline time.Duration, onClose func()) *BufferedAttachStream {
	return &BufferedAttachStream{
		pipe:      pipe,
		deadline:  deadline,
		onClose:   onClose,
		respReady: make(chan struct{}),
	}
}

// Write buffers stdin bytes into the pipe.
func (a *BufferedAttachStream) Write(p []byte) (int, error) {
	return a.pipe.Write(p)
}

// CloseWrite signals stdin EOF, letting a deferred start read the
// buffered script and launch.
func (a *BufferedAttachStream) CloseWrite() error {
	return a.pipe.Close()
}

// Read blocks until the response is published, the stream is closed, or
// the deadline passes, then drains the framed response.
func (a *BufferedAttachStream) Read(p []byte) (int, error) {
	select {
	case <-a.respReady:
	case <-time.After(a.deadline):
		return 0, io.EOF
	}
	a.respMu.Lock()
	defer a.respMu.Unlock()
	if a.respBuf.Len() == 0 {
		return 0, io.EOF
	}
	return a.respBuf.Read(p)
}

// Close releases the stream. Stdin is closed if the caller never
// half-closed it, so a deferred start is not held forever, and any
// blocked reader is released.
func (a *BufferedAttachStream) Close() error {
	_ = a.pipe.Close()
	a.respMu.Lock()
	if !a.respDone {
		a.respDone = true
		close(a.respReady)
	}
	a.respMu.Unlock()
	if a.onClose != nil {
		a.onClose()
	}
	return nil
}

// PublishResponse hands the invocation's output to the reader. Only the
// first publication counts; later ones are ignored.
func (a *BufferedAttachStream) PublishResponse(stdout, stderr []byte) {
	a.respMu.Lock()
	defer a.respMu.Unlock()
	if a.respDone {
		return
	}
	if len(stdout) > 0 {
		WriteMuxFrame(&a.respBuf, 0x01, stdout)
	}
	if len(stderr) > 0 {
		WriteMuxFrame(&a.respBuf, 0x02, stderr)
	}
	a.respDone = true
	close(a.respReady)
}

// WriteMuxFrame writes one Docker stdcopy frame: an 8-byte header of
// [stream_id, 0, 0, 0, big-endian length] followed by the payload.
func WriteMuxFrame(buf *bytes.Buffer, streamID byte, payload []byte) {
	header := make([]byte, 8)
	header[0] = streamID
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	buf.Write(header)
	buf.Write(payload)
}

// PublishBufferedAttachResponse removes the container's stream from the
// registry and publishes the output to it. It is a no-op when no attach
// is registered for the container.
func PublishBufferedAttachResponse(streams *sync.Map, containerID string, stdout, stderr []byte) {
	if v, ok := streams.LoadAndDelete(containerID); ok {
		if as, isStream := v.(*BufferedAttachStream); isStream {
			as.PublishResponse(stdout, stderr)
		}
	}
}

// CaptureBufferedStdin removes the container's stdin pipe from the
// registry and returns the script it holds once the attach caller has
// signalled EOF. The second result is false when no pipe was registered,
// so the caller runs the container's original command. A caller that
// never half-closes stdin is given `timeout` to do so, after which the
// bytes buffered so far are used; a cancelled ctx returns (nil, true) so
// the caller sees the shutdown rather than launching.
func CaptureBufferedStdin(ctx context.Context, pipes *sync.Map, containerID string, timeout time.Duration, logger zerolog.Logger) ([]byte, bool) {
	v, ok := pipes.LoadAndDelete(containerID)
	if !ok {
		return nil, false
	}
	pipe, isPipe := v.(*StdinPipe)
	if !isPipe {
		return nil, false
	}
	select {
	case <-pipe.Done():
	case <-time.After(timeout):
		logger.Warn().Str("container", containerID).Msg("stdin pipe reached the EOF timeout; proceeding with the bytes captured so far")
	case <-ctx.Done():
		return nil, true
	}
	return pipe.Bytes(), true
}
