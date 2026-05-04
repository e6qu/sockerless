package core

import (
	"context"
	"io"

	"github.com/sockerless/api"
)

// AttachViaCloudLogs returns a Docker-compatible io.ReadWriteCloser
// that streams the container's log output as if attached. Used by
// cloud backends that have no native bidirectional attach API but
// can serve a follow-mode log stream from their cloud-log API. Reads
// produce stdout/stderr bytes (mux-framed for non-TTY containers,
// raw for TTY); writes are discarded since cloud containers expose
// no remote stdin channel.
//
// The fetch closure is the same one the backend's ContainerLogs uses;
// see backends/ecs/attach.go for the ECS implementation that this
// helper generalises.
func AttachViaCloudLogs(s *BaseServer, ref string, opts api.ContainerAttachOptions, fetch CloudLogFetchFunc) (io.ReadWriteCloser, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	logOpts := api.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Stream,
	}
	// CheckLogBuffers: true mirrors the Logs driver — for FaaS backends
	// the container stdout returns in the HTTP invoke response body and
	// is stored in `Store.LogBuffers` synchronously. Cloud Logging is the
	// secondary path (with ingestion lag); LogBuffers is the authoritative
	// per-invocation source. Without checking it here, attach silently
	// loses the user output for fast-exit functions whose stdout comes
	// back in the response BEFORE Cloud Logging has indexed it.
	logReader, err := StreamCloudLogs(s, ref, logOpts, fetch, StreamCloudLogsOptions{
		AllowCreated:    true,
		CheckLogBuffers: true,
	})
	if err != nil {
		return nil, err
	}

	rwc := &cloudAttachStream{reader: logReader}
	if c.Config.Tty {
		return rwc, nil
	}
	return &cloudAttachMux{rwc: rwc}, nil
}

// cloudAttachStream adapts an io.ReadCloser (the cloud-log follower)
// to an io.ReadWriteCloser by discarding writes — cloud containers
// have no remote stdin channel.
type cloudAttachStream struct {
	reader io.ReadCloser
}

func (a *cloudAttachStream) Read(p []byte) (int, error)  { return a.reader.Read(p) }
func (a *cloudAttachStream) Write(p []byte) (int, error) { return len(p), nil }
func (a *cloudAttachStream) Close() error                { return a.reader.Close() }

// cloudAttachMux wraps a stream with Docker's multiplexed framing
// (8-byte header per chunk: stream id + length). Cloud log streams
// emit interleaved stdout/stderr already merged, so we tag every
// chunk as stdout (id=1) — matches what `docker attach` does for a
// container started without -it.
type cloudAttachMux struct {
	rwc io.ReadWriteCloser
	buf []byte
}

func (m *cloudAttachMux) Read(p []byte) (int, error) {
	if len(m.buf) > 0 {
		n := copy(p, m.buf)
		m.buf = m.buf[n:]
		return n, nil
	}
	raw := make([]byte, 4096)
	n, err := m.rwc.Read(raw)
	if n > 0 {
		header := []byte{0x01, 0, 0, 0, byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
		m.buf = append(header, raw[:n]...)
		c := copy(p, m.buf)
		m.buf = m.buf[c:]
		return c, nil
	}
	return 0, err
}

func (m *cloudAttachMux) Write(p []byte) (int, error) { return m.rwc.Write(p) }
func (m *cloudAttachMux) Close() error                { return m.rwc.Close() }
