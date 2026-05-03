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
		// BUG-918: replace bare digest ref with first RepoTag — Cloud
		// Run rewrites bare sha256: refs to mirror.gcr.io/library/...
		// which 404s. Image was pulled by tag so RepoTag exists.
		if strings.HasPrefix(config.Image, "sha256:") && len(img.RepoTags) > 0 {
			config.Image = img.RepoTags[0]
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

	// Named-volume binds (`-v volName:/mnt[:ro]`) land on sockerless-
	// managed GCS buckets via the underlying Cloud Run Service's
	// ServiceV2.Template.Volumes. Host-path binds translate via
	// SharedVolumes (config-driven). Mirror of `cloudrun.ContainerCreate`
	// translator + `lambda.fileSystemConfigsForBinds` shape (BUG-909).
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
	container.NetworkSettings.Networks[netName] = &api.EndpointSettings{
		NetworkID:   networkID,
		EndpointID:  core.GenerateID()[:16],
		Gateway:     "",
		IPAddress:   "",
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

	// Parent path used by CreateFunction below; the per-pool-shard
	// fullFunctionName is computed after the content tag is known.
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	_ = funcName // recomputed below once content tag drives the pool-shard naming
	var fullFunctionName string

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

	// Docker labels can contain `{`, `:`, `"` etc. which fail GCP's
	// label-value charset. Cloud Functions v2's
	// Function resource has no Annotations field (unlike Cloud Run's
	// Service resource), so carry the labels as a base64-encoded JSON
	// env var. CloudState.queryFunctions decodes it back into
	// container.Config.Labels.
	if len(config.Labels) > 0 {
		labelsJSON, _ := json.Marshal(config.Labels)
		envVars["SOCKERLESS_LABELS"] = base64.StdEncoding.EncodeToString(labelsJSON)
	}

	// Inject reverse-agent callback URL when configured so a bootstrap
	// inside the function container can dial back for docker top / exec
	// / cp. SOCKERLESS_CONTAINER_ID is already set above.
	if s.config.CallbackURL != "" {
		envVars["SOCKERLESS_CALLBACK_URL"] = s.config.CallbackURL
	}

	// Build service config
	serviceConfig := &functionspb.ServiceConfig{
		AvailableMemory:      s.config.Memory,
		AvailableCpu:         s.config.CPU,
		TimeoutSeconds:       int32(s.config.Timeout),
		EnvironmentVariables: envVars,
	}

	// Phase 122f: runner-pattern (long-lived) containers need
	// min_instance_count=1 so the underlying Cloud Run Service stays
	// warm between chained HTTP invocations (each docker exec = one
	// invocation). Detection mirrors cloudrun's isRunnerPattern.
	if isRunnerPatternGCF(&container) {
		serviceConfig.MinInstanceCount = 1
		s.Logger.Info().Str("container", id).Msg("Phase 122f: runner-pattern → min_instance_count=1")
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
		AutoRemove:  hostConfig.AutoRemove,
	}

	// Cloud Run Functions Gen2 deploy = stub-source CreateFunction +
	// post-create UpdateService image swap. See specs/CLOUD_RESOURCE_MAPPING.md
	// § GCP Cloud Run Functions for the full sequence rationale.
	overlaySpec := OverlayImageSpec{
		BaseImageRef:        config.Image,
		BootstrapBinaryPath: s.config.BootstrapBinaryPath,
		UserEntrypoint:      config.Entrypoint,
		UserCmd:             config.Cmd,
		UserWorkdir:         config.WorkingDir,
	}
	contentTag := OverlayContentTag(overlaySpec)
	overlayURI, err := s.ensureOverlayImage(s.ctx(), overlaySpec, contentTag)
	if err != nil {
		return nil, fmt.Errorf("ensure overlay image: %w", err)
	}

	// Pool query: try to claim a free pre-built Function with this overlay-hash.
	if claimed, claimErr := s.claimFreeFunction(s.ctx(), contentTag, id, name); claimErr == nil && claimed != "" {
		// Pool hit — function already exists with our overlay; allocation label was
		// CAS-claimed atomically. Skip CreateFunction + UpdateService entirely.
		s.PendingCreates.Put(id, container)
		// Stateless: do NOT cache function name/URL locally. Reads go to
		// `resolveGCFFromCloud` which queries `Functions.ListFunctions` by
		// `sockerless_allocation` label. The CAS claim above is the only
		// source of truth.
		s.Registry.Register(core.ResourceEntry{
			ContainerID:  id,
			Backend:      "gcf",
			ResourceType: "function",
			ResourceID:   claimed,
			InstanceID:   s.Desc.InstanceID,
			CreatedAt:    time.Now(),
			Metadata: map[string]string{
				"image":          container.Image,
				"name":           container.Name,
				"functionName":   shortFunctionName(claimed),
				"overlayHash":    contentTag,
				"reusedFromPool": "true",
			},
		})
		s.EmitEvent("container", "create", id, map[string]string{
			"name":  strings.TrimPrefix(name, "/"),
			"image": config.Image,
		})
		return &api.ContainerCreateResponse{ID: id, Warnings: []string{}}, nil
	}

	// Pool miss — provision a fresh Function. Stage stub source (once per project,
	// idempotent) and then call CreateFunction.
	stubObject := "sockerless-stub/sockerless-gcf-stub.zip"
	if err := stageStubSourceIfMissing(s.ctx(), s.gcp.Storage, s.config.BuildBucket, stubObject); err != nil {
		return nil, fmt.Errorf("stage stub source: %w", err)
	}

	// Pool-aware function name: include content-tag and a shard so multiple
	// pool entries for the same overlay coexist. Function ID rules: lowercase
	// alphanumeric + hyphen, max 63 chars.
	funcName = fmt.Sprintf("skls-%s-%s", contentTag, id[:6])
	fullFunctionName = fmt.Sprintf("%s/functions/%s", parent, funcName)

	// Pool labels: managed=true, overlay-hash=<tag>, allocation=<containerID>.
	if tags.Labels == nil {
		tags.Labels = make(map[string]string)
	}
	gcpLabels := tags.AsGCPLabels()
	gcpLabels["sockerless_overlay_hash"] = contentTag
	gcpLabels["sockerless_allocation"] = shortAllocLabel(id)

	// Create the Cloud Run Function with the stub Buildpacks-Go source. The
	// underlying Service's image gets replaced post-create via UpdateService.
	createReq := &functionspb.CreateFunctionRequest{
		Parent:     parent,
		FunctionId: funcName,
		Function: &functionspb.Function{
			Name:   fullFunctionName,
			Labels: gcpLabels,
			BuildConfig: &functionspb.BuildConfig{
				Runtime:    "go124",
				EntryPoint: "Stub",
				Source: &functionspb.Source{
					Source: &functionspb.Source_StorageSource{
						StorageSource: &functionspb.StorageSource{
							Bucket: s.config.BuildBucket,
							Object: stubObject,
						},
					},
				},
			},
			ServiceConfig: serviceConfig,
		},
	}
	_ = overlayURI // captured by deferred image swap below; suppress unused-warn until then.

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

	// Image swap: replace the stub Buildpacks-built image with our overlay
	// via Run.Services.UpdateService. Cloud Functions does not reconcile
	// Service.Template.Containers[0].Image; the swap holds for the lifetime
	// of the function. See specs/CLOUD_RESOURCE_MAPPING.md § GCP Cloud Run Functions.
	if err := s.swapServiceImage(s.ctx(), result, overlayURI); err != nil {
		// Best-effort cleanup so the create appears atomic.
		if delOp, delErr := s.gcp.Functions.DeleteFunction(s.ctx(), &functionspb.DeleteFunctionRequest{
			Name: fullFunctionName,
		}); delErr == nil {
			_ = delOp.Wait(s.ctx())
		}
		s.Logger.Error().Err(err).Str("function", funcName).Msg("failed to swap service image")
		return nil, &api.ServerError{Message: fmt.Sprintf("swap overlay image on %q: %v", funcName, err)}
	}

	// Re-read the function so result reflects post-swap state (URL, etc.).
	result, err = s.gcp.Functions.GetFunction(s.ctx(), &functionspb.GetFunctionRequest{Name: fullFunctionName})
	if err != nil {
		s.Logger.Warn().Err(err).Str("function", funcName).Msg("failed to re-read function after image swap")
	}

	// Authenticated invoke is required (see invokeFunction in containers.go).
	// We deliberately do NOT bind allUsers → roles/run.invoker — exposing the
	// function URL to the public internet to work around user-credential ADC's
	// inability to sign ID tokens would violate the operator's security
	// posture. If invocation fails with 403, the operator must switch to
	// service-account ADC; the failure surfaces in ContainerStart.

	// Function URL is now derived from cloud labels via resolveGCFFromCloud
	// at every read. No local cache. result is kept for the volume-attach
	// path below which needs the underlying-Service name.

	// If the request carries named-volume binds, attach them to the
	// underlying Cloud Run Service via the GetService /
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
	// Stateless: function name/URL are derived from cloud labels via
	// resolveGCFFromCloud — no local cache. functionURL captured above is
	// only used for the EmitEvent metadata payload below.

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
		return s.materializePodFunction(id, podContainers, exitCh)
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
	go func() {
		inv := core.InvocationResult{}
		if gcfState.FunctionURL == "" {
			s.Logger.Error().Str("function", gcfState.FunctionName).Msg("no function URL available for invocation")
			inv.ExitCode = 1
			inv.Error = "no function URL available"
		} else if resp, err := invokeFunction(s.ctx(), gcfState.FunctionURL); err != nil {
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
	if fullName != "" {
		fn, gerr := s.gcp.Functions.GetFunction(s.ctx(), &functionspb.GetFunctionRequest{Name: fullName})
		contentTag := ""
		if gerr == nil && fn != nil {
			contentTag = fn.GetLabels()["sockerless_overlay_hash"]
		}
		if err := s.releaseOrDeleteFunction(s.ctx(), fullName, contentTag); err != nil {
			s.Logger.Warn().Err(err).Str("function", fullName).Msg("pool release failed")
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
	resolved := gcpcommon.ResolveGCPImageURI(ref, s.config.Project, s.config.Region)
	if resolved != ref {
		auth = ""
	}
	return s.images.Pull(resolved, auth)
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
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); hasAgent {
		return s.BaseServer.ContainerAttach(id, opts)
	}
	if opts.Stdin {
		return nil, &api.NotImplementedError{Message: "interactive docker attach requires a reverse-agent bootstrap inside the function container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
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
