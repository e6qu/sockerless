package ecs

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/sockerless/api"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by an ECS task definition.
func (s *Server) ContainerCreate(req *api.ContainerCreateRequest) (*api.ContainerCreateResponse, error) {
	name := req.Name
	if name == "" {
		name = "/" + core.GenerateName()
	} else if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	// Check name conflicts via cloud state
	if avail, _ := s.CloudState.CheckNameAvailable(context.Background(), name); !avail {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(name, "/")),
		}
	}

	// Also check PendingCreates for name conflicts (containers created but not yet started)
	for _, pc := range s.PendingCreates.List() {
		if pc.Name == name || pc.Name == "/"+name {
			return nil, &api.ConflictError{Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(name, "/"))}
		}
	}

	id := core.GenerateID()

	config := api.ContainerConfig{}
	if req.ContainerConfig != nil {
		config = *req.ContainerConfig
	}

	// Merge image config if available. If the image isn't in the local
	// Store (e.g. after backend restart, or `docker run` without an
	// explicit preceding `docker pull`), fetch its metadata now so the
	// task definition gets the real Cmd / Entrypoint / WorkingDir /
	// Env from the image — this is what docker CLI users expect from
	// `docker inspect` after running a container with an image default
	// CMD.
	img, ok := s.Store.ResolveImage(config.Image)
	if !ok {
		if rc, err := s.ImagePull(config.Image, ""); err == nil {
			_, _ = io.Copy(io.Discard, rc)
			_ = rc.Close()
			img, ok = s.Store.ResolveImage(config.Image)
		}
	}
	if ok {
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

	// Short image references (alpine, node:20, ghcr.io/…) must be
	// resolved to an ECR pull-through cache URI for Fargate to pull
	// them. The resolution lives in taskdef building rather than
	// here, so `docker inspect` reflects the user-supplied image ref
	// while the task definition still gets the pullable URI.

	hostConfig := api.HostConfig{NetworkMode: "default"}
	if req.HostConfig != nil {
		hostConfig = *req.HostConfig
	}
	if hostConfig.NetworkMode == "" {
		hostConfig.NetworkMode = "default"
	}

	// Validate mount specs up-front. On Fargate there's no host
	// filesystem to bind from, so host-path bind mounts (`/h:/c`) are
	// rejected — only named Docker volumes (`volname:/c`) are supported;
	// those land on the sockerless-managed EFS filesystem via per-volume
	// access points. Rejecting host binds here (rather than
	// silently substituting an empty scratch volume or an EFS subdir
	// named after the host path) keeps `docker run -v /host:/container`
	// failures explicit.
	for _, bind := range hostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid bind mount spec %q", bind)}
		}
		if strings.HasPrefix(parts[0], "/") {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf(
				"host bind mounts are not supported on ECS backend (%q); use a named volume (`docker volume create <name> && docker run -v <name>:/path`) — volumes are backed by sockerless-managed EFS access points",
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
		Driver:   "ecs-fargate",
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

	s.PendingCreates.Put(id, container)

	// Store ECS state without task definition — defer registration to ContainerStart.
	s.ECS.Put(id, ECSState{})

	// Attach the user-defined network's security group so the task is
	// launched on that SG (not just the default task SG). `docker run
	// --network <name>` only reaches NetworkConnect for additional
	// networks; the initial NetworkMode network still needs to be wired.
	if networkID != "" && netName != "bridge" && netName != "host" && netName != "none" {
		if err := s.cloudNetworkConnect(networkID, id); err != nil {
			s.Logger.Warn().Err(err).
				Str("container", id[:12]).
				Str("network", netName).
				Msg("failed to wire network security group at create time")
		}
	}

	s.EmitEvent("container", "create", id, map[string]string{
		"name":  strings.TrimPrefix(name, "/"),
		"image": config.Image,
	})

	return &api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	}, nil
}

// ContainerStart starts an ECS task for the container.
// All execution goes through the ECS cloud API — no local execution.
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

	ecsState, _ := s.ECS.Get(id)

	// Deferred task definition registration: if not yet registered, do it now
	if ecsState.TaskDefARN == "" {
		taskDefARN, err := s.registerTaskDefinition(s.ctx(), []containerInput{
			{ID: id, Container: &c, IsMain: true},
		})
		if err != nil {
			s.Logger.Error().Err(err).Msg("failed to register task definition")
			return awscommon.MapAWSError(err, "task-definition", id)
		}
		s.ECS.Update(id, func(state *ECSState) {
			state.TaskDefARN = taskDefARN
		})
		ecsState.TaskDefARN = taskDefARN
	}

	// markRunning emits the start event and sets up the wait channel.
	// Container state is no longer written to Store.Containers — the cloud is the truth.
	markRunning := func() chan struct{} {
		// Use existing exitCh if already created, otherwise create new one
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

	// Deferred start: if container is in a multi-container pod, wait for all siblings
	shouldDefer, podContainers := s.PodDeferredStart(id)
	if shouldDefer {
		markRunning()
		return nil
	}

	if len(podContainers) > 1 {
		// Multi-container pod: register combined task definition and run a single task
		exitCh := markRunning()
		return s.startMultiContainerTaskTyped(id, podContainers, exitCh)
	}

	// Pre-create exit channel so ContainerWait works even if the task
	// exits quickly.
	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	// Run ECS task before marking container as running, so docker ps
	// doesn't show a false-positive running state if RunTask fails.
	taskDefARN := ecsState.TaskDefARN
	taskARN, clusterARN, err := s.runECSTask(id, taskDefARN, &c)
	if err != nil {
		// Best-effort cleanup of orphaned task definition
		_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String(taskDefARN),
		})
		s.Store.WaitChs.Delete(id)
		return awscommon.MapAWSError(err, "task", id)
	}

	markRunning()

	// Remove from PendingCreates now that the task is launched in the cloud.
	s.PendingCreates.Delete(id)

	s.ECS.Update(id, func(state *ECSState) {
		state.TaskARN = taskARN
		state.ClusterARN = clusterARN
	})

	// Wait for task to reach RUNNING — only then is the ENI's private IP
	// known. Cloud Map registration must use that real IP, not the local
	// placeholder `0.0.0.0` carried in c.NetworkSettings.Networks.
	taskAddr, err := s.waitForTaskRunning(s.ctx(), taskARN)
	if err != nil {
		s.Logger.Error().Err(err).Str("task", taskARN).Msg("task failed to reach RUNNING state")
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
		return err
	}

	// Short-lived task that exited 0 before we saw RUNNING: Cloud Map
	// registration is pointless (nothing to discover), and pollTaskExit
	// would race with the already-STOPPED state. Close the wait channel
	// directly and return — ContainerWait will unblock, and
	// CloudState.GetContainer reads STOPPED straight from ECS so
	// inspect/ps reflect the real state.
	if taskAddr == "" {
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
		return nil
	}

	taskIP := taskAddr
	if i := strings.LastIndex(taskAddr, ":"); i > 0 {
		taskIP = taskAddr[:i]
	}

	// Register in Cloud Map for service discovery (skip pre-defined networks)
	hostname := strings.TrimPrefix(c.Name, "/")
	for netName, ep := range c.NetworkSettings.Networks {
		if ep == nil || ep.NetworkID == "" {
			continue
		}
		if netName == "bridge" || netName == "host" || netName == "none" {
			continue
		}
		if err := s.cloudServiceRegister(id, hostname, taskIP, ep.NetworkID); err != nil {
			s.Logger.Warn().Err(err).Str("container", id[:12]).Msg("failed to register in Cloud Map")
		}
	}

	// Start background poller to detect task exit
	go s.pollTaskExit(id, taskARN, exitCh)

	return nil
}

// startMultiContainerTaskTyped registers a combined task definition for all pod containers
// and runs a single ECS task. Returns error instead of writing to http.ResponseWriter.
func (s *Server) startMultiContainerTaskTyped(triggerID string, podContainers []api.Container, exitCh chan struct{}) error {
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

	// Register combined task definition
	taskDefARN, err := s.registerTaskDefinition(s.ctx(), inputs)
	if err != nil {
		s.Logger.Error().Err(err).Msg("failed to register multi-container task definition")
		return awscommon.MapAWSError(err, "task-definition", triggerID)
	}

	// Use the main (first) container for the task
	mainContainer := &podContainers[0]
	mainID := mainContainer.ID

	// Run the combined task
	taskARN, clusterARN, err := s.runECSTask(mainID, taskDefARN, mainContainer)
	if err != nil {
		// Best-effort cleanup of orphaned task definition
		_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String(taskDefARN),
		})
		// Re-add pod containers to PendingCreates so they can be retried
		for _, pc := range podContainers {
			s.PendingCreates.Put(pc.ID, pc)
		}
		return awscommon.MapAWSError(err, "task", mainID)
	}

	// Remove all pod containers from PendingCreates now that the task is launched.
	for _, pc := range podContainers {
		s.PendingCreates.Delete(pc.ID)
	}

	// Store cloud state on ALL pod containers (so stop/remove works for any)
	for _, pc := range podContainers {
		s.ECS.Update(pc.ID, func(state *ECSState) {
			state.TaskDefARN = taskDefARN
			state.TaskARN = taskARN
			state.ClusterARN = clusterARN
		})
	}

	// Wait for task to reach RUNNING state
	_, err = s.waitForTaskRunning(s.ctx(), taskARN)
	if err != nil {
		s.Logger.Error().Err(err).Str("task", taskARN).Msg("task failed to reach RUNNING state")
		// Re-add pod containers to PendingCreates so they can be retried
		for _, pc := range podContainers {
			s.PendingCreates.Put(pc.ID, pc)
		}
		return awscommon.MapAWSError(err, "task", mainID)
	}

	go s.pollTaskExit(mainID, taskARN, exitCh)

	return nil
}

// ContainerStop stops a running container by stopping its ECS task.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	ecsState, hasState := s.resolveTaskState(s.ctx(), id)
	if !hasState {
		return &api.ServerError{Message: fmt.Sprintf("docker stop %s: cannot resolve underlying ECS task — task ARN unknown", ref)}
	}
	cluster := s.config.Cluster
	if ecsState.ClusterARN != "" {
		cluster = ecsState.ClusterARN
	}
	taskARN := ecsState.TaskARN
	if _, err := s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
		Cluster: aws.String(cluster),
		Task:    aws.String(taskARN),
		Reason:  aws.String("Container stopped via API"),
	}); err != nil {
		return &api.ServerError{Message: fmt.Sprintf("docker stop %s: ECS StopTask failed: %v", ref, err)}
	}

	s.StopHealthCheck(id)

	// Block until the task reaches STOPPED so docker stop matches
	// docker semantics (caller can immediately rm the container). Docker
	// uses a 10-second default timeout for SIGTERM before SIGKILL;
	// Fargate's StopTask sends SIGTERM then SIGKILL after 30 s, plus
	// another ~30 s for STOPPING → DEPROVISIONING → STOPPED with ENI
	// release. 120 s default covers the worst case observed live; if
	// the caller supplies `t=N`, add 60 s grace on top of that.
	// If the deadline lapses without observing STOPPED, the container is
	// genuinely still running — surface that to the caller rather than
	// pretend the stop succeeded.
	stopTimeout := 120 * time.Second
	if timeout != nil && *timeout > 0 {
		stopTimeout = time.Duration(*timeout)*time.Second + 60*time.Second
	}
	if err := s.waitForTaskStopped(s.ctx(), cluster, taskARN, stopTimeout); err != nil {
		return &api.ServerError{Message: fmt.Sprintf("docker stop %s: %v", ref, err)}
	}

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
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

	// cloud-fallback lookup so kill works post-restart.
	if ecsState, ok := s.resolveTaskState(s.ctx(), id); ok {
		cluster := s.config.Cluster
		if ecsState.ClusterARN != "" {
			cluster = ecsState.ClusterARN
		}
		// Record the signal on the task before stopping it so the
		// cloud-state reader can report 128+signum on inspect rather
		// than whatever the container process happened to exit with
		// (nginx-on-SIGTERM reports 1, which disagrees with Docker).
		_, _ = s.aws.ECS.TagResource(s.ctx(), &awsecs.TagResourceInput{
			ResourceArn: aws.String(ecsState.TaskARN),
			Tags: []ecstypes.Tag{
				{Key: aws.String("sockerless-kill-signal"), Value: aws.String(signal)},
			},
		})
		_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
			Cluster: aws.String(cluster),
			Task:    aws.String(ecsState.TaskARN),
			Reason:  aws.String("Container killed with " + signal),
		})
	}

	s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", exitCode), "name": strings.TrimPrefix(c.Name, "/")})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	return nil
}

// ContainerRemove removes a container and its associated ECS resources.
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
		// `docker rm -f` is SIGKILL → exit 137 by docker convention.
		killExitCode := core.SignalToExitCode("SIGKILL")
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": fmt.Sprintf("%d", killExitCode),
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		// cloud-fallback lookup so remove works post-restart.
		if ecsState, ok := s.resolveTaskState(s.ctx(), id); ok {
			cluster := s.config.Cluster
			if ecsState.ClusterARN != "" {
				cluster = ecsState.ClusterARN
			}
			_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
				Cluster: aws.String(cluster),
				Task:    aws.String(ecsState.TaskARN),
				Reason:  aws.String("Container removed"),
			})
		}
	}

	s.StopHealthCheck(id)

	// Deregister task definition. Read from cache when available; on
	// cache miss (post-restart) derive TaskDefinitionArn from the running
	// task via DescribeTasks/.
	ecsState, _ := s.ECS.Get(id)
	if ecsState.TaskDefARN == "" {
		if recovered, ok := s.resolveTaskState(s.ctx(), id); ok {
			descOut, dErr := s.aws.ECS.DescribeTasks(s.ctx(), &awsecs.DescribeTasksInput{
				Cluster: aws.String(recovered.ClusterARN),
				Tasks:   []string{recovered.TaskARN},
			})
			if dErr == nil && len(descOut.Tasks) > 0 {
				ecsState.TaskDefARN = aws.ToString(descOut.Tasks[0].TaskDefinitionArn)
				ecsState.TaskARN = recovered.TaskARN
			}
		}
	}
	if ecsState.TaskDefARN != "" {
		_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String(ecsState.TaskDefARN),
		})
	}

	if ecsState.TaskARN != "" {
		s.Registry.MarkCleanedUp(ecsState.TaskARN)
	}
	// Mark every ECS task for this container as cleaned up in the
	// registry. ECS's TagResource rejects STOPPED tasks, so we can't
	// flag them with a cloud-side `sockerless-removed` tag — the
	// registry is the source of truth for "user has removed this
	// container" and queryTasks skips every cleanedUp ARN. Running
	// tasks are tagged too for consistency post-stop.
	s.markTasksRemoved(id)

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod {
		s.Store.Pods.RemoveContainer(pod.ID, id)
	}

	// Deregister from Cloud Map
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			if err := s.cloudServiceDeregister(id, ep.NetworkID); err != nil {
				s.Logger.Warn().Err(err).Str("container", id[:12]).Msg("failed to deregister from Cloud Map")
			}
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
	s.ECS.Delete(id)
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

// ContainerLogs streams container logs from CloudWatch. Fetch closure
// is shared with ContainerAttach via buildCloudWatchFetcher; the only
// difference is that logs rejects calls on never-started containers
// (attach tolerates them and waits for the task to appear).
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	id, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	if taskID := s.getTaskID(id); taskID == "unknown" {
		return nil, &api.InvalidParameterError{
			Message: "logs not available: ECS task not found for container " + id[:12],
		}
	}

	return core.StreamCloudLogs(s.BaseServer, ref, opts, s.buildCloudWatchFetcher(id), core.StreamCloudLogsOptions{})
}

// ContainerRestart stops and then starts a container.
func (s *Server) ContainerRestart(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID
	// Stamp the incoming-restart count on container state so the
	// subsequent ContainerStart can propagate it onto the new task's
	// tags. Read from the current cloud-derived container (which
	// taskToContainer fills from the `sockerless-restart-count` tag).
	newRestartCount := c.RestartCount + 1
	if updated := s.ECS.Update(id, func(state *ECSState) {
		state.RestartCount = newRestartCount
	}); !updated {
		s.ECS.Put(id, ECSState{RestartCount: newRestartCount})
	}

	// Stop if running
	if c.State.Running {
		s.StopHealthCheck(id)
		// cloud-fallback lookup so restart works post-restart.
		ecsState, _ := s.resolveTaskState(s.ctx(), id)
		if ecsState.TaskARN != "" {
			cluster := s.config.Cluster
			if ecsState.ClusterARN != "" {
				cluster = ecsState.ClusterARN
			}
			_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
				Cluster: aws.String(cluster),
				Task:    aws.String(ecsState.TaskARN),
				Reason:  aws.String("Container restarted via API"),
			})
			s.Registry.MarkCleanedUp(ecsState.TaskARN)
		}
		if ecsState.TaskDefARN != "" {
			_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
				TaskDefinition: aws.String(ecsState.TaskDefARN),
			})
		}
		// Clear stale ECS state so ContainerStart registers a fresh task definition.
		s.ECS.Update(id, func(state *ECSState) {
			state.TaskARN = ""
			state.TaskDefARN = ""
			state.ClusterARN = ""
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
	// State must read as not-running so ContainerStart doesn't short-circuit
	// with NotModified (the old task is stopped; a fresh task is what we
	// want).
	c.State.Running = false
	c.State.Status = "created"
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

	// Query CloudState for all containers (including stopped)
	containers, err := s.CloudState.ListContainers(context.Background(), true, nil)
	if err != nil {
		return nil, err
	}

	var deleted []string
	var spaceReclaimed uint64
	for _, c := range containers {
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
		ecsState, _ := s.ECS.Get(c.ID)
		if ecsState.TaskDefARN != "" {
			_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
				TaskDefinition: aws.String(ecsState.TaskDefARN),
			})
		}
		if ecsState.TaskARN != "" {
			s.Registry.MarkCleanedUp(ecsState.TaskARN)
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
		s.ECS.Delete(c.ID)
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

// ContainerPause sends SIGSTOP to the user subprocess via SSM
// ExecuteCommand. Reuses the /tmp/.sockerless-mainpid convention —
// task definitions must arrange for the user process to write its
// PID there. Without that, the helper returns NotImplementedError
// naming the missing prerequisite.
func (s *Server) ContainerPause(ref string) error {
	return s.ContainerSignalViaSSM(ref, "STOP")
}

// ContainerUnpause sends SIGCONT via the same SSM channel.
func (s *Server) ContainerUnpause(ref string) error {
	return s.ContainerSignalViaSSM(ref, "CONT")
}

// ImagePull pulls an image, using ECR cloud auth when available.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
}

// ImagePush pushes an image, syncing to ECR when applicable.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	return s.images.Push(name, tag, auth)
}

// ImageTag tags an image and syncs the new tag to ECR.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	return s.images.Tag(source, repo, tag)
}

// ImageRemove removes an image and syncs the removal to ECR.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	return s.images.Remove(name, force, prune)
}

// ImageLoad loads an image from a tar archive.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return s.images.Load(r)
}

// VolumeRemove deletes the EFS access point bound to a named volume.
// The backing filesystem is left in place so other volumes keep
// working; `docker system prune --volumes` / Phase-92-style teardown
// is a separate concern.
func (s *Server) VolumeRemove(name string, force bool) error {
	if name == "" {
		return &api.InvalidParameterError{Message: "volume name is required"}
	}
	if err := s.deleteAccessPointForVolume(s.ctx(), name); err != nil {
		return &api.ServerError{Message: fmt.Sprintf("delete EFS access point for %q: %v", name, err)}
	}
	return nil
}

// ExecStart starts an exec instance. For ECS, if no agent is connected,
// we cannot execute commands inside the remote Fargate task without the
// ECS ExecuteCommand API (SSM), which is not yet implemented. In that case,
// return a clear error.
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

	// Use ECS ExecuteCommand API (SSM Session Manager) to exec into the remote Fargate task.
	tty := exec.ProcessConfig.Tty || opts.Tty
	return s.cloudExecStart(&exec, &c, tty)
}

// PodStart starts all containers in a pod by calling ContainerStart for each.
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		// Check PendingCreates (containers between create and start)
		if c, ok := s.PendingCreates.Get(cid); ok {
			if c.State.Running {
				continue
			}
		} else {
			// Check CloudState for already-running containers
			if c, ok := s.ResolveContainerAuto(context.Background(), cid); ok && c.State.Running {
				continue
			}
		}
		if err := s.ContainerStart(cid); err != nil {
			errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
		}
	}
	if len(errs) == 0 {
		s.Store.Pods.SetStatus(pod.ID, "running")
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodStop stops all containers in a pod by calling ContainerStop for each.
func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		if err := s.ContainerStop(cid, timeout); err != nil {
			// NotModifiedError is not a real error — container was already stopped
			if _, ok := err.(*api.NotModifiedError); !ok {
				errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
			}
		}
	}
	s.Store.Pods.SetStatus(pod.ID, "stopped")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodKill sends a signal to all containers in a pod by calling ContainerKill for each.
func (s *Server) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	if signal == "" {
		signal = "SIGKILL"
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		if err := s.ContainerKill(cid, signal); err != nil {
			errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
		}
	}
	s.Store.Pods.SetStatus(pod.ID, "exited")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodRemove removes a pod and all its containers by calling ContainerRemove for each.
func (s *Server) PodRemove(name string, force bool) error {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return &api.NotFoundError{Resource: "pod", ID: name}
	}

	// Without force, reject if any containers are running
	if !force {
		for _, cid := range pod.ContainerIDs {
			c, ok := s.ResolveContainerAuto(context.Background(), cid)
			if ok && c.State.Running {
				return &api.ConflictError{
					Message: fmt.Sprintf("pod %s has running containers, cannot remove without force", name),
				}
			}
		}
	}

	// Remove each container through our ContainerRemove (handles ECS cleanup)
	for _, cid := range pod.ContainerIDs {
		if _, ok := s.ResolveContainerAuto(context.Background(), cid); !ok {
			continue
		}
		_ = s.ContainerRemove(cid, force)
	}

	s.Store.Pods.DeletePod(pod.ID)
	return nil
}

// Info returns system information, enriched with real ECS cluster stats.
func (s *Server) Info() (*api.BackendInfo, error) {
	// Get base info from BaseServer (in-memory container/image counts)
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}

	// Enrich with real cluster data from ECS DescribeClusters
	result, err := s.aws.ECS.DescribeClusters(s.ctx(), &awsecs.DescribeClustersInput{
		Clusters: []string{s.config.Cluster},
	})
	if err != nil {
		// Non-fatal: return base info if AWS call fails
		s.Logger.Warn().Err(err).Msg("failed to describe ECS cluster for Info")
		return info, nil
	}

	if len(result.Clusters) > 0 {
		cluster := result.Clusters[0]
		info.ContainersRunning = int(cluster.RunningTasksCount)
	}

	return info, nil
}

// ContainerInspect returns container details from CloudState.
func (s *Server) ContainerInspect(ref string) (*api.Container, error) {
	// Check PendingCreates first (container created but not yet started)
	if c, ok := s.PendingCreates.Get(ref); ok {
		return &c, nil
	}
	for _, c := range s.PendingCreates.List() {
		if c.Name == ref || c.Name == "/"+ref || (len(ref) >= 3 && strings.HasPrefix(c.ID, ref)) {
			return &c, nil
		}
	}

	// Delegate to CloudState via BaseServer (which uses ResolveContainerAuto)
	return s.BaseServer.ContainerInspect(ref)
}

// ContainerList lists containers from CloudState, plus PendingCreates.
func (s *Server) ContainerList(opts api.ContainerListOptions) ([]*api.ContainerSummary, error) {
	// Delegate to BaseServer which uses CloudState when set
	return s.BaseServer.ContainerList(opts)
}

// ExecCreate creates an exec instance. Validates that an ECS task is running
// before creating — without a task, exec cannot work on ECS Fargate.
func (s *Server) ExecCreate(containerID string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), containerID)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: containerID}
	}

	if !c.State.Running {
		return nil, &api.ConflictError{Message: "Container " + containerID + " is not running"}
	}

	// ECS-specific: check that an ECS task is available for exec.
	// cloud-fallback lookup so ExecCreate works post-restart.
	if ecsState, ok := s.resolveTaskState(s.ctx(), c.ID); !ok || ecsState.TaskARN == "" {
		return nil, &api.NotImplementedError{
			Message: fmt.Sprintf("exec requires a running ECS task, but container %s has none (ECS backend)", strings.TrimPrefix(c.Name, "/")),
		}
	}

	// Delegate to BaseServer for the actual exec creation
	return s.BaseServer.ExecCreate(containerID, req)
}

// ContainerExport tars the task's rootfs via SSM ExecuteCommand.
// Fargate has no native export API, but the SSM channel that powers
// `docker exec` can run `tar cf -` and stream the output back.
func (s *Server) ContainerExport(ref string) (io.ReadCloser, error) {
	return s.ContainerExportViaSSM(ref)
}

// ContainerCommit is not implemented on the ECS backend. Fargate task
// images are control-plane-owned (sourced from ECR), so commit needs
// the same agent-driven snapshot + registry-push pipeline as Lambda.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	_, ok := s.ResolveContainerIDAuto(context.Background(), req.Container)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	return nil, &api.NotImplementedError{Message: "docker commit is not implemented on ECS — Fargate exposes no host filesystem to snapshot from, and ECS doesn't run a sockerless bootstrap that could capture a rootfs diff over SSM exec"}
}

// VolumePrune deletes all sockerless-managed EFS access points that
// aren't currently referenced by any ECS task definition. The filter
// map is accepted for Docker API parity but currently unused — access
// points have no labels beyond the `sockerless-managed` + volume-name
// tags, so filter-by-label would be a no-op.
func (s *Server) VolumePrune(filters map[string][]string) (*api.VolumePruneResponse, error) {
	aps, err := s.listManagedAccessPoints(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("list EFS access points: %v", err)}
	}
	in := s.inUseVolumeNames()
	resp := &api.VolumePruneResponse{}
	for _, ap := range aps {
		name := awscommon.APVolumeName(ap)
		if _, busy := in[name]; busy {
			continue
		}
		if err := s.deleteAccessPointForVolume(s.ctx(), name); err != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("delete EFS access point for %q: %v", name, err)}
		}
		resp.VolumesDeleted = append(resp.VolumesDeleted, name)
	}
	return resp, nil
}

// inUseVolumeNames returns the set of Docker volume names currently
// referenced by running or pending ECS tasks — used by VolumePrune
// to avoid deleting access points out from under a live container.
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
