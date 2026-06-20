package aca

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by an ACA Job.
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

	// A long-running *service* container (GitHub Actions `services:`) is
	// created without an explicit entrypoint/cmd — it uses the image default
	// (e.g. nginx). A *job* container overrides the entrypoint to a keepalive
	// (`tail -f /dev/null`) and runs its steps via exec; a stdin-attached
	// container (GitLab's piped-envelope flow) runs its workload via invoke.
	// Only a service's own workload must run at startup, so flag just that.
	serviceLike := len(config.Entrypoint) == 0 && len(config.Cmd) == 0 && !config.OpenStdin

	// Merge image config if available
	if img, ok := s.Store.ResolveImage(config.Image); ok {
		config.Env = core.MergeEnvByKey(img.Config.Env, config.Env)
		if len(config.Cmd) == 0 && len(config.Entrypoint) == 0 {
			config.Cmd = img.Config.Cmd
		}
		if len(config.Entrypoint) == 0 {
			config.Entrypoint = img.Config.Entrypoint
		}
		if config.WorkingDir == "" {
			config.WorkingDir = img.Config.WorkingDir
		}
		// Carry the image's declared ExposedPorts onto the container config so
		// `docker inspect` reports them — gitlab-runner reads a service
		// container's exposed ports to health-check it (a `services:` redis with
		// no reported ports is flagged "probably didn't start properly"). The
		// overlay rewrite below clears Entrypoint/Cmd but must not lose the
		// ports, so capture them from the base image here, before the rewrite.
		if len(config.ExposedPorts) == 0 && len(img.Config.ExposedPorts) > 0 {
			config.ExposedPorts = img.Config.ExposedPorts
		}
	}
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	// Resolve the image through the ACR pull-through cache if one is
	// configured. Falls through to the plain docker ref when
	// no registry or rule matches; ACA pulls Docker Hub refs directly.
	if resolved, err := azurecommon.ResolveAzureImageURIWithCache(
		s.ctx(),
		s.azure.ACRCacheRules,
		s.config.ResourceGroup,
		s.config.ACRName,
		config.Image,
	); err != nil {
		s.Logger.Warn().Err(err).Str("image", config.Image).Msg("ACR cache-rule lookup failed; using ref as-is")
	} else {
		config.Image = resolved
	}

	if s.useACAOverlayPath(config.Image) {
		if s.config.BootstrapBinaryPath == "" {
			return nil, &api.ServerError{Message: "SOCKERLESS_ACA_USE_APP=1 requires SOCKERLESS_ACA_BOOTSTRAP so ACA Apps run an image with the reverse-agent bootstrap baked in"}
		}
		originalImage := config.Image
		spec := acaOverlaySpec{
			BaseImageRef:        originalImage,
			BootstrapBinaryPath: s.config.BootstrapBinaryPath,
			BootstrapBinaryHash: s.config.BootstrapBinaryHash,
		}
		contentTag := acaOverlayContentTag("aca-", spec)
		overlayURI, err := s.ensureACAOverlayImage(s.ctx(), spec, contentTag)
		if err != nil {
			return nil, fmt.Errorf("ensure aca overlay image: %w", err)
		}
		config.Env = append(config.Env, acaOverlayUserEnv(config.Entrypoint, config.Cmd, config.WorkingDir)...)
		if serviceLike {
			// Run the service's own workload (nginx, postgres, …) at startup
			// so the container actually serves; the bootstrap still serves the
			// reverse-agent + HTTP for exec alongside it.
			config.Env = append(config.Env, "SOCKERLESS_RUN_USER_WORKLOAD=1")
		}
		if jt := core.JobTimeoutEnvIfUnset(config.Env); jt != "" {
			config.Env = append(config.Env, jt)
		}
		config.Image = overlayURI
		config.Entrypoint = nil
		config.Cmd = nil
	}

	hostConfig := api.HostConfig{NetworkMode: "default"}
	if req.HostConfig != nil {
		hostConfig = *req.HostConfig
	}
	if hostConfig.NetworkMode == "" {
		hostConfig.NetworkMode = "default"
	}

	// Validate + rewrite mount specs up-front via the shared-volume
	// translator (see shared_volumes.go for the full ACA bind-mount
	// model: named volumes pass through, mapped host binds rewrite to
	// named-volume references, sub-paths + docker.sock drop, anything
	// else rejects loudly).
	translatedBinds, droppedBinds, err := translateSharedVolumeBinds(s.config, hostConfig.Binds)
	if err != nil {
		return nil, err
	}
	for _, bind := range droppedBinds {
		s.Logger.Debug().Str("bind", bind).
			Msg("dropping bind mount; docker.sock has no ACA analogue / parent shared volume already exposes the sub-path")
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
		Driver:   "aca-jobs",
	}

	// Set up default network — resolve via store for correct ID and Containers map
	netName := hostConfig.NetworkMode
	if netName == "default" {
		netName = "bridge"
	}
	networkID := netName
	if net, ok := s.Store.ResolveNetwork(netName); ok {
		networkID = net.ID
	}
	// The network's container-membership map is NOT written to local Store
	// state here — a stateless backend never owns that. NetworkInspect /
	// NetworkList report membership from cloud-truth (the running Apps tagged
	// with this network), so a stopped container can't linger as a stale
	// "zombie" that makes a docker-host client (gitlab-runner) loop trying to
	// disconnect it.
	container.NetworkSettings.Networks[netName] = &api.EndpointSettings{
		NetworkID:   networkID,
		EndpointID:  core.GenerateID()[:16],
		Gateway:     "",
		IPAddress:   "",
		IPPrefixLen: 16,
		MacAddress:  "",
	}
	// Capture the network aliases the client requested at create time
	// (`docker create --network X --network-alias web`) — these are the
	// names peers resolve the container by (e.g. a GitHub Actions service
	// container's id). They're registered in Private DNS once the App is up.
	if req.NetworkingConfig != nil {
		if ep, ok := req.NetworkingConfig.EndpointsConfig[netName]; ok && ep != nil {
			container.NetworkSettings.Networks[netName].Aliases = ep.Aliases
		}
	}

	// Inject SOCKERLESS_DNS_SEARCH_DOMAIN so the bootstrap can append a
	// `search` line to /etc/resolv.conf and short-name lookups within the
	// network resolve. User's per-job override wins.
	if suffix, err := s.DNS.SearchDomain(s.ctx(), networkID); err == nil {
		if env := core.DNSSearchDomainEnvIfSet(config.Env, suffix); env != "" {
			config.Env = append(config.Env, env)
			container.Config.Env = config.Env
		}
	}

	// Pod association is handled by the core HTTP handler layer (query param).
	s.PendingCreates.Put(id, container)

	s.ACA.Put(id, ACAState{
		ResourceGroup: s.config.ResourceGroup,
	})

	s.EmitEvent("container", "create", id, map[string]string{
		"name":  strings.TrimPrefix(name, "/"),
		"image": config.Image,
	})

	return &api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	}, nil
}

// ContainerStart starts an ACA Job for the container.
func (s *Server) ContainerStart(ref string) error {
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
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	// Re-start of a kept-alive runner App: gitlab-runner cycles its predefined/
	// build helper start→wait→stop→start per stage on the SAME container. The
	// App was kept alive across the stop (see ContainerStop), so re-invoke the
	// next stage's script through the reverse-agent instead of re-deploying the
	// App (a slow ACR-Tasks redeploy that, repeated per stage, made the runner
	// loop). A fresh stdinPipe (registered by the per-stage attach) on a
	// container whose App is already deployed signals this. Mirrors cloudrun's
	// invokeRunningRunnerStage. A fresh wait channel scopes this stage.
	if s.config.UseApp && c.Config.OpenStdin {
		if _, hasPipe := s.stdinPipes.Load(id); hasPipe {
			if appState, ok := s.resolveAppACAState(s.ctx(), id); ok && appState.AppName != "" {
				exitCh := make(chan struct{})
				s.Store.WaitChs.Store(id, exitCh)
				s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
				go s.runACAInitialStdinStage(id, c)
				return nil
			}
		}
	}

	if c.State.Running {
		return &api.NotModifiedError{}
	}

	// markRunning emits the start event and sets up the wait channel.
	// Container state is no longer written to Store.Containers — the cloud is the truth.
	markRunning := func() chan struct{} {
		var exitCh chan struct{}
		if ch, ok := s.Store.WaitChs.Load(id); ok {
			exitCh = ch.(chan struct{})
		} else {
			exitCh = make(chan struct{})
			s.Store.WaitChs.Store(id, exitCh)
		}
		s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		return exitCh
	}

	exitCh := markRunning()

	// Deferred start: if container is in a multi-container pod, wait for all siblings
	shouldDefer, podContainers := s.PodDeferredStart(id)
	if shouldDefer {
		return nil
	}

	if len(podContainers) > 1 {
		// Multi-container pod: build combined resource and run.
		if s.config.UseApp {
			return s.startMultiContainerAppTyped(id, podContainers, exitCh)
		}
		return s.startMultiContainerJobTyped(id, podContainers, exitCh)
	}

	// — Apps path. Separate function so the Jobs branch
	// below can be deleted when Jobs support is sunset.
	if s.config.UseApp {
		acaState, _ := s.resolveAppACAState(s.ctx(), id)
		if err := s.startSingleContainerApp(id, c, acaState, exitCh); err != nil {
			return err
		}
		// Register the container's resolvable names (hostname + the
		// runner's `--network-alias` values) now that the App has an FQDN —
		// the create-with-network path never calls NetworkConnect.
		s.registerContainerServiceDiscovery(id, c)
		if c.Config.OpenStdin {
			if _, hasPipe := s.stdinPipes.Load(id); hasPipe {
				go s.runACAInitialStdinStage(id, c)
				return nil
			}
		}
		// Command container (e.g. gitlab-runner's cache-volume permission
		// container, whose command is `gitlab-runner-helper cache-init …`):
		// the overlay moved its user command into SOCKERLESS_USER_* env but
		// did NOT mark it to run at App startup (that marker, RUN_USER_WORKLOAD,
		// is set only for serviceLike service workloads). Such a container is
		// one the client `docker wait`s on, so run its command once via the
		// reverse-agent and close the wait channel; otherwise the App stays
		// alive only serving the agent and `docker wait` hangs. Long-lived
		// exec-driven job containers (e.g. a keepalive entrypoint) run their
		// command the same way — it stays up until the container is stopped,
		// which closes the wait channel then.
		if !c.Config.OpenStdin && acaCommandRunsViaAgent(c) {
			go s.runACAOneShotCommand(id, c)
			return nil
		}
		return s.waitForReverseAgentAfterStart(id, c.Config.OpenStdin)
	}

	// Build ACA Job spec
	jobName := buildJobName(id)
	jobSpec, err := s.buildJobSpec(s.ctx(), []containerInput{
		{ID: id, Container: &c, IsMain: true},
	})
	if err != nil {
		s.Store.WaitChs.Delete(id)
		return err
	}

	// Create the ACA Job
	createPoller, err := s.azure.Jobs.BeginCreateOrUpdate(s.ctx(), s.config.ResourceGroup, jobName, jobSpec, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to create ACA Job")

		s.Store.WaitChs.Delete(id)
		return azurecommon.MapAzureError(err, "job", id)
	}

	// Wait for job creation to complete
	_, err = createPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.deleteJob(jobName)

		s.Store.WaitChs.Delete(id)
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		return azurecommon.MapAzureError(err, "job", id)
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "aca",
		ResourceType: "job",
		ResourceID:   jobName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": c.Image, "name": c.Name, "jobName": jobName},
	})

	// Start the job (creates an execution)
	startPoller, err := s.azure.Jobs.BeginStart(s.ctx(), s.config.ResourceGroup, jobName, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to start ACA Job")
		s.deleteJob(jobName)

		s.Store.WaitChs.Delete(id)
		return azurecommon.MapAzureError(err, "execution", id)
	}

	// Wait for start to return execution info
	startResp, err := startPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("start job failed")
		s.deleteJob(jobName)

		s.Store.WaitChs.Delete(id)
		return azurecommon.MapAzureError(err, "execution", id)
	}

	// Remove from PendingCreates now that the job is launched in the cloud.
	s.PendingCreates.Delete(id)

	executionName := ""
	if startResp.Name != nil {
		executionName = *startResp.Name
	}

	s.ACA.Update(id, func(state *ACAState) {
		state.JobName = jobName
		state.ExecutionName = executionName
	})

	// Start background poller to detect execution exit
	go s.pollExecutionExit(id, jobName, executionName, exitCh)

	return nil
}

// startMultiContainerJobTyped creates and runs an ACA Job with all pod containers.
// Called when the last container in a pod is started.
func (s *Server) startMultiContainerJobTyped(triggerID string, podContainers []api.Container, exitCh chan struct{}) error {
	// Build containerInput slice
	var inputs []containerInput
	for i, pc := range podContainers {
		pcCopy := pc
		inputs = append(inputs, containerInput{
			ID:        pc.ID,
			Container: &pcCopy,
			IsMain:    i == 0,
		})
	}

	mainID := podContainers[0].ID

	// Build and create the combined job
	jobName := buildJobName(mainID)
	jobSpec, err := s.buildJobSpec(s.ctx(), inputs)
	if err != nil {
		return err
	}

	createPoller, err := s.azure.Jobs.BeginCreateOrUpdate(s.ctx(), s.config.ResourceGroup, jobName, jobSpec, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to create multi-container ACA Job")

		return azurecommon.MapAzureError(err, "job", mainID)
	}

	_, err = createPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.deleteJob(jobName)

		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		return azurecommon.MapAzureError(err, "job", mainID)
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainID,
		Backend:      "aca",
		ResourceType: "job",
		ResourceID:   jobName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": podContainers[0].Image, "name": podContainers[0].Name, "jobName": jobName},
	})

	startPoller, err := s.azure.Jobs.BeginStart(s.ctx(), s.config.ResourceGroup, jobName, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to start ACA Job")
		s.deleteJob(jobName)

		return azurecommon.MapAzureError(err, "execution", mainID)
	}

	startResp, err := startPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("start job failed")
		s.deleteJob(jobName)

		return azurecommon.MapAzureError(err, "execution", mainID)
	}

	// Remove all pod containers from PendingCreates now that the job is launched.
	for _, pc := range podContainers {
		s.PendingCreates.Delete(pc.ID)
	}

	executionName := ""
	if startResp.Name != nil {
		executionName = *startResp.Name
	}

	// Store cloud state on ALL pod containers
	for _, pc := range podContainers {
		s.ACA.Update(pc.ID, func(state *ACAState) {
			state.JobName = jobName
			state.ExecutionName = executionName
		})
	}

	// Start background poller to detect execution exit
	go s.pollExecutionExit(mainID, jobName, executionName, exitCh)

	return nil
}

// ContainerStop stops a running ACA container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// — for Apps, the ContainerApp IS the running instance; there's no
	// in-flight Execution to stop. Normally the App is deleted to stop the
	// container. EXCEPTION: gitlab-runner cycles its predefined/build helper
	// with start→wait→stop→start on the SAME container per stage; deleting the
	// App on each stop would force a slow ACR-Tasks redeploy every stage (and
	// the redeploy churn makes the runner loop). For these runner-pattern
	// stdin containers (OpenStdin), keep the App alive — the bootstrap's HTTP /
	// reverse-agent stays up to run the next stage's script (re-invoked from
	// ContainerStart), and final teardown happens in ContainerRemove. Mirrors
	// cloudrun's UseService runner-pattern exception. OpenStdin lives on the
	// original PendingCreates container (cloud-state reconstruction drops it).
	openStdin := false
	if pc, pcOK := s.PendingCreates.Get(id); pcOK {
		openStdin = pc.Config.OpenStdin
	}
	if s.config.UseApp {
		if openStdin {
			s.Logger.Info().Str("container", id).Msg("ContainerStop: OpenStdin runner-pattern — keeping App alive across stages")
		} else if appState, ok := s.resolveAppACAState(s.ctx(), id); ok && appState.AppName != "" {
			// A swallowed delete leaves the App running (billable) while
			// `docker stop` reports success — propagate like ContainerRemove.
			if err := s.deleteAppStrict(appState.AppName); err != nil {
				return &api.ServerError{Message: fmt.Sprintf("docker stop %s: delete Container App failed: %v", id, err)}
			}
			s.Registry.MarkCleanedUp(appState.AppName)
		}
	} else {
		// cloud-fallback lookup so stop works post-restart.
		if acaState, ok := s.resolveACAState(s.ctx(), id); ok && acaState.JobName != "" && acaState.ExecutionName != "" {
			s.stopExecution(acaState.JobName, acaState.ExecutionName)
		}
	}

	s.StopHealthCheck(id)
	s.Store.PutInvocationResult(id, core.InvocationResult{ExitCode: core.SignalToExitCode("SIGTERM")})

	// Close wait channel so ContainerWait unblocks
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	// `docker stop` is SIGTERM → exit 143.
	stopExitCode := core.SignalToExitCode("SIGTERM")
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", stopExitCode), "name": strings.TrimPrefix(c.Name, "/")})
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

	// — same as Stop: Apps delete, Jobs cancel execution.
	if s.config.UseApp {
		if appState, ok := s.resolveAppACAState(s.ctx(), id); ok && appState.AppName != "" {
			// A swallowed delete leaves the App running (billable) while
			// `docker kill` reports success — propagate like ContainerRemove.
			if err := s.deleteAppStrict(appState.AppName); err != nil {
				return &api.ServerError{Message: fmt.Sprintf("docker kill %s: delete Container App failed: %v", id, err)}
			}
			s.Registry.MarkCleanedUp(appState.AppName)
			s.ACA.Update(id, func(st *ACAState) { st.AppName = "" })
		}
	} else {
		// cloud-fallback lookup so kill works post-restart.
		if acaState, ok := s.resolveACAState(s.ctx(), id); ok && acaState.JobName != "" && acaState.ExecutionName != "" {
			s.stopExecution(acaState.JobName, acaState.ExecutionName)
		}
	}

	s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", exitCode), "name": strings.TrimPrefix(c.Name, "/")})

	// Record the kill outcome so `docker wait` after kill returns the
	// signal-derived code (e.g. 137 for SIGKILL) instead of -1.
	s.Store.PutInvocationResult(id, core.InvocationResult{ExitCode: exitCode})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	return nil
}

// ContainerRemove removes a container.
func (s *Server) ContainerRemove(ref string, force bool) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		// Also check PendingCreates (container created but never started)
		if pc, pcOK := s.PendingCreates.Get(ref); pcOK {
			c = pc
			ok = true
		}
	}
	if !ok && s.config.UseApp {
		if appState, appOK := s.resolveAppACAState(s.ctx(), ref); appOK && appState.AppName != "" {
			if err := s.deleteAppStrict(appState.AppName); err != nil {
				return err
			}
			s.Registry.MarkCleanedUp(appState.AppName)
			s.PendingCreates.Delete(ref)
			s.ACA.Delete(ref)
			if ch, ok := s.Store.WaitChs.LoadAndDelete(ref); ok {
				close(ch.(chan struct{}))
			}
			s.Store.LogBuffers.Delete(ref)
			s.Store.StagingDirs.Delete(ref)
			s.stdinPipes.Delete(ref)
			s.attachStreams.Delete(ref)
			return nil
		}
	}
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID
	if inv, ok := s.Store.GetInvocationResult(id); ok {
		c.State.Status = "exited"
		c.State.Running = false
		c.State.ExitCode = inv.ExitCode
	}

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
		if !s.config.UseApp {
			acaState, _ := s.resolveACAState(s.ctx(), id)
			if acaState.JobName != "" && acaState.ExecutionName != "" {
				s.stopExecution(acaState.JobName, acaState.ExecutionName)
			}
		}
	}

	s.StopHealthCheck(id)

	// — delete the backing cloud resource. Jobs and Apps are
	// distinct ARM resource types so cached state is unambiguous.
	// Errors propagate per the no-fallback rule.
	var cleanupErrs []error
	if s.config.UseApp {
		appState, _ := s.resolveAppACAState(s.ctx(), id)
		if appState.AppName != "" {
			if err := s.deleteAppStrict(appState.AppName); err != nil {
				cleanupErrs = append(cleanupErrs, err)
			}
			s.Registry.MarkCleanedUp(appState.AppName)
		}
	} else {
		acaState, _ := s.resolveACAState(s.ctx(), id)
		if acaState.JobName != "" {
			if err := s.deleteJobStrict(acaState.JobName); err != nil {
				cleanupErrs = append(cleanupErrs, err)
			}
			s.Registry.MarkCleanedUp(acaState.JobName)
		}
	}

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod {
		s.Store.Pods.RemoveContainer(pod.ID, id)
	}

	// Deregister from service discovery (CNAME for Apps, A for Jobs).
	hostname := strings.TrimPrefix(c.Name, "/")
	for _, ep := range c.NetworkSettings.Networks {
		if ep == nil || ep.NetworkID == "" {
			continue
		}
		// Route through the network-discovery driver. UseApp → CNAME,
		// else A-record.
		if cd, ok := s.NetworkDiscovery.(*azurecommon.PrivateDNSDiscovery); ok {
			if s.config.UseApp {
				_ = cd.DeregisterContainerCNAME(s.ctx(), ep.NetworkID, hostname)
			} else {
				_ = cd.DeregisterContainerARecord(s.ctx(), ep.NetworkID, hostname)
			}
		} else {
			_ = s.NetworkDiscovery.DeregisterContainer(s.ctx(), ep.NetworkID, hostname, id)
		}
	}

	// Clean up network associations
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			if derr := s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, id); derr != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("network %q disconnect: %w", ep.NetworkID, derr))
			}
		}
	}

	// Clean up PendingCreates (container may have been created but never started)
	s.PendingCreates.Delete(id)
	s.ACA.Delete(id)
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.Store.LogBuffers.Delete(id)
	s.Store.StagingDirs.Delete(id)
	s.stdinPipes.Delete(id)
	s.attachStreams.Delete(id)
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

// ContainerLogs streams container logs from Azure Monitor Log Analytics.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	id, _ := s.ResolveContainerIDAuto(context.Background(), ref)
	fetch := s.buildCloudLogsFetcher(id)
	return core.StreamCloudLogs(s.BaseServer, ref, opts, fetch, core.StreamCloudLogsOptions{})
}

// buildCloudLogsFetcher returns a CloudLogFetchFunc closure that
// queries Azure Monitor / Log Analytics. Filter depends on
// Config.UseApp: Jobs log under ContainerGroupName_s; Apps log under
// ContainerAppName_s in the same table. Shared by ContainerLogs +
// ContainerAttach + the typed Logs/Attach drivers.
func (s *Server) buildCloudLogsFetcher(id string) core.CloudLogFetchFunc {
	var whereClause string
	if s.config.UseApp {
		var appName string
		if id != "" {
			appState, _ := s.resolveAppACAState(s.ctx(), id)
			appName = appState.AppName
			if appName == "" {
				appName = buildAppName(id)
			}
		}
		whereClause = fmt.Sprintf(`ContainerAppName_s == "%s"`, appName)
	} else {
		var jobName string
		if id != "" {
			acaState, _ := s.resolveACAState(s.ctx(), id)
			jobName = acaState.JobName
			if jobName == "" {
				jobName = buildJobName(id)
			}
		}
		whereClause = fmt.Sprintf(`ContainerGroupName_s == "%s"`, jobName)
	}
	return s.azureLogsFetch(`ContainerAppConsoleLogs_CL`, whereClause, "Log_s")
}

// ContainerRestart stops and then starts a container.
func (s *Server) ContainerRestart(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	// Stop if running
	if c.State.Running {
		s.StopHealthCheck(id)

		acaState, _ := s.resolveACAState(s.ctx(), id)
		if acaState.JobName != "" && acaState.ExecutionName != "" {
			s.stopExecution(acaState.JobName, acaState.ExecutionName)
		}
		if acaState.JobName != "" {
			s.deleteJob(acaState.JobName)
			s.Registry.MarkCleanedUp(acaState.JobName)
		}
		// Clear stale ACA state so ContainerStart creates a fresh job.
		s.ACA.Update(id, func(state *ACAState) {
			state.JobName = ""
			state.ExecutionName = ""
		})
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

// ContainerPrune removes all stopped containers.
func (s *Server) ContainerPrune(filters map[string][]string) (*api.ContainerPruneResponse, error) {
	labelFilters := filters["label"]
	untilFilters := filters["until"]

	var deleted []string
	var spaceReclaimed uint64
	// Query all containers from CloudState (PendingCreates + Store.Containers)
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
		// Clean up ACA resources
		acaState, _ := s.resolveACAState(s.ctx(), c.ID)
		if acaState.JobName != "" {
			s.deleteJob(acaState.JobName)
			s.Registry.MarkCleanedUp(acaState.JobName)
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
		s.ACA.Delete(c.ID)
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

// ImagePull delegates to ImageManager for unified cloud image handling.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
}

// ImagePush delegates to ImageManager for unified cloud image handling.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	return s.images.Push(name, tag, auth)
}

// ImageTag delegates to ImageManager for unified cloud image handling.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	return s.images.Tag(source, repo, tag)
}

// ImageRemove delegates to ImageManager for unified cloud image handling.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	return s.images.Remove(name, force, prune)
}

// ImageBuild delegates to ImageManager for unified cloud image handling.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	return s.images.Build(opts, buildContext)
}

// ImageLoad delegates to ImageManager for unified cloud image handling.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return s.images.Load(r)
}

// VolumeRemove deletes the Azure Files share + managed-env storage
// resource backing a named volume. The storage account is left in
// place so other volumes keep working.
func (s *Server) VolumeRemove(name string, force bool) error {
	if name == "" {
		return &api.InvalidParameterError{Message: "volume name is required"}
	}
	if err := s.deleteShareForVolume(s.ctx(), name); err != nil {
		return &api.ServerError{Message: fmt.Sprintf("delete Azure Files share for %q: %v", name, err)}
	}
	return nil
}

// VolumePrune deletes every sockerless-managed Azure Files share that
// isn't currently referenced by a pending container's binds.
func (s *Server) VolumePrune(filters map[string][]string) (*api.VolumePruneResponse, error) {
	shares, err := s.listManagedShares(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("list managed Azure Files shares: %v", err)}
	}
	in := s.inUseVolumeNames()
	resp := &api.VolumePruneResponse{}
	for _, sh := range shares {
		name := azurecommon.ShareVolumeName(sh)
		if _, busy := in[name]; busy {
			continue
		}
		if err := s.deleteShareForVolume(s.ctx(), name); err != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("delete Azure Files share for %q: %v", name, err)}
		}
		resp.VolumesDeleted = append(resp.VolumesDeleted, name)
	}
	return resp, nil
}

// inUseVolumeNames returns the set of Docker volume names currently
// referenced by pending ACA jobs.
func (s *Server) inUseVolumeNames() map[string]struct{} {
	in := make(map[string]struct{})
	for _, c := range s.PendingCreates.List() {
		for _, b := range c.HostConfig.Binds {
			parts := strings.SplitN(b, ":", 3)
			if len(parts) >= 2 && !strings.HasPrefix(parts[0], "/") {
				in[parts[0]] = struct{}{}
			}
		}
	}
	return in
}
