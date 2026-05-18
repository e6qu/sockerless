package aca

import (
	"context"
	"fmt"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// waitForReverseAgentAfterStart blocks until an ACA App bootstrap dials
// back and registers a reverse-agent for `id`, or
// `SOCKERLESS_ACA_BOOTSTRAP_TIMEOUT_SEC` (default 90s) elapses. ACA
// Jobs still run as native one-shot job executions and do not call this;
// exec/archive APIs separately require a registered agent and fail loud
// when one is absent. OpenStdin containers are runner/script entrypoints
// whose start path must not block before attach provides stdin.
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
