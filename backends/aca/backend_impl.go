package aca

import (
	"context"
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

	hostConfig := api.HostConfig{NetworkMode: "default"}
	if req.HostConfig != nil {
		hostConfig = *req.HostConfig
	}
	if hostConfig.NetworkMode == "" {
		hostConfig.NetworkMode = "default"
	}

	// Named-volume binds (`volName:/mnt`) map to Azure Files shares
	// provisioned by VolumeCreate; see volumes.go. Host-path binds
	// (`/h:/c`) stay rejected — ACA containers have no host filesystem.
	for _, bind := range hostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid bind mount spec %q", bind)}
		}
		if strings.HasPrefix(parts[0], "/") {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf(
				"host bind mounts are not supported on ACA backend (%q); use a named volume (`docker volume create <name> && docker run -v <name>:/path`) — volumes are backed by sockerless-managed Azure Files shares",
				bind,
			)}
		}
	}

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
	container.NetworkSettings.Networks[netName] = &api.EndpointSettings{
		NetworkID:   networkID,
		EndpointID:  core.GenerateID()[:16],
		Gateway:     "",
		IPAddress:   "",
		IPPrefixLen: 16,
		MacAddress:  "",
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
		return s.startSingleContainerApp(id, c, acaState, exitCh)
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

	// — for Apps, the ContainerApp IS the running instance;
	// there's no in-flight Execution to stop. Delete the App to stop
	// the container; next Start re-creates it.
	if s.config.UseApp {
		if appState, ok := s.resolveAppACAState(s.ctx(), id); ok && appState.AppName != "" {
			s.deleteApp(appState.AppName)
			s.Registry.MarkCleanedUp(appState.AppName)
			s.ACA.Update(id, func(st *ACAState) { st.AppName = "" })
		}
	} else {
		// cloud-fallback lookup so stop works post-restart.
		if acaState, ok := s.resolveACAState(s.ctx(), id); ok && acaState.JobName != "" && acaState.ExecutionName != "" {
			s.stopExecution(acaState.JobName, acaState.ExecutionName)
		}
	}

	s.StopHealthCheck(id)

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
			s.deleteApp(appState.AppName)
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
	if s.config.UseApp {
		appState, _ := s.resolveAppACAState(s.ctx(), id)
		if appState.AppName != "" {
			s.deleteApp(appState.AppName)
			s.Registry.MarkCleanedUp(appState.AppName)
		}
	} else {
		acaState, _ := s.resolveACAState(s.ctx(), id)
		if acaState.JobName != "" {
			s.deleteJob(acaState.JobName)
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
		if s.config.UseApp {
			_ = s.cloudServiceDeregisterCNAME(s.ctx(), id, hostname, ep.NetworkID)
		} else {
			_ = s.cloudServiceDeregister(id, hostname, ep.NetworkID)
		}
	}

	// Clean up network associations
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, id)
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
	if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(id); ok {
		for _, d := range dirs.([]string) {
			os.RemoveAll(d)
		}
	}
	for _, eid := range c.ExecIDs {
		s.Store.Execs.Delete(eid)
	}

	s.EmitEvent("container", "destroy", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerLogs streams container logs from Azure Monitor Log Analytics.
// Filter depends on Config.UseApp: Jobs log under ContainerGroupName_s;
// Apps log under ContainerAppName_s in the same table.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	id, _ := s.ResolveContainerIDAuto(context.Background(), ref)

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

	fetch := s.azureLogsFetch(`ContainerAppConsoleLogs_CL`, whereClause, "Log_s")
	return core.StreamCloudLogs(s.BaseServer, ref, opts, fetch, core.StreamCloudLogsOptions{})
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
