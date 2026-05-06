package gcf

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/sockerless/api"
	gcpcommon "github.com/sockerless/gcp-common"
	"google.golang.org/api/idtoken"
)

// execStartViaInvoke implements Path B from
// specs/CLOUD_RESOURCE_MAPPING.md § Lesson 8 — `docker exec` against a
// long-lived Cloud Run Function via HTTP POST. Identical shape to the
// cloudrun + lambda backends so the wire format stays cross-cloud
// consistent. Reserved for non-interactive execs; interactive
// (TTY+stdin) falls through to the reverse-agent WS path.
func (s *Server) execStartViaInvoke(execID string, exec api.ExecInstance) (io.ReadWriteCloser, error) {
	s.Logger.Info().Str("execID", execID).Str("containerID", exec.ContainerID).Msg("execStartViaInvoke: entry")
	gcfState, ok := s.resolveGCFFromCloud(s.ctx(), exec.ContainerID)
	if !ok || gcfState.FunctionURL == "" {
		s.Logger.Warn().Str("execID", execID).Str("containerID", exec.ContainerID).Bool("resolved", ok).Str("url", gcfState.FunctionURL).Msg("execStartViaInvoke: no URL — returning NotImplemented")
		return nil, &api.NotImplementedError{Message: "docker exec via Path B requires a Cloud Run Function URL; container has no Function URL yet"}
	}

	argv := append([]string{exec.ProcessConfig.Entrypoint}, exec.ProcessConfig.Arguments...)
	if len(argv) == 0 || argv[0] == "" {
		return nil, &api.InvalidParameterError{Message: "exec command is empty"}
	}
	s.Logger.Info().Str("execID", execID).Str("url", gcfState.FunctionURL).Strs("argv", argv).Msg("execStartViaInvoke: posting envelope")

	envelope := gcpcommon.ExecEnvelopeExec{
		Argv:    argv,
		Tty:     exec.ProcessConfig.Tty,
		Workdir: exec.ProcessConfig.WorkingDir,
		Env:     exec.ProcessConfig.Env,
	}

	client, err := idtoken.NewClient(s.ctx(), gcfState.FunctionURL)
	if err != nil {
		return nil, fmt.Errorf("idtoken.NewClient(%s): %w", gcfState.FunctionURL, err)
	}
	client.Timeout = 10 * time.Minute

	startedAt := time.Now()
	res, err := gcpcommon.PostExecEnvelope(s.ctx(), client, gcfState.FunctionURL, "", envelope)
	if err != nil {
		s.Logger.Warn().Str("execID", execID).Err(err).Dur("duration", time.Since(startedAt)).Msg("execStartViaInvoke: post exec envelope failed")
		s.Store.Execs.Update(execID, func(e *api.ExecInstance) {
			e.Running = false
			e.ExitCode = 1
			e.CanRemove = true
		})
		return nil, fmt.Errorf("post exec envelope: %w", err)
	}
	s.Logger.Info().Str("execID", execID).Int("exitCode", res.ExitCode).Dur("duration", time.Since(startedAt)).Int("stdout_bytes", len(res.Stdout)).Int("stderr_bytes", len(res.Stderr)).Msg("execStartViaInvoke: envelope returned")

	s.Store.Execs.Update(execID, func(e *api.ExecInstance) {
		e.Running = false
		e.ExitCode = res.ExitCode
		e.CanRemove = true
	})

	// BUG-962: docker exec non-TTY response expects each chunk wrapped
	// in an 8-byte stdcopy stream-frame header (stream_id 0x01=stdout,
	// 0x02=stderr). Returning plain bytes makes the client read the
	// first byte as a header and reject it as "Unrecognized input
	// header: NN". Mirror what publishAttachResponse does.
	var framed bytes.Buffer
	if len(res.Stdout) > 0 {
		writeMuxFrame(&framed, 0x01, res.Stdout)
	}
	if len(res.Stderr) > 0 {
		writeMuxFrame(&framed, 0x02, res.Stderr)
	}
	return readOnlyRWC(framed.Bytes()), nil
}

type readOnlyBytesRWC struct {
	r io.Reader
}

func readOnlyRWC(b []byte) io.ReadWriteCloser {
	return &readOnlyBytesRWC{r: bytes.NewReader(b)}
}

func (r *readOnlyBytesRWC) Read(p []byte) (int, error)  { return r.r.Read(p) }
func (r *readOnlyBytesRWC) Write(p []byte) (int, error) { return len(p), nil }
func (r *readOnlyBytesRWC) Close() error                { return nil }
