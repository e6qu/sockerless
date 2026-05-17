package aca

import (
	"context"
	"fmt"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// waitForReverseAgentAfterStart blocks until the bootstrap dials back
// and registers a reverse-agent for `id`, or
// `SOCKERLESS_ACA_BOOTSTRAP_TIMEOUT_SEC` (default 90s) elapses.
// Returns a typed ServerError on timeout. Skips for OpenStdin
// one-shot starts. Per the no-fallback rule there is no exec path
// that doesn't require an agent.
func (s *Server) waitForReverseAgentAfterStart(id string, openStdin bool) error {
	if openStdin {
		return nil
	}
	timeout, err := core.BootstrapTimeoutFromEnv("aca")
	if err != nil {
		return &api.ServerError{Message: fmt.Sprintf("invalid bootstrap-timeout env: %v", err)}
	}
	waitCtx, cancel := context.WithTimeout(s.ctx(), timeout)
	defer cancel()
	if werr := s.reverseAgents.WaitForAgent(waitCtx, id); werr != nil {
		return &api.ServerError{Message: fmt.Sprintf(
			"reverse-agent did not register for container %s within %s "+
				"(SOCKERLESS_ACA_BOOTSTRAP_TIMEOUT_SEC). The App / Job was created and "+
				"started but the in-container bootstrap never dialled back to "+
				"SOCKERLESS_CALLBACK_URL=%s. Check egress / VNet integration / NSG.",
			id[:12], timeout, s.config.CallbackURL,
		)}
	}
	return nil
}
