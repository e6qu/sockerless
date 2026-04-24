package ecs

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// RunCommandViaSSM opens an ECS ExecuteCommand session against the
// given task, runs the shell command, and returns its stdout, stderr,
// and exit code. Mirrors the shape of
// core.ReverseAgentRegistry.RunAndCapture but tunnels through SSM
// instead of the reverse-agent WebSocket.
//
// The simulator wraps incoming commands in `sh -c`, so callers should
// pass a single shell-quoted command string (or rely on `sh -c` to
// parse argv-style with `--`).
func (s *Server) RunCommandViaSSM(taskARN, cmd string, stdin []byte) (stdout, stderr []byte, exitCode int, err error) {
	cluster := s.config.Cluster
	if taskARN == "" {
		return nil, nil, -1, fmt.Errorf("RunCommandViaSSM: empty task ARN")
	}

	result, ierr := s.aws.ECS.ExecuteCommand(s.ctx(), &awsecs.ExecuteCommandInput{
		Cluster:     aws.String(cluster),
		Task:        aws.String(taskARN),
		Command:     aws.String(cmd),
		Interactive: true,
	})
	if ierr != nil {
		return nil, nil, -1, fmt.Errorf("ECS ExecuteCommand: %w", ierr)
	}
	if result.Session == nil || result.Session.StreamUrl == nil {
		return nil, nil, -1, fmt.Errorf("ECS ExecuteCommand returned no session")
	}

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, derr := dialer.DialContext(s.ctx(), aws.ToString(result.Session.StreamUrl), nil)
	if derr != nil {
		return nil, nil, -1, fmt.Errorf("dial SSM WebSocket: %w", derr)
	}
	defer conn.Close() //nolint:errcheck

	tokenValue := aws.ToString(result.Session.TokenValue)
	handshake, _ := json.Marshal(map[string]string{
		"MessageSchemaVersion": "1.0",
		"RequestId":            uuid.New().String(),
		"TokenValue":           tokenValue,
	})
	if werr := conn.WriteMessage(websocket.TextMessage, handshake); werr != nil {
		return nil, nil, -1, fmt.Errorf("send SSM OpenDataChannel handshake: %w", werr)
	}

	bridge := newWSBridge(conn)
	defer bridge.Close() //nolint:errcheck

	// Send stdin (if any) as input_stream_data frames before reading.
	if len(stdin) > 0 {
		input, ierr := buildSSMInput(stdin)
		if ierr == nil {
			_, _ = bridge.Write(input)
		}
	}

	exitCode = -1
	var stdoutBuf, stderrBuf bytes.Buffer

	for {
		hdr := make([]byte, ssmFixedHeaderLen)
		if _, rerr := io.ReadFull(bridge, hdr); rerr != nil {
			if rerr == io.EOF {
				break
			}
			return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, fmt.Errorf("read SSM header: %w", rerr)
		}
		payloadLen := binary.BigEndian.Uint32(hdr[116:120])
		raw := hdr
		if payloadLen > 0 {
			body := make([]byte, payloadLen)
			if _, rerr := io.ReadFull(bridge, body); rerr != nil {
				return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, fmt.Errorf("read SSM body: %w", rerr)
			}
			raw = append(hdr, body...)
		}
		f, perr := parseSSMFrame(raw)
		if perr != nil {
			return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, fmt.Errorf("parse SSM frame: %w", perr)
		}

		switch f.MessageType {
		case ssmMTOutputStreamData:
			if f.PayloadType == ssmPayloadExitCode {
				if code, cerr := strconv.Atoi(string(bytes.TrimSpace(f.Payload))); cerr == nil {
					exitCode = code
				}
			} else if streamID, ok := ssmTextStreamID(f); ok {
				if streamID == 2 {
					stderrBuf.Write(f.Payload)
				} else {
					stdoutBuf.Write(f.Payload)
				}
			}
			if ack, aerr := buildSSMAck(f); aerr == nil {
				_, _ = bridge.Write(ack)
			}
		case ssmMTChannelClosed:
			return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, nil
		}
	}
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, nil
}
