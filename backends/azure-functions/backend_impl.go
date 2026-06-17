package azf

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

type azfExecEnvelopeRequest struct {
	Sockerless struct {
		Exec azfExecEnvelopeExec `json:"exec"`
	} `json:"sockerless"`
}

type azfExecEnvelopeExec struct {
	Argv    []string `json:"argv"`
	Tty     bool     `json:"tty,omitempty"`
	Workdir string   `json:"workdir,omitempty"`
	Env     []string `json:"env,omitempty"`
	Stdin   string   `json:"stdin,omitempty"`
}

type azfExecEnvelopeResponse struct {
	SockerlessExecResult struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	} `json:"sockerlessExecResult"`
}

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

	// Derive serviceLike from the ORIGINAL client config, before the image's
	// default entrypoint/cmd are merged in below — a service runs its image
	// as-is (no client override, not OpenStdin); anything else is exec-driven.
	clientServiceLike := len(config.Entrypoint) == 0 && len(config.Cmd) == 0 && !config.OpenStdin

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
		// Carry the image's declared ExposedPorts onto the container config so a
		// `services:` container (e.g. a redis sidecar on a FF_NETWORK_PER_BUILD
		// pod) advertises its ports: gitlab-runner reads the service container's
		// exposed ports during preparation to health-check it, and times out
		// ("getting exposed ports: service failed to start") when none surface.
		if len(config.ExposedPorts) == 0 && len(img.Config.ExposedPorts) > 0 {
			config.ExposedPorts = img.Config.ExposedPorts
		}
	}
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	// The overlay's `FROM` runs in the ACR-Tasks build environment, which can
	// only resolve a locally-present or registry-pullable ref — NOT the ACR
	// hostname rewrite below (`<acr>.azurecr.io/library/<digest>` doesn't
	// resolve there; the sim registry is reached via the ACR-endpoint
	// coordinate, not DNS). Capture the pre-rewrite ref for the overlay base so
	// the build's FROM stays pullable, exactly as aca keeps its overlay base
	// (ResolveAzureImageURIWithCache passes a local digest through unchanged).
	overlayBaseRef := config.Image

	// Resolve Docker Hub images to ACR or normalize for Azure Functions
	config.Image = azurecommon.ResolveAzureImageURI(config.Image, s.config.Registry)

	originalImage := config.Image
	// Stash the pre-overlay image + entrypoint/cmd. If this container ends up
	// a pod sidecar, materializePodSite runs its RAW service image (services
	// are reached over the shared loopback; only the pod main needs the
	// reverse-agent overlay).
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}
	config.Labels[labelBaseImage] = originalImage
	if clientServiceLike {
		config.Labels[labelServiceLike] = "true"
	}
	if len(config.Entrypoint) > 0 {
		b, _ := json.Marshal(config.Entrypoint)
		config.Labels[labelBaseEntrypoint] = base64.StdEncoding.EncodeToString(b)
	}
	if len(config.Cmd) > 0 {
		b, _ := json.Marshal(config.Cmd)
		config.Labels[labelBaseCmd] = base64.StdEncoding.EncodeToString(b)
	}
	if s.useAZFOverlayPath(originalImage) {
		spec := azfOverlaySpec{
			BaseImageRef:        overlayBaseRef,
			BootstrapBinaryPath: s.config.BootstrapBinaryPath,
			BootstrapBinaryHash: s.config.BootstrapBinaryHash,
		}
		contentTag := azfOverlayContentTag("azf-", spec)
		overlayURI, err := s.ensureAZFOverlayImage(s.ctx(), spec, contentTag)
		if err != nil {
			return nil, fmt.Errorf("ensure azf overlay image: %w", err)
		}
		config.Env = append(config.Env, azfOverlayUserEnv(config.Entrypoint, config.Cmd, config.WorkingDir)...)
		if jt := core.JobTimeoutEnvIfUnset(config.Env); jt != "" {
			config.Env = append(config.Env, jt)
		}
		config.Image = overlayURI
		config.Entrypoint = nil
		config.Cmd = nil
	} else if hasAZFOverlayRepo(originalImage) {
		config.Env = append(config.Env, azfOverlayUserEnv(config.Entrypoint, config.Cmd, config.WorkingDir)...)
		if jt := core.JobTimeoutEnvIfUnset(config.Env); jt != "" {
			config.Env = append(config.Env, jt)
		}
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
	// translator (see shared_volumes.go for the full Azure Functions
	// bind-mount model: named volumes pass through, mapped host binds
	// rewrite to named-volume references, sub-paths + docker.sock drop,
	// anything else rejects loudly). Named-volume binds attach to the
	// function site via WebApps.UpdateAzureStorageAccounts after
	// BeginCreateOrUpdate returns.
	translatedBinds, droppedBinds, err := translateSharedVolumeBinds(s.config, hostConfig.Binds)
	if err != nil {
		return nil, err
	}
	for _, bind := range droppedBinds {
		s.Logger.Debug().Str("bind", bind).
			Msg("dropping bind mount; docker.sock has no Azure Functions analogue / parent shared volume already exposes the sub-path")
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
	var netAliases []string
	if req.NetworkingConfig != nil {
		if ep, ok := req.NetworkingConfig.EndpointsConfig[netName]; ok && ep != nil {
			netAliases = ep.Aliases
		}
	}
	container.NetworkSettings.Networks[netName] = &api.EndpointSettings{
		NetworkID:   networkID,
		EndpointID:  core.GenerateID()[:16],
		Gateway:     "",
		IPAddress:   "",
		IPPrefixLen: 16,
		MacAddress:  "",
		Aliases:     netAliases,
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

	// Function App names must be globally unique -- use skls- prefix + truncated container ID
	funcAppName := "skls-" + id[:12]

	appSettings := s.buildAZFAppSettings(id, config)

	// Containers that join a user-defined network are potential pod members
	// (GitHub/GitLab `services:` topologies). Defer their Function App
	// creation: ContainerStart assembles the whole pod as ONE site whose
	// sitecontainers share a loopback. Single-container (bridge/default)
	// workloads create their site eagerly here.
	if _, onUserNet := s.userDefinedNetworkID(container); onUserNet {
		s.PendingCreates.Put(id, container)
		s.EmitEvent("container", "create", id, map[string]string{
			"name":  strings.TrimPrefix(name, "/"),
			"image": config.Image,
		})
		return &api.ContainerCreateResponse{ID: id, Warnings: []string{}}, nil
	}

	azfState, err := s.createFunctionSite(s.ctx(), id, funcAppName, container, appSettings)
	if err != nil {
		return nil, err
	}

	s.PendingCreates.Put(id, container)
	s.AZF.Put(id, azfState)

	s.EmitEvent("container", "create", id, map[string]string{
		"name":  strings.TrimPrefix(name, "/"),
		"image": config.Image,
	})

	return &api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	}, nil
}

// createFunctionSite creates the single-container Function App site for a
// container (LinuxFxVersion=DOCKER|<image>), attaches any named-volume binds,
// records the resource in the registry, and returns the resolved AZFState.
// Shared by the eager ContainerCreate path and the deferred-single
// ContainerStart path (a lone container on a user-defined network whose site
// creation was deferred at create time).
func (s *Server) createFunctionSite(ctx context.Context, id, funcAppName string, container api.Container, appSettings []*armappservice.NameValuePair) (AZFState, error) {
	siteConfig := &armappservice.SiteConfig{
		LinuxFxVersion: ptr("DOCKER|" + container.Config.Image),
		AppSettings:    appSettings,
	}
	tags := core.TagSet{
		ContainerID: id,
		Backend:     "azf",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}
	azTags := tags.AsAzurePtrMap()
	// Persist OpenStdin in the site tags: CloudState reconstruction drops the
	// container Config, but a gitlab-runner-pattern (OpenStdin) container must
	// be reported running across per-stage invokes (the cloud is the source of
	// truth), so the runner's per-stage docker exec resolves instead of 409ing
	// on a one-shot-FaaS "exited" overlay.
	if container.Config.OpenStdin {
		azTags["sockerless-open-stdin"] = ptr("true")
	}

	site := armappservice.Site{
		Location: ptr(s.config.Location),
		Kind:     ptr("functionapp,linux,container"),
		Tags:     azTags,
		Properties: &armappservice.SiteProperties{
			SiteConfig: siteConfig,
		},
	}
	if s.config.AppServicePlan != "" {
		site.Properties.ServerFarmID = ptr(s.config.AppServicePlan)
	}

	poller, err := s.azure.WebApps.BeginCreateOrUpdate(ctx, s.config.ResourceGroup, funcAppName, site, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("functionApp", funcAppName).Msg("failed to create Function App")
		return AZFState{}, azurecommon.MapAzureError(err, "function app", funcAppName)
	}
	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		_, _ = s.azure.WebApps.Delete(ctx, s.config.ResourceGroup, funcAppName, nil)
		s.Logger.Error().Err(err).Str("functionApp", funcAppName).Msg("Function App creation failed")
		return AZFState{}, azurecommon.MapAzureError(err, "function app", funcAppName)
	}

	if len(container.HostConfig.Binds) > 0 {
		if err := s.attachVolumesToFunctionSite(ctx, funcAppName, container.HostConfig.Binds); err != nil {
			_, _ = s.azure.WebApps.Delete(ctx, s.config.ResourceGroup, funcAppName, nil)
			s.Logger.Error().Err(err).Str("functionApp", funcAppName).Msg("failed to attach Azure Files volumes")
			return AZFState{}, &api.ServerError{Message: fmt.Sprintf("attach volumes to function app %q: %v", funcAppName, err)}
		}
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

	functionURL, functionHost := "", ""
	if result.Properties != nil && result.Properties.DefaultHostName != nil {
		functionHost = *result.Properties.DefaultHostName
		functionURL = invokeURLForHost(s.config.EndpointURL, functionHost)
	}
	return AZFState{
		FunctionAppName: funcAppName,
		ResourceID:      resourceID,
		FunctionURL:     functionURL,
		FunctionHost:    functionHost,
	}, nil
}

// buildAZFAppSettings builds the Function App app settings for a container:
// the Functions runtime defaults, the registry, the user env, the
// ENTRYPOINT/CMD (passed separately to preserve docker semantics), and the
// reverse-agent callback wiring.
func (s *Server) buildAZFAppSettings(id string, config api.ContainerConfig) []*armappservice.NameValuePair {
	envVars := make(map[string]string)
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

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

	for k, v := range envVars {
		appSettings = append(appSettings, &armappservice.NameValuePair{
			Name: ptr(k), Value: ptr(v),
		})
	}

	// Pass entrypoint + cmd SEPARATELY so the simulator preserves docker's
	// ENTRYPOINT/CMD semantics (an image's ENTRYPOINT must still fire when
	// the user only sets Cmd — flattening would override it).
	if len(config.Entrypoint) > 0 {
		epJSON, _ := json.Marshal(config.Entrypoint)
		appSettings = append(appSettings, &armappservice.NameValuePair{
			Name:  ptr("SOCKERLESS_ENTRYPOINT"),
			Value: ptr(base64.StdEncoding.EncodeToString(epJSON)),
		})
	}
	if len(config.Cmd) > 0 {
		cmdJSON, _ := json.Marshal(config.Cmd)
		appSettings = append(appSettings, &armappservice.NameValuePair{
			Name:  ptr("SOCKERLESS_CMD"),
			Value: ptr(base64.StdEncoding.EncodeToString(cmdJSON)),
		})
	}

	// Inject reverse-agent callback URL + container ID so a bootstrap in the
	// function container can dial back for docker top / exec / cp.
	appSettings = append(appSettings, &armappservice.NameValuePair{
		Name: ptr("SOCKERLESS_CONTAINER_ID"), Value: ptr(id),
	})
	appSettings = append(appSettings, &armappservice.NameValuePair{
		Name: ptr("SOCKERLESS_CALLBACK_URL"), Value: ptr(s.config.CallbackURL),
	})
	return appSettings
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
		// gitlab-runner cycles start→wait→stop→start on the SAME container id
		// per stage. After the first start the container leaves PendingCreates,
		// so a re-start must resolve via CloudState. Re-add to PendingCreates so
		// the network-pod / re-invoke flow below can run for the next stage.
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
		// Runner re-start for the next stage: the per-stage attach just
		// registered a fresh stdinPipe. The App Service site container is
		// persistent (always-on plan), so its in-container bootstrap HTTP
		// server is still alive — re-invoke it with the new stage's captured
		// script rather than reporting NotModified, which would strand the
		// stage waiting for output it never gets.
		if c.Config.OpenStdin {
			// Re-invoke for the next stage. Don't gate on the stdinPipe being
			// present yet — gitlab-runner's per-stage attach and start race, and
			// invokeFunctionAsync → captureAZFStdin(expectStdin) waits for the
			// pipe to appear before draining + POSTing the stage script.
			azfState, hasState := s.AZF.Get(id)
			if hasState && azfState.FunctionURL != "" {
				return s.invokeFunctionAsync(id, c, azfState)
			}
		}
		s.PendingCreates.Delete(id)
		return &api.NotModifiedError{}
	}

	// cloud-dns discovery: a container on a user-defined network is its own
	// App Service site, joined to the network's VNet via regional VNet
	// integration and resolvable by its --network-alias through the linked
	// Private DNS zone. This is the faithful Azure model (separate sites + VNet
	// + Private DNS), distinct from the host-aliases sitecontainer-pod below.
	if s.config.NetworkDiscovery == api.NetworkDiscoveryCloudDNS {
		if netID, ok := s.userDefinedNetworkID(c); ok {
			return s.startCloudDNSSite(id, c, netID)
		}
	}

	// Docker user-defined network → assemble a multi-container pod as ONE
	// App Service site whose sitecontainers share a loopback. Pure Docker
	// signals: network membership + Container.Config.OpenStdin.
	shouldDefer, members := s.shouldDeferOrMaterializeNetworkPod(c)
	if shouldDefer {
		// Service-style sidecar awaiting its pod peer; it deploys as a
		// sitecontainer when the pod materializes. Mark it running so the
		// runner's service health-check during preparation (docker inspect for
		// State.Running + the image's ExposedPorts) passes — the real service
		// process comes up when the pod main arrives and materializePodSite runs
		// it. Without this the deferred service reads as "created" and the runner
		// times out "getting exposed ports: service failed to start".
		s.PendingCreates.Update(id, func(pc *api.Container) {
			pc.State.Status = "running"
			pc.State.Running = true
			pc.State.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
		})
		s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		return nil
	}
	if len(members) > 1 {
		return s.materializePodSite(members)
	}

	azfState, ok := s.AZF.Get(id)
	if !ok || azfState.FunctionURL == "" {
		// A lone container on a user-defined network deferred its site
		// creation at ContainerCreate; create it now.
		var cerr error
		azfState, cerr = s.createFunctionSite(s.ctx(), id, "skls-"+id[:12], c, s.buildAZFAppSettings(id, c.Config))
		if cerr != nil {
			return cerr
		}
		s.AZF.Put(id, azfState)
	}

	return s.invokeFunctionAsync(id, c, azfState)
}

// invokeFunctionAsync invokes a deployed Function App's HTTP trigger
// asynchronously and (for non-OpenStdin containers) blocks until the
// in-function reverse-agent registers. Shared by the single-container start
// path and the materialized-pod main.
func (s *Server) invokeFunctionAsync(id string, c api.Container, azfState AZFState) error {
	// Remove from PendingCreates now that we're starting.
	s.PendingCreates.Delete(id)

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Invoke the Function App via HTTP POST asynchronously and capture
	// outcome in Store.InvocationResults so CloudState reflects the
	// container as exited with a real exit code.
	go func() {
		inv := core.InvocationResult{}
		capturedStdin, hasCapturedStdin := s.captureAZFStdin(id, c.Config.OpenStdin)
		if azfState.FunctionURL == "" {
			s.Logger.Warn().Str("functionApp", azfState.FunctionAppName).Msg("no function URL available, cannot invoke")
			inv.ExitCode = 1
			inv.Error = "no function URL available"
			s.publishAZFAttachResponse(id, nil, []byte(inv.Error))
		} else if readyErr := s.waitAZFFunctionListening(azfState.FunctionURL); readyErr != nil {
			// The in-container bootstrap binds its HTTP port a moment after
			// ContainerStart returns. POSTing the invoke before the listener
			// is up races startup and surfaces as a connection error that —
			// given the long invoke timeout — strands attached readers. Wait
			// for the listener, then fail fast and clearly if it never comes.
			s.Logger.Error().Err(readyErr).Str("functionApp", azfState.FunctionAppName).Msg("Function App HTTP trigger not ready before invoke")
			inv.ExitCode = 1
			inv.Error = fmt.Sprintf("function app not ready: %v", readyErr)
			s.publishAZFAttachResponse(id, nil, []byte(inv.Error))
		} else {
			client := &http.Client{Timeout: time.Duration(s.config.Timeout) * time.Second}
			newReq := func() *http.Request {
				var body io.Reader
				if hasCapturedStdin {
					body = azfExecEnvelopeBody(capturedStdin)
				}
				req, _ := http.NewRequest("POST", azfState.FunctionURL, body)
				req.Header.Set("Content-Type", "application/json")
				if azfState.FunctionHost != "" {
					req.Host = azfState.FunctionHost
				}
				return req
			}

			// Retry only on connection-refused — the brief window between the
			// reverse-agent registering and the HTTP listener binding its
			// port. ECONNREFUSED means the request never reached the server,
			// so re-POSTing cannot double-invoke the workload.
			var resp *http.Response
			var err error
			retryDeadline := time.Now().Add(s.azfBootstrapTimeout())
			for {
				resp, err = client.Do(newReq())
				if err == nil || !errors.Is(err, syscall.ECONNREFUSED) || !time.Now().Before(retryDeadline) {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}

			if err != nil {
				s.Logger.Error().Err(err).Str("functionApp", azfState.FunctionAppName).Msg("Function App invocation failed")
				inv.ExitCode = core.HTTPInvokeErrorExitCode(err)
				inv.Error = err.Error()
				s.publishAZFAttachResponse(id, nil, []byte(err.Error()))
			} else {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if hasCapturedStdin {
					if parsed, stdout, stderr, ok := azfParseExecEnvelopeResponse(body); ok {
						inv = parsed
						if len(stdout) > 0 || len(stderr) > 0 {
							s.Store.LogBuffers.Store(id, append(append([]byte{}, stdout...), stderr...))
						}
						s.publishAZFAttachResponse(id, stdout, stderr)
					} else {
						inv.ExitCode = azfBootstrapExitCode(resp)
						if len(body) > 0 && string(body) != "{}" {
							s.Store.LogBuffers.Store(id, body)
						}
						s.publishAZFAttachResponse(id, body, nil)
					}
				} else {
					if len(body) > 0 && string(body) != "{}" {
						s.Store.LogBuffers.Store(id, body)
					}
					inv.ExitCode = azfBootstrapExitCode(resp)
					if inv.ExitCode != 0 {
						inv.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
						s.Logger.Warn().Int("status", resp.StatusCode).Str("functionApp", azfState.FunctionAppName).Msg("Function App returned error")
					}
					// Always publish so an attached reader unblocks promptly
					// instead of stranding to the attach deadline; a no-op when
					// no reader is attached (non-interactive invoke).
					s.publishAZFAttachResponse(id, body, nil)
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
	// path. For exec-driven callers the first ExecStart MUST find an
	// agent (no fallback).
	if !c.Config.OpenStdin {
		timeout, terr := core.BootstrapTimeoutFromEnv("azf")
		if terr != nil {
			return &api.ServerError{Message: fmt.Sprintf("invalid bootstrap-timeout env: %v", terr)}
		}
		waitCtx, cancel := context.WithTimeout(s.ctx(), timeout)
		defer cancel()
		if werr := s.reverseAgents.WaitForAgent(waitCtx, id); werr != nil {
			return &api.ServerError{Message: fmt.Sprintf(
				"reverse-agent did not register for container %s within %s "+
					"(SOCKERLESS_AZF_BOOTSTRAP_TIMEOUT_SEC). The Function App was deployed and "+
					"invoked but the in-function bootstrap never dialled back to "+
					"SOCKERLESS_CALLBACK_URL=%s. Check egress / VNet / NSG.",
				id[:12], timeout, s.config.CallbackURL,
			)}
		}
	}

	return nil
}

// azfBootstrapTimeout returns the configured bootstrap-ready timeout, falling
// back to the documented default when the env var is unset or invalid.
func (s *Server) azfBootstrapTimeout() time.Duration {
	timeout, err := core.BootstrapTimeoutFromEnv("azf")
	if err != nil || timeout <= 0 {
		return 90 * time.Second
	}
	return timeout
}

// waitAZFFunctionListening blocks until the Function App HTTP trigger accepts
// TCP connections, so the buffered-attach invoke POST reaches a live listener
// instead of racing the in-container bootstrap binding its port (which
// surfaced as a connection error that — given the long invoke timeout —
// stranded attached readers). The probe only dials; it never sends the invoke
// body, so it cannot double-invoke the workload. This path deliberately does
// not depend on the reverse-agent: the buffered HTTP invoke is reachable
// host->container even where the container cannot dial back to the backend.
func (s *Server) waitAZFFunctionListening(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse function URL %q: %w", rawURL, err)
	}
	addr := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			addr = u.Hostname() + ":443"
		} else {
			addr = u.Hostname() + ":80"
		}
	}
	deadline := time.Now().Add(s.azfBootstrapTimeout())
	var lastErr error
	for time.Now().Before(deadline) {
		conn, derr := net.DialTimeout("tcp", addr, 2*time.Second)
		if derr == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = derr
		time.Sleep(200 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return fmt.Errorf("function app %s never accepted connections: %w", addr, lastErr)
}

// captureAZFStdin returns the buffered attach stdin for a container's invoke.
// When expectStdin is set (the container was created with OpenStdin), the
// attach handler stores the stdin pipe asynchronously — the hijacked attach
// connection is processed independently of /start — so /start's invoke
// goroutine can win the race and find no pipe yet. Without waiting it would run
// the workload with no stdin: `sh` reads EOF, exits with no output, and the
// attached reader gets an empty stream (the create→attach→start race, cf. ECS
// BUG-1798). Wait briefly for the pipe to appear+open before claiming it; a
// container that genuinely never attaches just falls through after the bound.
func (s *Server) captureAZFStdin(id string, expectStdin bool) ([]byte, bool) {
	if expectStdin {
		deadline := time.After(5 * time.Second)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			if v, ok := s.stdinPipes.Load(id); ok && v.(*stdinPipe).IsOpen() {
				break
			}
			select {
			case <-deadline:
			case <-tick.C:
				continue
			}
			break
		}
	}
	v, ok := s.stdinPipes.LoadAndDelete(id)
	if !ok {
		return nil, false
	}
	pipe := v.(*stdinPipe)
	select {
	case <-pipe.Done():
	case <-time.After(30 * time.Second):
		s.Logger.Warn().Str("container", id).Msg("AZF stdin pipe Done timeout; proceeding with captured bytes")
	}
	return pipe.Bytes(), true
}

func (s *Server) publishAZFAttachResponse(id string, stdout, stderr []byte) {
	if v, ok := s.attachStreams.LoadAndDelete(id); ok {
		v.(*attachStream).publishAttachResponse(stdout, stderr)
	}
}

func azfExecEnvelopeBody(stdin []byte) io.Reader {
	var req azfExecEnvelopeRequest
	req.Sockerless.Exec.Argv = []string{"/bin/sh"}
	req.Sockerless.Exec.Stdin = base64.StdEncoding.EncodeToString(stdin)
	body, _ := json.Marshal(req)
	return bytes.NewReader(body)
}

func azfParseExecEnvelopeResponse(body []byte) (core.InvocationResult, []byte, []byte, bool) {
	var resp azfExecEnvelopeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return core.InvocationResult{}, nil, nil, false
	}
	stdout, err := base64.StdEncoding.DecodeString(resp.SockerlessExecResult.Stdout)
	if err != nil {
		return core.InvocationResult{}, nil, nil, false
	}
	stderr, err := base64.StdEncoding.DecodeString(resp.SockerlessExecResult.Stderr)
	if err != nil {
		return core.InvocationResult{}, nil, nil, false
	}
	inv := core.InvocationResult{ExitCode: resp.SockerlessExecResult.ExitCode}
	if inv.ExitCode != 0 {
		inv.Error = fmt.Sprintf("subprocess exit %d", inv.ExitCode)
	}
	return inv, stdout, stderr, true
}

func azfBootstrapExitCode(resp *http.Response) int {
	if hdr := resp.Header.Get("X-Sockerless-Exit-Code"); hdr != "" {
		if n, err := strconv.Atoi(hdr); err == nil {
			return n
		}
	}
	return core.HTTPStatusToExitCode(resp.StatusCode)
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
	// Record stop outcome so CloudState reports exited with 137.
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
		// `docker rm -f` is SIGKILL → exit 137.
		killExitCode := core.SignalToExitCode("SIGKILL")
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": fmt.Sprintf("%d", killExitCode),
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
	}

	s.StopHealthCheck(id)

	// Delete Function App. Errors propagate per the no-fallback rule.
	var cleanupErrs []error
	azfState, _ := s.AZF.Get(id)
	if azfState.FunctionAppName != "" {
		_, err := s.azure.WebApps.Delete(s.ctx(), s.config.ResourceGroup, azfState.FunctionAppName, nil)
		if err != nil && !azurecommon.IsNotFound(err) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete function app %q: %w", azfState.FunctionAppName, err))
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
			if derr := s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, id); derr != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("network %q disconnect: %w", ep.NetworkID, derr))
			}
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

// ContainerLogs streams container logs from Azure Monitor.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	return core.StreamCloudLogs(s.BaseServer, ref, opts, s.buildCloudLogsFetcher(ref), core.StreamCloudLogsOptions{
		CheckLogBuffers: true,
	})
}

// buildCloudLogsFetcher returns a CloudLogFetchFunc closure that
// queries Azure Monitor for the given function app's traces. Shared
// by ContainerLogs and ContainerAttach.
func (s *Server) buildCloudLogsFetcher(ref string) core.CloudLogFetchFunc {
	var functionAppName string
	if id, ok := s.ResolveContainerIDAuto(context.Background(), ref); ok {
		azfState, _ := s.AZF.Get(id)
		functionAppName = azfState.FunctionAppName
		if functionAppName == "" {
			functionAppName = "skls-" + id[:12]
		}
	}
	return s.azureLogsFetch(
		`AppTraces`,
		fmt.Sprintf(`AppRoleName == "%s"`, functionAppName),
		"Message",
	)
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
// EnableCommit.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if req.Container == "" {
		return nil, &api.InvalidParameterError{Message: "container query parameter is required"}
	}
	if !s.config.EnableCommit {
		return nil, &api.NotImplementedError{Message: "docker commit on Azure Functions is gated — set SOCKERLESS_ENABLE_COMMIT=1"}
	}
	return core.CommitContainerRequestViaAgent(s.BaseServer, s.reverseAgents, req)
}

// ContainerAttach bridges stdin/stdout/stderr to the bootstrap process
// inside the function container via the reverse-agent WebSocket when a
// session is registered. Without an agent, fall back to streaming
// Azure Monitor for read-only attach (no stdin); interactive attach
// has no native Azure Functions surface (Kudu uses a different
// protocol that's not implemented) and stays NotImplementedError.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	// gitlab-runner attach-stdin pattern: a per-stage / prepare script is
	// written to the container's MAIN process stdin. This must take precedence
	// over the reverse-agent routing below — the reverse-agent registers no main
	// process (rt.mp==nil; reverse mode carries only exec sessions), so a stdin
	// attach routed to it fails "no main process to attach to" and the stage
	// never runs. The captured script belongs on the buffered-invoke stdinPipe
	// path (drained + POSTed by invokeFunctionAsync on the matching start).
	if opts.Stdin && hasAZFOverlayRepo(c.Config.Image) {
		p := newStdinPipe()
		actual, _ := s.stdinPipes.LoadOrStore(c.ID, p)
		pipe := actual.(*stdinPipe)
		pipe.Open()
		return s.newAttachStream(c.ID, pipe), nil
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); hasAgent {
		return s.BaseServer.ContainerAttach(id, opts)
	}
	if opts.Stdin {
		return nil, &api.NotImplementedError{Message: "interactive docker attach requires a reverse-agent bootstrap inside the function container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	return core.AttachViaCloudLogs(s.BaseServer, id, opts, s.buildCloudLogsFetcher(id))
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
