package gcf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	"cloud.google.com/go/logging/logadmin"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

func isImplicitDockerNetwork(networkID string) bool {
	switch strings.TrimSpace(networkID) {
	case "", "default", "bridge", "host", "none":
		return true
	default:
		return false
	}
}

// ContainerCreate creates a container backed by a Cloud Run Function.
func (s *Server) ContainerCreate(req *api.ContainerCreateRequest) (*api.ContainerCreateResponse, error) {
	name := req.Name
	if name == "" {
		name = "/" + core.GenerateName()
	} else if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	if avail, _ := s.CloudState.CheckNameAvailable(context.Background(), name); !avail {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(name, "/")),
		}
	}

	id := core.GenerateID()

	config := api.ContainerConfig{}
	if req.ContainerConfig != nil {
		config = *req.ContainerConfig
	}

	// Merge image config if available
	if img, ok := s.Store.ResolveImage(config.Image); ok {
		// Merge ENV by key — image provides defaults, container overrides
		config.Env = core.MergeEnvByKey(img.Config.Env, config.Env)
		// Docker clears image Cmd when Entrypoint is overridden in create
		if len(config.Cmd) == 0 && len(config.Entrypoint) == 0 {
			config.Cmd = img.Config.Cmd
		}
		if len(config.Entrypoint) == 0 {
			config.Entrypoint = img.Config.Entrypoint
		}
		if config.WorkingDir == "" {
			config.WorkingDir = img.Config.WorkingDir
		}
		// Replace bare digest ref with first RepoTag — Cloud Run
		// rewrites bare sha256: refs to mirror.gcr.io/library/...
		// which 404s. Image was pulled by tag so RepoTag exists.
		if strings.HasPrefix(config.Image, "sha256:") && len(img.RepoTags) > 0 {
			config.Image = img.RepoTags[0]
		}
	}
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	// Resolve Docker Hub images to Artifact Registry remote repository URIs
	config.Image = gcpcommon.ResolveGCPImageURI(config.Image, s.config.Project, s.config.Region, s.config.EndpointURL)

	hostConfig := api.HostConfig{NetworkMode: "default"}
	if req.HostConfig != nil {
		hostConfig = *req.HostConfig
	}
	if hostConfig.NetworkMode == "" {
		hostConfig.NetworkMode = "default"
	}

	// Named-volume binds (`-v volName:/mnt[:ro]`) land on sockerless-
	// managed GCS buckets via the underlying Cloud Run Service's
	// ServiceV2.Template.Volumes. Host-path binds translate via
	// SharedVolumes (config-driven). Mirror of `cloudrun.ContainerCreate`
	// translator + `lambda.fileSystemConfigsForBinds` shape.
	translatedBinds := make([]string, 0, len(hostConfig.Binds))
	for _, b := range hostConfig.Binds {
		parts := strings.SplitN(b, ":", 3)
		if len(parts) < 2 {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid bind %q: expected src:dst[:mode]", b)}
		}
		src, dst := parts[0], parts[1]
		mode := ""
		if len(parts) == 3 {
			mode = parts[2]
		}
		if src == "/var/run/docker.sock" {
			continue
		}
		if strings.HasPrefix(src, "/") || strings.HasPrefix(src, ".") {
			if sv := s.config.LookupSharedVolumeBySourcePath(src); sv != nil {
				translated := sv.Name + ":" + dst
				if mode != "" {
					translated += ":" + mode
				}
				translatedBinds = append(translatedBinds, translated)
				continue
			}
			if isSubPathOfSharedVolume(src, s.config.SharedVolumes) {
				continue
			}
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("host-path binds are not supported on Cloud Functions (%q); use a named volume (docker volume create + -v name:%s) — volumes are backed by sockerless-managed GCS buckets. Configure SOCKERLESS_GCP_SHARED_VOLUMES to translate runner-task bind mounts.", b, dst)}
		}
		translatedBinds = append(translatedBinds, b)
	}
	hostConfig.Binds = translatedBinds

	path := ""
	var args []string
	if len(config.Entrypoint) > 0 {
		path = config.Entrypoint[0]
		args = append(config.Entrypoint[1:], config.Cmd...)
	} else if len(config.Cmd) > 0 {
		path = config.Cmd[0]
		args = config.Cmd[1:]
	}

	container := api.Container{
		ID:      id,
		Name:    name,
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Path:    path,
		Args:    args,
		State: api.ContainerState{
			Status:     "created",
			FinishedAt: "0001-01-01T00:00:00Z",
			StartedAt:  "0001-01-01T00:00:00Z",
		},
		Image:      config.Image,
		Config:     config,
		HostConfig: hostConfig,
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts:   make([]api.MountPoint, 0),
		Platform: "linux",
		Driver:   "cloud-run-functions",
	}

	// Set up default network — resolve via store for correct ID and Containers map
	netName := hostConfig.NetworkMode
	if netName == "default" {
		netName = "bridge"
	}
	networkID := netName
	if net, ok := s.Store.ResolveNetwork(netName); ok {
		networkID = net.ID
		// Register container in the network's Containers map
		s.Store.Networks.Update(net.ID, func(n *api.Network) {
			if n.Containers == nil {
				n.Containers = make(map[string]api.EndpointResource)
			}
			n.Containers[id] = api.EndpointResource{
				Name:       strings.TrimPrefix(name, "/"),
				EndpointID: core.GenerateID()[:16],
			}
		})
	}
	endpoint := &api.EndpointSettings{
		NetworkID:   networkID,
		EndpointID:  core.GenerateID()[:16],
		Gateway:     "",
		IPAddress:   "",
		IPPrefixLen: 16,
		MacAddress:  "",
	}
	// Capture standard Docker NetworkingConfig.EndpointsConfig.Aliases
	// so the multi-container materialiser can source SOCKERLESS_HOST_
	// ALIASES from them. Pure Docker-API signal — no runner-specific code.
	if req.NetworkingConfig != nil {
		for refName, reqEp := range req.NetworkingConfig.EndpointsConfig {
			if reqEp == nil {
				continue
			}
			matches := refName == netName
			if !matches {
				if net, ok := s.Store.ResolveNetwork(refName); ok && net.ID == networkID {
					matches = true
				}
			}
			if matches && len(reqEp.Aliases) > 0 {
				endpoint.Aliases = append(endpoint.Aliases, reqEp.Aliases...)
			}
		}
	}
	container.NetworkSettings.Networks[netName] = endpoint

	// Inject SOCKERLESS_DNS_SEARCH_DOMAIN so the bootstrap can append a
	// `search` line to /etc/resolv.conf and short-name lookups within the
	// network resolve. User's per-job override wins.
	if suffix, err := s.DNS.SearchDomain(s.ctx(), networkID); err == nil {
		if env := core.DNSSearchDomainEnvIfSet(config.Env, suffix); env != "" {
			config.Env = append(config.Env, env)
			container.Config.Env = config.Env
		}
	}

	// Fast-path: store in PendingCreates immediately and run the slow
	// CreateFunction + UpdateService work in a background goroutine.
	// ContainerCreate returns 201 in <100 ms; ContainerStart waits on
	// s.deployFutures[id] before invoking the function.
	// gitlab-runner's 120 s docker-daemon timeout fires per HTTP call —
	// returning fast from /containers/create avoids the timeout. The
	// caller's natural next step is /containers/{id}/start which polls
	// without a hard timeout, so the deploy can take its full 200 s
	// without violating the contract.
	s.PendingCreates.Put(id, container)
	s.EmitEvent("container", "create", id, map[string]string{
		"name":  strings.TrimPrefix(name, "/"),
		"image": config.Image,
	})
	// Cancellation context lets ContainerStart abort the in-flight
	// async deploy when the container turns out to be a network-pod
	// member that should materialize as a multi-container Service.
	// Parent is Background — async deploy lifetime is independent of
	// the (short-lived) ContainerCreate request handler.
	deployCtx, cancel := context.WithCancel(context.Background())
	deployCh := make(chan error, 1)
	s.deployFutures.Store(id, &deployFuture{ch: deployCh, cancel: cancel})
	go s.deployFunctionAsync(deployCtx, id, container, deployCh)
	return &api.ContainerCreateResponse{ID: id, Warnings: []string{}}, nil
}

// cancelDeployFuture atomically removes the future for `id` from the
// futures map, fires its cancel func, and drains its result channel
// (blocks until the goroutine exits — typically <1s once the next ctx
// check fires; the deferred unwind in deployFunction releases any
// claimed function before exit). Safe to call when no future exists
// (returns immediately). Idempotent: a second call after the first
// drain is a no-op.
func (s *Server) cancelDeployFuture(id string) {
	v, ok := s.deployFutures.LoadAndDelete(id)
	if !ok {
		return
	}
	f, _ := v.(*deployFuture)
	if f == nil {
		return
	}
	if f.cancel != nil {
		f.cancel()
	}
	if f.ch != nil {
		// Drain — the goroutine will close(ch) after sending its result.
		// We don't care about the value (errDeployCancelled or nil); we
		// just need to be sure the goroutine has finished its unwind.
		<-f.ch
	}
}

// deployFunctionAsync runs the heavy CreateFunction.Wait + image swap
// work that ContainerCreate used to do synchronously. Sends the final
// error (or nil on success) on `done`. Invoked from a goroutine kicked
// by ContainerCreate; ContainerStart awaits this channel before going
// to invoke.
//
// Honours ctx for cancellation: if ContainerStart later decides this
// container is part of a multi-container pod that should materialize as
// a single Cloud Run Service revision (per network_pod.go), it calls
// the future's cancel func, this goroutine returns errDeployCancelled,
// and the deferred release-pool path unwinds any claim taken so far.
func (s *Server) deployFunctionAsync(ctx context.Context, id string, container api.Container, done chan<- error) {
	err := s.deployFunction(ctx, id, container)
	if err != nil {
		if ctx.Err() != nil {
			// Cancelled — surface the sentinel so the awaiter knows it's
			// expected (vs a real error). The deploy may have left a
			// half-committed pool claim; deployFunction is responsible
			// for unwinding via its own ctx.Err() checks. If we landed
			// here without unwinding, log so the inconsistency is visible.
			s.Logger.Info().Str("container", id).Err(err).Msg("async deploy cancelled — container will be materialized as part of a network-pod Service revision")
			err = errDeployCancelled
		} else {
			s.Logger.Error().Err(err).Str("container", id).Msg("async deploy failed")
		}
	}
	select {
	case done <- err:
	default:
	}
	close(done)
}

// deployFunction performs the original synchronous deploy work
// extracted from ContainerCreate. Builds the overlay, claims a pool
// entry or creates a fresh Function, swaps the underlying Service
// image, and attaches volumes. Mutates s.PendingCreates entry on
// completion so subsequent reads see fresh state.
//
// Honours ctx — at every cloud-API boundary checks ctx.Err() and
// unwinds the partial deploy. If a pool entry was already claimed when
// cancellation arrives, releases the claim (clears
// sockerless_allocation label) so the next attempt can reclaim. If a
// fresh Function was already created, deletes it. Returns the context
// error directly so deployFunctionAsync can surface errDeployCancelled.
//
// Routes the entire single-container deploy through the fast Cloud
// Run Services.CreateService path (deployContainerService). Cloud
// Functions Gen2's CreateFunction + Buildpacks build + UpdateService
// swap was ~90-150 s — long enough that gitlab-runner's permission
// container hit the 120 s timeout; the direct CR Service deploy is
// ~30-60 s. Skips the pool-claim path too — at this speed the pool
// optimisation is unnecessary, and pool entries are Functions which
// the cloud_state Services-side path doesn't index.
func (s *Server) deployFunction(ctx context.Context, id string, container api.Container) error {
	if err := s.deployContainerService(ctx, id, container); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// ContainerStart starts a Cloud Run Function invocation for the container.
func (s *Server) ContainerStart(ref string) error {
	s.Logger.Info().Str("ref", ref).Msg("ContainerStart: ENTRY")
	// Resolve from PendingCreates (containers between create and start)
	c, ok := s.PendingCreates.Get(ref)
	if !ok {
		// Try name/short-ID match in PendingCreates
		for _, pc := range s.PendingCreates.List() {
			if pc.Name == ref || pc.Name == "/"+ref || (len(ref) >= 3 && strings.HasPrefix(pc.ID, ref)) {
				c = pc
				ok = true
				break
			}
		}
	}
	if !ok {
		s.Logger.Warn().Str("ref", ref).Msg("ContainerStart: NOT FOUND in PendingCreates")
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID
	s.Logger.Info().Str("container", id).Bool("running", c.State.Running).Str("status", c.State.Status).Bool("openStdin", c.Config.OpenStdin).Msg("ContainerStart: resolved")

	if c.State.Running {
		// Multi-stage gitlab-runner pattern: per stage gitlab-runner does
		// `stop` then re-`attach` + re-`start`. Cloud Run Service revisions
		// are immutable so we can't actually restart, but the bootstrap
		// HTTP server inside the Service is still alive and accepts new
		// envelope POSTs. When a fresh stdinPipe was just registered (by
		// the per-stage attach) AND the container has OpenStdin=true,
		// kick off a NEW invoke goroutine to drain the new pipe + POST
		// the new stage's script to the bootstrap. Without this, only
		// the first stage's script runs and gitlab-runner hangs waiting
		// for stage 2's output.
		if c.Config.OpenStdin {
			if _, hasPipe := s.stdinPipes.Load(id); hasPipe {
				s.Logger.Info().Str("container", id).Msg("ContainerStart: already running but fresh stdinPipe registered — kicking new invoke goroutine for next stage")
				go s.invokeRunningRunnerStage(id, c)
				return nil
			}
		}
		s.Logger.Info().Str("container", id).Msg("ContainerStart: already running, returning NotModified")
		return &api.NotModifiedError{}
	}

	// Docker-network → multi-member pod auto-detection FIRST. The
	// decision is purely Standard-Docker-API: NetworkingConfig.EndpointsConfig
	// + Container.Config.OpenStdin. Doing this BEFORE the deploy-await
	// resolves the deploy/materialize architectural conflict — if this
	// container is a network-pod member that should materialize as a
	// multi-container Service revision, we cancel our own (and siblings')
	// in-flight async deploys before they complete, then take the
	// materialize path.
	netDefer, netMembers := s.shouldDeferOrMaterializeNetworkPod(c)
	s.Logger.Info().Str("container", id).Bool("openStdin", c.Config.OpenStdin).Bool("netDefer", netDefer).Int("netMembers", len(netMembers)).Msg("ContainerStart: network-pod decision")
	if netDefer {
		// Cancel our own in-flight deploy — a script-runner sibling will
		// eventually arrive and trigger materializePodFunction with us as
		// a member. The single-container deploy goroutine would race that
		// path and leave an orphan single-container function.
		s.cancelDeployFuture(id)
		s.PendingCreates.Update(id, func(pc *api.Container) {
			pc.State.Status = "running"
			pc.State.Running = true
			pc.State.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
		})
		return nil
	}
	if len(netMembers) > 1 {
		// Cancel + drain every member's in-flight async deploy before
		// materializing — a sibling's single-container deploy completing
		// concurrently with our multi-container Service deploy would leave
		// an orphan function and (worse) the runner's cross-container
		// loopback DNS would point at the wrong revision.
		for _, m := range netMembers {
			s.cancelDeployFuture(m.ID)
		}
		// netMembers[0].ID is the authoritative MAIN per the convention
		// in shouldDeferOrMaterializeNetworkPod. For gitlab-runner pattern
		// (OpenStdin=true) that's the current container; for GH actions/
		// runner pattern (OpenStdin=false service-arrival path)
		// it's the FIRST sibling — the long-lived job container created
		// before this service. Use that ID for Service naming and
		// allocation labels so resolveGCFFromCloud(mainID) finds the
		// pod-Service afterwards.
		mainID := netMembers[0].ID
		exitCh := make(chan struct{})
		s.Store.WaitChs.Store(mainID, exitCh)
		// Mark the materializing main container "running" in PendingCreates
		// so concurrent ContainerInspect / cleanup-script docker exec calls
		// during the 30-60s CreateService.Wait window resolve to a real
		// container instead of NotFound. queryPodServiceContainers takes
		// over once the Service is queryable; the entry is removed in
		// invokePodServiceMain when the pod completes.
		updated := s.PendingCreates.Update(mainID, func(pc *api.Container) {
			pc.State.Status = "running"
			pc.State.Running = true
			pc.State.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
		})
		if !updated {
			// Entry was already removed (race with cancelled async deploy
			// cleanup); recreate it from the resolved member so the
			// materialize window has a visible entry for inspect/exec.
			mCopy := netMembers[0]
			mCopy.State.Status = "running"
			mCopy.State.Running = true
			mCopy.State.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
			s.PendingCreates.Put(mainID, mCopy)
		}
		s.Logger.Info().Str("main", mainID).Str("trigger", id).Bool("updated", updated).Int("members", len(netMembers)).Msg("network-pod ContainerStart: marked running, entering materialize")
		err := s.materializePodService(mainID, netMembers, exitCh)
		if err != nil {
			s.PendingCreates.Delete(mainID)
		}
		return err
	}

	// Single-container fall-through: await OUR own deploy. ContainerCreate
	// kicked deployFunctionAsync immediately so this is gitlab-runner's
	// natural blocking wait point — taking the full 200s here is within
	// the docker-API contract.
	if v, ok := s.deployFutures.LoadAndDelete(id); ok {
		f, _ := v.(*deployFuture)
		if f != nil && f.ch != nil {
			if deployErr, alive := <-f.ch; alive && deployErr != nil {
				return deployErr
			}
		}
	}

	// Multi-container pod handling: defer until all members have been
	// started, then collapse the pod into a single Cloud Run Function
	// backed by a merged-rootfs overlay (per spec § "Podman pods on
	// FaaS backends — supervisor-in-overlay"). The supervisor (PID 1
	// of the function container) forks one chroot'd subprocess per
	// pod member; the main member's stdout becomes the HTTP response
	// body and sidecars run for the lifetime of the invocation.
	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod && len(pod.ContainerIDs) > 1 {
		exitCh := make(chan struct{})
		s.Store.WaitChs.Store(id, exitCh)
		shouldDefer, podContainers := s.PodDeferredStart(id)
		if shouldDefer {
			// Earlier pod members wait for the main's start to trigger
			// the merged-Function build. Their WaitChs stay registered
			// so `docker wait <member>` blocks until invokePodFunction
			// fans the result out.
			return nil
		}
		s.PendingCreates.Delete(id)
		return s.materializePodService(id, podContainers, exitCh)
	}

	gcfState, _ := s.resolveGCFFromCloud(s.ctx(), id)

	// Remove from PendingCreates now that we're starting.
	s.PendingCreates.Delete(id)

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Invoke function via HTTP trigger asynchronously and capture the
	// outcome in Store.InvocationResults so CloudState reflects the
	// container as exited with a real exit code.
	//
	// Pass user entrypoint+cmd as the exec envelope's argv so
	// pool-claimed Functions don't need a CR Service env update on
	// each claim. Bootstrap's parseExecEnvelope reads argv from the
	// request body and runs Path B; when argv is empty the bootstrap
	// falls back to the legacy default-invoke path that reads
	// SOCKERLESS_USER_* from env (still works for fresh deploys).
	argv := append([]string{}, c.Config.Entrypoint...)
	argv = append(argv, c.Config.Cmd...)
	envSlice := append([]string{}, c.Config.Env...)
	go func() {
		inv := core.InvocationResult{}
		capturedStdin, hasCapturedStdin := s.captureGCFStdin(id)
		if gcfState.FunctionURL == "" {
			s.Logger.Error().Str("function", gcfState.FunctionName).Msg("no function URL available for invocation")
			inv.ExitCode = 1
			inv.Error = "no function URL available"
			s.publishGCFAttachResponse(id, nil, []byte(inv.Error))
		} else if resp, err := s.invokeFunction(s.ctx(), gcfState.FunctionURL, argv, c.Config.WorkingDir, envSlice, capturedStdin); err != nil {
			if !c.Config.OpenStdin {
				if _, hasAgent := s.reverseAgents.Resolve(id); hasAgent {
					s.Logger.Warn().Err(err).Str("function", gcfState.FunctionName).Str("container", id).Msg("function invoke returned after reverse-agent registration; preserving running container state")
					return
				}
				agentCtx, agentCancel := context.WithTimeout(s.ctx(), 2*time.Second)
				agentErr := s.reverseAgents.WaitForAgent(agentCtx, id)
				agentCancel()
				if agentErr == nil {
					s.Logger.Warn().Err(err).Str("function", gcfState.FunctionName).Str("container", id).Msg("function invoke returned while reverse-agent was registering; preserving running container state")
					return
				}
			}
			s.Logger.Error().Err(err).Str("function", gcfState.FunctionName).Msg("function invocation failed")
			inv.ExitCode = core.HTTPInvokeErrorExitCode(err)
			inv.Error = err.Error()
			s.publishGCFAttachResponse(id, nil, []byte(err.Error()))
		} else {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				s.Logger.Error().Err(readErr).Str("function", gcfState.FunctionName).Msg("read invoke response body")
				inv.ExitCode = 1
				inv.Error = readErr.Error()
				s.publishGCFAttachResponse(id, nil, []byte(readErr.Error()))
			} else {
				// Bootstrap MUST return the exec envelope shape
				// `{"sockerlessExecResult":{"exitCode":N,"stdout":"<b64>","stderr":"<b64>"}}`.
				// Anything else is a bug in the bootstrap or the
				// transport — fail loudly rather than guess.
				execResult, perr := gcpcommon.ParseExecResult(body)
				if perr != nil {
					if !c.Config.OpenStdin {
						if _, hasAgent := s.reverseAgents.Resolve(id); hasAgent {
							s.Logger.Warn().Err(perr).Int("status", resp.StatusCode).Str("function", gcfState.FunctionName).Str("container", id).Msg("non-envelope invoke response after reverse-agent registration; preserving running container state")
							return
						}
						agentCtx, agentCancel := context.WithTimeout(s.ctx(), 2*time.Second)
						agentErr := s.reverseAgents.WaitForAgent(agentCtx, id)
						agentCancel()
						if agentErr == nil {
							s.Logger.Warn().Err(perr).Int("status", resp.StatusCode).Str("function", gcfState.FunctionName).Str("container", id).Msg("non-envelope invoke response while reverse-agent was registering; preserving running container state")
							return
						}
					}
					s.Logger.Error().Err(perr).Int("status", resp.StatusCode).Str("function", gcfState.FunctionName).Msg("bootstrap response is not an exec envelope")
					inv.ExitCode = 1
					inv.Error = fmt.Sprintf("non-envelope response: %v", perr)
					s.publishGCFAttachResponse(id, nil, []byte(inv.Error))
				} else {
					var combined []byte
					combined = append(combined, execResult.Stdout...)
					combined = append(combined, execResult.Stderr...)
					if len(combined) > 0 {
						s.Store.LogBuffers.Store(id, combined)
					}
					inv.ExitCode = execResult.ExitCode
					if inv.ExitCode != 0 {
						inv.Error = fmt.Sprintf("subprocess exit %d", inv.ExitCode)
						s.Logger.Warn().Int("status", resp.StatusCode).Int("exit", inv.ExitCode).Str("function", gcfState.FunctionName).Msg("function returned non-zero subprocess exit")
					}
					if hasCapturedStdin {
						s.publishGCFAttachResponse(id, execResult.Stdout, execResult.Stderr)
					}
				}
			}
		}
		s.Store.PutInvocationResult(id, inv)

		// Close wait channel so ContainerWait unblocks
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
	}()

	// Wait for the in-function bootstrap to register a reverse-agent
	// before ContainerStart returns. Skip in the OpenStdin one-shot
	// path (gitlab-runner stdin-piped script) — the function runs and
	// exits without docker exec calls. For exec-driven callers
	// the first ExecStart MUST find an agent (no Path B fallback).
	if !c.Config.OpenStdin {
		timeout, terr := core.BootstrapTimeoutFromEnv("gcf")
		if terr != nil {
			return &api.ServerError{Message: fmt.Sprintf("invalid bootstrap-timeout env: %v", terr)}
		}
		waitCtx, cancel := context.WithTimeout(s.ctx(), timeout)
		defer cancel()
		if werr := s.reverseAgents.WaitForAgent(waitCtx, id); werr != nil {
			return &api.ServerError{Message: fmt.Sprintf(
				"reverse-agent did not register for container %s within %s "+
					"(SOCKERLESS_GCF_BOOTSTRAP_TIMEOUT_SEC). Service URL invoke fired but the "+
					"in-function bootstrap never dialled back to SOCKERLESS_CALLBACK_URL=%s. "+
					"Check egress / VPC connector / firewall for the callback endpoint.",
				id[:12], timeout, s.config.CallbackURL,
			)}
		}
		if inv, ok := s.Store.GetInvocationResult(id); ok && isReverseAgentInvokeTransportError(inv.Error) {
			s.Logger.Warn().Str("container", id).Str("error", inv.Error).Msg("clearing transport-only invoke error after reverse-agent registration")
			s.Store.DeleteInvocationResult(id)
		}
	}

	return nil
}

func (s *Server) captureGCFStdin(id string) ([]byte, bool) {
	v, ok := s.stdinPipes.LoadAndDelete(id)
	if !ok {
		return nil, false
	}
	pipe := v.(*stdinPipe)
	select {
	case <-pipe.Done():
	case <-time.After(30 * time.Second):
		s.Logger.Warn().Str("container", id).Msg("GCF stdin pipe Done timeout; proceeding with captured bytes")
	case <-s.ctx().Done():
		return nil, true
	}
	return pipe.Bytes(), true
}

func (s *Server) publishGCFAttachResponse(id string, stdout, stderr []byte) {
	if v, ok := s.attachStreams.LoadAndDelete(id); ok {
		v.(*attachStream).publishAttachResponse(stdout, stderr)
	}
}

func isReverseAgentInvokeTransportError(errText string) bool {
	return strings.Contains(errText, "post exec envelope") ||
		strings.Contains(errText, "invoke bootstrap") ||
		strings.Contains(errText, "exec invoke returned status")
}

// ContainerStop stops a running Cloud Run Function container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Cloud Run Functions run to completion — stop transitions state
	s.StopHealthCheck(id)
	// Record the stop outcome so CloudState reports exited with code
	// 137 (Docker convention for force-stopped).
	s.Store.PutInvocationResult(id, core.InvocationResult{ExitCode: 137})
	// Close wait channel so ContainerWait unblocks
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": "137", "name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerKill kills a container with the given signal.
func (s *Server) ContainerKill(ref string, signal string) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		}
	}

	s.StopHealthCheck(id)

	exitCode := core.SignalToExitCode(signal)
	s.Store.PutInvocationResult(id, core.InvocationResult{ExitCode: exitCode})

	s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", exitCode), "name": strings.TrimPrefix(c.Name, "/")})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	return nil
}

// ContainerRemove removes a container and its associated Cloud Run Function resources.
func (s *Server) ContainerRemove(ref string, force bool) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		// Also check PendingCreates (container created but never started)
		if pc, pcOK := s.PendingCreates.Get(ref); pcOK {
			c = pc
			ok = true
		}
	}
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if c.State.Running && !force {
		return &api.ConflictError{
			Message: fmt.Sprintf("You cannot remove a running container %s. Stop the container before attempting removal or force remove", id[:12]),
		}
	}

	if c.State.Running {
		// `docker rm -f` is SIGKILL → exit 137.
		killExitCode := core.SignalToExitCode("SIGKILL")
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": fmt.Sprintf("%d", killExitCode),
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
	}

	s.StopHealthCheck(id)

	// Pool-aware release: derive (function-name, overlay-hash) from cloud labels
	// so the release path is correct after a backend restart (no in-memory state).
	gcfState, _ := s.resolveGCFFromCloud(s.ctx(), id)
	fullName := ""
	if gcfState.FunctionName != "" {
		// Cache hit
		if strings.HasPrefix(gcfState.FunctionName, "projects/") {
			fullName = gcfState.FunctionName
		} else {
			fullName = fmt.Sprintf("projects/%s/locations/%s/functions/%s", s.config.Project, s.config.Region, gcfState.FunctionName)
		}
	} else {
		// Recover from cloud labels: list sockerless-managed Functions
		// allocated to this container.
		parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
		filter := fmt.Sprintf(`labels.sockerless_allocation:"%s"`, shortAllocLabel(id))
		it := s.gcp.Functions.ListFunctions(s.ctx(), &functionspb.ListFunctionsRequest{Parent: parent, Filter: filter})
		if fn, err := it.Next(); err == nil && fn != nil {
			fullName = fn.GetName()
		}
	}
	var cleanupErrs []error
	if fullName != "" {
		fn, gerr := s.gcp.Functions.GetFunction(s.ctx(), &functionspb.GetFunctionRequest{Name: fullName})
		contentTag := ""
		if gerr == nil && fn != nil {
			contentTag = fn.GetLabels()["sockerless_overlay_hash"]
		}
		if err := s.releaseOrDeleteFunction(s.ctx(), fullName, contentTag); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("pool release function %q: %w", fullName, err))
		}
		s.Registry.MarkCleanedUp(fullName)
	}

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod {
		s.Store.Pods.RemoveContainer(pod.ID, id)
	}

	// Clean up network associations
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && !isImplicitDockerNetwork(ep.NetworkID) {
			if derr := s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, id); derr != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("network %q disconnect: %w", ep.NetworkID, derr))
			}
		}
	}

	s.PendingCreates.Delete(id)
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.Store.LogBuffers.Delete(id)
	s.Store.StagingDirs.Delete(id)
	s.Store.DeleteInvocationResult(id)
	if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(id); ok {
		for _, d := range dirs.([]string) {
			os.RemoveAll(d)
		}
	}
	for _, eid := range c.ExecIDs {
		s.Store.Execs.Delete(eid)
	}

	s.EmitEvent("container", "destroy", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	if len(cleanupErrs) > 0 {
		return &api.ServerError{Message: fmt.Sprintf("container %s removed locally but cloud cleanup had errors: %v", id[:12], errors.Join(cleanupErrs...))}
	}
	return nil
}

// ContainerLogs streams container logs from Cloud Logging.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	return core.StreamCloudLogs(s.BaseServer, ref, opts, s.buildCloudLogsFetcher(ref), core.StreamCloudLogsOptions{
		CheckLogBuffers: true,
	})
}

// buildCloudLogsFetcher returns a CloudLogFetchFunc closure that
// queries Cloud Logging for the given function. Shared by
// ContainerLogs and ContainerAttach.
//
// The `logName:"run.googleapis.com"` substring clause restricts the
// query to Cloud Run runtime logs (Gen2 functions run on Cloud Run).
// Without it, Cloud Audit Logs (`cloudaudit.googleapis.com/...`) match
// the same `resource.type="cloud_run_revision"` and would be merged
// into docker logs as multi-KB textproto AuditLog dumps.
func (s *Server) buildCloudLogsFetcher(ref string) core.CloudLogFetchFunc {
	var funcName string
	if id, ok := s.ResolveContainerIDAuto(context.Background(), ref); ok {
		gcfState, _ := s.resolveGCFFromCloud(s.ctx(), id)
		funcName = gcfState.FunctionName
	}
	baseFilter := fmt.Sprintf(
		`resource.type="cloud_run_revision" AND resource.labels.service_name="%s" AND logName:"run.googleapis.com"`,
		funcName,
	)
	return s.cloudLoggingFetch(baseFilter)
}

// gcfLogCursor mirrors cloudrun's `cloudLogCursor`: tracks lastTS plus a
// per-entry seen-set so tied-timestamp Cloud Logging entries (batched
// stdout writes) are not lost between fetches and not duplicated.
type gcfLogCursor struct {
	lastTS time.Time
	seen   map[string]struct{}
}

// cloudLoggingFetch returns a CloudLogFetchFunc that queries Cloud Logging
// using `timestamp>=cursor.lastTS` plus a `seen` set for dedup.
func (s *Server) cloudLoggingFetch(baseFilter string) core.CloudLogFetchFunc {
	return func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		logFilter := baseFilter

		c, _ := cursor.(*gcfLogCursor)
		if c == nil {
			c = &gcfLogCursor{seen: make(map[string]struct{})}
		}

		if !c.lastTS.IsZero() {
			logFilter += fmt.Sprintf(` AND timestamp>="%s"`, c.lastTS.UTC().Format(time.RFC3339Nano))
		} else {
			logFilter += params.CloudLoggingSinceFilter()
			logFilter += params.CloudLoggingUntilFilter()
		}

		fetchCtx, cancel := context.WithTimeout(s.ctx(), s.config.LogTimeout)
		defer cancel()

		it := s.gcp.LogAdmin.Entries(fetchCtx, logadmin.Filter(logFilter))

		var entries []core.CloudLogEntry
		for {
			entry, err := it.Next()
			if err != nil {
				break
			}
			line := extractLogLine(entry)
			if line == "" {
				continue
			}
			key := fmt.Sprintf("%d:%s", entry.Timestamp.UnixNano(), line)
			if _, dup := c.seen[key]; dup {
				continue
			}
			c.seen[key] = struct{}{}
			entries = append(entries, core.CloudLogEntry{Timestamp: entry.Timestamp, Message: line})
			if entry.Timestamp.After(c.lastTS) {
				c.lastTS = entry.Timestamp
			}
		}

		return entries, c, nil
	}
}

// ContainerRestart stops and then starts a container.
func (s *Server) ContainerRestart(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if c.State.Running {
		s.StopHealthCheck(id)
		// Close wait channel so ContainerWait unblocks
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
		// `docker restart` sends SIGTERM → exit 143.
		stopExitCode := core.SignalToExitCode("SIGTERM")
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": fmt.Sprintf("%d", stopExitCode),
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}

	// Re-add to PendingCreates so ContainerStart can find and launch it.
	s.PendingCreates.Put(id, c)

	// Start the container directly via typed method
	if err := s.ContainerStart(id); err != nil {
		return err
	}

	s.EmitEvent("container", "restart", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerPrune removes all stopped containers and their GCF state.
func (s *Server) ContainerPrune(filters map[string][]string) (*api.ContainerPruneResponse, error) {
	labelFilters := filters["label"]
	untilFilters := filters["until"]
	var deleted []string
	var spaceReclaimed uint64
	allContainers, _ := s.CloudState.ListContainers(context.Background(), true, nil)
	for _, c := range allContainers {
		if c.State.Status != "exited" && c.State.Status != "dead" {
			continue
		}
		if len(labelFilters) > 0 && !core.MatchLabels(c.Config.Labels, labelFilters) {
			continue
		}
		if len(untilFilters) > 0 && !core.MatchUntil(c.Created, untilFilters) {
			continue
		}
		// Sum image sizes for SpaceReclaimed
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		// Clean up Cloud Run Functions cloud resources
		gcfState, _ := s.resolveGCFFromCloud(s.ctx(), c.ID)
		if gcfState.FunctionName != "" {
			fullName := fmt.Sprintf("projects/%s/locations/%s/functions/%s", s.config.Project, s.config.Region, gcfState.FunctionName)
			if op, err := s.gcp.Functions.DeleteFunction(s.ctx(), &functionspb.DeleteFunctionRequest{
				Name: fullName,
			}); err == nil {
				_ = op.Wait(s.ctx())
			}
			s.Registry.MarkCleanedUp(fullName)
		}

		s.StopHealthCheck(c.ID)
		// Clean up network associations
		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, c.ID)
			}
		}
		if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
			s.Store.Pods.RemoveContainer(pod.ID, c.ID)
		}
		s.PendingCreates.Delete(c.ID)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
			close(ch.(chan struct{}))
		}
		s.Store.LogBuffers.Delete(c.ID)
		s.Store.StagingDirs.Delete(c.ID)
		if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(c.ID); ok {
			for _, d := range dirs.([]string) {
				os.RemoveAll(d)
			}
		}
		for _, eid := range c.ExecIDs {
			s.Store.Execs.Delete(eid)
		}
		s.EmitEvent("container", "destroy", c.ID, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
		deleted = append(deleted, c.ID)
	}
	if deleted == nil {
		deleted = []string{}
	}
	return &api.ContainerPruneResponse{
		ContainersDeleted: deleted,
		SpaceReclaimed:    spaceReclaimed,
	}, nil
}

// ContainerPause sends SIGSTOP to the user subprocess via the reverse-
// agent.
func (s *Server) ContainerPause(ref string) error {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return core.MapPauseErr(core.RunContainerPauseViaAgent(s.reverseAgents, cid))
}

// ContainerUnpause sends SIGCONT to the user subprocess via the
// reverse-agent.
func (s *Server) ContainerUnpause(ref string) error {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return core.MapPauseErr(core.RunContainerUnpauseViaAgent(s.reverseAgents, cid))
}

// ImagePull delegates to ImageManager which handles cloud auth and config fetching.
// Rewrites Docker Hub / GitLab Registry refs to the AR remote-proxy so all pulls
// in the project hit AR (avoids Docker Hub rate limits). When rewriting, discard
// the caller's auth — it was scoped to the original registry and is invalid for
// AR; ImageManager.Pull's cloud-auth path mints an AR token via ARAuthProvider.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	resolved := gcpcommon.ResolveGCPImageURI(ref, s.config.Project, s.config.Region, s.config.EndpointURL)
	if resolved == ref {
		return s.images.Pull(resolved, auth)
	}
	rc, err := s.images.Pull(resolved, "")
	if err != nil {
		return nil, err
	}
	if img, ok := s.Store.ResolveImage(resolved); ok {
		core.StoreImageWithAliases(s.Store, ref, img)
	}
	return rc, nil
}

// ImageLoad delegates to ImageManager.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return s.images.Load(r)
}

// PodStart starts all containers in a pod by calling ContainerStart for each,
// which triggers the GCF HTTP invocation. The BaseServer implementation only
// sets container state to "running" without invoking the function.
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}
	var errs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || c.State.Running {
			continue
		}
		if err := s.ContainerStart(cid); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if errs == nil {
		errs = []string{}
	}
	s.Store.Pods.SetStatus(pod.ID, "running")
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// ContainerExport streams the function container's rootfs as tar via
// the reverse-agent.
func (s *Server) ContainerExport(id string) (io.ReadCloser, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	rc, err := core.RunContainerExportViaAgent(s.reverseAgents, cid)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker export requires a reverse-agent bootstrap inside the function container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("export via reverse-agent: %v", err)}
	}
	return rc, nil
}

// ContainerCommit builds a new image from the function container's
// post-boot filesystem changes via the reverse-agent. Gated behind
// EnableCommit — the result is a single diff layer on top of the
// function's base image.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if req.Container == "" {
		return nil, &api.InvalidParameterError{Message: "container query parameter is required"}
	}
	if !s.config.EnableCommit {
		return nil, &api.NotImplementedError{Message: "docker commit on Cloud Run Functions is gated — set SOCKERLESS_ENABLE_COMMIT=1"}
	}
	return core.CommitContainerRequestViaAgent(s.BaseServer, s.reverseAgents, req)
}

// ContainerAttach bridges stdin/stdout/stderr to the bootstrap process
// inside the function container via the reverse-agent WebSocket when a
// session is registered. Without an agent, fall back to streaming
// Cloud Logging for read-only attach (no stdin); interactive attach
// has no native Cloud Run Functions surface and stays
// NotImplementedError.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	s.Logger.Info().Str("id", id).Bool("stdin", opts.Stdin).Bool("stdout", opts.Stdout).Bool("stderr", opts.Stderr).Bool("logs", opts.Logs).Bool("stream", opts.Stream).Msg("ContainerAttach: ENTRY")
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		s.Logger.Warn().Str("id", id).Msg("ContainerAttach: NOT FOUND")
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); hasAgent {
		s.Logger.Info().Str("id", id).Msg("ContainerAttach: routing to reverse-agent")
		return s.BaseServer.ContainerAttach(id, opts)
	}
	// gitlab-runner attach-stdin pattern (mirror of cloudrun's
	// ContainerAttach): when the caller wants stdin, register a
	// stdinPipe + attachStream so invokePodServiceMain (the network-pod
	// invoke goroutine) can wait for the script bytes to land before
	// POSTing to the Service URL. Without this, gcf's
	// invokePodServiceMain POSTs the user's default CMD immediately on
	// materialize completion, which exits 0 with no work and closes
	// WaitChs before gitlab-runner can attach + pipe its script.
	//
	// Note: c.Config.Image at attach time is the user-supplied original
	// (e.g. golang:1.22-alpine), NOT the overlay URI — the rewrite
	// happens inside materializePodService. We therefore allow stdin
	// for ANY container with a registered Cloud Run Service backing
	// (or about-to-be-created via materialize).
	if opts.Stdin {
		s.Logger.Info().Str("container", c.ID).Str("image", c.Config.Image).Msg("ContainerAttach: registering stdinPipe + attachStream")
		p := newStdinPipe()
		actual, _ := s.stdinPipes.LoadOrStore(c.ID, p)
		pipe := actual.(*stdinPipe)
		pipe.Open()
		return s.newAttachStream(c.ID, pipe), nil
	}
	s.Logger.Info().Str("id", id).Msg("ContainerAttach: routing to AttachViaCloudLogs (read-only)")
	return core.AttachViaCloudLogs(s.BaseServer, id, opts, s.buildCloudLogsFetcher(id))
}

// ImageBuild delegates to ImageManager.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	return s.images.Build(opts, buildContext)
}

// ImagePush delegates to ImageManager which handles cloud auth and OCI push.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	return s.images.Push(name, tag, auth)
}

// ImageTag delegates to ImageManager which handles cloud sync.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	return s.images.Tag(source, repo, tag)
}

// ImageRemove delegates to ImageManager which handles cloud sync.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	return s.images.Remove(name, force, prune)
}

// Info returns system information enriched with GCP-specific metadata.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}
	// Enrich with GCP project and region
	info.Name = fmt.Sprintf("%s (project=%s, region=%s)", info.Name, s.config.Project, s.config.Region)
	return info, nil
}
