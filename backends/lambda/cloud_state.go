package lambda

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
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
	// Short-circuit on the in-memory invocation result if the
	// goroutine has already recorded it.
	if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
		return inv.ExitCode, nil
	}
	ticker := time.NewTicker(p.server.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
			if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
				return inv.ExitCode, nil
			}
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

// ListImages queries ECR for every image sockerless can use. Phase
// 89 /step 2 cross-cloud sibling.
func (p *lambdaCloudState) ListImages(ctx context.Context) ([]*api.ImageSummary, error) {
	if p.server.aws.ECR == nil {
		return nil, nil
	}
	var result []*api.ImageSummary
	var nextToken *string
	for {
		reposOut, err := p.server.aws.ECR.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return result, err
		}
		for _, repo := range reposOut.Repositories {
			repoName := aws.ToString(repo.RepositoryName)
			repoURI := aws.ToString(repo.RepositoryUri)
			var imgToken *string
			for {
				imgsOut, imErr := p.server.aws.ECR.DescribeImages(ctx, &ecr.DescribeImagesInput{
					RepositoryName: aws.String(repoName),
					NextToken:      imgToken,
				})
				if imErr != nil {
					break
				}
				for _, img := range imgsOut.ImageDetails {
					var repoTags []string
					for _, t := range img.ImageTags {
						repoTags = append(repoTags, repoURI+":"+t)
					}
					digest := aws.ToString(img.ImageDigest)
					size := int64(0)
					if img.ImageSizeInBytes != nil {
						size = *img.ImageSizeInBytes
					}
					pushedAt := int64(0)
					if img.ImagePushedAt != nil {
						pushedAt = img.ImagePushedAt.Unix()
					}
					result = append(result, &api.ImageSummary{
						ID:          digest,
						RepoTags:    repoTags,
						RepoDigests: []string{repoURI + "@" + digest},
						Created:     pushedAt,
						Size:        size,
						VirtualSize: size,
					})
				}
				if imgsOut.NextToken == nil {
					break
				}
				imgToken = imgsOut.NextToken
			}
		}
		if reposOut.NextToken == nil {
			break
		}
		nextToken = reposOut.NextToken
	}
	return result, nil
}

// resolveFunctionARN returns the Lambda function ARN for a given
// container ID, or "" if no matching sockerless-managed function is
// found.//cross-cloud sibling: state is
// derived from cloud actuals (Lambda function tags), not from the
// in-memory cache.
func (p *lambdaCloudState) resolveFunctionARN(ctx context.Context, containerID string) (string, string, error) {
	var marker *string
	for {
		result, err := p.server.aws.Lambda.ListFunctions(ctx, &awslambda.ListFunctionsInput{
			Marker: marker,
		})
		if err != nil {
			return "", "", err
		}
		for _, fn := range result.Functions {
			tagsResult, err := p.server.aws.Lambda.ListTags(ctx, &awslambda.ListTagsInput{
				Resource: fn.FunctionArn,
			})
			if err != nil {
				continue
			}
			if tagsResult.Tags["sockerless-managed"] != "true" {
				continue
			}
			if tagsResult.Tags["sockerless-container-id"] == containerID {
				return aws.ToString(fn.FunctionArn), aws.ToString(fn.FunctionName), nil
			}
		}
		if result.NextMarker == nil {
			return "", "", nil
		}
		marker = result.NextMarker
	}
}

// resolveLambdaState returns LambdaState for the given container ID,
// deriving from cloud actuals when the in-memory cache is empty. Phase
// 89 /cross-cloud sibling.
func (s *Server) resolveLambdaState(ctx context.Context, containerID string) (LambdaState, bool) {
	if state, ok := s.Lambda.Get(containerID); ok && state.FunctionARN != "" {
		return state, true
	}
	csp, ok := s.CloudState.(*lambdaCloudState)
	if !ok {
		return LambdaState{}, false
	}
	arn, name, err := csp.resolveFunctionARN(ctx, containerID)
	if err != nil || arn == "" {
		return LambdaState{}, false
	}
	state := LambdaState{FunctionARN: arn, FunctionName: name}
	s.Lambda.Update(containerID, func(st *LambdaState) {
		if st.FunctionARN == "" {
			st.FunctionARN = arn
		}
		if st.FunctionName == "" {
			st.FunctionName = name
		}
	})
	return state, true
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

			// Pod Functions: one Lambda backs N container rows. The pod
			// manifest is in the SOCKERLESS_POD_CONTAINERS env var; each
			// member becomes a `docker ps` row keyed on its original
			// container ID (round-tripped through the manifest's
			// container_id field). Skip the per-member emit and fall
			// through to single-container handling when the function
			// is not pod-managed.
			if tags["sockerless-pod"] != "" {
				rows := podMembersFromLambda(ctx, p.server, funcName, fn, tags)
				containers = append(containers, rows...)
				continue
			}

			containerID := tags["sockerless-container-id"]
			name := tags["sockerless-name"]
			if name == "" {
				name = "/" + funcName
			}

			// If the invocation goroutine has recorded an exit
			// outcome, surface it. Otherwise the function is either
			// `running` (invocation in flight / available for a new one)
			// or `exited` (Lambda control-plane reports Failed/Inactive).
			state := api.ContainerState{
				Status:  "running",
				Running: true,
			}
			if inv, ok := p.server.Store.GetInvocationResult(containerID); ok {
				state = api.ContainerState{
					Status:     "exited",
					Running:    false,
					ExitCode:   inv.ExitCode,
					FinishedAt: inv.FinishedAt.UTC().Format(time.RFC3339Nano),
					Error:      inv.Error,
				}
			} else if fn.State == "Failed" || fn.State == "Inactive" {
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

			// AWS Lambda's `LastModified` ships in
			// `"2006-01-02T15:04:05.000+0000"` format (`%Y-%m-%dT%H:%M:%S.fff%z`),
			// which is not RFC3339-parseable. The shared
			// `ContainerSummary.Created` projection in `backend-core`
			// parses `c.Created` as RFC3339Nano to convert to Unix
			// seconds — without normalising here, that parse fails
			// silently and `docker ps` rendered Created as 0
			// (1970-01-01 → "292 years ago" by 2026).
			created := ""
			if fn.LastModified != nil {
				if t, perr := time.Parse("2006-01-02T15:04:05.000-0700", *fn.LastModified); perr == nil {
					created = t.UTC().Format(time.RFC3339Nano)
				}
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
						"bridge": {NetworkID: "bridge", IPAddress: ""},
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

// podMembersFromLambda emits one `docker ps` row per pod member when
// the Lambda function is sockerless-pod-managed. The pod manifest is in
// the function's Environment.Variables["SOCKERLESS_POD_CONTAINERS"]
// (round-tripped via EncodePodManifest). For each member: build a row
// with the spec's honest namespace-degradation surface — labels carry
// the per-namespace state and HostConfig.PidMode = "shared-degraded"
// matches docker's native schema for the PID mode.
func podMembersFromLambda(ctx context.Context, srv *Server, funcName string, fn lambdatypes.FunctionConfiguration, tags map[string]string) []api.Container {
	// ListFunctions returns FunctionConfiguration without
	// Environment.Variables; need a GetFunction to read envs.
	out, err := srv.aws.Lambda.GetFunction(ctx, &awslambda.GetFunctionInput{
		FunctionName: aws.String(funcName),
	})
	if err != nil || out.Configuration == nil || out.Configuration.Environment == nil {
		return nil
	}
	enc := out.Configuration.Environment.Variables["SOCKERLESS_POD_CONTAINERS"]
	members, err := DecodePodManifest(enc)
	if err != nil || len(members) == 0 {
		return nil
	}
	created := ""
	if fn.LastModified != nil {
		if t, perr := time.Parse("2006-01-02T15:04:05.000-0700", *fn.LastModified); perr == nil {
			created = t.UTC().Format(time.RFC3339Nano)
		}
	}
	state := api.ContainerState{Status: "running", Running: true}
	if fn.State == "Failed" || fn.State == "Inactive" {
		state = api.ContainerState{Status: "exited", Error: aws.ToString(fn.StateReason)}
	}
	rows := make([]api.Container, 0, len(members))
	for _, m := range members {
		if m.ContainerID == "" {
			continue
		}
		rowState := state
		if inv, ok := srv.Store.GetInvocationResult(m.ContainerID); ok {
			rowState = api.ContainerState{
				Status:     "exited",
				Running:    false,
				ExitCode:   inv.ExitCode,
				FinishedAt: inv.FinishedAt.UTC().Format(time.RFC3339Nano),
				Error:      inv.Error,
			}
		}
		name := "/" + m.Name
		if m.Name == "" {
			name = "/" + m.ContainerID[:12]
		}
		rows = append(rows, api.Container{
			ID:      m.ContainerID,
			Name:    name,
			Created: created,
			Image:   m.Image,
			State:   rowState,
			Config: api.ContainerConfig{
				Image:      m.Image,
				Entrypoint: m.Entrypoint,
				Cmd:        m.Cmd,
				Env:        m.Env,
				WorkingDir: m.Workdir,
				// Per spec § "Podman pods on FaaS backends — Honest mapping",
				// pod members on FaaS share mount-ns (chroot only) and PID-ns
				// because the cloud sandbox blocks `unshare(CLONE_NEWNS|CLONE_NEWPID)`.
				// Surfacing this via Labels alongside HostConfig.PidMode below
				// is the operator's signal to fall through to a real-isolation
				// backend (ecs-fargate / aca) when isolation is load-bearing.
				Labels: map[string]string{
					"sockerless.pod":               tags["sockerless-pod"],
					"sockerless.pod.member":        m.Name,
					"sockerless.namespace.mount":   "shared-degraded",
					"sockerless.namespace.pid":     "shared-degraded",
					"sockerless.namespace.user":    "shared-degraded",
					"sockerless.namespace.cgroup":  "shared-degraded",
					"sockerless.namespace.network": "shared",
					"sockerless.namespace.ipc":     "shared",
					"sockerless.namespace.uts":     "shared",
				},
			},
			HostConfig: api.HostConfig{
				NetworkMode: "bridge",
				PidMode:     "shared-degraded",
			},
			NetworkSettings: api.NetworkSettings{
				Networks: map[string]*api.EndpointSettings{
					"bridge": {NetworkID: "bridge"},
				},
			},
			Platform: "linux",
			Driver:   "lambda",
		})
	}
	return rows
}
