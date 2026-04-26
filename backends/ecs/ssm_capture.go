package ecs

import (
	"bytes"
	"context"
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

// waitForExecuteCommandAgentReady polls DescribeTasks until the task's
// ExecuteCommandAgent reports lastStatus=RUNNING. Returns nil when the
// agent is RUNNING (or the cloud doesn't report managed agents — this
// is the simulator path, where exec works as soon as the task does).
// Returns an error when the timeout elapses without the agent
// transitioning. Polls every 2 s.
func (s *Server) waitForExecuteCommandAgentReady(ctx context.Context, cluster, taskARN string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		out, err := s.aws.ECS.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
			Cluster: aws.String(cluster),
			Tasks:   []string{taskARN},
		})
		if err == nil && len(out.Tasks) > 0 {
			task := out.Tasks[0]
			seen := false
			for _, c := range task.Containers {
				for _, agent := range c.ManagedAgents {
					if string(agent.Name) != "ExecuteCommandAgent" {
						continue
					}
					seen = true
					if agent.LastStatus != nil && *agent.LastStatus == "RUNNING" {
						return nil
					}
					if agent.LastStatus != nil && (*agent.LastStatus == "STOPPED" || *agent.LastStatus == "STOPPING") {
						return fmt.Errorf("ExecuteCommandAgent %s on task %s — sockerless cannot exec into this task", *agent.LastStatus, taskARN)
					}
				}
			}
			if !seen && s.config.EndpointURL != "" {
				// Simulator path: doesn't populate managedAgents; exec
				// is wired directly. Treat as ready.
				return nil
			}
			// Real AWS but no managed-agent entries yet — task may
			// still be PROVISIONING. Keep polling.
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for ExecuteCommandAgent on task %s to reach RUNNING", taskARN)
		case <-ticker.C:
		}
	}
}

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

	// ECS ExecuteCommand will accept the call as soon as the task is
	// RUNNING, but the session won't actually carry traffic until the
	// platform's ExecuteCommand managed agent transitions to RUNNING
	// itself — which lags task-RUNNING by 5-30 s on Fargate. Without
	// this poll the command starts, the exec channel returns -1
	// immediately, and the caller sees a misleading "find failed
	// (exit -1)" instead of "agent not ready". 60 s upper bound; on
	// the simulator (no managed agents reported) we skip the wait.
	if waitErr := s.waitForExecuteCommandAgentReady(s.ctx(), cluster, taskARN, 60*time.Second); waitErr != nil {
		return nil, nil, -1, fmt.Errorf("ECS ExecuteCommand prerequisites: %w", waitErr)
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

	cap := openSSMCapture(taskARN, cmd)
	defer cap.Close()

	for {
		hdr := make([]byte, ssmFixedHeaderLen)
		if _, rerr := io.ReadFull(bridge, hdr); rerr != nil {
			if rerr == io.EOF {
				cap.note("EOF before channel_closed; exitCode=%d stdoutLen=%d stderrLen=%d", exitCode, stdoutBuf.Len(), stderrBuf.Len())
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
			cap.frame("PARSE-FAIL", raw, fmt.Sprintf("err=%v", perr))
			return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, fmt.Errorf("parse SSM frame: %w", perr)
		}
		cap.frame(f.MessageType, raw, fmt.Sprintf("payloadType=%d seq=%d", f.PayloadType, f.SequenceNumber))

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
			cap.note("channel_closed; exitCode=%d stdoutLen=%d stderrLen=%d", exitCode, stdoutBuf.Len(), stderrBuf.Len())
			// AWS ECS ExecuteCommand (Interactive=true) closes the
			// session with channel_closed but does NOT send a separate
			// output_stream_data frame with payloadType=exit_code for
			// short-lived one-shot commands — confirmed against live
			// Fargate by capturing every WS frame. Returning -1 here is
			// correct for the raw transport; one-shot callers above
			// (`runViaSSMOrNotImpl`) wrap commands in an exit-marker
			// printf so they recover the real exit code from stdout.
			// `cloudExecStart` (docker exec) doesn't need a fabricated
			// exit code — docker reads it via ExecInspect afterwards.
			return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, nil
		}
	}
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, nil
}
