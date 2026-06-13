package aca

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
	"time"

	core "github.com/sockerless/backend-core"
)

type attachStream struct {
	server      *Server
	containerID string
	pipe        *stdinPipe

	respMu    sync.Mutex
	respBuf   bytes.Buffer
	respDone  bool
	respReady chan struct{}
	closed    bool
	deadline  time.Duration
}

// attachDeadline bounds the buffered-attach read so a reader is never
// stranded if the ACA Job stalls or hits its replica-timeout before
// publishing output. The read clock starts at ContainerStart, BEFORE the
// container bootstraps, so the budget must cover the bootstrap window PLUS
// the job-run budget — bounding it by the run budget alone would strand the
// reader when a healthy bootstrap is slow on a contended CI runner. This is
// the same safety net AZF carries (the BUG-1505 fix); ACA had a bare,
// unbounded `<-respReady` until now.
func (s *Server) attachDeadline() time.Duration {
	bootstrap, err := core.BootstrapTimeoutFromEnv("aca")
	if err != nil || bootstrap <= 0 {
		bootstrap = 90 * time.Second
	}
	run := time.Duration(core.JobTimeoutDefault()) * time.Second
	if run <= 0 {
		run = 600 * time.Second
	}
	return bootstrap + run
}

func (s *Server) newAttachStream(containerID string, pipe *stdinPipe) *attachStream {
	a := &attachStream{
		server:      s,
		containerID: containerID,
		pipe:        pipe,
		respReady:   make(chan struct{}),
		deadline:    s.attachDeadline(),
	}
	s.attachStreams.Store(containerID, a)
	return a
}

func (a *attachStream) Write(p []byte) (int, error) {
	return a.pipe.Write(p)
}

func (a *attachStream) CloseWrite() error {
	return a.pipe.Close()
}

func (a *attachStream) Read(p []byte) (int, error) {
	// Wait for the invoke to publish captured output — but never past the
	// deadline. A healthy invoke always publishes within its own budget, so
	// this is a pure safety net for a stalled/lifetime-capped pod that would
	// otherwise strand an attached docker/StdCopy reader forever.
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

func writeMuxFrame(buf *bytes.Buffer, streamID byte, payload []byte) {
	header := make([]byte, 8)
	header[0] = streamID
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	buf.Write(header)
	buf.Write(payload)
}
