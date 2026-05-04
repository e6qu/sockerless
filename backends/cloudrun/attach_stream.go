package cloudrun

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
)

// attachStream is the hijacked io.ReadWriteCloser returned to a
// gitlab-runner-style attach caller. Writes route into the per-
// container stdinPipe (captured + replayed by invokeServiceDefaultCmd
// at deferred-invoke time). Reads block until invokeServiceDefaultCmd
// publishes the bootstrap response (mux-framed stdout + stderr) via
// publishAttachResponse.
type attachStream struct {
	server      *Server
	containerID string
	pipe        *stdinPipe

	respMu    sync.Mutex
	respBuf   bytes.Buffer
	respDone  bool
	respReady chan struct{}
	closed    bool
}

func (s *Server) newAttachStream(containerID string, pipe *stdinPipe) *attachStream {
	a := &attachStream{
		server:      s,
		containerID: containerID,
		pipe:        pipe,
		respReady:   make(chan struct{}),
	}
	// Register so invokeServiceDefaultCmd can call publishAttachResponse
	// when the bootstrap returns. One stream per container at a time —
	// gitlab-runner cycles attach→start→stop per stage, so each new
	// stage gets a fresh attachStream.
	s.attachStreams.Store(containerID, a)
	return a
}

// Write buffers stdin bytes (the caller's per-stage script) into the
// stdinPipe.
func (a *attachStream) Write(p []byte) (int, error) {
	return a.pipe.Write(p)
}

// CloseWrite signals stdin EOF — the deferred invoke can now read
// pipe.Bytes() and POST to the bootstrap.
func (a *attachStream) CloseWrite() error {
	return a.pipe.Close()
}

// Read blocks until publishAttachResponse fires (or the stream is
// closed), then returns mux-framed bytes from respBuf.
func (a *attachStream) Read(p []byte) (int, error) {
	<-a.respReady
	a.respMu.Lock()
	defer a.respMu.Unlock()
	if a.respBuf.Len() == 0 {
		return 0, io.EOF
	}
	return a.respBuf.Read(p)
}

// Close releases the stream. If stdin EOF wasn't signalled by the
// caller, closes the pipe so deferred invoke isn't held forever.
func (a *attachStream) Close() error {
	_ = a.pipe.Close()
	a.respMu.Lock()
	a.closed = true
	if !a.respDone {
		a.respDone = true
		close(a.respReady)
	}
	a.respMu.Unlock()
	a.server.attachStreams.Delete(a.containerID)
	return nil
}

// publishAttachResponse is called by invokeServiceDefaultCmd to surface
// the bootstrap's stdout+stderr to the attach reader. Mux-frames per
// Docker's stdcopy convention (stream-id 0x01 = stdout, 0x02 = stderr).
func (a *attachStream) publishAttachResponse(stdout, stderr []byte) {
	a.respMu.Lock()
	defer a.respMu.Unlock()
	if a.respDone {
		return
	}
	if len(stdout) > 0 {
		writeMuxFrame(&a.respBuf, 0x01, stdout)
	}
	if len(stderr) > 0 {
		writeMuxFrame(&a.respBuf, 0x02, stderr)
	}
	a.respDone = true
	close(a.respReady)
}

// writeMuxFrame writes one Docker stdcopy-framed chunk: 8-byte header
// [stream_id, 0, 0, 0, length_be32] then payload.
func writeMuxFrame(buf *bytes.Buffer, streamID byte, payload []byte) {
	header := make([]byte, 8)
	header[0] = streamID
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	buf.Write(header)
	buf.Write(payload)
}
