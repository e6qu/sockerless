package aca

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
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
}

func (s *Server) newAttachStream(containerID string, pipe *stdinPipe) *attachStream {
	a := &attachStream{
		server:      s,
		containerID: containerID,
		pipe:        pipe,
		respReady:   make(chan struct{}),
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
	<-a.respReady
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
