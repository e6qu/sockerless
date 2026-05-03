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
	gcfState, ok := s.resolveGCFFromCloud(s.ctx(), exec.ContainerID)
	if !ok || gcfState.FunctionURL == "" {
		return nil, &api.NotImplementedError{Message: "docker exec via Path B requires a Cloud Run Function URL; container has no Function URL yet"}
	}

	argv := append([]string{exec.ProcessConfig.Entrypoint}, exec.ProcessConfig.Arguments...)
	if len(argv) == 0 || argv[0] == "" {
		return nil, &api.InvalidParameterError{Message: "exec command is empty"}
	}

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

	res, err := gcpcommon.PostExecEnvelope(s.ctx(), client, gcfState.FunctionURL, "", envelope)
	if err != nil {
		s.Store.Execs.Update(execID, func(e *api.ExecInstance) {
			e.Running = false
			e.ExitCode = 1
			e.CanRemove = true
		})
		return nil, fmt.Errorf("post exec envelope: %w", err)
	}

	s.Store.Execs.Update(execID, func(e *api.ExecInstance) {
		e.Running = false
		e.ExitCode = res.ExitCode
		e.CanRemove = true
	})

	combined := append(res.Stdout, res.Stderr...)
	return readOnlyRWC(combined), nil
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
