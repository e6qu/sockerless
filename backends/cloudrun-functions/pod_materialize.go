package gcf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// materializePodFunction collapses a multi-container pod into a single
// Cloud Run Function backed by a merged-rootfs overlay image. Called
// from ContainerStart on the LAST pod member's start (when
// PodDeferredStart returns shouldDefer=false and len(podContainers) > 1).
//
// Per spec § "Podman pods on FaaS backends — supervisor-in-overlay":
//
//  1. The overlay bakes each pod member's rootfs into /containers/<name>/.
//  2. The bootstrap (PID 1) parses SOCKERLESS_POD_CONTAINERS and forks
//     one chroot'd subprocess per member; the main member's stdout
//     becomes the HTTP response body, sidecars run in background.
//  3. Net+IPC+UTS namespaces are shared per podman pod default (matches);
//     mount-ns degraded to chroot path-isolation, PID-ns shared (no
//     CAP_SYS_ADMIN in Cloud Run sandbox). Surfaced to operators via
//     `docker inspect <pod-member>.HostConfig.MountNamespaceMode`.
//
// Per-member ContainerCreate already created throwaway single-container
// Functions; this materialisation deletes them atomically before
// creating the merged pod Function so the pod Function is the only
// cloud-side resource that maps back to any of the member containers.
//
// All members share the InvocationResult: the pod is one Function;
// `docker wait <any-member>` returns when the pod's HTTP invoke completes.
func (s *Server) materializePodFunction(mainContainerID string, containers []api.Container, exitCh chan struct{}) error {
	ctx := s.ctx()

	pod, _ := s.Store.Pods.GetPodForContainer(mainContainerID)
	podName := ""
	if pod != nil {
		podName = pod.Name
	}

	// 1. Build the pod overlay spec from the live container set. The
	//    main member is the LAST container in StartedIDs order — that's
	//    the one whose ContainerStart triggered this materialisation
	//    (gitlab-runner / github-runner start sidecars first and the
	//    main step container last; the trailing entry is the user's
	//    foreground workload).
	podSpec := containersToPodOverlaySpec(s.config.BootstrapBinaryPath, podName, mainContainerID, containers)
	contentTag := PodOverlayContentTag(podSpec)

	overlayURI, err := s.ensurePodOverlayImage(ctx, podSpec, contentTag)
	if err != nil {
		return fmt.Errorf("ensure pod overlay image: %w", err)
	}

	// 2. Atomically delete the per-member Functions created by ContainerCreate.
	//    On any deletion failure, we still proceed with pod-Function creation
	//    (the per-member Functions become unreferenced cloud debris that the
	//    operator's cleanup sweep handles). Logging surfaces the leak so it
	//    isn't silent.
	for _, c := range containers {
		state, ok := s.resolveGCFFromCloud(ctx, c.ID)
		if !ok || state.FunctionName == "" {
			continue
		}
		fullName := fmt.Sprintf("projects/%s/locations/%s/functions/%s",
			s.config.Project, s.config.Region, state.FunctionName)
		op, derr := s.gcp.Functions.DeleteFunction(ctx, &functionspb.DeleteFunctionRequest{
			Name: fullName,
		})
		if derr != nil {
			s.Logger.Warn().Err(derr).Str("function", fullName).Msg("pre-pod cleanup: per-member function delete failed")
			continue
		}
		if waitErr := op.Wait(ctx); waitErr != nil {
			s.Logger.Warn().Err(waitErr).Str("function", fullName).Msg("pre-pod cleanup: per-member function delete wait failed")
		}
		s.Registry.MarkCleanedUp(fullName)
	}

	// 3. Stage the stub Buildpacks-Go source (idempotent).
	stubObject := "sockerless-stub/sockerless-gcf-stub.zip"
	if err := stageStubSourceIfMissing(ctx, s.gcp.Storage, s.config.BuildBucket, stubObject); err != nil {
		return fmt.Errorf("stage stub source: %w", err)
	}

	// 4. Allocation label: the pod Function's `sockerless_allocation`
	//    holds the MAIN container's short ID. Cloud_state recovers
	//    membership for non-main pod containers via the
	//    `sockerless_pod=<podName>` label so all members map back to
	//    the same Function.
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	funcName := fmt.Sprintf("skls-pod-%s-%s", contentTag, mainContainerID[:6])
	fullFunctionName := fmt.Sprintf("%s/functions/%s", parent, funcName)

	tags := core.TagSet{
		ContainerID: mainContainerID,
		Backend:     "gcf",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
		AutoRemove:  false, // pod cleanup is explicit via ContainerRemove
	}
	gcpLabels := tags.AsGCPLabels()
	gcpLabels["sockerless_overlay_hash"] = contentTag
	gcpLabels["sockerless_allocation"] = shortAllocLabel(mainContainerID)
	if podName != "" {
		gcpLabels["sockerless_pod"] = sanitizePodLabelValue(podName)
	}

	// 5. Build the env vars carried on the pod Function. The pod manifest
	//    is also baked into the overlay's ENV so the bootstrap can read
	//    it from os.Environ() even on warm reuse where the runtime's
	//    environment overrides are unrelated. Carry it on the Function
	//    too so cloud_state can reconstruct membership without rebuilding.
	envVars := map[string]string{
		"SOCKERLESS_CONTAINER_ID": mainContainerID,
		"SOCKERLESS_POD_NAME":     podName,
	}
	if manifest, err := EncodePodManifest(podSpec.Members); err == nil {
		envVars["SOCKERLESS_POD_CONTAINERS"] = manifest
	}
	if podSpec.MainName != "" {
		envVars["SOCKERLESS_POD_MAIN"] = podSpec.MainName
	}
	// Network-pod sidecars share loopback in the merged-rootfs supervisor.
	// Surface the standard Docker NetworkingConfig.EndpointsConfig.Aliases
	// of every member so the bootstrap can write `127.0.0.1 <alias>` lines
	// to /etc/hosts before exec.
	netID := ""
	if id, ok := s.userDefinedNetworkID(containers[0]); ok {
		netID = id
	}
	if aliases := hostAliasesForMembers(containers, netID); len(aliases) > 0 {
		envVars["SOCKERLESS_HOST_ALIASES"] = strings.Join(aliases, ",")
	}

	serviceConfig := &functionspb.ServiceConfig{
		AvailableMemory:      s.config.Memory,
		AvailableCpu:         s.config.CPU,
		TimeoutSeconds:       int32(s.config.Timeout),
		EnvironmentVariables: envVars,
	}
	if s.config.ServiceAccount != "" {
		serviceConfig.ServiceAccountEmail = s.config.ServiceAccount
	}

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

	op, err := s.gcp.Functions.CreateFunction(ctx, createReq)
	if err != nil {
		return gcpcommon.MapGCPError(err, "function", funcName)
	}
	result, err := op.Wait(ctx)
	if err != nil {
		// Best-effort cleanup of the partially-created Function so the
		// pod start appears atomic.
		if delOp, delErr := s.gcp.Functions.DeleteFunction(ctx, &functionspb.DeleteFunctionRequest{
			Name: fullFunctionName,
		}); delErr == nil {
			_ = delOp.Wait(ctx)
		}
		return gcpcommon.MapGCPError(err, "function", funcName)
	}

	if err := s.swapServiceImage(ctx, result, overlayURI); err != nil {
		if delOp, delErr := s.gcp.Functions.DeleteFunction(ctx, &functionspb.DeleteFunctionRequest{
			Name: fullFunctionName,
		}); delErr == nil {
			_ = delOp.Wait(ctx)
		}
		return &api.ServerError{Message: fmt.Sprintf("swap pod overlay image on %q: %v", funcName, err)}
	}

	// Re-read so we have the post-swap ServiceConfig.Uri.
	result, _ = s.gcp.Functions.GetFunction(ctx, &functionspb.GetFunctionRequest{Name: fullFunctionName})

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainContainerID,
		Backend:      "gcf",
		ResourceType: "function",
		ResourceID:   fullFunctionName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata: map[string]string{
			"functionName": funcName,
			"podName":      podName,
			"podMembers":   fmt.Sprintf("%d", len(containers)),
			"overlayHash":  contentTag,
			"role":         "pod-supervisor",
		},
	})
	for _, c := range containers {
		s.EmitEvent("container", "start", c.ID, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
			"pod":  podName,
		})
	}

	// 6. Invoke the pod Function. Result fan-out: every pod member
	//    receives the same exit code + stdout buffer. `docker wait`
	//    on any member unblocks when the pod's HTTP invoke completes.
	go s.invokePodFunction(ctx, result, containers, exitCh)
	return nil
}

// invokePodFunction performs the HTTP invoke against the pod Function
// and fans the InvocationResult out to every member container. Each
// member's WaitCh is closed so `docker wait <any-member>` unblocks.
func (s *Server) invokePodFunction(ctx context.Context, result *functionspb.Function, containers []api.Container, mainExitCh chan struct{}) {
	inv := core.InvocationResult{}
	url := ""
	if result != nil && result.ServiceConfig != nil {
		url = result.ServiceConfig.Uri
	}

	if url == "" {
		s.Logger.Error().Msg("pod invoke: no function URL")
		inv.ExitCode = 1
		inv.Error = "no function URL available"
	} else if resp, err := invokeFunction(ctx, url); err != nil {
		s.Logger.Error().Err(err).Msg("pod function invocation failed")
		inv.ExitCode = core.HTTPInvokeErrorExitCode(err)
		inv.Error = err.Error()
	} else {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		// Each pod member shares the same response body (the supervisor
		// returns the main member's stdout as the body). Per-member log
		// streams ride Cloud Logging via the per-line `[<name>] ` prefix
		// the supervisor writes to its own stdout.
		for _, c := range containers {
			if len(body) > 0 && string(body) != "{}" {
				s.Store.LogBuffers.Store(c.ID, body)
			}
		}
		inv.ExitCode = core.HTTPStatusToExitCode(resp.StatusCode)
		if inv.ExitCode != 0 {
			inv.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}

	for _, c := range containers {
		s.Store.PutInvocationResult(c.ID, inv)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
			close(ch.(chan struct{}))
		}
	}
	// mainExitCh may already be closed by the WaitChs loop above; the
	// LoadAndDelete contract removes it before close, so the loop above
	// handles it. No double-close concern.
	_ = mainExitCh
}

// ensurePodOverlayImage builds + pushes the pod overlay image for `spec`
// to AR if it isn't already present. Returns the fully-qualified AR
// image URI. Mirrors `ensureOverlayImage` but for the pod variant.
func (s *Server) ensurePodOverlayImage(ctx context.Context, spec PodOverlaySpec, contentTag string) (string, error) {
	imageURI := fmt.Sprintf(
		"%s-docker.pkg.dev/%s/sockerless-overlay/gcf:%s",
		s.config.Region, s.config.Project, contentTag,
	)
	if s.images.BuildService == nil {
		return "", fmt.Errorf("cloud Build service is required for gcf pod overlay (set SOCKERLESS_GCP_BUILD_BUCKET)")
	}
	contextTar, err := TarPodOverlayContext(spec)
	if err != nil {
		return "", fmt.Errorf("tar pod overlay context: %w", err)
	}
	result, err := s.images.BuildService.Build(ctx, core.CloudBuildOptions{
		Dockerfile: "Dockerfile",
		ContextTar: bytes.NewReader(contextTar),
		Tags:       []string{imageURI},
		Platform:   "linux/amd64",
	})
	if err != nil {
		return "", fmt.Errorf("cloud build pod overlay %s: %w", imageURI, err)
	}
	return result.ImageRef, nil
}

// containersToPodOverlaySpec converts the live container set into the
// build-time PodOverlaySpec the renderer consumes. Member order is
// preserved so the pod's main container (the one whose ContainerStart
// triggered materialisation) lands as MainName.
func containersToPodOverlaySpec(bootstrapPath, podName, mainID string, containers []api.Container) PodOverlaySpec {
	members := make([]PodMemberSpec, 0, len(containers))
	mainName := ""
	for _, c := range containers {
		// Member name = container's docker name (without leading slash)
		// or its short ID when unnamed. Both round-trip cleanly through
		// /containers/<name>/ chroot subdirs and the supervisor log prefix.
		name := strings.TrimPrefix(c.Name, "/")
		if name == "" {
			name = c.ID[:12]
		}
		// Sanitise to lowercase alnum + dash so it's safe for both the
		// chroot path and the GCP label slot if we end up writing it.
		name = sanitizePodMemberName(name)

		members = append(members, PodMemberSpec{
			Name:         name,
			ContainerID:  c.ID,
			BaseImageRef: c.Config.Image,
			Entrypoint:   c.Config.Entrypoint,
			Cmd:          c.Config.Cmd,
			Workdir:      c.Config.WorkingDir,
			Env:          c.Config.Env,
		})
		if c.ID == mainID {
			mainName = name
		}
	}
	return PodOverlaySpec{
		PodName:             podName,
		MainName:            mainName,
		BootstrapBinaryPath: bootstrapPath,
		Members:             members,
	}
}

// sanitizePodMemberName lowercases and strips characters outside
// [a-z0-9-]. The result is safe for use as a chroot subdir AND a GCP
// label fragment. Empty results fall back to "x" so we never end up
// with /containers// or an empty member identifier.
func sanitizePodMemberName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-':
			b.WriteRune(r)
		case r == '_', r == '.':
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		out = "x"
	}
	return out
}

// sanitizePodLabelValue applies the GCP label-value charset to a pod
// name (lowercase letters + digits + `_-` only). Pods named with chars
// outside that set get the unsafe chars stripped; if the result would
// be empty we drop the label entirely (callers check for "").
func sanitizePodLabelValue(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}
