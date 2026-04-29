package lambda

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/rs/zerolog"

	"github.com/sockerless/api"
)

// execEnvelopeRequest is the JSON shape sent to the in-Lambda bootstrap
// when sockerless dispatches `docker exec` via `lambda.Invoke`. Mirrors
// `agent/cmd/sockerless-lambda-bootstrap/main.go::execEnvelope`.
type execEnvelopeRequest struct {
	Sockerless struct {
		Exec execEnvelopeExec `json:"exec"`
	} `json:"sockerless"`
}

type execEnvelopeExec struct {
	Argv    []string `json:"argv"`
	Tty     bool     `json:"tty,omitempty"`
	Workdir string   `json:"workdir,omitempty"`
	Env     []string `json:"env,omitempty"`
	Stdin   string   `json:"stdin,omitempty"`
}

// execEnvelopeResponse is the JSON shape the bootstrap returns.
type execEnvelopeResponse struct {
	SockerlessExecResult struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	} `json:"sockerlessExecResult"`
}

// execStartViaInvoke implements Path B from
// `specs/CLOUD_RESOURCE_MAPPING.md` § Lambda exec semantics. Each
// `docker exec` triggers a fresh `lambda.Invoke`; the bootstrap parses
// the payload as an execEnvelope, runs the command, returns stdout +
// exit-code in the response. The lambda backend tunnels stdout back
// through the docker-exec hijacked connection.
//
// Used when sockerless's lambda backend is in-Lambda (no inbound
// network for the reverse-agent path) and the operator hasn't wired
// SOCKERLESS_CALLBACK_URL.
func (s *Server) execStartViaInvoke(execID string, exec api.ExecInstance, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	state, ok := s.resolveLambdaState(s.ctx(), exec.ContainerID)
	if !ok || state.FunctionName == "" {
		return nil, &api.NotFoundError{Resource: "container", ID: exec.ContainerID}
	}

	argv := append([]string{exec.ProcessConfig.Entrypoint}, exec.ProcessConfig.Arguments...)
	if len(argv) == 0 || argv[0] == "" {
		return nil, &api.InvalidParameterError{Message: "exec command is empty"}
	}

	envelope := execEnvelopeRequest{}
	envelope.Sockerless.Exec = execEnvelopeExec{
		Argv:    argv,
		Tty:     exec.ProcessConfig.Tty || opts.Tty,
		Workdir: exec.ProcessConfig.WorkingDir,
		Env:     exec.ProcessConfig.Env,
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal exec envelope: %w", err)
	}

	result, err := s.aws.Lambda.Invoke(s.ctx(), &awslambda.InvokeInput{
		FunctionName: aws.String(state.FunctionName),
		Payload:      payload,
	})
	if err != nil {
		return nil, fmt.Errorf("lambda invoke for exec: %w", err)
	}
	if result.FunctionError != nil {
		s.Store.Execs.Update(execID, func(e *api.ExecInstance) {
			e.Running = false
			e.ExitCode = 1
			e.CanRemove = true
		})
		return nil, fmt.Errorf("lambda function error during exec (%s): %s", aws.ToString(result.FunctionError), string(result.Payload))
	}

	var res execEnvelopeResponse
	if err := json.Unmarshal(result.Payload, &res); err != nil {
		s.Store.Execs.Update(execID, func(e *api.ExecInstance) {
			e.Running = false
			e.ExitCode = 1
			e.CanRemove = true
		})
		return nil, fmt.Errorf("bootstrap exec response not in expected envelope format: %w (raw=%q)", err, truncate(result.Payload, 256))
	}

	stdoutBytes, _ := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stdout)
	stderrBytes, _ := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stderr)

	s.Store.Execs.Update(execID, func(e *api.ExecInstance) {
		e.Running = false
		e.ExitCode = res.SockerlessExecResult.ExitCode
		e.CanRemove = true
	})

	// stderr appended after stdout — the docker exec attach reader
	// receives a single byte stream; clients muxing via the stdcopy
	// header expect framed bytes, but unframed concatenation matches
	// what the existing reverse-agent path hands back for non-tty execs.
	combined := append(stdoutBytes, stderrBytes...)
	return readOnlyRWC(combined), nil
}

// truncate returns at most n bytes of b. Used to bound diagnostics so
// a multi-megabyte error payload doesn't drown the log.
func truncate(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

// lambdaInvokeExecDriver implements core.LegacyExecDriver against
// lambda.Invoke per Path B (`specs/CLOUD_RESOURCE_MAPPING.md` §
// "Lambda exec semantics"). Wired in `server.go` when no
// SOCKERLESS_CALLBACK_URL is configured (the in-Lambda host can't
// expose the reverse-agent listener for sub-task dial-back).
//
// `conn` is the hijacked docker-exec HTTP connection. For non-tty
// execs the connection expects 8-byte-prefixed multiplexed frames
// (stream type + length); for tty execs the bytes are written
// straight through.
type lambdaInvokeExecDriver struct {
	server *Server
	logger zerolog.Logger
}

// Exec implements core.LegacyExecDriver. Marshals an exec envelope,
// invokes the function synchronously, writes the response stdout +
// stderr to `conn` framed for the docker exec hijacked-conn protocol,
// returns the exit code.
//
// Caller (handle_exec.go) does the bookkeeping (Store.Execs.Update on
// Running/ExitCode/Pid) — this driver only delivers output + exit.
func (d *lambdaInvokeExecDriver) Exec(
	ctx context.Context,
	containerID string,
	execID string,
	cmd []string,
	env []string,
	workDir string,
	tty bool,
	conn net.Conn,
) (exitCode int) {
	state, ok := d.server.resolveLambdaState(ctx, containerID)
	if !ok || state.FunctionName == "" {
		writeFramedStderr(conn, []byte("sockerless: lambda function not found for container "+containerID+"\n"), tty)
		return 126
	}

	envelope := execEnvelopeRequest{}
	envelope.Sockerless.Exec = execEnvelopeExec{
		Argv:    cmd,
		Tty:     tty,
		Workdir: workDir,
		Env:     env,
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		writeFramedStderr(conn, []byte("sockerless: marshal exec envelope: "+err.Error()+"\n"), tty)
		return 126
	}

	result, err := d.server.aws.Lambda.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName: aws.String(state.FunctionName),
		Payload:      payload,
	})
	if err != nil {
		d.logger.Error().Err(err).Str("container", containerID).Msg("lambda invoke for exec failed")
		writeFramedStderr(conn, []byte("sockerless: lambda invoke for exec failed: "+err.Error()+"\n"), tty)
		return 1
	}
	if result.FunctionError != nil {
		fe := aws.ToString(result.FunctionError)
		d.logger.Warn().Str("container", containerID).Str("function_error", fe).Bytes("payload", truncate(result.Payload, 512)).Msg("lambda function error during exec")
		writeFramedStderr(conn, []byte("sockerless: lambda function error ("+fe+"): "+string(result.Payload)+"\n"), tty)
		return 1
	}

	var res execEnvelopeResponse
	if err := json.Unmarshal(result.Payload, &res); err != nil {
		d.logger.Warn().Err(err).Bytes("payload", truncate(result.Payload, 512)).Msg("bootstrap exec response not in expected envelope")
		writeFramedStdout(conn, result.Payload, tty)
		return 1
	}

	stdoutBytes, _ := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stdout)
	stderrBytes, _ := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stderr)
	if len(stdoutBytes) > 0 {
		writeFramedStdout(conn, stdoutBytes, tty)
	}
	if len(stderrBytes) > 0 {
		writeFramedStderr(conn, stderrBytes, tty)
	}
	return res.SockerlessExecResult.ExitCode
}

// writeFramedStdout writes data to a docker-exec hijacked conn,
// applying the 8-byte multiplexed-stream prefix when tty=false.
func writeFramedStdout(conn net.Conn, data []byte, tty bool) {
	writeFramed(conn, 1, data, tty)
}

func writeFramedStderr(conn net.Conn, data []byte, tty bool) {
	writeFramed(conn, 2, data, tty)
}

func writeFramed(conn net.Conn, streamType byte, data []byte, tty bool) {
	if tty {
		_, _ = conn.Write(data)
		return
	}
	hdr := make([]byte, 8)
	hdr[0] = streamType
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(data)))
	_, _ = conn.Write(hdr)
	_, _ = conn.Write(data)
}

// readOnlyRWC wraps a byte slice as an io.ReadWriteCloser whose reads
// drain the slice once and EOF; writes are silently discarded (stdin
// already travelled in the Invoke payload at the time of the call, so
// late writes to the exec stream have no destination).
type readOnlyRWCImpl struct {
	r      *bytes.Reader
	closed bool
}

func readOnlyRWC(b []byte) io.ReadWriteCloser {
	return &readOnlyRWCImpl{r: bytes.NewReader(b)}
}

func (r *readOnlyRWCImpl) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.EOF
	}
	return r.r.Read(p)
}

func (r *readOnlyRWCImpl) Write(p []byte) (int, error) { return len(p), nil }

func (r *readOnlyRWCImpl) Close() error {
	r.closed = true
	return nil
}
