package gcf

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	}
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	// Resolve Docker Hub images to Artifact Registry remote repository URIs
	config.Image = gcpcommon.ResolveGCPImageURI(config.Image, s.config.Project, s.config.Region)

	hostConfig := api.HostConfig{NetworkMode: "default"}
	if req.HostConfig != nil {
		hostConfig = *req.HostConfig
	}
	if hostConfig.NetworkMode == "" {
		hostConfig.NetworkMode = "default"
	}

	// Phase 94: named-volume binds are allowed (`-v volName:/mnt[:ro]`)
	// and land on sockerless-managed GCS buckets attached to the
	// underlying Cloud Run Service. Host-path binds (`/h:/c`) are
	// rejected — GCF containers have no host filesystem.
	for _, b := range hostConfig.Binds {
		parts := strings.SplitN(b, ":", 3)
		if len(parts) < 2 {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid bind %q: expected src:dst[:mode]", b)}
		}
		if strings.HasPrefix(parts[0], "/") || strings.HasPrefix(parts[0], ".") {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("host-path binds are not supported on Cloud Functions; use a named volume (docker volume create + -v name:%s)", parts[1])}
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
	container.NetworkSettings.Networks[netName] = &api.EndpointSettings{
		NetworkID:   networkID,
		EndpointID:  core.GenerateID()[:16],
		Gateway:     "",
		IPAddress:   "0.0.0.0",
		IPPrefixLen: 16,
		MacAddress:  "",
	}

	// Build function name from container ID
	funcName := "skls-" + id[:12]

	// Build environment variables
	envVars := make(map[string]string)
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	// Build fully qualified function name
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	fullFunctionName := fmt.Sprintf("%s/functions/%s", parent, funcName)

	// Pass entrypoint + cmd SEPARATELY so the simulator preserves
	// docker's ENTRYPOINT/CMD semantics. Flattening them into one
	// slice loses the distinction: with image ENTRYPOINT=/usr/local/bin/foo
	// and user Cmd=["arg"], a flattened slice yields ["arg"] and the
	// sim would override ENTRYPOINT with "arg" — breaking tests like
	// eval-arithmetic where the image entrypoint is the actual binary.
	if len(config.Entrypoint) > 0 {
		epJSON, _ := json.Marshal(config.Entrypoint)
		envVars["SOCKERLESS_ENTRYPOINT"] = base64.StdEncoding.EncodeToString(epJSON)
	}
	if len(config.Cmd) > 0 {
		cmdJSON, _ := json.Marshal(config.Cmd)
		envVars["SOCKERLESS_CMD"] = base64.StdEncoding.EncodeToString(cmdJSON)
	}

	// Pass the container image so the simulator can run it directly
	envVars["SOCKERLESS_IMG"] = config.Image

	// Container IDs are 64 chars; GCP labels truncate at 63. Persist the
	// full ID in an environment variable so CloudState.GetContainer can
	// match requests by full ID post-start (when PendingCreates is empty).
	envVars["SOCKERLESS_CONTAINER_ID"] = id

	// Build service config
	serviceConfig := &functionspb.ServiceConfig{
		AvailableMemory:      s.config.Memory,
		AvailableCpu:         s.config.CPU,
		TimeoutSeconds:       int32(s.config.Timeout),
		EnvironmentVariables: envVars,
	}

	if s.config.ServiceAccount != "" {
		serviceConfig.ServiceAccountEmail = s.config.ServiceAccount
	}

	// Build resource labels
	tags := core.TagSet{
		ContainerID: id,
		Backend:     "gcf",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	// Create the Cloud Run Function
	createReq := &functionspb.CreateFunctionRequest{
		Parent:     parent,
		FunctionId: funcName,
		Function: &functionspb.Function{
			Name:   fullFunctionName,
			Labels: tags.AsGCPLabels(),
			BuildConfig: &functionspb.BuildConfig{
				Runtime:    "docker",
				EntryPoint: "",
			},
			ServiceConfig: serviceConfig,
		},
	}

	op, err := s.gcp.Functions.CreateFunction(s.ctx(), createReq)
	if err != nil {
		s.Logger.Error().Err(err).Str("function", funcName).Msg("failed to create Cloud Run Function")
		return nil, gcpcommon.MapGCPError(err, "function", funcName)
	}

	result, err := op.Wait(s.ctx())
	if err != nil {
		// Best-effort: delete potentially-created function
		if delOp, delErr := s.gcp.Functions.DeleteFunction(s.ctx(), &functionspb.DeleteFunctionRequest{
			Name: fullFunctionName,
		}); delErr == nil {
			_ = delOp.Wait(s.ctx())
		}
		s.Logger.Error().Err(err).Str("function", funcName).Msg("failed to wait for Cloud Run Function creation")
		return nil, gcpcommon.MapGCPError(err, "function", funcName)
	}

	// Get function URL from the result
	functionURL := ""
	if result.ServiceConfig != nil {
		functionURL = result.ServiceConfig.Uri
	}

	// Phase 94: if the request carries named-volume binds, attach them
	// to the underlying Cloud Run Service via the GetService /
	// UpdateService escape hatch. GCF's Functions v2 API has no direct
	// Volumes primitive in ServiceConfig (only SecretVolumes), so every
	// other volume must be appended to the backing service's
	// RevisionTemplate.
	if len(hostConfig.Binds) > 0 {
		if err := s.attachVolumesToFunctionService(s.ctx(), result, hostConfig.Binds); err != nil {
			// Best-effort: delete the partially-configured function so
			// the create appears atomic to the docker client.
			if delOp, delErr := s.gcp.Functions.DeleteFunction(s.ctx(), &functionspb.DeleteFunctionRequest{
				Name: fullFunctionName,
			}); delErr == nil {
				_ = delOp.Wait(s.ctx())
			}
			s.Logger.Error().Err(err).Str("function", funcName).Msg("failed to attach named-volume binds to underlying Cloud Run Service")
			return nil, &api.ServerError{Message: fmt.Sprintf("attach volumes to function %q: %v", funcName, err)}
		}
	}

	s.PendingCreates.Put(id, container)

	s.GCF.Put(id, GCFState{
		FunctionName: funcName,
		FunctionURL:  functionURL,
		LogResource:  funcName,
	})

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "gcf",
		ResourceType: "function",
		ResourceID:   fullFunctionName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": container.Image, "name": container.Name, "functionName": funcName},
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

// ContainerStart starts a Cloud Run Function invocation for the container.
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

	// Multi-container pods are not supported by FaaS backends
	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod && len(pod.ContainerIDs) > 1 {
		return &api.InvalidParameterError{
			Message: "multi-container pods are not supported by the cloudrun-functions backend",
		}
	}

	gcfState, _ := s.GCF.Get(id)

	// Remove from PendingCreates now that we're starting.
	s.PendingCreates.Delete(id)

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Invoke function via HTTP trigger asynchronously. Phase 95:
	// capture the outcome in Store.InvocationResults so CloudState
	// reflects the container as exited with a real exit code.
	go func() {
		inv := core.InvocationResult{}
		if gcfState.FunctionURL == "" {
			s.Logger.Error().Str("function", gcfState.FunctionName).Msg("no function URL available for invocation")
			inv.ExitCode = 1
			inv.Error = "no function URL available"
		} else if resp, err := gcfHTTPClient.Post(gcfState.FunctionURL, "application/json", nil); err != nil {
			s.Logger.Error().Err(err).Str("function", gcfState.FunctionName).Msg("function invocation failed")
			inv.ExitCode = core.HTTPInvokeErrorExitCode(err)
			inv.Error = err.Error()
		} else {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if len(body) > 0 && string(body) != "{}" {
				s.Store.LogBuffers.Store(id, body)
			}
			inv.ExitCode = core.HTTPStatusToExitCode(resp.StatusCode)
			if inv.ExitCode != 0 {
				inv.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
				s.Logger.Warn().Int("status", resp.StatusCode).Str("function", gcfState.FunctionName).Msg("function returned error status")
			}
		}
		s.Store.PutInvocationResult(id, inv)

		// Close wait channel so ContainerWait unblocks
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
	}()

	return nil
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
	// Phase 95: record the stop outcome so CloudState reports exited with
	// code 137 (Docker convention for force-stopped).
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
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
	}

	s.StopHealthCheck(id)

	// Delete Cloud Run Function (best-effort)
	gcfState, _ := s.GCF.Get(id)
	if gcfState.FunctionName != "" {
		fullName := fmt.Sprintf("projects/%s/locations/%s/functions/%s", s.config.Project, s.config.Region, gcfState.FunctionName)
		op, err := s.gcp.Functions.DeleteFunction(s.ctx(), &functionspb.DeleteFunctionRequest{
			Name: fullName,
		})
		if err == nil {
			_ = op.Wait(s.ctx()) // best-effort wait
		}
		s.Registry.MarkCleanedUp(fullName)
	}

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod {
		s.Store.Pods.RemoveContainer(pod.ID, id)
	}

	// Clean up network associations
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, id)
		}
	}

	s.PendingCreates.Delete(id)
	s.GCF.Delete(id)
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
	return nil
}

// ContainerLogs streams container logs from Cloud Logging.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	var funcName string
	if id, ok := s.ResolveContainerIDAuto(context.Background(), ref); ok {
		gcfState, _ := s.GCF.Get(id)
		funcName = gcfState.FunctionName
	}

	baseFilter := fmt.Sprintf(
		`resource.type="cloud_run_revision" AND resource.labels.service_name="%s"`,
		funcName,
	)

	fetch := s.cloudLoggingFetch(baseFilter)

	return core.StreamCloudLogs(s.BaseServer, ref, opts, fetch, core.StreamCloudLogsOptions{
		CheckLogBuffers: true,
	})
}

// cloudLoggingFetch returns a CloudLogFetchFunc that queries Cloud Logging.
// cursor is a time.Time tracking the latest seen timestamp for dedup.
func (s *Server) cloudLoggingFetch(baseFilter string) core.CloudLogFetchFunc {
	return func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		logFilter := baseFilter

		var lastTS time.Time
		if cursor != nil {
			lastTS = cursor.(time.Time)
		}

		if !lastTS.IsZero() {
			logFilter += fmt.Sprintf(` AND timestamp>"%s"`, lastTS.UTC().Format(time.RFC3339Nano))
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
			entries = append(entries, core.CloudLogEntry{Timestamp: entry.Timestamp, Message: line})
			if entry.Timestamp.After(lastTS) {
				lastTS = entry.Timestamp
			}
		}

		return entries, lastTS, nil
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
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
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
		gcfState, _ := s.GCF.Get(c.ID)
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
		s.GCF.Delete(c.ID)
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

// ContainerPause is not supported by the Cloud Run Functions backend.
func (s *Server) ContainerPause(ref string) error {
	_, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Cloud Run Functions backend does not support pause"}
}

// ContainerUnpause is not supported by the Cloud Run Functions backend.
func (s *Server) ContainerUnpause(ref string) error {
	_, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Cloud Run Functions backend does not support unpause"}
}

// ImagePull delegates to ImageManager which handles cloud auth and config fetching.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
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

// ContainerExport is not supported by the Cloud Run Functions backend.
// Cloud Run Functions have no local filesystem to export.
func (s *Server) ContainerExport(id string) (io.ReadCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return nil, &api.NotImplementedError{
		Message: "Cloud Run Functions backend does not support container export; functions have no local filesystem",
	}
}

// ContainerCommit is not supported by the Cloud Run Functions backend.
// Cloud Run Functions have no local filesystem to commit.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if req.Container == "" {
		return nil, &api.InvalidParameterError{Message: "container query parameter is required"}
	}
	if _, ok := s.ResolveContainerIDAuto(context.Background(), req.Container); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	return nil, &api.NotImplementedError{
		Message: "Cloud Run Functions backend does not support container commit; functions have no local filesystem",
	}
}

// ContainerAttach is not supported by the Cloud Run Functions backend.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return nil, &api.NotImplementedError{
		Message: "Cloud Run Functions backend does not support attach",
	}
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
