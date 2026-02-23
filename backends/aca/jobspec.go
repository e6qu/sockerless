package aca

import (
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// containerInput groups the data needed to build one ACA container spec.
type containerInput struct {
	ID         string
	Container  *api.Container
	AgentToken string
	IsMain     bool // true = inject agent entrypoint
}

// buildJobName generates an ACA Job name from a container ID.
func buildJobName(containerID string) string {
	return fmt.Sprintf("sockerless-%s", containerID[:12])
}

// buildContainerSpec builds a single ACA container spec.
// IsMain containers get agent injection; sidecars use their original entrypoint.
func (s *Server) buildContainerSpec(ci containerInput) *armappcontainers.Container {
	config := ci.Container.Config

	// Build environment variables
	var envVars []*armappcontainers.EnvironmentVar
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars = append(envVars, &armappcontainers.EnvironmentVar{
				Name:  ptr(parts[0]),
				Value: ptr(parts[1]),
			})
		}
	}

	var entrypoint []string
	if ci.IsMain {
		// Agent injection for main container
		envVars = append(envVars,
			&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_AGENT_TOKEN"), Value: ptr(ci.AgentToken)},
		)

		if s.config.CallbackURL != "" {
			callbackURL := fmt.Sprintf("%s/internal/v1/agent/connect?id=%s&token=%s", s.config.CallbackURL, ci.ID, ci.AgentToken)
			entrypoint = core.BuildAgentCallbackEntrypoint(config, callbackURL)
			envVars = append(envVars,
				&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_CONTAINER_ID"), Value: ptr(ci.ID)},
				&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_AGENT_CALLBACK_URL"), Value: ptr(callbackURL)},
			)
		} else {
			entrypoint, _ = core.BuildAgentEntrypoint(config)
		}
	} else {
		// Sidecar: use original entrypoint, no agent
		if len(config.Entrypoint) > 0 {
			entrypoint = config.Entrypoint
		} else if len(config.Cmd) > 0 {
			entrypoint = config.Cmd
		}
	}

	// Convert entrypoint to []*string
	var command []*string
	for _, arg := range entrypoint {
		command = append(command, ptr(arg))
	}

	// Container name
	defName := "main"
	if !ci.IsMain {
		defName = sanitizeContainerName(ci.Container.Name)
	}

	cpu, memory := mapCPUTier()

	return &armappcontainers.Container{
		Name:    ptr(defName),
		Image:   ptr(config.Image),
		Command: command,
		Env:     envVars,
		Resources: &armappcontainers.ContainerResources{
			CPU:    &cpu,
			Memory: ptr(memory),
		},
	}
}

// buildJobSpec creates an ACA Job resource from one or more containers.
func (s *Server) buildJobSpec(containers []containerInput) armappcontainers.Job {
	var specs []*armappcontainers.Container
	for _, ci := range containers {
		specs = append(specs, s.buildContainerSpec(ci))
	}

	replicaTimeout := int32(3600 * 4) // 4 hours max

	environmentID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
		s.config.SubscriptionID, s.config.ResourceGroup, s.config.Environment)

	triggerType := armappcontainers.TriggerTypeManual

	tags := core.TagSet{
		ContainerID: containers[0].ID,
		Backend:     "aca",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	return armappcontainers.Job{
		Location: ptr(s.config.Location),
		Tags:     tags.AsAzurePtrMap(),
		Properties: &armappcontainers.JobProperties{
			EnvironmentID: ptr(environmentID),
			Configuration: &armappcontainers.JobConfiguration{
				TriggerType:    &triggerType,
				ReplicaTimeout: &replicaTimeout,
				ManualTriggerConfig: &armappcontainers.JobConfigurationManualTriggerConfig{
					Parallelism:            ptr(int32(1)),
					ReplicaCompletionCount: ptr(int32(1)),
				},
				ReplicaRetryLimit: ptr(int32(0)),
			},
			Template: &armappcontainers.JobTemplate{
				Containers: specs,
			},
		},
	}
}

// mapCPUTier returns the default ACA CPU/memory tier.
// Valid ACA CPU tiers: 0.25, 0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0, 4.0.
// Default: 1.0 CPU, 2Gi.
func mapCPUTier() (float64, string) {
	return 1.0, "2Gi"
}

func ptr[T any](v T) *T {
	return &v
}

// sanitizeContainerName converts a container name to a valid ACA container name.
// Strips leading "/" and replaces non-alphanumeric characters with "-". Lowercased.
func sanitizeContainerName(name string) string {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return "sidecar"
	}
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		} else if c >= 'A' && c <= 'Z' {
			b.WriteRune(c + 32) // lowercase
		} else {
			b.WriteByte('-')
		}
	}
	result := b.String()
	if result == "" {
		return "sidecar"
	}
	return result
}
