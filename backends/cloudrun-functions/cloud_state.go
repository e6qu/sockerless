package gcf

import (
	"context"
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
	// Check WaitChs first — FaaS containers use exit channels
	if ch, ok := p.server.Store.WaitChs.Load(containerID); ok {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ch.(chan struct{}):
			// Channel closed — container exited.
			// Re-query to get exit code.
			containers, err := p.queryFunctions(ctx)
			if err == nil {
				for _, c := range containers {
					if c.ID == containerID {
						return c.State.ExitCode, nil
					}
				}
			}
			return 0, nil
		}
	}

	// Fallback: poll cloud API
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

		containerID := labels["sockerless_container_id"]
		if containerID == "" || seen[containerID] {
			continue
		}
		seen[containerID] = true

		c := functionToContainer(fn, labels)

		// Sync GCF state store with cloud state
		if _, exists := p.server.GCF.Get(containerID); !exists {
			funcName := extractFunctionName(fn.Name)
			functionURL := ""
			if fn.ServiceConfig != nil {
				functionURL = fn.ServiceConfig.Uri
			}
			p.server.GCF.Put(containerID, GCFState{
				FunctionName: funcName,
				FunctionURL:  functionURL,
				LogResource:  funcName,
			})
		}

		containers = append(containers, c)
	}

	return containers, nil
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

	// Parse Docker labels from GCP labels (convert underscores back to hyphens)
	hyphenLabels := gcpLabelsToHyphenMap(labels)
	dockerLabels := core.ParseLabelsFromTags(hyphenLabels)
	if dockerLabels == nil {
		dockerLabels = make(map[string]string)
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
					IPAddress: "0.0.0.0",
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

// extractFunctionName extracts the short function name from a fully qualified name.
// e.g. "projects/my-project/locations/us-central1/functions/skls-abc123" -> "skls-abc123"
func extractFunctionName(fullName string) string {
	if i := strings.LastIndex(fullName, "/"); i >= 0 && i < len(fullName)-1 {
		return fullName[i+1:]
	}
	return fullName
}
