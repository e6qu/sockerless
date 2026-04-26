package azf

import (
	"context"
	"strings"
	"time"

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
	registry := p.server.config.Registry
	token, err := p.server.images.Auth.GetToken(registry)
	if err != nil {
		return nil, err
	}
	return core.OCIListImages(ctx, core.OCIListOptions{
		Registry:  registry,
		AuthToken: token,
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
	for _, c := range containers {
		if c.Name == name || c.Name == "/"+name {
			return false, nil
		}
	}
	return true, nil
}

func (p *azfCloudState) WaitForExit(ctx context.Context, containerID string) (int, error) {
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
			containers, err := p.queryFunctionApps(ctx)
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
			// If the API call fails, return what we have from PendingCreates
			break
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

			containerID := derefTag(site.Tags["sockerless-container-id"])
			if containerID == "" || seen[containerID] {
				continue
			}
			seen[containerID] = true

			c := siteToContainer(site.Tags, site.Properties, site.Name)

			// Overlay recorded invocation outcome.
			if inv, ok := p.server.Store.GetInvocationResult(c.ID); ok {
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
				functionURL := ""
				if site.Properties != nil && site.Properties.DefaultHostName != nil {
					scheme := "https"
					if strings.HasPrefix(p.server.config.EndpointURL, "http://") {
						scheme = "http"
					}
					functionURL = scheme + "://" + *site.Properties.DefaultHostName + "/api/function"
				}
				p.server.AZF.Put(containerID, AZFState{
					FunctionAppName: funcAppName,
					ResourceID:      resourceID,
					FunctionURL:     functionURL,
				})
			}

			containers = append(containers, c)
		}
	}

	return containers, nil
}

// siteToContainer reconstructs an api.Container from Azure Function App tags and properties.
func siteToContainer(tags map[string]*string, props interface{}, siteName *string) api.Container {
	containerID := derefTag(tags["sockerless-container-id"])
	name := derefTag(tags["sockerless-name"])
	if name == "" && containerID != "" {
		name = "/" + containerID[:12]
	}
	if name != "" && !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	// Derive image from LinuxFxVersion (format: "DOCKER|<image>")
	image := ""
	// We can't easily access SiteProperties fields without a type assertion,
	// but we have the tags which carry the image info from creation.
	// The image was set during ContainerCreate and stored in registry metadata.
	if imgTag := derefTag(tags["sockerless-image"]); imgTag != "" {
		image = imgTag
	}

	// Function Apps that exist in Azure are considered "running" (available for invocation)
	state := api.ContainerState{
		Status:  "running",
		Running: true,
	}

	// Parse Docker labels from tags (Azure tags use hyphens, matching ParseLabelsFromTags directly)
	hyphenTags := azureTagsToMap(tags)
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
			Image:  image,
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
		Driver:   "azure-functions",
	}
}

// azureTagsToMap converts Azure ptr-based tags to a plain string map.
func azureTagsToMap(tags map[string]*string) map[string]string {
	m := make(map[string]string, len(tags))
	for k, v := range tags {
		if v != nil {
			m[k] = *v
		}
	}
	return m
}

// derefTag safely dereferences an Azure tag pointer.
func derefTag(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
