package aca

import (
	"context"
	"fmt"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// waitForReverseAgentAfterStart blocks until an ACA App bootstrap dials
// back and registers a reverse-agent for `id`, or
// `SOCKERLESS_ACA_BOOTSTRAP_TIMEOUT_SEC` (default 90s) elapses. A Job
// execution waits through waitForJobAgentOrExit, which also accepts the
// execution's end. OpenStdin containers are runner/script entrypoints
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
		return s.reverseAgentAbsentError(id, timeout)
	}
	return nil
}

// waitForJobAgentOrExit blocks after an ACA Job execution starts until its
// bootstrap has dialled back and registered the reverse-agent for `id`, or
// the execution has run to its end — a one-shot command can finish before
// its agent registers, and a finished job needs none — or the bootstrap
// timeout elapses. Docker's start returns with the container's process
// running and ready for exec; the job's process is its bootstrap, which is
// running once it has dialled back.
func (s *Server) waitForJobAgentOrExit(id string, exited <-chan struct{}) error {
	timeout, err := core.BootstrapTimeoutFromEnv("aca")
	if err != nil {
		return &api.ServerError{Message: fmt.Sprintf("invalid bootstrap-timeout env: %v", err)}
	}
	waitCtx, cancel := context.WithTimeout(s.ctx(), timeout)
	defer cancel()
	agent := make(chan error, 1)
	go func() { agent <- s.reverseAgents.WaitForAgent(waitCtx, id) }()
	select {
	case <-exited:
		return nil
	case werr := <-agent:
		if werr != nil {
			select {
			case <-exited:
				return nil
			default:
			}
			return s.reverseAgentAbsentError(id, timeout)
		}
		return nil
	}
}

func (s *Server) reverseAgentAbsentError(id string, timeout time.Duration) error {
	return &api.ServerError{Message: fmt.Sprintf(
		"reverse-agent did not register for container %s within %s "+
			"(SOCKERLESS_ACA_BOOTSTRAP_TIMEOUT_SEC). The App / Job was created and "+
			"started but the in-container bootstrap never dialled back to "+
			"SOCKERLESS_CALLBACK_URL=%s. Check egress / VNet integration / NSG.",
		id[:12], timeout, s.config.CallbackURL,
	)}
}
