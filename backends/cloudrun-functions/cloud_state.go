package gcf

import (
	"context"
	"fmt"
	"strings"
	"time"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/iterator"
)

// gcfCloudState implements core.CloudStateProvider for Cloud Run Functions.
// All container state is derived from Cloud Functions tagged with sockerless_managed=true,
// merged with PendingCreates for containers between create and start.
type gcfCloudState struct {
	server *Server
}

// ListImages queries GCP Artifact Registry via the OCI distribution
// catalog + tags endpoints./step 2 cross-cloud
// sibling.
func (p *gcfCloudState) ListImages(ctx context.Context) ([]*api.ImageSummary, error) {
	if p.server.config.Region == "" || p.server.config.Project == "" {
		return nil, nil
	}
	if p.server.images == nil || p.server.images.Auth == nil {
		return nil, nil
	}
	auth := p.server.images.Auth
	registry := p.server.config.Region + "-docker.pkg.dev"
	return core.OCIListImages(ctx, core.OCIListOptions{
		Registry: registry,
		Endpoint: core.RegistryEndpointFor(auth, registry),
		TokenFor: func(repo string) (string, error) {
			return auth.GetToken(registry, repo, core.ActionMetadataRead)
		},
	})
}

func (p *gcfCloudState) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
	containers, err := p.queryFunctions(ctx)
	if err != nil {
		return api.Container{}, false, err
	}

	for _, c := range containers {
		if c.ID == ref {
			return c, true, nil
		}
		if c.Name == ref || c.Name == "/"+ref || strings.TrimPrefix(c.Name, "/") == ref {
			return c, true, nil
		}
		if len(ref) >= 3 && strings.HasPrefix(c.ID, ref) {
			return c, true, nil
		}
	}
	return api.Container{}, false, nil
}

func (p *gcfCloudState) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
	containers, err := p.queryFunctions(ctx)
	if err != nil {
		return nil, err
	}

	var result []api.Container
	for _, c := range containers {
		if !all && !c.State.Running {
			continue
		}
		if !core.MatchContainerFilters(c, filters) {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

func (p *gcfCloudState) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
	containers, err := p.queryFunctions(ctx)
	if err != nil {
		return false, err
	}
	for _, c := range containers {
		if c.Name == name || c.Name == "/"+name {
			return false, nil
		}
	}
	return true, nil
}

func (p *gcfCloudState) WaitForExit(ctx context.Context, containerID string) (int, error) {
	// Fast path — invocation goroutine records the outcome.
	if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
		return inv.ExitCode, nil
	}
	// Check WaitChs — FaaS containers use exit channels
	if ch, ok := p.server.Store.WaitChs.Load(containerID); ok {
		wc, isCh := ch.(chan struct{})
		if !isCh {
			return -1, fmt.Errorf("wait channel for %s held unexpected type %T", containerID, ch)
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-wc:
			if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
				return inv.ExitCode, nil
			}
			// Channel closed without a recorded result (force-stop /
			// restart race). Never fabricate a successful exit — report a
			// failure sentinel rather than a misleading 0.
			return -1, nil
		}
	}

	// Fallback: poll cloud API (post-restart case)
	interval := p.server.config.PollInterval
	if interval == 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Resolve the single backing (non-pod) Function name once; if found,
	// GetFunction just that one per tick and derive its state instead of
	// re-listing every Function + pod-Service each tick. Pod-backed members
	// and resolve failures fall back to the full scan, which keeps its
	// gone-counter so a vanished resource still returns -1.
	fnName, _ := p.resolveFunctionName(ctx, containerID)

	gone := 0
	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
			if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
				return inv.ExitCode, nil
			}
			if fnName != "" {
				st, ok := p.functionStateByName(ctx, fnName, containerID)
				if ok {
					if !st.Running && st.Status == "exited" {
						return st.ExitCode, nil
					}
					continue
				}
				// Function vanished — fall through to the full scan below.
			}
			containers, err := p.queryFunctions(ctx)
			if err != nil {
				continue
			}
			if exit, found, exited := core.ScanContainersForExit(containers, containerID); exited {
				return exit, nil
			} else if !found {
				if gone++; gone >= core.WaitGoneThreshold {
					return -1, nil
				}
			} else {
				gone = 0
			}
		}
	}
}

// resolveFunctionName returns the Cloud Function name backing a single
// (non-pod) container ID, or "" if no matching sockerless-managed
// single-container function is found. Pod-managed functions are skipped
// so WaitForExit falls back to the full scan for pod members.
func (p *gcfCloudState) resolveFunctionName(ctx context.Context, containerID string) (string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", p.server.config.Project, p.server.config.Region)
	it := p.server.gcp.Functions.ListFunctions(ctx, &functionspb.ListFunctionsRequest{Parent: parent})
	for {
		fn, err := it.Next()
		if err == iterator.Done {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		labels := fn.Labels
		if labels["sockerless_managed"] != "true" {
			continue
		}
		// Pod-managed functions back N members — single-resource derivation
		// doesn't apply; let those fall back to the scan.
		if labels["sockerless_pod"] != "" {
			continue
		}
		cid := labels["sockerless_allocation"]
		if cid == "" {
			cid = labels["sockerless_container_id"]
		}
		if cid == "" && fn.ServiceConfig != nil {
			cid = fn.ServiceConfig.EnvironmentVariables["SOCKERLESS_CONTAINER_ID"]
		}
		if cid == containerID {
			return fn.Name, nil
		}
	}
}

// functionStateByName GetFunctions a single Cloud Function by name and
// derives its Docker container state via the same mapFunctionState +
// invocation-result overlay queryFunctions uses. Returns ok=false when
// the function can't be fetched (vanished/error) so the caller can fall
// back to the full scan.
func (p *gcfCloudState) functionStateByName(ctx context.Context, fnName, containerID string) (api.ContainerState, bool) {
	fn, err := p.server.gcp.Functions.GetFunction(ctx, &functionspb.GetFunctionRequest{Name: fnName})
	if err != nil || fn == nil {
		return api.ContainerState{}, false
	}
	if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
		return api.ContainerState{
			Status:     "exited",
			Running:    false,
			ExitCode:   inv.ExitCode,
			FinishedAt: inv.FinishedAt.UTC().Format(time.RFC3339Nano),
			Error:      inv.Error,
		}, true
	}
	return mapFunctionState(fn), true
}

// queryFunctions lists all sockerless-managed Cloud Functions and merges with PendingCreates.
func (p *gcfCloudState) queryFunctions(ctx context.Context) ([]api.Container, error) {
	seen := make(map[string]bool)
	var containers []api.Container

	// PendingCreates (containers between create and start)
	for _, c := range p.server.PendingCreates.List() {
		seen[c.ID] = true
		containers = append(containers, c)
	}

	// Query Cloud Functions API
	parent := fmt.Sprintf("projects/%s/locations/%s", p.server.config.Project, p.server.config.Region)
	it := p.server.gcp.Functions.ListFunctions(ctx, &functionspb.ListFunctionsRequest{
		Parent: parent,
	})

	for {
		fn, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// If the API call fails, return what we have from PendingCreates
			break
		}

		labels := fn.Labels
		if labels["sockerless_managed"] != "true" {
			continue
		}

		// Pod Functions: one Function backs N container rows. The pod
		// manifest is in the SOCKERLESS_POD_CONTAINERS env var; each
		// member becomes a `docker ps` row keyed on its original
		// container ID (round-tripped through the manifest's
		// container_id field). Skip the per-member emit and fall
		// through to single-container handling when the function is
		// not pod-managed.
		if labels["sockerless_pod"] != "" {
			members, err := podMembersFromFunction(fn)
			if err != nil {
				p.server.Logger.Warn().Err(err).Str("function", fn.Name).
					Msg("podMembersFromFunction: skipping pod function with undecodable manifest")
				continue
			}
			for _, m := range members {
				if m.ContainerID == "" || seen[m.ContainerID] {
					continue
				}
				seen[m.ContainerID] = true
				c := podMemberToContainer(fn, labels, m)
				if inv, ok := p.server.Store.GetInvocationResult(c.ID); ok {
					c.State = api.ContainerState{
						Status:     "exited",
						Running:    false,
						ExitCode:   inv.ExitCode,
						FinishedAt: inv.FinishedAt.UTC().Format(time.RFC3339Nano),
						Error:      inv.Error,
					}
				}
				containers = append(containers, c)
			}
			continue
		}

		// Free pool entries are not containers — they have no
		// `sockerless_allocation` label set. Skip them in container listings.
		// Pre-pool builds used `sockerless_container_id` directly; honour both
		// during the migration window.
		containerID := labels["sockerless_allocation"]
		if containerID == "" {
			containerID = labels["sockerless_container_id"]
		}
		if containerID == "" || seen[containerID] {
			continue
		}
		seen[containerID] = true

		c, err := functionToContainer(fn, labels)
		if err != nil {
			p.server.Logger.Warn().Err(err).Str("function", fn.Name).
				Msg("functionToContainer: skipping inconsistent function")
			continue
		}

		// Overlay recorded invocation outcome so exited state is
		// visible to docker ps / docker inspect / docker wait.
		if inv, ok := p.server.Store.GetInvocationResult(c.ID); ok {
			c.State = api.ContainerState{
				Status:     "exited",
				Running:    false,
				ExitCode:   inv.ExitCode,
				FinishedAt: inv.FinishedAt.UTC().Format(time.RFC3339Nano),
				Error:      inv.Error,
			}
		}

		// Stateless: function name/URL are read directly from `fn` whenever
		// needed (this is itself the cloud-side query). No local cache.

		containers = append(containers, c)
	}

	// Pod-mode resources are now Cloud Run Services (not Functions) for
	// deploy-speed reasons — see pod_service.go. Query Services tagged
	// with sockerless_managed=true + sockerless_pod=* and emit one
	// container row per pod member (same shape as the pod-Function path
	// above).
	podContainers, podErr := p.queryPodServiceContainers(ctx, seen)
	if podErr != nil {
		// Don't fail the whole listing — Functions-side results stand;
		// Service-side errors get logged once.
		p.server.Logger.Debug().Err(podErr).Msg("queryFunctions: pod-Service listing partial failure")
	}
	containers = append(containers, podContainers...)

	return containers, nil
}

// queryPodServiceContainers lists sockerless-managed pod-mode Cloud
// Run Services (sockerless_pod label set) and reconstructs an
// api.Container for each pod member listed in the
// `sockerless_pod_members` annotation. seen is updated in place to
// avoid double-counting members already covered by the Function path.
func (p *gcfCloudState) queryPodServiceContainers(ctx context.Context, seen map[string]bool) ([]api.Container, error) {
	if p.server.gcp == nil || p.server.gcp.Services == nil {
		return nil, nil
	}
	parent := fmt.Sprintf("projects/%s/locations/%s", p.server.config.Project, p.server.config.Region)
	it := p.server.gcp.Services.ListServices(ctx, &runpb.ListServicesRequest{Parent: parent})
	var out []api.Container
	for {
		svc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, err
		}
		if svc.Labels["sockerless_managed"] != "true" {
			continue
		}
		// Pod-mode resources are tagged with sockerless_pod_members
		// annotation. ListServices may not always populate Annotations
		// (some gRPC list responses abbreviate fields) — when our
		// match-by-name returns a Service with empty Annotations and
		// the labels look pod-shaped (no sockerless_overlay_hash, has
		// sockerless_allocation), do a GetService follow-up to
		// retrieve the full proto.
		members := strings.Split(svc.Annotations["sockerless_pod_members"], ",")
		if len(members) == 1 && members[0] == "" && strings.HasPrefix(svc.Name, p.server.buildPodServiceParent()) && strings.Contains(svc.Name, "/services/sockerless-svc-") {
			if full, ferr := p.server.gcp.Services.GetService(ctx, &runpb.GetServiceRequest{Name: svc.Name}); ferr == nil && full != nil {
				svc = full
				members = strings.Split(svc.Annotations["sockerless_pod_members"], ",")
			}
		}
		if svc.Labels["sockerless_pod"] == "" && svc.Annotations["sockerless_pod_members"] == "" {
			continue
		}
		p.server.Logger.Debug().Str("service", svc.Name).Int("member_count", len(members)).Msg("queryPodServiceContainers: matched pod service")
		for _, mid := range members {
			mid = strings.TrimSpace(mid)
			if mid == "" || seen[mid] {
				continue
			}
			seen[mid] = true
			c, err := serviceToPodMemberContainer(svc, mid)
			if err != nil {
				p.server.Logger.Warn().Err(err).Str("service", svc.Name).Str("member", mid).
					Msg("serviceToPodMemberContainer: skipping inconsistent pod member")
				continue
			}
			if inv, ok := p.server.Store.GetInvocationResult(mid); ok {
				if _, hasAgent := p.server.reverseAgents.Resolve(mid); hasAgent && isReverseAgentInvokeTransportError(inv.Error) {
					c.State = api.ContainerState{Status: "running", Running: true}
					out = append(out, c)
					continue
				}
				c.State = api.ContainerState{
					Status:     "exited",
					Running:    false,
					ExitCode:   inv.ExitCode,
					FinishedAt: inv.FinishedAt.UTC().Format(time.RFC3339Nano),
					Error:      inv.Error,
				}
			} else if _, hasAgent := p.server.reverseAgents.Resolve(mid); hasAgent {
				c.State = api.ContainerState{Status: "running", Running: true}
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// serviceToPodMemberContainer constructs a `docker ps` row for one
// pod member from its multi-container Cloud Run Service. Per-member
// fields (image, entrypoint, cmd) are read off the corresponding
// runpb.Container in the revision template; identity fields come
// from the Service labels + annotations.
func serviceToPodMemberContainer(svc *runpb.Service, mid string) (api.Container, error) {
	created := ""
	if svc.CreateTime != nil {
		created = svc.CreateTime.AsTime().Format(time.RFC3339Nano)
	}
	state := api.ContainerState{Status: "running", Running: true}
	if svc.TerminalCondition != nil && svc.TerminalCondition.State != runpb.Condition_CONDITION_SUCCEEDED {
		state = api.ContainerState{Status: "created", Running: false}
	}
	name := "/" + mid
	if len(mid) > 12 {
		name = "/" + mid[:12]
	}
	image := ""
	var cmd, entrypoint, env []string
	var memBytes, nanoCPUs int64
	var mounts []api.MountPoint
	if svc.Template != nil && len(svc.Template.Containers) > 0 {
		main := svc.Template.Containers[0]
		image = main.Image
		entrypoint = main.Command
		cmd = main.Args
		for _, e := range main.Env {
			if v, ok := e.Values.(*runpb.EnvVar_Value); ok {
				env = append(env, e.Name+"="+v.Value)
			}
		}
		mounts = gcfMounts(main.VolumeMounts)
		if main.Resources != nil {
			memBytes = core.DockerMemoryBytes(main.Resources.Limits["memory"])
			nanoCPUs = core.DockerNanoCPUs(main.Resources.Limits["cpu"])
		}
	}
	// Docker labels round-trip via the single authoritative SOCKERLESS_LABELS
	// env var (core.LabelsEnvVar) on the main container.
	dockerLabels, err := core.LabelsFromEnvSlice(env)
	if err != nil {
		return api.Container{}, fmt.Errorf("pod service %q: %w", svc.Name, err)
	}
	return api.Container{
		ID:      mid,
		Name:    name,
		Created: created,
		Image:   image,
		State:   state,
		Config: api.ContainerConfig{
			Image:      image,
			Cmd:        cmd,
			Entrypoint: entrypoint,
			Env:        env,
			Labels:     dockerLabels,
		},
		HostConfig: api.HostConfig{NetworkMode: "default", Memory: memBytes, NanoCPUs: nanoCPUs},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"bridge": {NetworkID: "bridge"},
			},
		},
		Mounts:   mounts,
		Platform: "linux",
		Driver:   "cloud-run-service",
	}, nil
}

// podMembersFromFunction extracts the per-member manifest from the
// Function's SOCKERLESS_POD_CONTAINERS env var. Returns (nil, nil) when
// the env is absent (the Function is not a pod). A present-but-undecodable
// manifest means the writing backend produced garbage — return the decode
// error so the caller surfaces the inconsistency rather than silently
// dropping every pod member from `docker ps`.
func podMembersFromFunction(fn *functionspb.Function) ([]PodMemberJSON, error) {
	if fn.ServiceConfig == nil {
		return nil, nil
	}
	enc := fn.ServiceConfig.EnvironmentVariables["SOCKERLESS_POD_CONTAINERS"]
	if enc == "" {
		return nil, nil
	}
	members, err := DecodePodManifest(enc)
	if err != nil {
		return nil, fmt.Errorf("malformed SOCKERLESS_POD_CONTAINERS on function %q: %w", fn.Name, err)
	}
	return members, nil
}

// podMemberToContainer builds a `docker ps` row for one pod member.
// Stateless: every field is derived from the Function's labels + envs
// + per-member manifest entry. HostConfig.MountNamespaceMode and PidMode
// surface the spec's "shared-degraded" honesty so operators detecting
// the field can choose a non-FaaS backend (cloudrun-jobs / aca) when
// they need real per-container isolation.
func podMemberToContainer(fn *functionspb.Function, labels map[string]string, m PodMemberJSON) api.Container {
	name := "/" + m.Name
	if m.ContainerID != "" && m.Name == "" {
		name = "/" + m.ContainerID[:12]
	}
	state := mapFunctionState(fn)
	created := labels["sockerless_created_at"]
	netName := "bridge"
	return api.Container{
		ID:      m.ContainerID,
		Name:    name,
		Created: created,
		Image:   m.Image,
		State:   state,
		Config: api.ContainerConfig{
			Image:      m.Image,
			Entrypoint: m.Entrypoint,
			Cmd:        m.Cmd,
			Env:        m.Env,
			WorkingDir: m.Workdir,
			// Per spec § "Podman pods on FaaS backends — Honest mapping",
			// pod members on FaaS share mount-ns (chroot only — no real
			// mount-ns) and PID-ns because the cloud sandbox blocks
			// `unshare(CLONE_NEWNS|CLONE_NEWPID)`. Surfacing this via
			// `docker inspect` is the operator's signal to fall through
			// to a real-isolation backend (cloudrun-jobs / aca) when
			// isolation is load-bearing. Labels carry this since
			// api.HostConfig has only PidMode (no MountNamespaceMode);
			// PidMode below carries the same signal in docker's native
			// schema.
			Labels: map[string]string{
				"sockerless.pod":               labels["sockerless_pod"],
				"sockerless.pod.member":        m.Name,
				"sockerless.namespace.mount":   "shared-degraded",
				"sockerless.namespace.pid":     "shared-degraded",
				"sockerless.namespace.user":    "shared-degraded",
				"sockerless.namespace.cgroup":  "shared-degraded",
				"sockerless.namespace.network": "shared",
				"sockerless.namespace.ipc":     "shared",
				"sockerless.namespace.uts":     "shared",
			},
		},
		HostConfig: api.HostConfig{
			NetworkMode: netName,
			PidMode:     "shared-degraded",
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				netName: {NetworkID: netName},
			},
		},
		Platform: "linux",
		Driver:   "cloud-run-functions",
	}
}

// functionToContainer reconstructs an api.Container from a Cloud Function and its labels.
// Returns an error if SOCKERLESS_LABELS (the authoritative docker-labels
// env var) is present but malformed — that means the writing backend
// produced garbage and the function is in an inconsistent state.
func functionToContainer(fn *functionspb.Function, labels map[string]string) (api.Container, error) {
	// Full container ID from env vars (labels truncate at 63 chars, IDs are 64)
	containerID := ""
	if fn.ServiceConfig != nil {
		containerID = fn.ServiceConfig.EnvironmentVariables["SOCKERLESS_CONTAINER_ID"]
	}
	if containerID == "" {
		containerID = labels["sockerless_container_id"]
	}
	name := labels["sockerless_name"]
	if name == "" && containerID != "" {
		name = "/" + containerID[:12]
	}
	if name != "" && !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	// Derive image from service config container image
	image := ""
	if fn.ServiceConfig != nil && fn.ServiceConfig.Uri != "" {
		image = fn.ServiceConfig.Uri
	}

	// Map function state to Docker state
	state := mapFunctionState(fn)

	// Docker labels round-trip via the single authoritative SOCKERLESS_LABELS
	// env var (core.LabelsEnvVar). A present-but-malformed value means the
	// writing backend produced garbage — surface it rather than reconstruct
	// an empty label set.
	dockerLabels := map[string]string{}
	if fn.ServiceConfig != nil {
		decoded, derr := core.DecodeLabelsEnvValue(fn.ServiceConfig.EnvironmentVariables[core.LabelsEnvVar])
		if derr != nil {
			return api.Container{}, fmt.Errorf("function %q: %w", fn.Name, derr)
		}
		if decoded != nil {
			dockerLabels = decoded
		}
	}

	// Extract environment variables
	var env []string
	var memBytes, nanoCPUs int64
	if fn.ServiceConfig != nil {
		for k, v := range fn.ServiceConfig.EnvironmentVariables {
			env = append(env, k+"="+v)
		}
		memBytes = core.DockerMemoryBytes(fn.ServiceConfig.AvailableMemory)
		nanoCPUs = core.DockerNanoCPUs(fn.ServiceConfig.AvailableCpu)
	}

	created := labels["sockerless_created_at"]

	networkName := "bridge"

	return api.Container{
		ID:      containerID,
		Name:    name,
		Created: created,
		Image:   image,
		State:   state,
		Config: api.ContainerConfig{
			Image:  image,
			Env:    env,
			Labels: dockerLabels,
		},
		HostConfig: api.HostConfig{NetworkMode: networkName, Memory: memBytes, NanoCPUs: nanoCPUs},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				networkName: {
					NetworkID: networkName,
					IPAddress: "",
				},
			},
		},
		Platform: "linux",
		Driver:   "cloud-run-functions",
	}, nil
}

// mapFunctionState converts Cloud Function state to Docker container state.
func mapFunctionState(fn *functionspb.Function) api.ContainerState {
	fnState := fn.State

	switch fnState {
	case functionspb.Function_DEPLOYING:
		return api.ContainerState{
			Status: "created",
		}
	case functionspb.Function_ACTIVE:
		return api.ContainerState{
			Status:  "running",
			Running: true,
		}
	case functionspb.Function_DELETING:
		return api.ContainerState{
			Status: "removing",
		}
	case functionspb.Function_FAILED:
		errMsg := ""
		if fn.StateMessages != nil {
			for _, msg := range fn.StateMessages {
				if msg.Message != "" {
					errMsg = msg.Message
					break
				}
			}
		}
		return api.ContainerState{
			Status: "exited",
			Error:  errMsg,
		}
	default:
		// UNKNOWN or unrecognized — treat as running if the function exists
		return api.ContainerState{
			Status:  "running",
			Running: true,
		}
	}
}

// gcfMounts reconstructs the docker-inspect Mounts list from a Cloud Run
// container's VolumeMounts — each preserves the original Docker bind name and
// destination set at create time.
func gcfMounts(vms []*runpb.VolumeMount) []api.MountPoint {
	out := make([]api.MountPoint, 0, len(vms))
	for _, vm := range vms {
		if vm == nil {
			continue
		}
		out = append(out, api.MountPoint{
			Type:        "volume",
			Name:        vm.Name,
			Source:      vm.Name,
			Destination: vm.MountPath,
			RW:          true,
			Mode:        "rw",
		})
	}
	return out
}
