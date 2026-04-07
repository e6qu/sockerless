package azf

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v4"
	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by an Azure Function App.
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

	// Resolve Docker Hub images to ACR or normalize for Azure Functions
	config.Image = azurecommon.ResolveAzureImageURI(config.Image, s.config.Registry)

	hostConfig := api.HostConfig{NetworkMode: "default"}
	if req.HostConfig != nil {
		hostConfig = *req.HostConfig
	}
	if hostConfig.NetworkMode == "" {
		hostConfig.NetworkMode = "default"
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
		Driver:   "azure-functions",
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

	// Function App names must be globally unique -- use skls- prefix + truncated container ID
	funcAppName := "skls-" + id[:12]

	// Build environment variables for App Settings
	envVars := make(map[string]string)
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	// Build App Settings
	appSettings := []*armappservice.NameValuePair{
		{Name: ptr("FUNCTIONS_EXTENSION_VERSION"), Value: ptr("~4")},
		{Name: ptr("WEBSITES_ENABLE_APP_SERVICE_STORAGE"), Value: ptr("false")},
		{Name: ptr("AzureWebJobsStorage"), Value: ptr(fmt.Sprintf("DefaultEndpointsProtocol=https;AccountName=%s;EndpointSuffix=core.windows.net", s.config.StorageAccount))},
	}

	if s.config.Registry != "" {
		appSettings = append(appSettings, &armappservice.NameValuePair{
			Name: ptr("DOCKER_REGISTRY_SERVER_URL"), Value: ptr(s.config.Registry),
		})
	}

	// Add user environment variables as App Settings
	for k, v := range envVars {
		appSettings = append(appSettings, &armappservice.NameValuePair{
			Name: ptr(k), Value: ptr(v),
		})
	}

	// Pass command via app setting (cloud-native) for short-lived containers
	cmd := core.BuildOriginalCommand(config.Entrypoint, config.Cmd)
	if len(cmd) > 0 {
		cmdJSON, _ := json.Marshal(cmd)
		appSettings = append(appSettings, &armappservice.NameValuePair{
			Name:  ptr("SOCKERLESS_CMD"),
			Value: ptr(base64.StdEncoding.EncodeToString(cmdJSON)),
		})
	}

	// Build the Function App Site resource
	siteConfig := &armappservice.SiteConfig{
		LinuxFxVersion: ptr("DOCKER|" + config.Image),
		AppSettings:    appSettings,
	}
	// Build resource tags
	tags := core.TagSet{
		ContainerID: id,
		Backend:     "azf",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	site := armappservice.Site{
		Location: ptr(s.config.Location),
		Kind:     ptr("functionapp,linux,container"),
		Tags:     tags.AsAzurePtrMap(),
		Properties: &armappservice.SiteProperties{
			SiteConfig: siteConfig,
		},
	}

	if s.config.AppServicePlan != "" {
		site.Properties.ServerFarmID = ptr(s.config.AppServicePlan)
	}

	// Create Function App
	poller, err := s.azure.WebApps.BeginCreateOrUpdate(s.ctx(), s.config.ResourceGroup, funcAppName, site, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("functionApp", funcAppName).Msg("failed to create Function App")
		return nil, azurecommon.MapAzureError(err, "function app", funcAppName)
	}

	result, err := poller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		// Best-effort: delete potentially-created Function App
		_, _ = s.azure.WebApps.Delete(s.ctx(), s.config.ResourceGroup, funcAppName, nil)
		s.Logger.Error().Err(err).Str("functionApp", funcAppName).Msg("Function App creation failed")
		return nil, azurecommon.MapAzureError(err, "function app", funcAppName)
	}

	resourceID := ""
	if result.ID != nil {
		resourceID = *result.ID
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "azf",
		ResourceType: "site",
		ResourceID:   resourceID,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": container.Image, "name": container.Name, "functionAppName": funcAppName},
	})

	functionURL := ""
	if result.Properties != nil && result.Properties.DefaultHostName != nil {
		scheme := "https"
		if strings.HasPrefix(s.config.EndpointURL, "http://") {
			scheme = "http"
		}
		functionURL = fmt.Sprintf("%s://%s/api/function", scheme, *result.Properties.DefaultHostName)
	}

	s.PendingCreates.Put(id, container)

	s.AZF.Put(id, AZFState{
		FunctionAppName: funcAppName,
		ResourceID:      resourceID,
		FunctionURL:     functionURL,
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

// ContainerStart starts a Function App invocation for the container.
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
			Message: "multi-container pods are not supported by the azure-functions backend",
		}
	}

	azfState, _ := s.AZF.Get(id)

	// Remove from PendingCreates now that we're starting.
	s.PendingCreates.Delete(id)

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Invoke the Function App via HTTP POST asynchronously
	go func() {
		if azfState.FunctionURL != "" {
			client := &http.Client{Timeout: time.Duration(s.config.Timeout) * time.Second}
			resp, err := client.Post(azfState.FunctionURL, "application/json", nil)
			if err != nil {
				s.Logger.Error().Err(err).Str("functionApp", azfState.FunctionAppName).Msg("Function App invocation failed")
			} else {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if len(body) > 0 && string(body) != "{}" {
					s.Store.LogBuffers.Store(id, body)
				}
				if resp.StatusCode >= 400 {
					s.Logger.Warn().Int("status", resp.StatusCode).Str("functionApp", azfState.FunctionAppName).Msg("Function App returned error")
				}
			}
		} else {
			s.Logger.Warn().Str("functionApp", azfState.FunctionAppName).Msg("no function URL available, cannot invoke")
		}

		// Close wait channel so ContainerWait unblocks
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
	}()

	return nil
}

// ContainerStop stops a running Azure Functions container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Azure Functions run to completion — stop transitions state
	s.StopHealthCheck(id)
	// Close wait channel so ContainerWait unblocks
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": "0", "name": strings.TrimPrefix(c.Name, "/")})
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

	s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", exitCode), "name": strings.TrimPrefix(c.Name, "/")})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	return nil
}

// ContainerRemove removes a container and its associated Azure Functions resources.
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
	}

	s.StopHealthCheck(id)

	// Delete Function App (best-effort)
	azfState, _ := s.AZF.Get(id)
	if azfState.FunctionAppName != "" {
		_, err := s.azure.WebApps.Delete(s.ctx(), s.config.ResourceGroup, azfState.FunctionAppName, nil)
		if err != nil {
			s.Logger.Debug().Err(err).Str("functionApp", azfState.FunctionAppName).Msg("failed to delete Function App")
		}
	}

	if azfState.ResourceID != "" {
		s.Registry.MarkCleanedUp(azfState.ResourceID)
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

	// Clean up PendingCreates (container may have been created but never started)
	s.PendingCreates.Delete(id)
	s.AZF.Delete(id)
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

// ContainerLogs streams container logs from Azure Monitor.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	var functionAppName string
	if id, ok := s.ResolveContainerIDAuto(context.Background(), ref); ok {
		azfState, _ := s.AZF.Get(id)
		functionAppName = azfState.FunctionAppName
		if functionAppName == "" {
			functionAppName = "skls-" + id[:12]
		}
	}

	fetch := s.azureLogsFetch(
		`AppTraces`,
		fmt.Sprintf(`AppRoleName == "%s"`, functionAppName),
		"Message",
	)

	return core.StreamCloudLogs(s.BaseServer, ref, opts, fetch, core.StreamCloudLogsOptions{
		CheckLogBuffers: true,
	})
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

// ContainerPrune removes all stopped containers and their AZF state.
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
		// Clean up Azure Functions cloud resources
		azfState, _ := s.AZF.Get(c.ID)
		if azfState.FunctionAppName != "" {
			_, _ = s.azure.WebApps.Delete(s.ctx(), s.config.ResourceGroup, azfState.FunctionAppName, nil)
		}
		if azfState.ResourceID != "" {
			s.Registry.MarkCleanedUp(azfState.ResourceID)
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
		s.AZF.Delete(c.ID)
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

// ContainerPause is not supported by the Azure Functions backend.
func (s *Server) ContainerPause(ref string) error {
	_, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Azure Functions backend does not support pause"}
}

// ContainerUnpause is not supported by the Azure Functions backend.
func (s *Server) ContainerUnpause(ref string) error {
	_, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Azure Functions backend does not support unpause"}
}

// ImagePull delegates to ImageManager for unified cloud image handling.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
}

// Info returns system information enriched with Azure-specific metadata.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}

	// Enrich with Azure-specific context
	info.OperatingSystem = fmt.Sprintf("Azure Functions (%s)", s.config.Location)
	info.Name = fmt.Sprintf("sockerless-azf/%s/%s", s.config.SubscriptionID, s.config.ResourceGroup)

	return info, nil
}

// ContainerExport is not supported by the Azure Functions backend.
// Azure Functions have no local filesystem to export.
func (s *Server) ContainerExport(id string) (io.ReadCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return nil, &api.NotImplementedError{
		Message: "Azure Functions backend does not support container export; functions have no local filesystem",
	}
}

// ContainerCommit is not supported by the Azure Functions backend.
// Azure Functions have no local filesystem to commit.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if req.Container == "" {
		return nil, &api.InvalidParameterError{Message: "container query parameter is required"}
	}
	if _, ok := s.ResolveContainerIDAuto(context.Background(), req.Container); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	return nil, &api.NotImplementedError{
		Message: "Azure Functions backend does not support container commit; functions have no local filesystem",
	}
}

// ContainerAttach is not supported by the Azure Functions backend.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return nil, &api.NotImplementedError{
		Message: "Azure Functions backend does not support attach",
	}
}

// ImageBuild delegates to ImageManager for unified cloud image handling.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	return s.images.Build(opts, buildContext)
}

// ImageTag delegates to ImageManager for unified cloud image handling.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	return s.images.Tag(source, repo, tag)
}

// ImageRemove delegates to ImageManager for unified cloud image handling.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	return s.images.Remove(name, force, prune)
}

// ImagePush delegates to ImageManager for unified cloud image handling.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	return s.images.Push(name, tag, auth)
}

// ImageLoad delegates to ImageManager for unified cloud image handling.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return s.images.Load(r)
}

// AuthLogin handles registry authentication.
// For ACR registries (*.azurecr.io), logs a warning about using managed identity.
// For all other registries, delegates to BaseServer directly.
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	if strings.HasSuffix(req.ServerAddress, ".azurecr.io") {
		s.Logger.Warn().
			Str("registry", req.ServerAddress).
			Msg("ACR login: credentials stored locally; use managed identity for production Azure Functions")
		return s.BaseServer.AuthLogin(req)
	}
	return s.BaseServer.AuthLogin(req)
}
