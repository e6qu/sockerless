package gcf

// Pod materialization via direct multi-container Cloud Run Service
// deploy (skipping the slow Cloud Functions wrapper).
//
// The previous implementation (materializePodFunction) did three
// sequential operations — (1) merged-rootfs Cloud Build for a pod
// overlay, (2) Functions.CreateFunction with a Buildpacks-Go stub
// source which itself runs Cloud Build internally + creates the
// underlying CR Service, and (3) Services.UpdateService to swap the
// stub image for the pod overlay. Total ~150-180 s, exceeding
// gitlab-runner's 120 s ContainerExec timeout.
//
// The new path mirrors what cloudrun does for pod-mode (~90 s total):
// build per-container overlay images in parallel, then call
// Services.CreateService once with a multi-container RevisionTemplate.
// Cloud Run schedules every member on the same instance — sidecars
// share loopback with main, so the postgres service is reachable from
// the build container via 127.0.0.1:5432 (the standard `services:`
// clause behaviour gitlab-runner expects).
//
// Track the resulting Service via labels so cloud_state can find pod
// members through Services.ListServices alongside the existing
// Functions.ListFunctions path.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
	"google.golang.org/api/idtoken"
	"google.golang.org/protobuf/types/known/durationpb"
)

// materializePodService deploys a multi-container pod as a single
// Cloud Run Service revision (one container per pod member). Returns
// the Service name; ContainerStart's caller uses it to invoke the
// pod's main bootstrap.
//
// Replaces materializePodFunction's slow Cloud Functions path. Same
// signature so backend_impl.go's call sites swap cleanly.
func (s *Server) materializePodService(mainContainerID string, containers []api.Container, exitCh chan struct{}) error {
	ctx := s.ctx()
	memberIDsLog := make([]string, len(containers))
	for i, c := range containers {
		memberIDsLog[i] = c.ID
	}
	s.Logger.Info().Str("main", mainContainerID).Strs("members", memberIDsLog).Msg("materializePodService: entry")
	defer s.Logger.Info().Str("main", mainContainerID).Msg("materializePodService: exit")

	pod, _ := s.Store.Pods.GetPodForContainer(mainContainerID)
	podName := ""
	if pod != nil {
		podName = pod.Name
	}

	// 1. Build per-member overlay images in parallel. Each member's
	//    overlay = (member's image, bootstrap binary) tuple — same
	//    content-hash as the single-container path, so prewarmed
	//    images already in AR (e.g. gitlab-runner-helper) get cache hits.
	type overlayResult struct {
		index int
		uri   string
		err   error
	}
	resultsCh := make(chan overlayResult, len(containers))
	var wg sync.WaitGroup
	for i, c := range containers {
		wg.Add(1)
		go func(idx int, container api.Container) {
			defer wg.Done()
			imageRef := gcpcommon.ResolveGCPImageURI(container.Config.Image, s.config.Project, s.config.Region, s.config.EndpointURL)
			spec := OverlayImageSpec{
				BaseImageRef:        imageRef,
				BootstrapBinaryPath: s.config.BootstrapBinaryPath,
				BootstrapBinaryHash: s.config.BootstrapBinaryHash,
			}
			tag := OverlayContentTag(spec)
			uri, err := s.ensureOverlayImage(ctx, spec, tag)
			resultsCh <- overlayResult{index: idx, uri: uri, err: err}
		}(i, c)
	}
	wg.Wait()
	close(resultsCh)
	overlayURIs := make([]string, len(containers))
	for r := range resultsCh {
		if r.err != nil {
			return fmt.Errorf("ensure overlay image for pod member %d: %w", r.index, r.err)
		}
		overlayURIs[r.index] = r.uri
	}

	// 2. Atomically delete any per-member Functions left over from the
	//    cancelled async deploys (the network-pod path cancels them when
	//    deferring to materialize). Best-effort — leftovers become
	//    sweep-able orphans rather than blocking the materialize.
	for _, c := range containers {
		state, ok := s.resolveGCFFromCloud(ctx, c.ID)
		if !ok || state.FunctionName == "" {
			continue
		}
		fullName := fmt.Sprintf("projects/%s/locations/%s/functions/%s",
			s.config.Project, s.config.Region, state.FunctionName)
		if delOp, derr := s.gcp.Functions.DeleteFunction(ctx, &functionspb.DeleteFunctionRequest{Name: fullName}); derr == nil {
			_ = delOp.Wait(ctx)
			s.Registry.MarkCleanedUp(fullName)
		}
	}

	// 3. Build the multi-container Cloud Run Service spec.
	svcName := podServiceName(mainContainerID)
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	fullSvcName := fmt.Sprintf("%s/services/%s", parent, svcName)

	specs, volumes, persistEntries, err := s.buildPodContainerSpecs(ctx, containers, overlayURIs)
	if err != nil {
		return err
	}
	injectPodPersistEnv(specs, persistEntries)
	injectPodHostAliases(specs, containers, s.userDefinedNetworkIDOrEmpty(containers[0]))

	revTemplate := &runpb.RevisionTemplate{
		Containers: specs,
		Volumes:    volumes,
		Scaling: &runpb.RevisionScaling{
			// Scale to zero between exec POSTs so the regional CPU
			// quota isn't pinned by always-on revisions. Each ad-hoc
			// pod-Service serves a few /exec POSTs over its short lifetime;
			// keeping a min-1 instance burns ~1-2 vCPUs of regional quota
			// for the entire pipeline duration even when idle. With the 5+
			// pod-Services a single GH actions/runner pipeline spawns, that
			// adds up to >10 vCPU of quota debt that survives across the
			// pipeline and causes later container deploys to fail with the
			// misleading "container failed to bind PORT=8080" error. The
			// cold-start latency on first /exec POST after idle is <5s
			// (bootstrap binds 8080 in ~1.4s on Cloud Run direct deploy).
			MinInstanceCount: 0,
			MaxInstanceCount: 1,
		},
		Timeout: durationpb.New(1 * time.Hour),
	}
	// VPC connector + ALL_TRAFFIC routes per-step Service POSTs through
	// the in-VPC source so cross-Cloud-Run calls (gitlab-runner-gcf
	// invoking sockerless-svc-* over .a.run.app) appear as same-project
	// to Cloud Run's edge — without it Cloud Run rejects them as
	// external. Mirror of cloudrun/servicespec.go::buildServiceSpec
	// VpcAccess block. Required for cell 7 cloudrun GREEN — also needed
	// for cell 8 gcf network-pod parity.
	if s.config.VPCConnector != "" {
		revTemplate.VpcAccess = &runpb.VpcAccess{
			Connector: s.config.VPCConnector,
			Egress:    runpb.VpcAccess_ALL_TRAFFIC,
		}
	}

	tags := core.TagSet{
		ContainerID: mainContainerID,
		Backend:     "gcf",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
		AutoRemove:  false,
	}
	gcpLabels := tags.AsGCPLabels()
	gcpLabels["sockerless_managed"] = "true"
	gcpLabels["sockerless_allocation"] = shortAllocLabel(mainContainerID)
	if podName != "" {
		gcpLabels["sockerless_pod"] = sanitizePodLabelValue(podName)
	}
	if owner := gcpcommon.OwnerRunnerTaskLabelValue(); owner != "" {
		gcpLabels[gcpcommon.OwnerRunnerTaskLabel] = owner
	}
	gcpAnnotations := tags.AsGCPAnnotations()
	if gcpAnnotations == nil {
		gcpAnnotations = map[string]string{}
	}
	memberIDs := make([]string, 0, len(containers))
	for _, c := range containers {
		memberIDs = append(memberIDs, c.ID)
	}
	gcpAnnotations["sockerless_pod_members"] = strings.Join(memberIDs, ",")

	svc := &runpb.Service{
		Labels:             gcpLabels,
		Annotations:        gcpAnnotations,
		Ingress:            runpb.IngressTraffic_INGRESS_TRAFFIC_ALL,
		DefaultUriDisabled: false,
		Template:           revTemplate,
	}

	// 4. Deploy directly via Services.CreateService — no Cloud Functions
	//    wrapper, no buildpacks build, no swap. Same primitive cloudrun
	//    uses for cell 7's pod-materialize, which deploys in ~30-60 s.
	createOp, err := s.gcp.Services.CreateService(ctx, &runpb.CreateServiceRequest{
		Parent:    parent,
		ServiceId: svcName,
		Service:   svc,
	})
	if err != nil {
		return gcpcommon.MapGCPError(err, "service", svcName)
	}
	result, err := createOp.Wait(ctx)
	if err != nil {
		// Best-effort cleanup on partial-create failure.
		if delOp, delErr := s.gcp.Services.DeleteService(ctx, &runpb.DeleteServiceRequest{Name: fullSvcName}); delErr == nil {
			_, _ = delOp.Wait(ctx)
		}
		return gcpcommon.MapGCPError(err, "service", svcName)
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainContainerID,
		Backend:      "gcf",
		ResourceType: "service",
		ResourceID:   result.Name,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata: map[string]string{
			"serviceName": svcName,
			"podName":     podName,
			"podMembers":  fmt.Sprintf("%d", len(containers)),
			"role":        "pod-multi-container-service",
		},
	})
	for _, c := range containers {
		s.EmitEvent("container", "start", c.ID, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
			"pod":  podName,
		})
	}

	// 5. Invoke the pod's ingress container (main is index 0). The
	//    bootstrap on main runs the user's argv via the exec envelope
	//    posted to the Service URL. skipIfNoStdin=true mirrors the
	//    Pod-Service materializes are runner-style long-lived containers
	//    that must NOT default-invoke when no stdinPipe is captured.
	//    The bootstrap stays on its HTTP listener for the runner's
	//    subsequent /exec POSTs instead.
	go s.invokePodServiceMain(ctx, result, containers, exitCh, true)
	return nil
}

// podServiceName returns the Cloud Run Service name for a pod. Same
// shape as buildServiceName in cloudrun ("sockerless-svc-<id12>") so
// cloud_state heuristics that match the prefix still find it.
func podServiceName(mainID string) string {
	if len(mainID) < 12 {
		return "sockerless-svc-" + mainID
	}
	return "sockerless-svc-" + mainID[:12]
}

// buildPodServiceParent returns the Cloud Run parent path for pod-
// mode Services. Used by cloud_state.queryPodServiceContainers when
// it needs to detect whether a list-result Service is one of ours
// before issuing a GetService follow-up.
func (s *Server) buildPodServiceParent() string {
	return fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
}

// deployContainerService creates a single-container Cloud Run Service
// for a sockerless container. This replaces the slow Cloud Functions
// path (CreateFunction with stub Buildpacks-Go source + UpdateService
// swap) with a direct Services.CreateService call, mirroring cloudrun's
// single-container deploy speed (~30-60 s vs the old ~90-150 s).
//
// Used for cell 8's cache-permission helper container (a solo
// container that arrives before its sibling build/postgres members).
// Multi-container pods continue through materializePodService.
//
// The Service is tagged sockerless_managed=true with sockerless_allocation
// = short(containerID); resolveGCFFromCloud → resolvePodServiceFromCloud
// finds it by allocation label.
func (s *Server) deployContainerService(ctx context.Context, id string, container api.Container) error {
	imageRef := gcpcommon.ResolveGCPImageURI(container.Config.Image, s.config.Project, s.config.Region, s.config.EndpointURL)
	spec := OverlayImageSpec{
		BaseImageRef:        imageRef,
		BootstrapBinaryPath: s.config.BootstrapBinaryPath,
		BootstrapBinaryHash: s.config.BootstrapBinaryHash,
	}
	tag := OverlayContentTag(spec)
	overlayURI, err := s.ensureOverlayImage(ctx, spec, tag)
	if err != nil {
		return fmt.Errorf("ensure overlay image: %w", err)
	}

	cs, mounts, err := s.buildPodContainerSpec(container, overlayURI, true)
	if err != nil {
		return err
	}
	specs := []*runpb.Container{cs}

	volSeen := map[string]struct{}{}
	var volumes []*runpb.Volume
	var persistEntries []string
	for _, mp := range mounts {
		if _, done := volSeen[mp.Name]; done {
			continue
		}
		vol, persist, verr := s.buildVolumeForBindGCF(ctx, mp.Name, mp.MountPath)
		if verr != nil {
			return verr
		}
		volumes = append(volumes, vol)
		if persist != "" {
			persistEntries = append(persistEntries, persist)
		}
		volSeen[mp.Name] = struct{}{}
	}
	injectPodPersistEnv(specs, persistEntries)

	revTemplate := &runpb.RevisionTemplate{
		Containers: specs,
		Volumes:    volumes,
		Scaling: &runpb.RevisionScaling{
			// Scale to zero between exec POSTs so the regional CPU
			// quota isn't pinned by always-on revisions. Each ad-hoc
			// pod-Service serves a few /exec POSTs over its short lifetime;
			// keeping a min-1 instance burns ~1-2 vCPUs of regional quota
			// for the entire pipeline duration even when idle. With the 5+
			// pod-Services a single GH actions/runner pipeline spawns, that
			// adds up to >10 vCPU of quota debt that survives across the
			// pipeline and causes later container deploys to fail with the
			// misleading "container failed to bind PORT=8080" error. The
			// cold-start latency on first /exec POST after idle is <5s
			// (bootstrap binds 8080 in ~1.4s on Cloud Run direct deploy).
			MinInstanceCount: 0,
			MaxInstanceCount: 1,
		},
		Timeout: durationpb.New(1 * time.Hour),
	}
	if s.config.VPCConnector != "" {
		revTemplate.VpcAccess = &runpb.VpcAccess{
			Connector: s.config.VPCConnector,
			Egress:    runpb.VpcAccess_ALL_TRAFFIC,
		}
	}

	tags := core.TagSet{
		ContainerID: id,
		Backend:     "gcf",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
		AutoRemove:  container.HostConfig.AutoRemove,
	}
	gcpLabels := tags.AsGCPLabels()
	gcpLabels["sockerless_managed"] = "true"
	gcpLabels["sockerless_allocation"] = shortAllocLabel(id)
	if owner := gcpcommon.OwnerRunnerTaskLabelValue(); owner != "" {
		gcpLabels[gcpcommon.OwnerRunnerTaskLabel] = owner
	}
	gcpAnnotations := tags.AsGCPAnnotations()
	if gcpAnnotations == nil {
		gcpAnnotations = map[string]string{}
	}
	// Single-container deploys still set sockerless_pod_members with
	// just this container's ID so cloud_state.queryPodServiceContainers
	// finds it through the same path multi-container resources use.
	gcpAnnotations["sockerless_pod_members"] = id

	parent := s.buildPodServiceParent()
	svcName := podServiceName(id)
	fullSvcName := fmt.Sprintf("%s/services/%s", parent, svcName)

	svc := &runpb.Service{
		Labels:             gcpLabels,
		Annotations:        gcpAnnotations,
		Ingress:            runpb.IngressTraffic_INGRESS_TRAFFIC_ALL,
		DefaultUriDisabled: false,
		Template:           revTemplate,
	}

	createOp, err := s.gcp.Services.CreateService(ctx, &runpb.CreateServiceRequest{
		Parent:    parent,
		ServiceId: svcName,
		Service:   svc,
	})
	if err != nil {
		return gcpcommon.MapGCPError(err, "service", svcName)
	}
	result, err := createOp.Wait(ctx)
	if err != nil {
		// Best-effort cleanup on partial-create failure.
		if delOp, delErr := s.gcp.Services.DeleteService(ctx, &runpb.DeleteServiceRequest{Name: fullSvcName}); delErr == nil {
			_, _ = delOp.Wait(ctx)
		}
		return gcpcommon.MapGCPError(err, "service", svcName)
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "gcf",
		ResourceType: "service",
		ResourceID:   result.Name,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata: map[string]string{
			"image":       container.Image,
			"name":        container.Name,
			"serviceName": svcName,
			"role":        "single-container-service",
		},
	})
	return nil
}

// userDefinedNetworkIDOrEmpty is a non-erroring variant for the
// SOCKERLESS_HOST_ALIASES injection — empty network ID means the
// hostAliasesForMembers helper falls back to per-container aliases.
func (s *Server) userDefinedNetworkIDOrEmpty(c api.Container) string {
	if id, ok := s.userDefinedNetworkID(c); ok {
		return id
	}
	return ""
}

// buildPodContainerSpecs builds per-member Cloud Run Container specs
// for a multi-container Service revision. Each member's image is the
// caller-built overlay (so the bootstrap is the ENTRYPOINT). Container
// 0 (main) binds PORT for the Cloud Run ingress probe; the rest run
// as sidecars (SOCKERLESS_SIDECAR=1, no port).
//
// Volume mounts are translated from the caller's HostConfig.Binds:
// non-shared volumes become Volume_EmptyDir{MEMORY} backed by tar-pack
// persistence; shared volumes stay as raw GCSFuse.
func (s *Server) buildPodContainerSpecs(ctx context.Context, containers []api.Container, overlayURIs []string) ([]*runpb.Container, []*runpb.Volume, []string, error) {
	specs := make([]*runpb.Container, 0, len(containers))
	volSeen := map[string]struct{}{}
	nameSeen := map[string]int{}
	var volumes []*runpb.Volume
	var persistEntries []string
	for i, c := range containers {
		isMain := i == 0
		spec, mounts, err := s.buildPodContainerSpec(c, overlayURIs[i], isMain)
		if err != nil {
			return nil, nil, nil, err
		}
		// Ensure unique container names across the pod. Sanitization may
		// collapse two distinct docker container names to the same
		// Cloud Run name (e.g. two postgres sidecars from prepare-stage
		// + step-script stage). Cloud Run rejects duplicates with
		// `template.containers: Containers [N, M] have duplicate container names`.
		base := spec.Name
		count := nameSeen[base]
		nameSeen[base] = count + 1
		if count > 0 {
			spec.Name = fmt.Sprintf("%s-%d", base, count)
		}
		specs = append(specs, spec)
		for _, mp := range mounts {
			if _, done := volSeen[mp.Name]; done {
				continue
			}
			vol, persist, verr := s.buildVolumeForBindGCF(ctx, mp.Name, mp.MountPath)
			if verr != nil {
				return nil, nil, nil, verr
			}
			volumes = append(volumes, vol)
			if persist != "" {
				persistEntries = append(persistEntries, persist)
			}
			volSeen[mp.Name] = struct{}{}
		}
	}
	return specs, volumes, persistEntries, nil
}

// buildPodContainerSpec produces one runpb.Container for a pod member.
// The bootstrap is already baked in via the overlay image; the
// caller's entrypoint+cmd+workdir flow through env so the bootstrap
// reads them at runtime — keeping argv out of the image content keeps
// the overlay tag stable across containers that share the same base
// image + bootstrap pair.
func (s *Server) buildPodContainerSpec(c api.Container, overlayURI string, isMain bool) (*runpb.Container, []*runpb.VolumeMount, error) {
	cfg := c.Config
	envVars := make([]*runpb.EnvVar, 0, len(cfg.Env)+8)
	for _, e := range cfg.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars = append(envVars, &runpb.EnvVar{
				Name:   parts[0],
				Values: &runpb.EnvVar_Value{Value: parts[1]},
			})
		}
	}
	if len(cfg.Entrypoint) > 0 {
		if b, err := json.Marshal(cfg.Entrypoint); err == nil {
			envVars = append(envVars, &runpb.EnvVar{
				Name:   "SOCKERLESS_USER_ENTRYPOINT",
				Values: &runpb.EnvVar_Value{Value: base64.StdEncoding.EncodeToString(b)},
			})
		}
	}
	if len(cfg.Cmd) > 0 {
		if b, err := json.Marshal(cfg.Cmd); err == nil {
			envVars = append(envVars, &runpb.EnvVar{
				Name:   "SOCKERLESS_USER_CMD",
				Values: &runpb.EnvVar_Value{Value: base64.StdEncoding.EncodeToString(b)},
			})
		}
	}
	if cfg.WorkingDir != "" {
		envVars = append(envVars, &runpb.EnvVar{
			Name:   "SOCKERLESS_USER_WORKDIR",
			Values: &runpb.EnvVar_Value{Value: cfg.WorkingDir},
		})
	}
	if !isMain {
		envVars = append(envVars, &runpb.EnvVar{
			Name:   "SOCKERLESS_SIDECAR",
			Values: &runpb.EnvVar_Value{Value: "1"},
		})
	}

	defName := "main"
	if !isMain {
		defName = sanitizeServiceContainerName(c.Name)
	}

	var mounts []*runpb.VolumeMount
	var syncMountEntries []string
	for _, bind := range c.HostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		mounts = append(mounts, &runpb.VolumeMount{
			Name:      parts[0],
			MountPath: parts[1],
		})
		// When this bind references a gcs-sync SharedVolume, record the
		// mount path so the bootstrap-side restore knows where to untar
		// each per-exec GCS object. The runner-task's PreExec emits just
		// `name=GCS_URL` (the bind target lives on the JOB container
		// side, not the runner-task side); the bootstrap joins the two
		// by name at exec time.
		// At ContainerCreate time backend_impl rewrites bind sources from
		// host-path (`/tmp/runner-work`) to SharedVolume.Name
		// (`runner-workspace`) — see backend_impl.go::overlayHostConfig.
		// So at materialize time parts[0] IS already the SharedVolume
		// name; look up directly by name.
		if sv := s.config.LookupSharedVolumeByName(parts[0]); sv != nil && core.StorageBacking(sv.Backing) == core.BackingGCSSync {
			syncMountEntries = append(syncMountEntries, fmt.Sprintf("%s=%s", sv.Name, parts[1]))
		}
	}
	if isMain && len(syncMountEntries) > 0 {
		envVars = append(envVars, &runpb.EnvVar{
			Name:   "SOCKERLESS_SYNC_MOUNTS",
			Values: &runpb.EnvVar_Value{Value: strings.Join(syncMountEntries, ",")},
		})
	}

	cs := &runpb.Container{
		Name:         defName,
		Image:        overlayURI,
		Env:          envVars,
		VolumeMounts: mounts,
		Resources: &runpb.ResourceRequirements{
			Limits: map[string]string{
				"cpu":    s.config.CPU,
				"memory": memoryLimitForContainer(s.config.Memory, isMain),
			},
		},
	}
	if isMain {
		cs.Ports = []*runpb.ContainerPort{{ContainerPort: 8080}}
	}
	// Deliberately DO NOT set cs.WorkingDir. Cloud Run validates the
	// WorkingDir exists on the container's mount filesystem before
	// starting the process — under gcs-sync the workspace mount is an
	// empty tmpfs at boot (the bootstrap restores from GCS per-exec),
	// so a workdir like /__w/sockerless/sockerless wouldn't exist yet
	// and Cloud Run would fail "Application failed to start: failed
	// to find initial working directory". The bootstrap chdir's per-
	// exec via envelope.Workdir (runExecEnvelope) and per-default-
	// invoke via SOCKERLESS_USER_WORKDIR env, so the user's workdir
	// is still honoured at the right scope.
	return cs, mounts, nil
}

// sanitizeServiceContainerName converts a docker container name to a
// valid Cloud Run container name (RFC 1123 lowercase + digits + hyphen,
// must begin/end with letter or digit, < 64 chars).
func sanitizeServiceContainerName(name string) string {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return "sidecar"
	}
	var b strings.Builder
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		case c >= 'A' && c <= 'Z':
			b.WriteRune(c + 32)
		case c == '-' || c == '.':
			b.WriteRune(c)
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	for len(out) > 0 && (out[0] < 'a' || out[0] > 'z') && (out[0] < '0' || out[0] > '9') {
		out = out[1:]
	}
	for len(out) > 0 && (out[len(out)-1] < 'a' || out[len(out)-1] > 'z') && (out[len(out)-1] < '0' || out[len(out)-1] > '9') {
		out = out[:len(out)-1]
	}
	if out == "" {
		return "sidecar"
	}
	if len(out) > 50 {
		out = out[:50]
	}
	// Re-trim trailing non-alphanumeric AFTER the truncation — the cut
	// can land on a hyphen (e.g. GH actions/runner names like
	// `<32hex>_postgres16alpine_<6hex>` truncate to end in `-`), which
	// fails Cloud Run's RFC 1123 "must end with letter or digit" check.
	for len(out) > 0 && (out[len(out)-1] < 'a' || out[len(out)-1] > 'z') && (out[len(out)-1] < '0' || out[len(out)-1] > '9') {
		out = out[:len(out)-1]
	}
	if out == "" {
		return "sidecar"
	}
	return out
}

// buildVolumeForBindGCF mirrors the cloudrun buildVolumeForBind
// helper. Operator-pinned SharedVolume entries route through the
// storage backing driver (gcs-fuse / gcs-sync per the
// SharedVolume.Backing field); ad-hoc binds (no SharedVolume entry) get
// Volume_EmptyDir{MEMORY} + a SOCKERLESS_PERSIST_VOLUMES entry for the
// bootstrap's tar-pack persistence.
func (s *Server) buildVolumeForBindGCF(ctx context.Context, volName, mountPath string) (*runpb.Volume, string, error) {
	bucket, err := s.bucketForVolume(ctx, volName)
	if err != nil {
		return nil, "", fmt.Errorf("provision GCS bucket for volume %q: %w", volName, err)
	}
	if shared := s.config.LookupSharedVolumeByName(volName); shared != nil {
		// Route through the storage backing driver. The driver's
		// CloudSpec returns a cloud-agnostic BackingSpec; the
		// translator emits the runpb.Volume. Empty Backing on a
		// SharedVolume falls through to gcs-fuse (legacy default for
		// cells 7+8 — see SharedVolume.AsRef).
		vol := *shared
		if vol.Bucket == "" {
			vol.Bucket = bucket
		}
		runVol, err := s.cloudRunVolumeFromBacking(vol)
		if err != nil {
			return nil, "", err
		}
		return runVol, "", nil
	}
	// Ad-hoc bind: in-memory tmpfs + tar-pack persist hint.
	return &runpb.Volume{
		Name: volName,
		VolumeType: &runpb.Volume_EmptyDir{
			EmptyDir: &runpb.EmptyDirVolumeSource{
				Medium: runpb.EmptyDirVolumeSource_MEMORY,
			},
		},
	}, fmt.Sprintf("%s=%s=%s", volName, mountPath, bucket), nil
}

// injectPodPersistEnv appends SOCKERLESS_PERSIST_VOLUMES to the main
// (index 0) container so the bootstrap's tar-pack module restores +
// saves bind volumes across exec boundaries.
func injectPodPersistEnv(specs []*runpb.Container, entries []string) {
	if len(entries) == 0 || len(specs) == 0 {
		return
	}
	specs[0].Env = append(specs[0].Env, &runpb.EnvVar{
		Name:   "SOCKERLESS_PERSIST_VOLUMES",
		Values: &runpb.EnvVar_Value{Value: strings.Join(entries, ",")},
	})
}

// injectPodHostAliases sets SOCKERLESS_HOST_ALIASES on the main
// container so the bootstrap writes `127.0.0.1 <alias>` lines to
// /etc/hosts. Sidecars share loopback in a multi-container Cloud Run
// revision, so a postgres sidecar's port 5432 is reachable from main
// at 127.0.0.1:5432 once the alias is resolvable.
func injectPodHostAliases(specs []*runpb.Container, members []api.Container, netID string) {
	if len(specs) == 0 || len(members) <= 1 {
		return
	}
	aliases := hostAliasesForMembers(members, netID)
	if len(aliases) == 0 {
		return
	}
	specs[0].Env = append(specs[0].Env, &runpb.EnvVar{
		Name:   "SOCKERLESS_HOST_ALIASES",
		Values: &runpb.EnvVar_Value{Value: strings.Join(aliases, ",")},
	})
}

// invokePodServiceMain POSTs an exec envelope to the Service's URI
// and fans the InvocationResult out to every pod member. Mirrors
// cloudrun's invokeServiceDefaultCmd: when an attach-stdin pipe was
// registered (gitlab-runner attach pattern), waits for stdin EOF
// before POSTing the captured script bytes as the envelope. Without
// this wait, gcf POSTed the user's default CMD immediately on
// materialize completion, which exits 0 with no work and closes
// WaitChs before gitlab-runner can attach + pipe its script.
//
// `skipIfNoStdin`: when true, also skip the default-invoke POST for
// OpenStdin=false main containers. The GH actions/runner pattern
// materializes a long-lived `tail -f /dev/null`-style JOB container
// that the runner expects to keep alive for `docker exec`. POSTing
// the default CMD would run that long-lived process as a one-shot
// subprocess and block forever, holding invokeMu so the subsequent
// /exec POST never reaches the bootstrap. Mirrors the cloudrun
// skip-default-invoke path.
func (s *Server) invokePodServiceMain(ctx context.Context, svc *runpb.Service, containers []api.Container, _ chan struct{}, skipIfNoStdin bool) {
	mainContainer := containers[0]
	mainID := mainContainer.ID

	s.Logger.Info().Str("main", mainID).Msg("invokePodServiceMain: goroutine entered")

	// gitlab-runner attach-stdin pattern: when the build container
	// has a stdinPipe registered (via ContainerAttach), wait for the
	// caller to half-close (= stdin EOF) so we can replay the captured
	// bytes as the bootstrap's exec envelope Stdin.
	//
	// Race with ContainerStart return: gitlab-runner v17 docker
	// executor calls ContainerStart, then ContainerAttach. There's a
	// window where this goroutine has started (after Services.CreateService
	// completes) but ContainerAttach hasn't hit yet. Without a pre-check
	// wait we'd LoadAndDelete an empty map and fall through to default-
	// invoke, racing past gitlab-runner's pipe registration. cloudrun
	// hides this race behind waitForServiceURL's polling delay; we
	// replicate it explicitly with a short pre-check window for any
	// late-arriving attach.
	var capturedStdin []byte
	pipeWaitDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(pipeWaitDeadline) {
		if _, ok := s.stdinPipes.Load(mainID); ok {
			break
		}
		select {
		case <-ctx.Done():
			s.Logger.Warn().Str("main", mainID).Err(ctx.Err()).Msg("invokePodServiceMain: ctx cancelled while waiting for stdinPipe registration")
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	if v, ok := s.stdinPipes.LoadAndDelete(mainID); ok {
		pipe := v.(*stdinPipe)
		s.Logger.Info().Str("main", mainID).Msg("invokePodServiceMain: stdinPipe registered, waiting for stdin EOF")
		// 30s upper bound mirrors cloudrun's invokeServiceDefaultCmd.
		select {
		case <-pipe.Done():
		case <-time.After(30 * time.Second):
			s.Logger.Warn().Str("main", mainID).Msg("invokePodServiceMain: stdin pipe Done timeout — proceeding with whatever was captured")
		case <-ctx.Done():
			s.Logger.Warn().Str("main", mainID).Err(ctx.Err()).Msg("invokePodServiceMain: ctx cancelled while waiting on stdinPipe")
			return
		}
		capturedStdin = pipe.Bytes()
		s.Logger.Info().Str("main", mainID).Int("stdin_bytes", len(capturedStdin)).Msg("invokePodServiceMain: stdin pipe drained")
	} else {
		s.Logger.Info().Str("main", mainID).Msg("invokePodServiceMain: no stdinPipe registered after 5 s wait — falling through to default-invoke")
	}

	url := ""
	if svc != nil {
		url = svc.Uri
	}

	inv := core.InvocationResult{}
	var stdoutResp, stderrResp []byte

	if url == "" {
		s.Logger.Error().Str("main", mainID).Msg("pod service invoke: no service URL")
		inv.ExitCode = 1
		inv.Error = "no service URL available"
	} else if len(capturedStdin) > 0 {
		// Attach-stdin path: POST the captured script bytes as the
		// envelope's Stdin. Bootstrap pipes the script into sh.
		s.Logger.Info().Str("main", mainID).Str("url", url).Int("stdin_bytes", len(capturedStdin)).Msg("invokePodServiceMain: posting envelope with captured stdin")
		client, err := idtoken.NewClient(ctx, url)
		if err != nil {
			s.Logger.Error().Err(err).Str("main", mainID).Msg("idtoken client for pod service invoke")
			inv.ExitCode = core.HTTPInvokeErrorExitCode(err)
			inv.Error = err.Error()
		} else {
			client.Timeout = 10 * time.Minute
			res, err := gcpcommon.PostExecEnvelope(ctx, client, url, "", gcpcommon.ExecEnvelopeExec{
				Argv:  []string{"/bin/sh"},
				Stdin: gcpcommon.EncodeStdin(capturedStdin),
			})
			if err != nil {
				s.Logger.Error().Err(err).Str("main", mainID).Msg("pod service envelope POST failed")
				inv.ExitCode = core.HTTPInvokeErrorExitCode(err)
				inv.Error = err.Error()
			} else {
				s.Logger.Info().Str("main", mainID).Int("exit", res.ExitCode).Int("stdout_bytes", len(res.Stdout)).Int("stderr_bytes", len(res.Stderr)).Msg("invokePodServiceMain: bootstrap response")
				stdoutResp = res.Stdout
				stderrResp = res.Stderr
				inv.ExitCode = res.ExitCode
				if inv.ExitCode != 0 {
					inv.Error = fmt.Sprintf("subprocess exit %d", inv.ExitCode)
				}
			}
		}
	} else if mainContainer.Config.OpenStdin {
		// Runner-pattern container (OpenStdin=true) without a captured
		// stdin: this is a long-lived build container that gitlab-runner
		// will docker-exec into for each stage's script. The bootstrap
		// is already up as the HTTP server holding the Service revision
		// alive. DON'T POST anything — we want the container to stay
		// "running" indefinitely, NOT exited. gitlab-runner v17's docker
		// executor expects the build container to persist across stages
		// so it can issue ContainerExec calls. Posting the user CMD here
		// (gitlab-runner-build / dumb-init) would exit 0 with no work,
		// close WaitChs, and make sockerless report the container as
		// exited — gitlab-runner then waits silently for a container
		// it thinks is dead.
		s.Logger.Info().Str("main", mainID).Msg("invokePodServiceMain: OpenStdin runner-pattern — skipping default-invoke; container stays alive for docker-exec")
		// invokePodServiceMain returns without closing WaitChs or
		// PutInvocationResult. ContainerRemove (via gitlab-runner's
		// cleanup) will eventually delete the Service. Until then the
		// build container is "running" from sockerless's perspective.
		return
	} else if skipIfNoStdin {
		// GH actions/runner pattern: the runner-task spawns a JOB
		// container with OpenStdin=false but a long-lived
		// `tail -f /dev/null`-style entrypoint. The runner then issues
		// `docker exec <job> sh -c <step.sh>` for each step. Like the
		// OpenStdin path above, we must NOT default-invoke — that would
		// run the long-lived process as a one-shot subprocess and
		// block invokeMu forever. The bootstrap stays listening on
		// :8080 for the runner's /exec POSTs.
		s.Logger.Info().Str("main", mainID).Msg("invokePodServiceMain: skipIfNoStdin + no captured stdin (GH actions/runner pattern) — skipping default-invoke; container stays alive for docker-exec")
		return
	} else {
		// Default-invoke path: POST with user's entrypoint+cmd. The
		// bootstrap will exec it as a subprocess. Used for `docker run`
		// semantics on pod-mode containers without a docker-attach.
		argv := append([]string{}, mainContainer.Config.Entrypoint...)
		argv = append(argv, mainContainer.Config.Cmd...)
		envSlice := append([]string{}, mainContainer.Config.Env...)
		s.Logger.Info().Str("main", mainID).Str("url", url).Strs("argv", argv).Msg("invokePodServiceMain: default-invoke (no captured stdin, not OpenStdin) — posting user entrypoint+cmd")
		if resp, err := invokeFunction(ctx, url, argv, mainContainer.Config.WorkingDir, envSlice); err != nil {
			s.Logger.Error().Err(err).Str("main", mainID).Msg("pod service default-invoke failed")
			inv.ExitCode = core.HTTPInvokeErrorExitCode(err)
			inv.Error = err.Error()
		} else {
			body, _ := readResponseBody(resp.Body)
			_ = resp.Body.Close()
			stdoutResp = body
			inv.ExitCode = core.HTTPStatusToExitCode(resp.StatusCode)
			if inv.ExitCode != 0 {
				inv.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
		}
	}

	// Fan-out to every pod member: invocation result + LogBuffers +
	// WaitChs close.
	for _, c := range containers {
		if len(stdoutResp) > 0 {
			s.Store.LogBuffers.Store(c.ID, stdoutResp)
		}
		s.Store.PutInvocationResult(c.ID, inv)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
			close(ch.(chan struct{}))
		}
	}

	// Fan-out stdout+stderr to the attached gitlab-runner's
	// attachStream (if any) — the hijacked attach connection's Read
	// blocks until publishAttachResponse fires.
	if v, ok := s.attachStreams.LoadAndDelete(mainID); ok {
		v.(*attachStream).publishAttachResponse(stdoutResp, stderrResp)
	}
}

// invokeRunningRunnerStage handles the per-stage attach+start pattern
// gitlab-runner v17 docker executor uses. After the FIRST stage's
// invokePodServiceMain goroutine completes, the build container's
// Cloud Run Service is still alive (bootstrap HTTP server holding the
// port). gitlab-runner then does `stop`, `attach`, `start` for each
// subsequent stage. Cloud Run Service revisions are immutable so the
// stop+start is a no-op at the cloud layer; this function processes
// the new stdinPipe registered by the new attach and POSTs the
// captured stage script bytes to the same Service URL.
//
// Mirror of invokePodServiceMain's per-stage section, refactored so
// ContainerStart's already-running branch can call it on each stage.
func (s *Server) invokeRunningRunnerStage(mainID string, mainContainer api.Container) {
	ctx := s.ctx()
	s.Logger.Info().Str("main", mainID).Msg("invokeRunningRunnerStage: goroutine entered")

	// Block on stdinPipe.Done() (gitlab-runner half-closes after piping
	// the script). 30s upper bound mirrors invokePodServiceMain.
	v, ok := s.stdinPipes.LoadAndDelete(mainID)
	if !ok {
		s.Logger.Warn().Str("main", mainID).Msg("invokeRunningRunnerStage: no stdinPipe registered (race)")
		return
	}
	pipe := v.(*stdinPipe)
	select {
	case <-pipe.Done():
	case <-time.After(30 * time.Second):
		s.Logger.Warn().Str("main", mainID).Msg("invokeRunningRunnerStage: stdin pipe Done timeout")
	case <-ctx.Done():
		return
	}
	capturedStdin := pipe.Bytes()
	s.Logger.Info().Str("main", mainID).Int("stdin_bytes", len(capturedStdin)).Msg("invokeRunningRunnerStage: stdin pipe drained")

	// Resolve Service URL via the same path invokePodServiceMain uses.
	// In stage 2+, the underlying Service was created by the first
	// stage's materializePodService. resolvePodServiceFromCloud finds
	// it by allocation label or annotation match.
	state, ok := s.resolveGCFFromCloud(ctx, mainID)
	if !ok || state.FunctionURL == "" {
		s.Logger.Error().Str("main", mainID).Msg("invokeRunningRunnerStage: no service URL")
		if v, ok := s.attachStreams.LoadAndDelete(mainID); ok {
			v.(*attachStream).publishAttachResponse(nil, []byte("no service URL"))
		}
		return
	}
	url := state.FunctionURL

	client, err := idtoken.NewClient(ctx, url)
	if err != nil {
		s.Logger.Error().Err(err).Str("main", mainID).Msg("invokeRunningRunnerStage: idtoken client")
		if v, ok := s.attachStreams.LoadAndDelete(mainID); ok {
			v.(*attachStream).publishAttachResponse(nil, []byte(err.Error()))
		}
		return
	}
	client.Timeout = 10 * time.Minute

	envelope := gcpcommon.ExecEnvelopeExec{
		Argv: []string{"/bin/sh"},
	}
	if len(capturedStdin) > 0 {
		envelope.Stdin = gcpcommon.EncodeStdin(capturedStdin)
	}
	res, err := gcpcommon.PostExecEnvelope(ctx, client, url, "", envelope)
	if err != nil {
		s.Logger.Error().Err(err).Str("main", mainID).Msg("invokeRunningRunnerStage: envelope POST failed")
		if v, ok := s.attachStreams.LoadAndDelete(mainID); ok {
			v.(*attachStream).publishAttachResponse(nil, []byte(err.Error()))
		}
		return
	}
	s.Logger.Info().Str("main", mainID).Int("exit", res.ExitCode).Int("stdout_bytes", len(res.Stdout)).Int("stderr_bytes", len(res.Stderr)).Msg("invokeRunningRunnerStage: bootstrap response")

	if v, ok := s.attachStreams.LoadAndDelete(mainID); ok {
		v.(*attachStream).publishAttachResponse(res.Stdout, res.Stderr)
	}

	// Fan-out exit code via WaitChs. gitlab-runner does
	// /containers/{id}/wait?condition=not-running between stages — we
	// must close the WaitCh so it returns.
	inv := core.InvocationResult{
		ExitCode: res.ExitCode,
	}
	if res.ExitCode != 0 {
		inv.Error = fmt.Sprintf("subprocess exit %d", res.ExitCode)
	}
	s.Store.PutInvocationResult(mainID, inv)
	if ch, ok := s.Store.WaitChs.LoadAndDelete(mainID); ok {
		close(ch.(chan struct{}))
	}
	// Re-register a fresh WaitCh for the next stage's wait/stop cycle.
	s.Store.WaitChs.Store(mainID, make(chan struct{}))
}

// readResponseBody is a small helper so this file doesn't pull io.ReadAll
// from another package as a single-use import.
// memoryLimitForContainer doubles the per-container memory for the main
// (port-bound) container in a multi-container revision so workloads that
// download a Go toolchain + clone a repo + build still fit alongside a
// postgres sidecar. Cloud Run gen2's revision-level memory cap is the
// SUM of every container's limit; the previous symmetric default
// (1Gi/container, 2Gi total) OOM'd at "Memory limit of 2048 MiB
// exceeded with 2112 MiB used" during a build that pulled go1.24.0
// into the same revision running postgres. Sidecars stay at the
// original limit since postgres / similar service containers idle at
// ~200-300Mi.
func memoryLimitForContainer(base string, isMain bool) string {
	if !isMain {
		return base
	}
	switch base {
	case "1Gi":
		return "2Gi"
	case "2Gi":
		return "4Gi"
	default:
		return base
	}
}

func readResponseBody(r interface {
	Read(p []byte) (int, error)
}) ([]byte, error) {
	var buf bytes.Buffer
	chunk := make([]byte, 4096)
	for {
		n, err := r.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
		}
		if err != nil {
			break
		}
	}
	return buf.Bytes(), nil
}
