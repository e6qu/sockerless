package core

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sockerless/api"
)

// Container operations that every FaaS-shaped backend implements the same
// way over the reverse agent or over PendingCreates. Each backend's
// api.Backend method resolves the container and calls one of these.

// RestartCloudContainer implements `docker restart` for a backend whose
// workload is one invocation: a running container is recorded as stopped
// by SIGTERM (exit 143) so a racing `docker wait` sees that rather than
// the channel-close sentinel, its waiters are released, and it is put back
// into PendingCreates for start to launch again.
func (s *BaseServer) RestartCloudContainer(ref string, start func(id string) error) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID
	name := strings.TrimPrefix(c.Name, "/")
	if c.State.Running {
		s.StopHealthCheck(id)
		stopExitCode := SignalToExitCode("SIGTERM")
		s.Store.PutInvocationResult(id, InvocationResult{ExitCode: stopExitCode})
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			if wc, isCh := ch.(chan struct{}); isCh {
				close(wc)
			}
		}
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": fmt.Sprintf("%d", stopExitCode),
			"name":     name,
		})
		s.EmitEvent("container", "stop", id, map[string]string{"name": name})
	}
	s.PendingCreates.Put(id, c)
	if err := start(id); err != nil {
		return err
	}
	s.EmitEvent("container", "restart", id, map[string]string{"name": name})
	return nil
}

// ExportViaReverseAgent implements `docker export` through the container's
// reverse agent. workload names the cloud unit for the error a container
// without a connected agent receives, e.g. "function container".
func (s *BaseServer) ExportViaReverseAgent(reg *ReverseAgentRegistry, ref, workload string) (io.ReadCloser, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	rc, err := RunContainerExportViaAgent(reg, cid)
	if err == ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: fmt.Sprintf("docker export requires a reverse-agent bootstrap inside the %s (SOCKERLESS_CALLBACK_URL); no session registered", workload)}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("export via reverse-agent: %v", err)}
	}
	return rc, nil
}

// PauseViaReverseAgent implements `docker pause` through the container's
// reverse agent.
func (s *BaseServer) PauseViaReverseAgent(reg *ReverseAgentRegistry, ref string) error {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return MapPauseErr(RunContainerPauseViaAgent(reg, cid))
}

// UnpauseViaReverseAgent implements `docker unpause` through the
// container's reverse agent.
func (s *BaseServer) UnpauseViaReverseAgent(reg *ReverseAgentRegistry, ref string) error {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return MapPauseErr(RunContainerUnpauseViaAgent(reg, cid))
}
