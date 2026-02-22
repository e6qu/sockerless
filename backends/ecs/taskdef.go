package ecs

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// registerTaskDefinition creates an ECS task definition from a container config.
func (s *Server) registerTaskDefinition(ctx context.Context, containerID string, container *api.Container, agentToken string) (string, error) {
	config := container.Config

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

	// Build the entrypoint/command with agent injection
	var entrypoint, command []string
	if s.config.CallbackURL != "" {
		callbackURL := fmt.Sprintf("%s/internal/v1/agent/connect?id=%s&token=%s", s.config.CallbackURL, containerID, agentToken)
		entrypoint = core.BuildAgentCallbackEntrypoint(config, callbackURL)

		// Add agent env vars
		envVars = append(envVars,
			ecstypes.KeyValuePair{Name: aws.String("SOCKERLESS_CONTAINER_ID"), Value: aws.String(containerID)},
			ecstypes.KeyValuePair{Name: aws.String("SOCKERLESS_AGENT_TOKEN"), Value: aws.String(agentToken)},
			ecstypes.KeyValuePair{Name: aws.String("SOCKERLESS_AGENT_CALLBACK_URL"), Value: aws.String(callbackURL)},
		)
	} else {
		entrypoint, command = core.BuildAgentEntrypoint(config)
	}

	// Container definition
	containerDef := ecstypes.ContainerDefinition{
		Name:       aws.String("main"),
		Image:      aws.String(config.Image),
		Essential:  aws.Bool(true),
		EntryPoint: entrypoint,
		Command:    command,
		Environment: envVars,
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwslogs,
			Options: map[string]string{
				"awslogs-group":         s.config.LogGroup,
				"awslogs-region":        s.config.Region,
				"awslogs-stream-prefix": containerID[:12],
			},
		},
	}

	if config.WorkingDir != "" {
		containerDef.WorkingDirectory = aws.String(config.WorkingDir)
	}

	if config.User != "" {
		containerDef.User = aws.String(config.User)
	}

	// Port mapping for agent
	containerDef.PortMappings = []ecstypes.PortMapping{
		{
			ContainerPort: aws.Int32(9111),
			Protocol:      ecstypes.TransportProtocolTcp,
		},
	}

	// Build volumes and mount points for bind mounts
	var volumes []ecstypes.Volume
	var mountPoints []ecstypes.MountPoint
	for i, bind := range container.HostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		volName := fmt.Sprintf("bind-%d", i)
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

	// Task definition family name
	family := fmt.Sprintf("sockerless-%s", containerID[:12])

	input := &awsecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(family),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		ContainerDefinitions:    []ecstypes.ContainerDefinition{containerDef},
		Volumes:                 volumes,
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
