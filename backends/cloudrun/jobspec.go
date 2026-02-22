package cloudrun

import (
	"fmt"
	"strings"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	"google.golang.org/protobuf/types/known/durationpb"
)

// buildJobName generates a Cloud Run Job name from a container ID.
func buildJobName(containerID string) string {
	return fmt.Sprintf("sockerless-%s", containerID[:12])
}

// buildJobParent returns the Cloud Run parent resource path.
func (s *Server) buildJobParent() string {
	return fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
}

// buildJobSpec creates a Cloud Run Job protobuf from a container config.
func (s *Server) buildJobSpec(containerID string, container *api.Container, agentToken string) *runpb.Job {
	config := container.Config

	// Build environment variables
	var envVars []*runpb.EnvVar
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars = append(envVars, &runpb.EnvVar{
				Name:   parts[0],
				Values: &runpb.EnvVar_Value{Value: parts[1]},
			})
		}
	}

	// Add agent env vars
	envVars = append(envVars,
		&runpb.EnvVar{Name: "SOCKERLESS_AGENT_TOKEN", Values: &runpb.EnvVar_Value{Value: agentToken}},
	)

	// Build entrypoint: callback mode or forward agent mode
	var entrypoint []string
	if s.config.CallbackURL != "" {
		callbackURL := fmt.Sprintf("%s/internal/v1/agent/connect?id=%s&token=%s", s.config.CallbackURL, containerID, agentToken)
		entrypoint = core.BuildAgentCallbackEntrypoint(config, callbackURL)
		envVars = append(envVars,
			&runpb.EnvVar{Name: "SOCKERLESS_CONTAINER_ID", Values: &runpb.EnvVar_Value{Value: containerID}},
			&runpb.EnvVar{Name: "SOCKERLESS_AGENT_CALLBACK_URL", Values: &runpb.EnvVar_Value{Value: callbackURL}},
		)
	} else {
		entrypoint, _ = core.BuildAgentEntrypoint(config)
	}

	cpu, memory := mapCPUMemory()

	containerSpec := &runpb.Container{
		Name:    "main",
		Image:   config.Image,
		Command: entrypoint,
		Env:     envVars,
		Resources: &runpb.ResourceRequirements{
			Limits: map[string]string{
				"cpu":    cpu,
				"memory": memory,
			},
		},
		Ports: []*runpb.ContainerPort{
			{ContainerPort: 9111},
		},
	}

	if config.WorkingDir != "" {
		containerSpec.WorkingDir = config.WorkingDir
	}

	taskTemplate := &runpb.TaskTemplate{
		Containers: []*runpb.Container{containerSpec},
		Retries:    &runpb.TaskTemplate_MaxRetries{MaxRetries: 0},
		Timeout:    durationpb.New(3600 * 4), // 4 hour max
	}

	// Add VPC connector if configured
	if s.config.VPCConnector != "" {
		taskTemplate.VpcAccess = &runpb.VpcAccess{
			Connector: s.config.VPCConnector,
			Egress:    runpb.VpcAccess_ALL_TRAFFIC,
		}
	}

	return &runpb.Job{
		Labels: map[string]string{
			"sockerless-container-id": containerID[:12],
			"managed-by":             "sockerless",
		},
		Template: &runpb.ExecutionTemplate{
			TaskCount:   1,
			Parallelism: 1,
			Template:    taskTemplate,
		},
	}
}

// mapCPUMemory returns the default Cloud Run resource limits.
// Cloud Run valid CPU: 1, 2, 4, 8. Default: 1 CPU, 512Mi.
func mapCPUMemory() (string, string) {
	return "1", "512Mi"
}
