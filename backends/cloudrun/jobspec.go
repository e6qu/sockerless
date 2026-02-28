package cloudrun

import (
	"fmt"
	"strings"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	"google.golang.org/protobuf/types/known/durationpb"
)

// containerInput groups the data needed to build one Cloud Run container spec.
type containerInput struct {
	ID         string
	Container  *api.Container
	AgentToken string
	IsMain     bool // true = inject agent entrypoint + port 9111
}

// buildJobName generates a Cloud Run Job name from a container ID.
func buildJobName(containerID string) string {
	return fmt.Sprintf("sockerless-%s", containerID[:12])
}

// buildJobParent returns the Cloud Run parent resource path.
func (s *Server) buildJobParent() string {
	return fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
}

// buildContainerSpec builds a single Cloud Run container spec.
// IsMain containers get agent injection and port 9111; sidecars use their original entrypoint.
func (s *Server) buildContainerSpec(ci containerInput) *runpb.Container {
	config := ci.Container.Config

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

	var entrypoint []string
	if ci.IsMain {
		// Agent injection for main container
		envVars = append(envVars,
			&runpb.EnvVar{Name: "SOCKERLESS_AGENT_TOKEN", Values: &runpb.EnvVar_Value{Value: ci.AgentToken}},
		)

		if s.config.CallbackURL != "" {
			callbackURL := fmt.Sprintf("%s/internal/v1/agent/connect?id=%s&token=%s", s.config.CallbackURL, ci.ID, ci.AgentToken)
			entrypoint = core.BuildAgentCallbackEntrypoint(config, callbackURL)
			envVars = append(envVars,
				&runpb.EnvVar{Name: "SOCKERLESS_CONTAINER_ID", Values: &runpb.EnvVar_Value{Value: ci.ID}},
				&runpb.EnvVar{Name: "SOCKERLESS_AGENT_CALLBACK_URL", Values: &runpb.EnvVar_Value{Value: callbackURL}},
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

	// Container name
	defName := "main"
	if !ci.IsMain {
		defName = sanitizeContainerName(ci.Container.Name)
	}

	cpu, memory := mapCPUMemory()

	containerSpec := &runpb.Container{
		Name:    defName,
		Image:   config.Image,
		Command: entrypoint,
		Env:     envVars,
		Resources: &runpb.ResourceRequirements{
			Limits: map[string]string{
				"cpu":    cpu,
				"memory": memory,
			},
		},
	}

	// Port mapping for agent (main container only)
	if ci.IsMain {
		containerSpec.Ports = []*runpb.ContainerPort{
			{ContainerPort: 9111},
		}
	}

	if config.WorkingDir != "" {
		containerSpec.WorkingDir = config.WorkingDir
	}

	return containerSpec
}

// buildJobSpec creates a Cloud Run Job protobuf from one or more containers.
func (s *Server) buildJobSpec(containers []containerInput) *runpb.Job {
	var specs []*runpb.Container
	for _, ci := range containers {
		specs = append(specs, s.buildContainerSpec(ci))
	}

	taskTemplate := &runpb.TaskTemplate{
		Containers: specs,
		Retries:    &runpb.TaskTemplate_MaxRetries{MaxRetries: 0},
		Timeout:    durationpb.New(4 * time.Hour),
	}

	// Add VPC connector if configured
	if s.config.VPCConnector != "" {
		taskTemplate.VpcAccess = &runpb.VpcAccess{
			Connector: s.config.VPCConnector,
			Egress:    runpb.VpcAccess_ALL_TRAFFIC,
		}
	}

	tags := core.TagSet{
		ContainerID: containers[0].ID,
		Backend:     "cloudrun",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	return &runpb.Job{
		Labels: tags.AsGCPLabels(),
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

// sanitizeContainerName converts a container name to a valid Cloud Run container name.
// Strips leading "/" and replaces non-alphanumeric characters with "-".
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
