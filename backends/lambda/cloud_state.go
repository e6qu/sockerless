package lambda

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// lambdaCloudState implements core.CloudStateProvider for Lambda.
// Queries Lambda functions tagged with sockerless-managed=true.
type lambdaCloudState struct {
	server *Server
}

func (p *lambdaCloudState) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
	containers, err := p.queryFunctions(ctx)
	if err != nil {
		return api.Container{}, false, err
	}
	for _, c := range containers {
		if c.ID == ref || c.Name == ref || c.Name == "/"+ref || strings.TrimPrefix(c.Name, "/") == ref {
			return c, true, nil
		}
		if len(ref) >= 3 && strings.HasPrefix(c.ID, ref) {
			return c, true, nil
		}
	}
	return api.Container{}, false, nil
}

func (p *lambdaCloudState) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
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

func (p *lambdaCloudState) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
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

func (p *lambdaCloudState) WaitForExit(ctx context.Context, containerID string) (int, error) {
	ticker := time.NewTicker(p.server.config.PollInterval)
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

// queryFunctions lists all sockerless-managed Lambda functions.
func (p *lambdaCloudState) queryFunctions(ctx context.Context) ([]api.Container, error) {
	var containers []api.Container
	var marker *string

	for {
		result, err := p.server.aws.Lambda.ListFunctions(ctx, &awslambda.ListFunctionsInput{
			Marker: marker,
		})
		if err != nil {
			return nil, err
		}

		for _, fn := range result.Functions {
			funcName := aws.ToString(fn.FunctionName)

			// Get tags to check if sockerless-managed
			tagsResult, err := p.server.aws.Lambda.ListTags(ctx, &awslambda.ListTagsInput{
				Resource: fn.FunctionArn,
			})
			if err != nil {
				continue
			}

			tags := tagsResult.Tags
			if tags["sockerless-managed"] != "true" {
				continue
			}

			containerID := tags["sockerless-container-id"]
			name := tags["sockerless-name"]
			if name == "" {
				name = "/" + funcName
			}

			// Lambda functions are "running" (available for invocation) or don't exist
			// Check Lambda state for function state
			state := api.ContainerState{
				Status:  "running",
				Running: true,
			}

			// Check if function is in a terminal state
			if fn.State == "Failed" || fn.State == "Inactive" {
				state = api.ContainerState{
					Status: "exited",
					Error:  aws.ToString(fn.StateReason),
				}
			}

			image := string(fn.PackageType)
			// Lambda FunctionConfiguration doesn't include Code in list response;
			// image URI is in tags or can be fetched via GetFunction
			if imgTag, ok := tags["sockerless-image"]; ok {
				image = imgTag
			}

			labels := core.ParseLabelsFromTags(tags)
			if labels == nil {
				labels = make(map[string]string)
			}

			created := ""
			if fn.LastModified != nil {
				created = *fn.LastModified
			}

			containers = append(containers, api.Container{
				ID:      containerID,
				Name:    name,
				Created: created,
				Image:   image,
				State:   state,
				Config: api.ContainerConfig{
					Image:  image,
					Labels: labels,
				},
				HostConfig: api.HostConfig{NetworkMode: "bridge"},
				NetworkSettings: api.NetworkSettings{
					Networks: map[string]*api.EndpointSettings{
						"bridge": {NetworkID: "bridge", IPAddress: "0.0.0.0"},
					},
				},
			})
		}

		if result.NextMarker == nil {
			break
		}
		marker = result.NextMarker
	}

	return containers, nil
}
