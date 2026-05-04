package cloudrun

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	gcpcommon "github.com/sockerless/gcp-common"
	"google.golang.org/api/idtoken"
)

// execStartViaInvoke implements Path B from
// specs/CLOUD_RESOURCE_MAPPING.md § Lesson 8 — `docker exec` against a
// long-lived Cloud Run Service backed by sockerless-cloudrun-bootstrap.
// Builds an execEnvelope, POSTs to the Service URL with a fresh ID
// token (Cloud Run requires authenticated invokes by default), parses
// the response, and exposes stdout as an io.ReadWriteCloser the docker
// exec attach handler expects.
//
// Reverse-agent WS still required for interactive TTY+stdin (HTTP
// req/resp can't stream); ExecStart routes there when opts.Tty &&
// opts.Stdin. This Path B handler covers the common non-interactive
// case (gitlab-runner / github-runner per-stage scripts).
func (s *Server) execStartViaInvoke(execID string, exec api.ExecInstance, _ api.ExecStartRequest) (io.ReadWriteCloser, error) {
	// Resolve the Service URL via cloud lookup. Returns ("", false) for
	// containers still on the Job path — caller falls through to the
	// reverse-agent / NotImpl shape.
	url, ok := s.serviceInvokeURL(s.ctx(), exec.ContainerID)
	if !ok || url == "" {
		return nil, &api.NotImplementedError{Message: "docker exec via Path B requires a Cloud Run Service URL; container is on the Jobs path or Service is not yet ready"}
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

	// idtoken.NewClient mints + auto-attaches a Google ID token whose
	// audience is the Service URL. Cloud Run rejects unauthenticated
	// invokes by default; without this Authorization header the POST
	// returns 401/403 and exec fails before the bootstrap sees it.
	// Service-account ADC required (user creds can't sign ID tokens —
	// same constraint as gcf invokeFunction).
	client, err := idtoken.NewClient(s.ctx(), url)
	if err != nil {
		return nil, fmt.Errorf("idtoken.NewClient(%s): %w", url, err)
	}
	client.Timeout = 10 * time.Minute

	res, err := gcpcommon.PostExecEnvelope(s.ctx(), client, url, "", envelope)
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

	// stderr appended after stdout — same shape as lambda's
	// execStartViaInvoke (matches what reverse-agent path returns for
	// non-tty execs: a single concatenated byte stream).
	combined := append(res.Stdout, res.Stderr...)
	return readOnlyRWC(combined), nil
}

// serviceInvokeURL returns the Cloud Run Service.Uri for a container,
// looked up via CloudState. Empty + false when the container is on the
// Jobs path (no Service exists) or the Service is still provisioning
// (Uri unset).
func (s *Server) serviceInvokeURL(ctx context.Context, containerID string) (string, bool) {
	state, ok := s.resolveServiceCloudRunState(ctx, containerID)
	if !ok || state.ServiceName == "" {
		return "", false
	}
	if s.gcp == nil || s.gcp.Services == nil {
		return "", false
	}
	svc, err := s.gcp.Services.GetService(ctx, &runpb.GetServiceRequest{Name: state.ServiceName})
	if err != nil || svc.Uri == "" {
		return "", false
	}
	return svc.Uri, true
}

// readOnlyRWC wraps a byte slice as an io.ReadWriteCloser; writes
// (stdin) are silently dropped because the exec already completed
// when the bootstrap returned.
type readOnlyBytesRWC struct {
	r io.Reader
}

func readOnlyRWC(b []byte) io.ReadWriteCloser {
	return &readOnlyBytesRWC{r: bytes.NewReader(b)}
}

func (r *readOnlyBytesRWC) Read(p []byte) (int, error)  { return r.r.Read(p) }
func (r *readOnlyBytesRWC) Write(p []byte) (int, error) { return len(p), nil }
func (r *readOnlyBytesRWC) Close() error                { return nil }
