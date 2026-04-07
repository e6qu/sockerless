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
	ID        string
	Container *api.Container
	IsMain    bool // true = primary container in a pod
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

	entrypoint := config.Entrypoint
	command := config.Cmd

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
		Args:    command,
		Env:     envVars,
		Resources: &runpb.ResourceRequirements{
			Limits: map[string]string{
				"cpu":    cpu,
				"memory": memory,
			},
		},
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
		Name:        containers[0].Container.Name,
		Network:     containers[0].Container.HostConfig.NetworkMode,
	}

	return &runpb.Job{
		Labels:      tags.AsGCPLabels(),
		Annotations: tags.AsGCPAnnotations(),
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
