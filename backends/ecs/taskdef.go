package ecs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// containerInput groups the data needed to build one ECS container definition.
type containerInput struct {
	ID         string
	Container  *api.Container
	AgentToken string
	IsMain     bool // true = inject agent entrypoint + port 9111
}

// buildContainerDef builds a single ECS container definition.
// IsMain containers get agent injection and port 9111; sidecars use their original entrypoint.
func (s *Server) buildContainerDef(ci containerInput) (ecstypes.ContainerDefinition, []ecstypes.Volume) {
	config := ci.Container.Config

	// Build environment variables
	var envVars []ecstypes.KeyValuePair
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars = append(envVars, ecstypes.KeyValuePair{
				Name:  aws.String(parts[0]),
				Value: aws.String(parts[1]),
			})
		}
	}

	var entrypoint, command []string
	if ci.IsMain {
		if s.config.EndpointURL != "" {
			// Simulator mode: pass original command through, no agent wrapping
			entrypoint = config.Entrypoint
			command = config.Cmd
		} else if s.config.CallbackURL != "" {
			callbackURL := fmt.Sprintf("%s/internal/v1/agent/connect?id=%s&token=%s", s.config.CallbackURL, ci.ID, ci.AgentToken)
			entrypoint = core.BuildAgentCallbackEntrypoint(config, callbackURL)
			envVars = append(envVars,
				ecstypes.KeyValuePair{Name: aws.String("SOCKERLESS_CONTAINER_ID"), Value: aws.String(ci.ID)},
				ecstypes.KeyValuePair{Name: aws.String("SOCKERLESS_AGENT_TOKEN"), Value: aws.String(ci.AgentToken)},
				ecstypes.KeyValuePair{Name: aws.String("SOCKERLESS_AGENT_CALLBACK_URL"), Value: aws.String(callbackURL)},
			)
		} else {
			entrypoint, command = core.BuildAgentEntrypoint(config)
		}
	} else {
		// Sidecar: use original entrypoint/command, no agent
		if len(config.Entrypoint) > 0 {
			entrypoint = config.Entrypoint
		}
		if len(config.Cmd) > 0 {
			command = config.Cmd
		}
	}

	// Container name: "main" for the primary, sanitized name for sidecars
	defName := "main"
	if !ci.IsMain {
		defName = sanitizeContainerName(ci.Container.Name)
	}

	containerDef := ecstypes.ContainerDefinition{
		Name:        aws.String(defName),
		Image:       aws.String(config.Image),
		Essential:   aws.Bool(ci.IsMain),
		EntryPoint:  entrypoint,
		Command:     command,
		Environment: envVars,
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwslogs,
			Options: map[string]string{
				"awslogs-group":         s.config.LogGroup,
				"awslogs-region":        s.config.Region,
				"awslogs-stream-prefix": ci.ID[:12],
			},
		},
	}

	if config.WorkingDir != "" {
		containerDef.WorkingDirectory = aws.String(config.WorkingDir)
	}

	if config.User != "" {
		containerDef.User = aws.String(config.User)
	}

	// Port mapping for agent (main container only, skip in simulator mode)
	if ci.IsMain && s.config.EndpointURL == "" {
		containerDef.PortMappings = []ecstypes.PortMapping{
			{
				ContainerPort: aws.Int32(9111),
				Protocol:      ecstypes.TransportProtocolTcp,
			},
		}
	}

	// Build volumes and mount points for bind mounts
	var volumes []ecstypes.Volume
	var mountPoints []ecstypes.MountPoint
	for i, bind := range ci.Container.HostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		volName := fmt.Sprintf("%s-bind-%d", defName, i)
		volumes = append(volumes, ecstypes.Volume{
			Name: aws.String(volName),
		})
		readOnly := false
		if len(parts) == 3 && parts[2] == "ro" {
			readOnly = true
		}
		mountPoints = append(mountPoints, ecstypes.MountPoint{
			SourceVolume:  aws.String(volName),
			ContainerPath: aws.String(parts[1]),
			ReadOnly:      aws.Bool(readOnly),
		})
	}

	containerDef.MountPoints = mountPoints

	return containerDef, volumes
}

// registerTaskDefinition creates an ECS task definition from one or more containers.
func (s *Server) registerTaskDefinition(ctx context.Context, containers []containerInput) (string, error) {
	var allDefs []ecstypes.ContainerDefinition
	var allVolumes []ecstypes.Volume

	for _, ci := range containers {
		def, vols := s.buildContainerDef(ci)
		allDefs = append(allDefs, def)
		allVolumes = append(allVolumes, vols...)
	}

	// Family name uses the first (main) container ID
	family := fmt.Sprintf("sockerless-%s", containers[0].ID[:12])

	tags := core.TagSet{
		ContainerID: containers[0].ID,
		Backend:     "ecs",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	input := &awsecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(family),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		ContainerDefinitions:    allDefs,
		Volumes:                 allVolumes,
		Tags:                    mapToECSTags(tags.AsMap()),
	}

	if s.config.ExecutionRoleARN != "" {
		input.ExecutionRoleArn = aws.String(s.config.ExecutionRoleARN)
	}
	if s.config.TaskRoleARN != "" {
		input.TaskRoleArn = aws.String(s.config.TaskRoleARN)
	}

	result, err := s.aws.ECS.RegisterTaskDefinition(ctx, input)
	if err != nil {
		return "", err
	}

	return aws.ToString(result.TaskDefinition.TaskDefinitionArn), nil
}

// sanitizeContainerName converts a container name to a valid ECS container definition name.
// Strips leading "/" and replaces non-alphanumeric characters with "-".
func sanitizeContainerName(name string) string {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return "sidecar"
	}
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
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
