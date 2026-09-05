package azf

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	azurecommon "github.com/sockerless/azure-common"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// azfCloudState implements core.CloudStateProvider for Azure Functions.
// All container state is derived from Azure Function Apps tagged with sockerless-managed=true,
// merged with PendingCreates for containers between create and start.
type azfCloudState struct {
	server *Server
}

// ListImages queries Azure Container Registry via the OCI distribution
// catalog + tags endpoints./step 2 cross-cloud
// sibling. `config.Registry` is the ACR hostname
// (e.g. `myregistry.azurecr.io`).
func (p *azfCloudState) ListImages(ctx context.Context) ([]*api.ImageSummary, error) {
	if p.server.config.Registry == "" {
		return nil, nil
	}
	if p.server.images == nil || p.server.images.Auth == nil {
		return nil, nil
	}
	auth := p.server.images.Auth
	registry := p.server.config.Registry
	return core.OCIListImages(ctx, core.OCIListOptions{
		Registry: registry,
		Endpoint: core.RegistryEndpointFor(auth, registry),
		TokenFor: func(repo string) (string, error) {
			return auth.GetToken(registry, repo, core.ActionMetadataRead)
		},
	})
}

func (p *azfCloudState) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
	containers, err := p.queryFunctionApps(ctx)
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

func (p *azfCloudState) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
	containers, err := p.queryFunctionApps(ctx)
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

func (p *azfCloudState) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
	containers, err := p.queryFunctionApps(ctx)
	if err != nil {
		return false, err
	}
	return !core.ContainerNameTaken(containers, name), nil
}

func (p *azfCloudState) WaitForExit(ctx context.Context, containerID string) (int, error) {
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

	// Resolve the single backing (non-pod) Function App name once; if found,
	// Get just that one per tick instead of re-listing every Function App
	// each tick. A pod-site member / resolve failure falls back to the full
	// scan, which keeps its gone-counter so a vanished resource still
	// returns -1. The single-resource path likewise detects a deleted site
	// (Get NotFound persistently) and returns -1 after the threshold.
	appName, _ := p.resolveFunctionAppName(ctx, containerID)

	gone := 0
	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
			if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
				return inv.ExitCode, nil
			}
			if appName != "" {
				_, err := p.server.azure.WebApps.Get(ctx, p.server.config.ResourceGroup, appName, nil)
				if err == nil {
					// Site still exists and no invocation result yet — the
					// container is still available for invocation. A deleted
					// site is the only exit signal for a one-shot FaaS
					// container without a recorded result.
					gone = 0
					continue
				}
				// Site vanished (deleted) — count toward the gone threshold
				// directly so we return -1 without a per-tick full scan.
				if gone++; gone >= core.WaitGoneThreshold {
					return -1, nil
				}
				continue
			}
			containers, err := p.queryFunctionApps(ctx)
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

// resolveFunctionAppName returns the Azure Function App name backing a
// single (non-pod) container ID, or "" if no matching sockerless-managed
// single-container site is found. Pod sites (carrying a members manifest)
// are skipped so WaitForExit falls back to the full scan for pod members.
func (p *azfCloudState) resolveFunctionAppName(ctx context.Context, containerID string) (string, error) {
	pager := p.server.azure.WebApps.NewListByResourceGroupPager(p.server.config.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}
		for _, site := range page.Value {
			if site.Tags == nil {
				continue
			}
			if derefTag(site.Tags["sockerless-managed"]) != "true" {
				continue
			}
			// Pod sites back N members — single-resource derivation doesn't
			// apply; let those fall back to the scan.
			if derefTag(site.Tags[podMembersTagKey]) != "" {
				continue
			}
			if derefTag(site.Tags["sockerless-container-id"]) == containerID {
				if site.Name != nil {
					return *site.Name, nil
				}
			}
		}
	}
	return "", nil
}

// queryFunctionApps lists all sockerless-managed Azure Function Apps and merges with PendingCreates.
func (p *azfCloudState) queryFunctionApps(ctx context.Context) ([]api.Container, error) {
	seen := make(map[string]bool)
	var containers []api.Container

	// PendingCreates (containers between create and start)
	for _, c := range p.server.PendingCreates.List() {
		seen[c.ID] = true
		containers = append(containers, c)
	}

	// Query Azure Function Apps via ARM API
	pager := p.server.azure.WebApps.NewListByResourceGroupPager(p.server.config.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			// Never treat a partial list as authoritative — the cloud is the
			// source of truth, so surface the error to the caller rather than
			// silently dropping the un-paged Function Apps.
			return containers, err
		}

		for _, site := range page.Value {
			if site.Tags == nil {
				continue
			}

			// Only include sockerless-managed Function Apps
			managedVal, hasManagedTag := site.Tags["sockerless-managed"]
			if !hasManagedTag || managedVal == nil || *managedVal != "true" {
				continue
			}

			// A multi-container pod site carries a members manifest: expand it
			// into one running container per member (cloud is the source of
			// truth for the whole pod, not just the main).
			if membersTag := derefTag(site.Tags[podMembersTagKey]); membersTag != "" {
				containers = append(containers, p.expandPodSite(site, membersTag, seen)...)
				continue
			}

			containerID := derefTag(site.Tags["sockerless-container-id"])
			if containerID == "" || seen[containerID] {
				continue
			}
			seen[containerID] = true

			c, err := siteToContainer(site.Tags, site.Properties, site.Name)
			if err != nil {
				p.server.Logger.Warn().Err(err).Str("site", derefTag(site.Name)).
					Msg("siteToContainer: skipping inconsistent function app")
				continue
			}

			// Overlay recorded invocation outcome. A gitlab-runner-pattern
			// (OpenStdin) container is re-invoked per stage and must stay
			// running across stages so the runner's docker exec resolves —
			// docker wait still returns each stage's exit code via WaitForExit
			// (InvocationResult / WaitChs), independent of this running flag.
			// It is reported exited only once the site is deleted. A one-shot
			// FaaS container (no OpenStdin) overlays exited as before.
			if inv, ok := p.server.Store.GetInvocationResult(c.ID); ok && !c.Config.OpenStdin {
				c.State = api.ContainerState{
					Status:     "exited",
					Running:    false,
					ExitCode:   inv.ExitCode,
					FinishedAt: inv.FinishedAt.UTC().Format(time.RFC3339Nano),
					Error:      inv.Error,
				}
			}

			// Sync AZF state store with cloud state
			if _, exists := p.server.AZF.Get(containerID); !exists {
				funcAppName := ""
				if site.Name != nil {
					funcAppName = *site.Name
				}
				resourceID := ""
				if site.ID != nil {
					resourceID = *site.ID
				}
				functionURL, functionHost := "", ""
				if site.Properties != nil && site.Properties.DefaultHostName != nil {
					functionHost = *site.Properties.DefaultHostName
					functionURL = invokeURLForHost(p.server.config.EndpointURL, functionHost)
				}
				p.server.AZF.Put(containerID, AZFState{
					FunctionAppName: funcAppName,
					ResourceID:      resourceID,
					FunctionURL:     functionURL,
					FunctionHost:    functionHost,
				})
			}

			containers = append(containers, c)
		}
	}

	return containers, nil
}

// expandPodSite reconstructs one api.Container per member of a multi-
// container pod site from its members manifest. The main carries any
// recorded invocation outcome (exited); sidecars are reported running while
// the site exists. AZFState for every member is synced to the site so exec /
// inspect / wait route to it.
func (p *azfCloudState) expandPodSite(site *armappservice.Site, membersTag string, seen map[string]bool) []api.Container {
	members := decodePodMembers(membersTag)
	if len(members) == 0 {
		return nil
	}

	funcAppName := ""
	if site.Name != nil {
		funcAppName = *site.Name
	}
	resourceID := ""
	if site.ID != nil {
		resourceID = *site.ID
	}
	functionURL, functionHost := "", ""
	if site.Properties != nil && site.Properties.DefaultHostName != nil {
		functionHost = *site.Properties.DefaultHostName
		functionURL = invokeURLForHost(p.server.config.EndpointURL, functionHost)
	}
	state := AZFState{
		FunctionAppName: funcAppName,
		ResourceID:      resourceID,
		FunctionURL:     functionURL,
		FunctionHost:    functionHost,
	}

	created := derefTag(site.Tags["sockerless-created-at"])
	dockerLabels := core.ParseLabelsFromTags(azurecommon.TagsToMap(site.Tags))
	if dockerLabels == nil {
		dockerLabels = make(map[string]string)
	}

	var out []api.Container
	for _, m := range members {
		if m.ID == "" || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		if _, exists := p.server.AZF.Get(m.ID); !exists {
			p.server.AZF.Put(m.ID, state)
		}

		name := m.Name
		if name == "" {
			name = m.ID[:12]
		}
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}
		c := api.Container{
			ID:      m.ID,
			Name:    name,
			Created: created,
			Image:   m.Image,
			State:   api.ContainerState{Status: "running", Running: true},
			Config: api.ContainerConfig{
				Image:  m.Image,
				Labels: dockerLabels,
			},
			HostConfig: api.HostConfig{NetworkMode: "bridge"},
			NetworkSettings: api.NetworkSettings{
				Networks: map[string]*api.EndpointSettings{"bridge": {NetworkID: "bridge"}},
			},
			Platform: "linux",
			Driver:   "azure-functions",
		}
		// The main carries the recorded invocation outcome.
		if m.IsMain {
			if inv, ok := p.server.Store.GetInvocationResult(m.ID); ok {
				c.State = api.ContainerState{
					Status:     "exited",
					Running:    false,
					ExitCode:   inv.ExitCode,
					FinishedAt: inv.FinishedAt.UTC().Format(time.RFC3339Nano),
					Error:      inv.Error,
				}
			}
		}
		out = append(out, c)
	}
	return out
}

// siteToContainer reconstructs an api.Container from Azure Function App tags and properties.
func siteToContainer(tags map[string]*string, props interface{}, siteName *string) (api.Container, error) {
	containerID := derefTag(tags["sockerless-container-id"])
	name := derefTag(tags["sockerless-name"])
	if name == "" && containerID != "" {
		name = "/" + containerID[:12]
	}
	if name != "" && !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	// Derive image from the Function App's site config. ARM stores
	// container images as LinuxFxVersion="DOCKER|<image>" on the
	// site's SiteConfig. Tag fallback covers older sites that don't
	// have site config populated (recovered from registry metadata
	// alone).
	image := imageFromSiteProps(props)
	if image == "" {
		image = derefTag(tags["sockerless-image"])
	}
	cmd, entrypoint, env, err := azfSpecFromProps(props)
	if err != nil {
		return api.Container{}, err
	}

	// Function Apps that exist in Azure are considered "running" (available for invocation)
	state := api.ContainerState{
		Status:  "running",
		Running: true,
	}

	// Parse Docker labels from tags (Azure tags use hyphens, matching ParseLabelsFromTags directly)
	hyphenTags := azurecommon.TagsToMap(tags)
	dockerLabels := core.ParseLabelsFromTags(hyphenTags)
	if dockerLabels == nil {
		dockerLabels = make(map[string]string)
	}

	created := derefTag(tags["sockerless-created-at"])

	networkName := "bridge"

	return api.Container{
		ID:      containerID,
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
			OpenStdin:  derefTag(tags["sockerless-open-stdin"]) == "true",
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
		Driver:   "azure-functions",
	}, nil
}

// derefTag safely dereferences an Azure tag pointer.
func derefTag(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// azfSpecFromProps recovers the container's docker Cmd / Entrypoint / Env from
// the Function App's site config app-settings. Cmd/Entrypoint are stored exactly
// (base64-JSON in SOCKERLESS_CMD / SOCKERLESS_ENTRYPOINT by buildAZFAppSettings);
// the user Env is every app-setting that isn't Azure/sockerless plumbing.
func azfSpecFromProps(props interface{}) (cmd, entrypoint, env []string, err error) {
	sp, ok := props.(*armappservice.SiteProperties)
	if !ok || sp == nil || sp.SiteConfig == nil {
		return nil, nil, nil, nil
	}
	plumbing := func(name string) bool {
		for _, p := range []string{"FUNCTIONS_", "WEBSITE", "AzureWebJobs", "DOCKER_REGISTRY_", "SOCKERLESS_"} {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
		return false
	}
	for _, s := range sp.SiteConfig.AppSettings {
		if s == nil || s.Name == nil {
			continue
		}
		name := *s.Name
		val := ""
		if s.Value != nil {
			val = *s.Value
		}
		switch name {
		case "SOCKERLESS_ENTRYPOINT":
			if entrypoint, err = decodeAZFStringSlice(val); err != nil {
				return nil, nil, nil, fmt.Errorf("malformed SOCKERLESS_ENTRYPOINT app-setting: %w", err)
			}
		case "SOCKERLESS_CMD":
			if cmd, err = decodeAZFStringSlice(val); err != nil {
				return nil, nil, nil, fmt.Errorf("malformed SOCKERLESS_CMD app-setting: %w", err)
			}
		default:
			if !plumbing(name) {
				env = append(env, name+"="+val)
			}
		}
	}
	return cmd, entrypoint, env, nil
}

// decodeAZFStringSlice decodes a base64-JSON string slice that
// buildAZFAppSettings wrote (SOCKERLESS_CMD / SOCKERLESS_ENTRYPOINT).
// A present-but-undecodable value means the writing backend produced
// garbage — return the error so the caller surfaces the inconsistent
// resource rather than reconstructing a container with an empty command.
func decodeAZFStringSlice(val string) ([]string, error) {
	raw, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	return out, nil
}

// imageFromSiteProps extracts the container image from a Function App's site
// config. ARM stores it as `LinuxFxVersion="DOCKER|<image>"` on
// `Properties.SiteConfig`. Returns empty when the site has no SiteConfig, no
// LinuxFxVersion, or a non-DOCKER prefix.
func imageFromSiteProps(props interface{}) string {
	sp, ok := props.(*armappservice.SiteProperties)
	if !ok || sp == nil || sp.SiteConfig == nil || sp.SiteConfig.LinuxFxVersion == nil {
		return ""
	}
	v := *sp.SiteConfig.LinuxFxVersion
	const prefix = "DOCKER|"
	if !strings.HasPrefix(v, prefix) {
		return ""
	}
	return strings.TrimPrefix(v, prefix)
}
