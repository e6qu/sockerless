package cloudrun

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/logging/logadmin"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by a Cloud Run Job.
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

	// Merge image config if available + resolve digest refs to RepoTag
	// (BUG-918: gitlab-runner uses image ID `sha256:<digest>` for create
	// after pull-by-tag; Cloud Run rejects bare digest refs because it
	// rewrites them to `mirror.gcr.io/library/sha256:<digest>` which 404s.
	// Replace `sha256:<digest>` with the first RepoTag from the local
	// Store entry — the image was pulled by tag, so a tag is available).
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
		if strings.HasPrefix(config.Image, "sha256:") && len(img.RepoTags) > 0 {
			config.Image = img.RepoTags[0]
		}
	}
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	// Resolve Docker Hub images to Artifact Registry remote repository URIs
	config.Image = gcpcommon.ResolveGCPImageURI(config.Image, s.config.Project, s.config.Region)

	// Phase 122g: when BootstrapBinaryPath is configured, COPY the
	// sockerless-cloudrun-bootstrap into the user's image via Cloud
	// Build and use the overlay URI as the actual image for Cloud Run.
	// This makes the deployed Service host an HTTP endpoint that
	// ContainerExec can POST envelope payloads against (Path B). Cache
	// hits via OverlayContentTag mean the second container of any given
	// (image, entrypoint, cmd, workdir) tuple skips Cloud Build entirely.
	originalImage := config.Image
	if s.useOverlayPath(originalImage) {
		spec := gcpcommon.OverlayImageSpec{
			BaseImageRef:        originalImage,
			BootstrapBinaryPath: s.config.BootstrapBinaryPath,
			UserEntrypoint:      config.Entrypoint,
			UserCmd:             config.Cmd,
			UserWorkdir:         config.WorkingDir,
		}
		contentTag := gcpcommon.OverlayContentTag("cloudrun-", spec)
		overlayURI, err := s.ensureOverlayImage(s.ctx(), spec, contentTag)
		if err != nil {
			return nil, fmt.Errorf("ensure cloudrun overlay image: %w", err)
		}
		config.Image = overlayURI
		// Bootstrap owns the entrypoint; it parses SOCKERLESS_USER_*
		// env vars (baked into the overlay at build time) on each
		// invocation. Drop the user's entrypoint+cmd from the Cloud
		// Run container spec so Cloud Run doesn't re-override the
		// bootstrap with the user's argv.
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

	// Named-volume binds (`volName:/mnt`) map to Cloud Run
	// `Volume{Gcs{Bucket}}` on the sockerless-owned project. Host-path
	// binds (`/h:/c`) translate via SharedVolumes (config-driven map
	// from caller-side mount path → sockerless-managed named-volume +
	// GCS bucket). Mirrors the ECS + Lambda translators.
	translatedBinds := make([]string, 0, len(hostConfig.Binds))
	for _, bind := range hostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid bind mount spec %q", bind)}
		}
		src, dst := parts[0], parts[1]
		mode := ""
		if len(parts) == 3 {
			mode = parts[2]
		}
		// /var/run/docker.sock — silently dropped (no docker socket
		// on Cloud Run; the github-runner adds this unconditionally).
		if src == "/var/run/docker.sock" {
			continue
		}
		if strings.HasPrefix(src, "/") {
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
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf(
				"host bind mounts are not supported on Cloud Run backend (%q); use a named volume (`docker volume create <name> && docker run -v <name>:/path`) — volumes are backed by sockerless-managed GCS buckets. Configure SOCKERLESS_GCP_SHARED_VOLUMES to translate runner-task bind mounts to shared GCS buckets.",
				bind,
			)}
		}
		// Already a named volume — pass through.
		translatedBinds = append(translatedBinds, bind)
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
		Driver:   "cloudrun-jobs",
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

	s.CloudRun.Put(id, CloudRunState{})

	s.EmitEvent("container", "create", id, map[string]string{
		"name":  strings.TrimPrefix(name, "/"),
		"image": config.Image,
	})

	return &api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	}, nil
}

// ContainerStart starts a Cloud Run Job for the container.
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
		// BUG-922 fix: gitlab-runner does start→wait→stop→start cycling
		// per stage on the SAME container ID. After first ContainerStart
		// PendingCreates is cleared, so subsequent restarts must look up
		// via CloudState. Re-add to PendingCreates so the existing flow
		// below can re-create the Cloud Run Job (with potentially new cmd).
		if got, hit := s.ResolveContainerAuto(s.ctx(), ref); hit {
			c = got
			ok = true
			s.PendingCreates.Put(c.ID, c)
		}
	}
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if c.State.Running {
		return &api.NotModifiedError{}
	}

	crState, _ := s.resolveCloudRunState(s.ctx(), id)

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Deferred start: if container is in a multi-container pod, wait for all siblings
	shouldDefer, podContainers := s.PodDeferredStart(id)
	if shouldDefer {
		return nil
	}

	if len(podContainers) > 1 {
		// Multi-container pod: build combined resource and run
		if s.config.UseService && isRunnerPattern(&c) {
			return s.startMultiContainerServiceTyped(id, podContainers, exitCh)
		}
		return s.startMultiContainerJobTyped(id, podContainers, exitCh)
	}

	// Phase 122f: Service path ONLY for runner-pattern (long-lived
	// containers that bind $PORT or run `tail -f /dev/null`-style
	// hold-open commands). Permission containers (one-shot chown,
	// alpine `echo hello`, etc.) go via Cloud Run Job — they exit
	// immediately and don't bind $PORT, which Cloud Run Service
	// would reject as failed-startup.
	if s.config.UseService && isRunnerPattern(&c) {
		return s.startSingleContainerService(id, c, crState, exitCh)
	}

	// Clean up any existing Cloud Run Job from a previous start
	if crState.JobName != "" {
		s.deleteJob(crState.JobName)
		s.Registry.MarkCleanedUp(crState.JobName)
	}

	// Build Cloud Run Job spec
	jobName := buildJobName(id)
	jobSpec, err := s.buildJobSpec(s.ctx(), []containerInput{
		{ID: id, Container: &c, IsMain: true},
	})
	if err != nil {
		s.Store.WaitChs.Delete(id)
		return err
	}

	// Create the Cloud Run Job
	createOp, err := s.gcp.Jobs.CreateJob(s.ctx(), &runpb.CreateJobRequest{
		Parent: s.buildJobParent(),
		JobId:  jobName,
		Job:    jobSpec,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to create Cloud Run Job")
		s.Store.WaitChs.Delete(id)
		return gcpcommon.MapGCPError(err, "job", id)
	}

	// Wait for job creation to complete
	job, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteJob(fmt.Sprintf("%s/jobs/%s", s.buildJobParent(), jobName))
		s.Store.WaitChs.Delete(id)
		s.PendingCreates.Delete(id)
		s.CloudRun.Delete(id)
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		return gcpcommon.MapGCPError(err, "job", id)
	}

	jobFullName := job.Name

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "cloudrun",
		ResourceType: "job",
		ResourceID:   jobFullName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": c.Image, "name": c.Name, "jobName": jobName},
	})

	// Run the job (creates an execution)
	runOp, err := s.gcp.Jobs.RunJob(s.ctx(), &runpb.RunJobRequest{
		Name: jobFullName,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("failed to run job")
		s.deleteJob(jobFullName)
		s.Store.WaitChs.Delete(id)
		s.PendingCreates.Delete(id)
		s.CloudRun.Delete(id)
		return gcpcommon.MapGCPError(err, "execution", id)
	}

	// BUG-921: do NOT runOp.Wait — that blocks until execution
	// COMPLETES (~10-30 min for real CI workloads), holding the docker
	// /start HTTP handler open and tripping gitlab-runner's 120s docker
	// connection timeout. Instead, extract the execution name from the
	// operation's metadata (populated as soon as RunJob is accepted).
	// Same shape fix as BUG-912 in github-runner-dispatcher-gcp/spawner.
	executionName := ""
	if md, mdErr := runOp.Metadata(); mdErr == nil && md != nil {
		executionName = md.Name
	}
	if executionName == "" {
		// Fallback: list executions on the job + take the most recent.
		// Operation metadata may be empty for very fast initial calls.
		it := s.gcp.Executions.ListExecutions(s.ctx(), &runpb.ListExecutionsRequest{
			Parent: jobFullName,
		})
		if e, err := it.Next(); err == nil && e != nil {
			executionName = e.Name
		}
	}
	if executionName == "" {
		s.Logger.Warn().Str("job", jobFullName).Msg("RunJob accepted but execution name not yet available; pollExecutionExit will rediscover")
	}

	// Remove from PendingCreates now that the job is launched in the cloud.
	s.PendingCreates.Delete(id)

	s.CloudRun.Update(id, func(state *CloudRunState) {
		state.JobName = jobFullName
		state.ExecutionName = executionName
	})

	// Start background poller to detect execution exit
	go s.pollExecutionExit(id, executionName, exitCh)

	return nil
}

// startMultiContainerJobTyped creates and runs a Cloud Run Job with all pod containers.
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

	createOp, err := s.gcp.Jobs.CreateJob(s.ctx(), &runpb.CreateJobRequest{
		Parent: s.buildJobParent(),
		JobId:  jobName,
		Job:    jobSpec,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to create multi-container Cloud Run Job")
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		return gcpcommon.MapGCPError(err, "job", mainID)
	}

	job, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteJob(fmt.Sprintf("%s/jobs/%s", s.buildJobParent(), jobName))
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		return gcpcommon.MapGCPError(err, "job", mainID)
	}

	jobFullName := job.Name

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainID,
		Backend:      "cloudrun",
		ResourceType: "job",
		ResourceID:   jobFullName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": podContainers[0].Image, "name": podContainers[0].Name, "jobName": jobName},
	})

	runOp, err := s.gcp.Jobs.RunJob(s.ctx(), &runpb.RunJobRequest{
		Name: jobFullName,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("failed to run job")
		s.deleteJob(jobFullName)
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		return gcpcommon.MapGCPError(err, "execution", mainID)
	}

	// BUG-921: extract execution name from operation metadata, don't
	// block on Wait — see single-container path above for full rationale.
	executionName := ""
	if md, mdErr := runOp.Metadata(); mdErr == nil && md != nil {
		executionName = md.Name
	}
	if executionName == "" {
		it := s.gcp.Executions.ListExecutions(s.ctx(), &runpb.ListExecutionsRequest{
			Parent: jobFullName,
		})
		if e, err := it.Next(); err == nil && e != nil {
			executionName = e.Name
		}
	}

	// Remove all pod containers from PendingCreates now that the job is launched.
	for _, pc := range podContainers {
		s.PendingCreates.Delete(pc.ID)
	}

	// Store cloud state on ALL pod containers
	for _, pc := range podContainers {
		s.CloudRun.Update(pc.ID, func(state *CloudRunState) {
			state.JobName = jobFullName
			state.ExecutionName = executionName
		})
	}

	// Start background poller to detect execution exit
	go s.pollExecutionExit(mainID, executionName, exitCh)

	return nil
}

// ContainerStop stops a running Cloud Run container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// — for Services, the Service resource IS the running
	// instance; there's no in-flight Execution to cancel. Delete the
	// Service to stop the container. Restart re-creates via
	// CreateService in the next ContainerStart.
	if s.config.UseService {
		if svcState, ok := s.resolveServiceCloudRunState(s.ctx(), id); ok && svcState.ServiceName != "" {
			s.deleteService(svcState.ServiceName)
			s.Registry.MarkCleanedUp(svcState.ServiceName)
			s.CloudRun.Update(id, func(st *CloudRunState) { st.ServiceName = "" })
		}
	} else {
		// cloud-fallback lookup so stop works post-restart.
		if crState, ok := s.resolveCloudRunState(s.ctx(), id); ok && crState.ExecutionName != "" {
			s.cancelExecution(crState.ExecutionName)
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

	// Disconnect reverse agent if connected (unblocks invoke goroutine)
	s.StopHealthCheck(id)

	exitCode := core.SignalToExitCode(signal)

	// — same story as Stop: for Services we delete the
	// resource; for Jobs we cancel the execution.
	if s.config.UseService {
		if svcState, ok := s.resolveServiceCloudRunState(s.ctx(), id); ok && svcState.ServiceName != "" {
			s.deleteService(svcState.ServiceName)
			s.Registry.MarkCleanedUp(svcState.ServiceName)
			s.CloudRun.Update(id, func(st *CloudRunState) { st.ServiceName = "" })
		}
	} else {
		// cloud-fallback lookup so kill works post-restart.
		if crState, ok := s.resolveCloudRunState(s.ctx(), id); ok && crState.ExecutionName != "" {
			s.cancelExecution(crState.ExecutionName)
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

	// Disconnect reverse agent if connected (unblocks invoke goroutine)

	if c.State.Running {
		// `docker rm -f` is SIGKILL → exit 137.
		killExitCode := core.SignalToExitCode("SIGKILL")
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": fmt.Sprintf("%d", killExitCode),
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		if !s.config.UseService {
			crState, _ := s.resolveCloudRunState(s.ctx(), id)
			if crState.ExecutionName != "" {
				s.cancelExecution(crState.ExecutionName)
			}
		}
	}

	s.StopHealthCheck(id)

	// — delete the backing cloud resource. Jobs and Services
	// live in distinct GCP resource namespaces so cached state is
	// unambiguous.
	if s.config.UseService {
		svcState, _ := s.resolveServiceCloudRunState(s.ctx(), id)
		if svcState.ServiceName != "" {
			s.deleteService(svcState.ServiceName)
			s.Registry.MarkCleanedUp(svcState.ServiceName)
		}
	} else {
		crState, _ := s.resolveCloudRunState(s.ctx(), id)
		if crState.JobName != "" {
			s.deleteJob(crState.JobName)
			s.Registry.MarkCleanedUp(crState.JobName)
		}
	}

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod {
		s.Store.Pods.RemoveContainer(pod.ID, id)
	}

	// Deregister from Cloud DNS (CNAME for Services, A for Jobs)
	hostname := strings.TrimPrefix(c.Name, "/")
	for _, ep := range c.NetworkSettings.Networks {
		if ep == nil || ep.NetworkID == "" {
			continue
		}
		if s.config.UseService {
			if err := s.cloudServiceDeregisterCNAME(s.ctx(), id, hostname, ep.NetworkID); err != nil {
				s.Logger.Warn().Err(err).Str("container", id[:12]).Msg("failed to deregister CNAME from Cloud DNS")
			}
		} else if err := s.cloudServiceDeregister(id, hostname, ep.NetworkID); err != nil {
			s.Logger.Warn().Err(err).Str("container", id[:12]).Msg("failed to deregister from Cloud DNS")
		}
	}

	// Clean up network associations
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, id)
		}
	}

	s.PendingCreates.Delete(id)
	s.CloudRun.Delete(id)
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

// ContainerLogs streams container logs from Cloud Logging. Filter
// shape depends on Config.UseService: Jobs emit under resource.type=
// "cloud_run_job" with a job_name label; Services emit under
// "cloud_run_revision" with a service_name label.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	return core.StreamCloudLogs(s.BaseServer, ref, opts, s.buildCloudLogsFetcher(ref), core.StreamCloudLogsOptions{})
}

// buildCloudLogsFetcher returns a CloudLogFetchFunc closure that
// queries Cloud Logging for the given container's Job (or Service).
// Shared by ContainerLogs and ContainerAttach.
//
// The `logName:"run.googleapis.com"` substring clause restricts the
// query to Cloud Run runtime logs (stdout / stderr / varlog system).
// Without it, Cloud Audit Logs (`cloudaudit.googleapis.com/...`) share
// the same `resource.type="cloud_run_job"` and would be merged into the
// docker logs stream as multi-KB textproto AuditLog dumps. Substring
// match is used (instead of exact `logName=` with the canonical
// `…/logs/run.googleapis.com%2Fstdout` form) because the cloud.google.com/go
// logadmin client double-encodes `%` in the filter, which makes the
// canonical form silently match nothing.
func (s *Server) buildCloudLogsFetcher(ref string) core.CloudLogFetchFunc {
	id, _ := s.ResolveContainerIDAuto(context.Background(), ref)

	const logNameClause = `logName:"run.googleapis.com"`

	var baseFilter string
	if s.config.UseService {
		var shortSvcName string
		if id != "" {
			svcState, _ := s.resolveServiceCloudRunState(s.ctx(), id)
			name := svcState.ServiceName
			if name == "" {
				name = buildServiceName(id)
			}
			parts := strings.Split(name, "/")
			shortSvcName = parts[len(parts)-1]
		}
		baseFilter = fmt.Sprintf(
			`resource.type="cloud_run_revision" AND resource.labels.service_name="%s" AND %s`,
			shortSvcName, logNameClause,
		)
	} else {
		var shortJobName string
		if id != "" {
			crState, _ := s.resolveCloudRunState(s.ctx(), id)
			jobName := crState.JobName
			if jobName == "" {
				jobName = buildJobName(id)
			}
			parts := strings.Split(jobName, "/")
			shortJobName = parts[len(parts)-1]
		}
		baseFilter = fmt.Sprintf(
			`resource.type="cloud_run_job" AND resource.labels.job_name="%s" AND %s`,
			shortJobName, logNameClause,
		)
	}

	return s.cloudLoggingFetch(baseFilter)
}

// cloudLogCursor tracks the cloud-logs follow-mode position. Strict
// `timestamp>lastTS` cursor causes silent loss when Cloud Logging
// timestamps multiple entries identically (batched stdout writes from
// a fast-exit container). The cursor uses `timestamp>=lastTS` plus
// per-entry `seen` dedup keyed on (timestamp, insertId-or-message-hash)
// so we never miss a tied entry and never emit a duplicate.
type cloudLogCursor struct {
	lastTS time.Time
	seen   map[string]struct{} // key: <unix-nano>:<message-hash>
}

// cloudLoggingFetch returns a CloudLogFetchFunc that queries Cloud Logging.
// Uses `timestamp>=cursor.lastTS` for the next-page query (so tied-timestamp
// entries are not dropped) plus a `seen` set to prevent duplicate emission.
func (s *Server) cloudLoggingFetch(baseFilter string) core.CloudLogFetchFunc {
	return func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		logFilter := baseFilter

		c, _ := cursor.(*cloudLogCursor)
		if c == nil {
			c = &cloudLogCursor{seen: make(map[string]struct{})}
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

	// Stop if running
	if c.State.Running {
		s.StopHealthCheck(id)

		crState, _ := s.resolveCloudRunState(s.ctx(), id)
		if crState.ExecutionName != "" {
			s.cancelExecution(crState.ExecutionName)
		}
		if crState.JobName != "" {
			s.deleteJob(crState.JobName)
			s.Registry.MarkCleanedUp(crState.JobName)
		}
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

	// Re-add to PendingCreates so ContainerStart can find it
	c.State.Status = "created"
	c.State.Running = false
	c.State.Pid = 0
	c.State.StartedAt = "0001-01-01T00:00:00Z"
	c.RestartCount++
	s.PendingCreates.Put(id, c)

	// Start the container directly via typed method
	if err := s.ContainerStart(id); err != nil {
		return err
	}

	s.EmitEvent("container", "restart", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerPrune removes all stopped containers.
// In the stateless model, only PendingCreates (never-started) containers are local.
func (s *Server) ContainerPrune(filters map[string][]string) (*api.ContainerPruneResponse, error) {
	labelFilters := filters["label"]
	untilFilters := filters["until"]

	var deleted []string
	var spaceReclaimed uint64

	// Check PendingCreates for containers that were created but never started
	for _, c := range s.PendingCreates.List() {
		if len(labelFilters) > 0 && !core.MatchLabels(c.Config.Labels, labelFilters) {
			continue
		}
		if len(untilFilters) > 0 && !core.MatchUntil(c.Created, untilFilters) {
			continue
		}
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		// Clean up Cloud Run resources
		crState, _ := s.resolveCloudRunState(s.ctx(), c.ID)
		if crState.JobName != "" {
			s.deleteJob(crState.JobName)
			s.Registry.MarkCleanedUp(crState.JobName)
		}
		s.StopHealthCheck(c.ID)

		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, c.ID)
			}
		}
		if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
			s.Store.Pods.RemoveContainer(pod.ID, c.ID)
		}
		s.PendingCreates.Delete(c.ID)
		s.CloudRun.Delete(c.ID)
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
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
}

// ImageLoad delegates to ImageManager.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return s.images.Load(r)
}

// VolumeRemove deletes the GCS bucket bound to a named volume. When
// `force` is true, objects are deleted first (GCS refuses to delete
// non-empty buckets). Cloud Run's Runtime IAM stays intact because
// buckets are per-volume, not shared.
func (s *Server) VolumeRemove(name string, force bool) error {
	if name == "" {
		return &api.InvalidParameterError{Message: "volume name is required"}
	}
	if err := s.deleteBucketForVolume(s.ctx(), name, force); err != nil {
		return &api.ServerError{Message: fmt.Sprintf("delete GCS bucket for %q: %v", name, err)}
	}
	return nil
}

// ExecStart runs the exec inside the container via the reverse-agent
// WebSocket. Cloud Run Jobs/Services expose no native exec API, so
// the bootstrap is the only path; if no session is registered for the
// container, return NotImplementedError with the specific reason
// instead of falling through to a generic failure.
func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "exec instance", ID: id}
	}
	c, ok := s.ResolveContainerAuto(context.Background(), exec.ContainerID)
	if !ok {
		return nil, &api.ConflictError{Message: fmt.Sprintf("Container %s has been removed", exec.ContainerID)}
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); !hasAgent {
		return nil, &api.NotImplementedError{Message: "docker exec requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	return s.BaseServer.ExecStart(id, opts)
}

// ContainerExport streams the container's rootfs as tar via the
// reverse-agent.
func (s *Server) ContainerExport(ref string) (io.ReadCloser, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	rc, err := core.RunContainerExportViaAgent(s.reverseAgents, cid)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker export requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("export via reverse-agent: %v", err)}
	}
	return rc, nil
}

// ContainerCommit is not supported by Cloud Run backend.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), req.Container); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	if !s.config.EnableCommit {
		return nil, &api.NotImplementedError{Message: "docker commit on Cloud Run is gated — set SOCKERLESS_ENABLE_COMMIT=1 (agent-driven commit captures added/modified files since container boot as a new layer)"}
	}
	return core.CommitContainerRequestViaAgent(s.BaseServer, s.reverseAgents, req)
}

// PodStart starts all containers in a pod by calling ContainerStart for each.
// This triggers the Cloud Run Job creation via the deferred-start mechanism.
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
			errs = append(errs, fmt.Sprintf("%s: %v", cid, err))
		}
	}

	s.Store.Pods.SetStatus(pod.ID, "running")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodStop stops all containers in a pod by calling ContainerStop for each.
// This cancels the Cloud Run executions.
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
			errs = append(errs, fmt.Sprintf("%s: %v", cid, err))
		}
	}

	s.Store.Pods.SetStatus(pod.ID, "stopped")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodKill sends a signal to all containers in a pod by calling ContainerKill for each.
// This cancels the Cloud Run executions.
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
			errs = append(errs, fmt.Sprintf("%s: %v", cid, err))
		}
	}

	s.Store.Pods.SetStatus(pod.ID, "exited")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodRemove removes a pod and its containers by calling ContainerRemove for each.
// This deletes the Cloud Run Jobs.
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

	// Remove each container via the typed method (cleans up Cloud Run resources)
	for _, cid := range pod.ContainerIDs {
		if _, ok := s.ResolveContainerAuto(context.Background(), cid); !ok {
			continue
		}
		_ = s.ContainerRemove(cid, force)
	}

	s.Store.Pods.DeletePod(pod.ID)
	return nil
}

// Info returns system information enriched with GCP project/region metadata.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}

	// Enrich with GCP-specific information
	info.OperatingSystem = fmt.Sprintf("Google Cloud Run (%s/%s)", s.config.Project, s.config.Region)
	info.Driver = "cloudrun-jobs"
	info.KernelVersion = fmt.Sprintf("cloudrun/%s/%s", s.config.Project, s.config.Region)

	return info, nil
}

// ContainerAttach bridges stdin/stdout/stderr to the bootstrap process
// inside the container via the reverse-agent WebSocket when a session
// is registered. When no agent is registered and the caller doesn't
// need stdin (read-only attach), fall back to streaming Cloud Logging
// as the attached output. Interactive attach without an agent has no
// native Cloud Run surface, so it stays NotImplementedError.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); hasAgent {
		return s.BaseServer.ContainerAttach(id, opts)
	}
	if opts.Stdin {
		return nil, &api.NotImplementedError{Message: "interactive docker attach requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	return core.AttachViaCloudLogs(s.BaseServer, id, opts, s.buildCloudLogsFetcher(id))
}

// ContainerTop runs `ps` inside the container via the reverse-agent
// and parses the output. Requires a bootstrap inside the container
// (SOCKERLESS_CALLBACK_URL).
func (s *Server) ContainerTop(id string, psArgs string) (*api.ContainerTopResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	if !c.State.Running {
		return nil, &api.ConflictError{Message: fmt.Sprintf("Container %s is not running", id)}
	}

	resp, err := core.RunContainerTopViaAgent(s.reverseAgents, c.ID, psArgs)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker top requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("top via reverse-agent: %v", err)}
	}
	return resp, nil
}

// ContainerGetArchive runs `tar -cf - -C <parent> <name>` inside the
// container via the reverse-agent.
func (s *Server) ContainerGetArchive(id string, path string) (*api.ContainerArchiveResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	resp, err := core.RunContainerGetArchiveViaAgent(s.reverseAgents, c.ID, path)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker cp requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("archive via reverse-agent: %v", err)}
	}
	return resp, nil
}

// ContainerPutArchive extracts the incoming tar body into <path> via
// the reverse-agent.
func (s *Server) ContainerPutArchive(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}
	err := core.RunContainerPutArchiveViaAgent(s.reverseAgents, c.ID, path, body)
	if err == core.ErrNoReverseAgent {
		return &api.NotImplementedError{Message: "docker cp requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return &api.ServerError{Message: fmt.Sprintf("put-archive via reverse-agent: %v", err)}
	}
	return nil
}

// ContainerStatPath runs `stat` inside the Cloud Run task via the
// reverse-agent.
func (s *Server) ContainerStatPath(id string, path string) (*api.ContainerPathStat, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	stat, err := core.RunContainerStatPathViaAgent(s.reverseAgents, c.ID, path)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker container stat requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("stat via reverse-agent: %v", err)}
	}
	return stat, nil
}

// ContainerUpdate updates container resource constraints.
// Cloud Run does not support live resource updates.
func (s *Server) ContainerUpdate(id string, req *api.ContainerUpdateRequest) (*api.ContainerUpdateResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	s.Logger.Warn().Str("container", c.ID[:12]).Msg("ContainerUpdate: Cloud Run does not support live resource updates")
	return s.BaseServer.ContainerUpdate(c.ID, req)
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

// AuthLogin handles Docker registry authentication.
// For GCR/Artifact Registry addresses, logs a warning and delegates to BaseServer.
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	addr := req.ServerAddress
	if strings.HasSuffix(addr, ".gcr.io") ||
		strings.HasSuffix(addr, "-docker.pkg.dev") ||
		strings.Contains(addr, ".pkg.dev") {
		s.Logger.Warn().
			Str("registry", addr).
			Msg("GCR/Artifact Registry login: credentials stored locally; use `gcloud auth configure-docker` for production")
	}
	return s.BaseServer.AuthLogin(req)
}

// VolumePrune deletes every sockerless-managed GCS bucket that isn't
// currently referenced by a pending container's binds. Bucket labels
// already carry the `sockerless-managed` marker so this path only
// touches buckets provisioned by sockerless.
func (s *Server) VolumePrune(filters map[string][]string) (*api.VolumePruneResponse, error) {
	buckets, err := s.listManagedBuckets(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("list managed GCS buckets: %v", err)}
	}
	in := s.inUseVolumeNames()
	resp := &api.VolumePruneResponse{}
	for _, b := range buckets {
		name := gcpcommon.BucketVolumeName(b)
		if _, busy := in[name]; busy {
			continue
		}
		if err := s.deleteBucketForVolume(s.ctx(), name, true); err != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("delete GCS bucket for %q: %v", name, err)}
		}
		resp.VolumesDeleted = append(resp.VolumesDeleted, name)
	}
	return resp, nil
}

// inUseVolumeNames returns the set of Docker volume names currently
// referenced by pending Cloud Run jobs.
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
