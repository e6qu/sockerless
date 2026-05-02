package gcf

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
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
	registry := p.server.config.Region + "-docker.pkg.dev"
	token, err := p.server.images.Auth.GetToken(registry)
	if err != nil {
		return nil, err
	}
	return core.OCIListImages(ctx, core.OCIListOptions{
		Registry:  registry,
		AuthToken: token,
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
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ch.(chan struct{}):
			if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
				return inv.ExitCode, nil
			}
			return 0, nil
		}
	}

	// Fallback: poll cloud API (post-restart case)
	interval := p.server.config.PollInterval
	if interval == 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
			if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
				return inv.ExitCode, nil
			}
			containers, err := p.queryFunctions(ctx)
			if err != nil {
				continue
			}
			for _, c := range containers {
				if c.ID == containerID && !c.State.Running && c.State.Status == "exited" {
					return c.State.ExitCode, nil
				}
			}
		}
	}
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
			members := podMembersFromFunction(fn)
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

		c := functionToContainer(fn, labels)

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

	return containers, nil
}

// podMembersFromFunction extracts the per-member manifest from the
// Function's SOCKERLESS_POD_CONTAINERS env var. Returns nil if the
// env is missing or undecodable — the caller treats the Function as
// non-pod in that case.
func podMembersFromFunction(fn *functionspb.Function) []PodMemberJSON {
	if fn.ServiceConfig == nil {
		return nil
	}
	enc := fn.ServiceConfig.EnvironmentVariables["SOCKERLESS_POD_CONTAINERS"]
	if enc == "" {
		return nil
	}
	members, err := DecodePodManifest(enc)
	if err != nil {
		return nil
	}
	return members
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
func functionToContainer(fn *functionspb.Function, labels map[string]string) api.Container {
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

	// Docker labels are carried as a base64-encoded JSON env var
	// because GCP's label-value charset rejects the sockerless-labels
	// JSON blob and Functions v2 has no Annotations field. Prefer the
	// env-var source if present; fall back to the legacy
	// split-across-labels path for resources created before the fix.
	dockerLabels := map[string]string{}
	if fn.ServiceConfig != nil {
		if b64, ok := fn.ServiceConfig.EnvironmentVariables["SOCKERLESS_LABELS"]; ok && b64 != "" {
			if raw, err := base64.StdEncoding.DecodeString(b64); err == nil {
				_ = json.Unmarshal(raw, &dockerLabels)
			}
		}
	}
	if len(dockerLabels) == 0 {
		hyphenLabels := gcpLabelsToHyphenMap(labels)
		if parsed := core.ParseLabelsFromTags(hyphenLabels); parsed != nil {
			dockerLabels = parsed
		}
	}

	// Extract environment variables
	var env []string
	if fn.ServiceConfig != nil {
		for k, v := range fn.ServiceConfig.EnvironmentVariables {
			env = append(env, k+"="+v)
		}
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
		HostConfig: api.HostConfig{NetworkMode: networkName},
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
	}
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

// gcpLabelsToHyphenMap converts GCP underscore-based label keys back to hyphen format
// so that ParseLabelsFromTags can find the sockerless-labels key.
func gcpLabelsToHyphenMap(labels map[string]string) map[string]string {
	m := make(map[string]string, len(labels))
	for k, v := range labels {
		hyphenKey := strings.ReplaceAll(k, "_", "-")
		m[hyphenKey] = v
	}
	return m
}
