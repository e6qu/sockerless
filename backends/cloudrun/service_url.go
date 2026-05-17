package cloudrun

import (
	"context"
	"fmt"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// waitForReverseAgentAfterStart blocks until the bootstrap dials back
// and registers a reverse-agent for `id`, or
// `SOCKERLESS_CLOUDRUN_BOOTSTRAP_TIMEOUT_SEC` (default 90s) elapses.
// Returns a typed ServerError on timeout so callers can return it
// directly to the docker client. Skips entirely for OpenStdin
// gitlab-runner one-shot starts — those don't trigger exec calls.
// Per the no-fallback rule there is no exec path that doesn't
// require an agent.
func (s *Server) waitForReverseAgentAfterStart(id string, openStdin bool) error {
	if openStdin {
		return nil
	}
	timeout, err := core.BootstrapTimeoutFromEnv("cloudrun")
	if err != nil {
		return &api.ServerError{Message: fmt.Sprintf("invalid bootstrap-timeout env: %v", err)}
	}
	waitCtx, cancel := context.WithTimeout(s.ctx(), timeout)
	defer cancel()
	if werr := s.reverseAgents.WaitForAgent(waitCtx, id); werr != nil {
		return &api.ServerError{Message: fmt.Sprintf(
			"reverse-agent did not register for container %s within %s "+
				"(SOCKERLESS_CLOUDRUN_BOOTSTRAP_TIMEOUT_SEC). The Service / Job was created and "+
				"invoked but the in-container bootstrap never dialled back to "+
				"SOCKERLESS_CALLBACK_URL=%s. Check egress / VPC connector / firewall.",
			id[:12], timeout, s.config.CallbackURL,
		)}
	}
	return nil
}

// serviceInvokeURL resolves the Cloud Run Service's invoke URL for the
// container. Returns ("", false) when the Service hasn't materialised
// yet (Uri unset) or when the cloud client isn't available. Used by
// the start-service goroutine to know when the Service is reachable
// for downstream operations (e.g. waiting for the bootstrap to dial
// back, materialising peer-pod members).
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
