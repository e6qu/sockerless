package aca

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// PodStart starts every member through the cloud-aware ContainerStart.
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	return s.CloudPodStart(name, s.ContainerStart)
}

// PodStop stops every running member through the cloud-aware ContainerStop.
func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	return s.CloudPodStop(name, timeout, s.ContainerStop)
}

// PodKill signals every running member through the cloud-aware ContainerKill.
func (s *Server) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	return s.CloudPodKill(name, signal, s.ContainerKill)
}

// PodRemove removes every member through the cloud-aware ContainerRemove,
// so no Container Apps job or app is orphaned, then deletes the pod.
func (s *Server) PodRemove(name string, force bool) error {
	return s.CloudPodRemove(name, force, s.ContainerRemove)
}

// ExecCreate creates an exec instance. ExecStart requires the
// in-container reverse-agent (no fallback to ACA management-API
// WebSocket exec — that path silently swaps execution semantics).
func (s *Server) ExecCreate(containerID string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), containerID)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: containerID}
	}

	if !c.State.Running {
		return nil, &api.ConflictError{Message: "Container " + containerID + " is not running"}
	}

	return s.BaseServer.ExecCreate(containerID, req)
}

// ExecStart starts an exec instance. Requires a registered
// reverse-agent for the container; fails loud if missing. No
// fallback to the ACA management-API WebSocket exec — that path
// runs in a separate ad-hoc shell session with different env /
// stream encoding and would hide reverse-agent setup bugs.
func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "exec instance", ID: id}
	}

	c, ok := s.ResolveContainerAuto(context.Background(), exec.ContainerID)
	if !ok {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Container %s has been removed", exec.ContainerID),
		}
	}

	if s.reverseAgents.IsLifetimeExpired(c.ID) {
		return nil, &api.ServerError{Message: fmt.Sprintf(
			"container %s exceeded ACA's max invocation lifetime. "+
				"FaaS pods are not extended transparently — for sustained workloads use ACA Apps (always-on replicas) "+
				"or switch to a longer-lived backend. (FaaSPodLifetimeExceeded)",
			c.ID[:12],
		)}
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); !hasAgent {
		return nil, &api.ServerError{Message: fmt.Sprintf(
			"reverse-agent WebSocket not registered for container %s. "+
				"ACA exec requires SOCKERLESS_CALLBACK_URL reachable from inside the App / Job "+
				"so the bootstrap can dial back. See backends/aca/README.md § reverse-agent prerequisites. "+
				"(Was the bootstrap able to start and reach the callback URL?)",
			c.ID[:12],
		)}
	}
	return s.BaseServer.ExecStart(id, opts)
}

// ContainerAttach attaches to a container's streams. Requires a
// registered reverse-agent (same reason as ExecStart — no
// management-API fallback).
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	// gitlab-runner attach-stdin pattern: a per-stage / prepare script is
	// written to the container's MAIN process stdin. This must take precedence
	// over the reverse-agent routing below — the reverse-agent never registers
	// a main process (mp==nil; reverse mode carries only exec sessions), so a
	// stdin attach routed to it fails "no main process to attach to". The App
	// bootstrap registers a reverse-agent (and on a per-build network it
	// registers before the runner attaches), so without this precedence the
	// stdin attach resolves the agent and breaks — the script never runs and
	// the runner loops recreating the container. The stdin script always
	// belongs on the buffered-invoke stdinPipe path (runACAInitialStdinStage).
	if opts.Stdin && s.config.UseApp {
		p := core.NewStdinPipe()
		actual, _ := s.stdinPipes.LoadOrStore(c.ID, p)
		pipe, isPipe := actual.(*core.StdinPipe)
		if !isPipe {
			return nil, &api.ServerError{Message: fmt.Sprintf("ContainerAttach %s: stdin pipe map held unexpected type %T", c.ID, actual)}
		}
		pipe.Open()
		return s.newAttachStream(c.ID, pipe), nil
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); !hasAgent {
		return nil, &api.ServerError{Message: fmt.Sprintf(
			"reverse-agent WebSocket not registered for container %s. "+
				"ACA attach requires SOCKERLESS_CALLBACK_URL reachable from inside the App / Job "+
				"so the bootstrap can dial back. See backends/aca/README.md § reverse-agent prerequisites.",
			c.ID[:12],
		)}
	}
	// Read-only attach is bridged through the reverse agent by the base server;
	// pass the cloud-resolved container ID (ResolveContainerAuto above) — never
	// raw local-store state.
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerAttach(cid, opts)
}

// ContainerExport streams the container's root filesystem through the
// reverse agent.
func (s *Server) ContainerExport(ref string) (io.ReadCloser, error) {
	return s.ExportViaReverseAgent(s.reverseAgents, ref, "container")
}

// ContainerCommit is not supported by the ACA backend.
// ACA containers cannot be snapshotted into images.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), req.Container); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	if !s.config.EnableCommit {
		return nil, &api.NotImplementedError{Message: "docker commit on ACA is gated — set SOCKERLESS_ENABLE_COMMIT=1 (agent-driven commit captures added/modified files since container boot as a new layer)"}
	}
	return core.CommitContainerRequestViaAgent(s.BaseServer, s.reverseAgents, req)
}

// AuthLogin handles registry authentication.
// For ACR registries (*.azurecr.io), logs a warning and delegates to BaseServer.
// For all other registries, delegates to BaseServer directly.
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	if strings.HasSuffix(req.ServerAddress, ".azurecr.io") {
		s.Logger.Warn().
			Str("registry", req.ServerAddress).
			Msg("ACR login: credentials stored locally; use `az acr login` for production")
		return s.BaseServer.AuthLogin(req)
	}
	return s.BaseServer.AuthLogin(req)
}

// Info returns system information enriched with ACA-specific metadata.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}

	// Enrich the Name field with ACA environment details
	info.Name = fmt.Sprintf("%s (aca:%s/%s)", info.Name, s.config.ResourceGroup, s.config.Environment)

	return info, nil
}
