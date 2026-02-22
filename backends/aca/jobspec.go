package aca

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// buildJobName generates an ACA Job name from a container ID.
func buildJobName(containerID string) string {
	return fmt.Sprintf("sockerless-%s", containerID[:12])
}

// buildJobSpec creates an ACA Job resource from a container config.
func (s *Server) buildJobSpec(containerID string, container *api.Container, agentToken string) armappcontainers.Job {
	config := container.Config

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

	// Add agent env vars
	envVars = append(envVars,
		&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_AGENT_TOKEN"), Value: ptr(agentToken)},
	)

	// Build entrypoint: callback mode or forward agent mode
	var entrypoint []string
	if s.config.CallbackURL != "" {
		callbackURL := fmt.Sprintf("%s/internal/v1/agent/connect?id=%s&token=%s", s.config.CallbackURL, containerID, agentToken)
		entrypoint = core.BuildAgentCallbackEntrypoint(config, callbackURL)
		envVars = append(envVars,
			&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_CONTAINER_ID"), Value: ptr(containerID)},
			&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_AGENT_CALLBACK_URL"), Value: ptr(callbackURL)},
		)
	} else {
		entrypoint, _ = core.BuildAgentEntrypoint(config)
	}

	// Convert entrypoint to []*string
	var command []*string
	for _, arg := range entrypoint {
		command = append(command, ptr(arg))
	}

	cpu, memory := mapCPUTier()
	replicaTimeout := int32(3600 * 4) // 4 hours max

	environmentID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
		s.config.SubscriptionID, s.config.ResourceGroup, s.config.Environment)

	triggerType := armappcontainers.TriggerTypeManual

	return armappcontainers.Job{
		Location: ptr(s.config.Location),
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
				Containers: []*armappcontainers.Container{
					{
						Name:    ptr("main"),
						Image:   ptr(config.Image),
						Command: command,
						Env:     envVars,
						Resources: &armappcontainers.ContainerResources{
							CPU:    &cpu,
							Memory: ptr(memory),
						},
					},
				},
			},
		},
		Tags: map[string]*string{
			"sockerless-container-id": ptr(containerID[:12]),
			"managed-by":             ptr("sockerless"),
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
