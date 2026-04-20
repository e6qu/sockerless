package ecs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/gorilla/websocket"
	"github.com/sockerless/api"
)

// cloudExecStart executes a command inside an ECS task using the
// ExecuteCommand API (backed by SSM Session Manager). It returns an
// io.ReadWriteCloser that bridges the SSM WebSocket session.
func (s *Server) cloudExecStart(exec *api.ExecInstance, c *api.Container, tty bool) (io.ReadWriteCloser, error) {
	ecsState, ok := s.ECS.Get(c.ID)
	if !ok || ecsState.TaskARN == "" {
		return nil, fmt.Errorf("no ECS task associated with container %s", c.ID[:12])
	}

	cluster := s.config.Cluster
	if ecsState.ClusterARN != "" {
		cluster = ecsState.ClusterARN
	}

	// Build the full command string from the exec process config.
	// ECS ExecuteCommand takes a single command string that the simulator
	// wraps in sh -c. We must produce a valid shell command.
	var envPrefix string
	for _, e := range exec.ProcessConfig.Env {
		envPrefix += fmt.Sprintf("export %s; ", e)
	}

	// Reconstruct the command preserving sh -c script quoting.
	// Input: Entrypoint="sh", Arguments=["-c", "echo $VAR"]
	// Must produce: "export VAR=val; echo $VAR" (unwrap sh -c since simulator wraps again)
	entrypoint := exec.ProcessConfig.Entrypoint
	args := exec.ProcessConfig.Arguments

	// Add working directory change if specified
	workDir := exec.ProcessConfig.WorkingDir
	if workDir == "" {
		workDir = c.Config.WorkingDir
	}
	var cdPrefix string
	if workDir != "" {
		cdPrefix = fmt.Sprintf("cd %s && ", workDir)
	}

	var cmd string
	if (entrypoint == "sh" || entrypoint == "/bin/sh" || entrypoint == "bash" || entrypoint == "/bin/bash") && len(args) >= 2 && args[0] == "-c" {
		// sh -c "script" — extract the script and prepend env vars directly
		// The simulator will wrap the final command in sh -c, so we just send the script
		cmd = cdPrefix + envPrefix + strings.Join(args[1:], " ")
	} else {
		// Regular command — join all parts
		parts := []string{}
		if entrypoint != "" {
			parts = append(parts, entrypoint)
		}
		parts = append(parts, args...)
		cmd = cdPrefix + envPrefix + strings.Join(parts, " ")
	}

	result, err := s.aws.ECS.ExecuteCommand(s.ctx(), &awsecs.ExecuteCommandInput{
		Cluster:     aws.String(cluster),
		Task:        aws.String(ecsState.TaskARN),
		Command:     aws.String(cmd),
		Interactive: true,
	})
	if err != nil {
		return nil, fmt.Errorf("ECS ExecuteCommand failed: %w", err)
	}

	if result.Session == nil || result.Session.StreamUrl == nil {
		return nil, fmt.Errorf("ECS ExecuteCommand returned no session")
	}

	streamURL := aws.ToString(result.Session.StreamUrl)
	s.Logger.Debug().
		Str("container", c.ID[:12]).
		Str("stream_url", streamURL).
		Msg("connecting to ECS exec session")

	// Dial the SSM Session Manager WebSocket endpoint.
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(s.ctx(), streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to exec session WebSocket: %w", err)
	}

	// Set a close handler so that when the server closes the connection,
	// the next Read() call returns promptly.
	conn.SetCloseHandler(func(code int, text string) error {
		_ = conn.SetReadDeadline(time.Now())
		return nil
	})

	bridge := newWSBridge(conn)

	// SSM Session Manager wraps the application stream in a binary
	// AgentMessage protocol (see ssm_proto.go + BUG-717). Decode SSM frames,
	// send acks back through the WebSocket, and surface the inner
	// stdout/stderr to the Docker client. For non-TTY exec we additionally
	// wrap the extracted bytes in Docker's 8-byte multiplexed stream
	// headers since `docker exec` expects that framing.
	dec := newSSMDecoder(bridge)
	if !tty {
		return &muxBridge{rwc: dec}, nil
	}
	return dec, nil
}

// muxBridge wraps an io.ReadWriteCloser and adds Docker multiplexed stream
// headers to each read. The stream id (1=stdout, 2=stderr) is taken from
// the underlying reader if it implements `lastStream()`; otherwise stdout.
type muxBridge struct {
	rwc io.ReadWriteCloser
	buf bytes.Buffer
}

type streamTagger interface {
	lastStream() byte
}

func (m *muxBridge) Read(p []byte) (int, error) {
	if m.buf.Len() > 0 {
		return m.buf.Read(p)
	}
	raw := make([]byte, 4096)
	n, err := m.rwc.Read(raw)
	if n > 0 {
		stream := byte(0x01)
		if t, ok := m.rwc.(streamTagger); ok {
			if s := t.lastStream(); s != 0 {
				stream = s
			}
		}
		header := [8]byte{stream, 0, 0, 0, byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
		m.buf.Write(header[:])
		m.buf.Write(raw[:n])
		return m.buf.Read(p)
	}
	return 0, err
}

func (m *muxBridge) Write(p []byte) (int, error) { return m.rwc.Write(p) }
func (m *muxBridge) Close() error                { return m.rwc.Close() }

// ssmDecoder reads SSM AgentMessage frames from an underlying WebSocket
// bridge, replies with acknowledgements, and presents the decoded
// stdout/stderr text as a plain io.Reader. Writes (stdin) are wrapped in
// `input_stream_data` frames and sent through the WebSocket.
type ssmDecoder struct {
	wire     io.ReadWriteCloser
	pending  bytes.Buffer // decoded text not yet returned to caller
	lastTag  byte         // 0x01 stdout / 0x02 stderr from last frame
	closeErr error
}

func newSSMDecoder(wire io.ReadWriteCloser) *ssmDecoder {
	return &ssmDecoder{wire: wire}
}

func (d *ssmDecoder) lastStream() byte { return d.lastTag }

func (d *ssmDecoder) Read(p []byte) (int, error) {
	for d.pending.Len() == 0 {
		if d.closeErr != nil {
			return 0, d.closeErr
		}
		// Read exactly one SSM frame: fixed header first, then payload.
		hdr := make([]byte, ssmFixedHeaderLen)
		if _, err := io.ReadFull(d.wire, hdr); err != nil {
			d.closeErr = err
			continue
		}
		payloadLen := binary.BigEndian.Uint32(hdr[116:120])
		var raw []byte
		if payloadLen > 0 {
			body := make([]byte, payloadLen)
			if _, err := io.ReadFull(d.wire, body); err != nil {
				d.closeErr = err
				continue
			}
			raw = append(hdr, body...)
		} else {
			raw = hdr
		}
		f, perr := parseSSMFrame(raw)
		if perr != nil {
			d.closeErr = perr
			continue
		}
		switch f.MessageType {
		case ssmMTOutputStreamData:
			if streamID, ok := ssmTextStreamID(f); ok {
				d.lastTag = streamID
				d.pending.Write(f.Payload)
			}
			if f.PayloadType == ssmPayloadExitCode {
				d.closeErr = io.EOF
			}
			if ack, aerr := buildSSMAck(f); aerr == nil {
				_, _ = d.wire.Write(ack)
			}
		case ssmMTChannelClosed:
			d.closeErr = io.EOF
		case ssmMTAcknowledge, ssmMTStartPublication, ssmMTPausePublication:
			// flow-control / handshake — nothing to surface
		}
	}
	return d.pending.Read(p)
}

func (d *ssmDecoder) Write(p []byte) (int, error) {
	// stdin wrapping: input_stream_data with PayloadType=1 (raw bytes).
	out, err := buildSSMInput(p)
	if err != nil {
		return 0, err
	}
	if _, err := d.wire.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (d *ssmDecoder) Close() error { return d.wire.Close() }

// wsBridge adapts a gorilla/websocket.Conn to io.ReadWriteCloser.
type wsBridge struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	reader io.Reader // current message reader
}

func newWSBridge(conn *websocket.Conn) *wsBridge {
	return &wsBridge{conn: conn}
}

// Read reads from the WebSocket, consuming binary/text messages sequentially.
func (w *wsBridge) Read(p []byte) (int, error) {
	for {
		if w.reader != nil {
			n, err := w.reader.Read(p)
			if err == io.EOF {
				w.reader = nil
				if n > 0 {
					return n, nil
				}
				continue
			}
			return n, err
		}

		_, r, err := w.conn.NextReader()
		if err != nil {
			return 0, err
		}
		w.reader = r
	}
}

// Write sends a binary message on the WebSocket.
func (w *wsBridge) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close closes the underlying WebSocket connection.
func (w *wsBridge) Close() error {
	return w.conn.Close()
}

// LocalAddr satisfies net.Conn if needed (returns nil).
func (w *wsBridge) LocalAddr() net.Addr  { return nil }
func (w *wsBridge) RemoteAddr() net.Addr { return nil }
