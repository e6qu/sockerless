package lambda

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/sockerless/api"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by an AWS Lambda function.
func (s *Server) ContainerCreate(req *api.ContainerCreateRequest) (*api.ContainerCreateResponse, error) {
	name := req.Name
	if name == "" {
		name = "/" + core.GenerateName()
	} else if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	if avail, _ := s.CloudState.CheckNameAvailable(context.Background(), name); !avail {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(name, "/")),
		}
	}

	id := core.GenerateID()

	config := api.ContainerConfig{}
	if req.ContainerConfig != nil {
		config = *req.ContainerConfig
	}

	// Merge image config if available
	if img, ok := s.Store.ResolveImage(config.Image); ok {
		// Merge ENV by key — image provides defaults, container overrides
		config.Env = core.MergeEnvByKey(img.Config.Env, config.Env)
		// Docker clears image Cmd when Entrypoint is overridden in create
		if len(config.Cmd) == 0 && len(config.Entrypoint) == 0 {
			config.Cmd = img.Config.Cmd
		}
		if len(config.Entrypoint) == 0 {
			config.Entrypoint = img.Config.Entrypoint
		}
		if config.WorkingDir == "" {
			config.WorkingDir = img.Config.WorkingDir
		}
	}
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	hostConfig := api.HostConfig{NetworkMode: "default"}
	if req.HostConfig != nil {
		hostConfig = *req.HostConfig
	}
	if hostConfig.NetworkMode == "" {
		hostConfig.NetworkMode = "default"
	}

	path := ""
	var args []string
	if len(config.Entrypoint) > 0 {
		path = config.Entrypoint[0]
		args = append(config.Entrypoint[1:], config.Cmd...)
	} else if len(config.Cmd) > 0 {
		path = config.Cmd[0]
		args = config.Cmd[1:]
	}

	container := api.Container{
		ID:      id,
		Name:    name,
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Path:    path,
		Args:    args,
		State: api.ContainerState{
			Status:     "created",
			FinishedAt: "0001-01-01T00:00:00Z",
			StartedAt:  "0001-01-01T00:00:00Z",
		},
		Image:      config.Image,
		Config:     config,
		HostConfig: hostConfig,
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts:   make([]api.MountPoint, 0),
		Platform: "linux",
		Driver:   "lambda",
	}

	// Set up default network — resolve via store for correct ID and Containers map
	netName := hostConfig.NetworkMode
	if netName == "default" {
		netName = "bridge"
	}
	networkID := netName
	if net, ok := s.Store.ResolveNetwork(netName); ok {
		networkID = net.ID
		// Register container in the network's Containers map
		s.Store.Networks.Update(net.ID, func(n *api.Network) {
			if n.Containers == nil {
				n.Containers = make(map[string]api.EndpointResource)
			}
			n.Containers[id] = api.EndpointResource{
				Name:       strings.TrimPrefix(name, "/"),
				EndpointID: core.GenerateID()[:16],
			}
		})
	}
	container.NetworkSettings.Networks[netName] = &api.EndpointSettings{
		NetworkID:   networkID,
		EndpointID:  core.GenerateID()[:16],
		Gateway:     "",
		IPAddress:   "",
		IPPrefixLen: 16,
		MacAddress:  "",
	}

	// Build function name from container name
	funcName := "skls-" + id[:12]

	// Build environment variables. Lambda caps Environment.Variables
	// JSON at 4 KB total; gitlab-runner sets ~3 KB of CI_* vars at
	// /create time. Drop the user-supplied vars when adding them
	// would push us over budget — they are re-exported at the top of
	// every gitlab-runner stage script (and embedded in the Invoke
	// Payload for Path-B execs) so the runtime values are still
	// available to user processes; only the Lambda-config-level vars
	// suffer (which is fine — gitlab-runner doesn't read them from
	// the environment, it reads them via its own protocol).
	envVars := make(map[string]string)
	// Lambda's 4 KB Environment.Variables JSON budget is small for
	// runners that pass huge env up front (gitlab-runner ships ~50
	// CI_* vars, three JWT tokens at ~600 bytes each, and a 1 KB
	// GitLab features list — together >4 KB before sockerless adds
	// its own SOCKERLESS_* vars). Two-stage filter:
	//
	//  1. Drop entries that gitlab-runner re-exports at the top of
	//     every script anyway (the `: | eval $'export CI_…'` block);
	//     keeping them in Lambda's config-level env is redundant and
	//     leaks credentials into Lambda's GetFunction response.
	//  2. Hard cap remaining entries at 2 KB to leave 2 KB headroom
	//     for the SOCKERLESS_* additions below
	//     (SOCKERLESS_LAMBDA_BIND_LINKS alone can be ~500 B for
	//     gitlab-runner's two-volume setup).
	const lambdaEnvBudget = 2000
	estimatedSize := 2 // for `{}`
	dropped := 0
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		// Filter rule (1): gitlab-runner / GitLab CI vars + GitLab feature
		// flags are re-exported at runtime by gitlab-runner's script
		// preamble. Forwarding them via Lambda env is pure overhead.
		if strings.HasPrefix(parts[0], "CI_") ||
			strings.HasPrefix(parts[0], "FF_") ||
			strings.HasPrefix(parts[0], "GITLAB_") ||
			parts[0] == "GIT_TERMINAL_PROMPT" ||
			parts[0] == "GCM_INTERACTIVE" ||
			parts[0] == "RUNNER_TEMP_PROJECT_DIR" {
			dropped++
			continue
		}
		entrySize := len(parts[0]) + len(parts[1]) + 6 // `"k":"v",`
		if estimatedSize+entrySize > lambdaEnvBudget {
			dropped++
			continue
		}
		envVars[parts[0]] = parts[1]
		estimatedSize += entrySize
	}
	if dropped > 0 {
		s.Logger.Info().Int("dropped", dropped).Int("kept", len(envVars)).Msg("lambda env: dropped CI/FF/GITLAB vars + size-cap user vars (Lambda 4KB Environment limit; runner script re-exports them at runtime)")
	}

	// Build resource tags
	tags := core.TagSet{
		ContainerID: id,
		Backend:     "lambda",
		Cluster:     s.config.Region,
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
		Name:        name,
		Labels:      config.Labels,
	}

	// Resolve the image URI. Lambda image-mode imposes two hard
	// constraints on every function image: (a) Docker schema 2 manifest
	// (OCI rejected), and (b) ENTRYPOINT must be a Lambda Runtime API
	// client (poll /next, post /response). User images rarely satisfy
	// either. Sockerless's responsibility per
	// `specs/CLOUD_RESOURCE_MAPPING.md` § Lambda mapping: route every
	// CreateFunction through the overlay-inject path which (a) bakes
	// `sockerless-lambda-bootstrap` as ENTRYPOINT (resolves the
	// Runtime-API gap) and (b) runs `docker build` + `docker push` /
	// CodeBuild — both produce Docker schema 2 (resolves the manifest
	// gap). The user's original ENTRYPOINT + CMD ride along as
	// `SOCKERLESS_USER_*` env vars; the bootstrap exec's them as a
	// subprocess on each invocation.
	var imageURI string
	envVars["SOCKERLESS_CONTAINER_ID"] = id
	if s.config.CallbackURL != "" {
		envVars["SOCKERLESS_CALLBACK_URL"] = s.config.CallbackURL
	}
	// Encode argv as base64(JSON) so every byte round-trips cleanly
	// through the env var without Dockerfile / shell quoting.
	if len(config.Entrypoint) > 0 {
		b, _ := json.Marshal(config.Entrypoint)
		envVars["SOCKERLESS_USER_ENTRYPOINT"] = base64.StdEncoding.EncodeToString(b)
	}
	if len(config.Cmd) > 0 {
		b, _ := json.Marshal(config.Cmd)
		envVars["SOCKERLESS_USER_CMD"] = base64.StdEncoding.EncodeToString(b)
	}
	// Resolve bind-mount FileSystemConfigs early so the bind-link
	// symlinks can be baked into the overlay image at build time
	// (Lambda's runtime root filesystem is read-only — runtime
	// symlink creation fails).
	var fsConfigs []lambdatypes.FileSystemConfig
	var bindLinks []string
	if len(hostConfig.Binds) > 0 {
		var err error
		fsConfigs, bindLinks, err = s.fileSystemConfigsForBinds(s.ctx(), hostConfig.Binds)
		if err != nil {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("resolve Lambda file-system configs: %v", err)}
		}
	}

	switch {
	case s.config.PrebuiltOverlayImage != "":
		// Operator shipped a ready overlay — skip the build, use as-is.
		imageURI = s.config.PrebuiltOverlayImage
	case s.config.EndpointURL != "":
		// Custom-endpoint mode (sim Lambda / integration tests). The
		// sim doesn't enforce real Lambda's Docker-schema-2 +
		// Runtime-API constraints, so overlay-inject is unnecessary.
		// Resolve and use the base image directly.
		var resolveErr error
		imageURI, resolveErr = s.resolveImageURI(s.ctx(), config.Image)
		if resolveErr != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("failed to resolve image %q to ECR URI: %v", config.Image, resolveErr)}
		}
	default:
		base, err := s.resolveImageURI(s.ctx(), config.Image)
		if err != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("failed to resolve image %q to ECR URI: %v", config.Image, err)}
		}
		spec := OverlayImageSpec{
			BaseImageRef:        base,
			AgentBinaryPath:     s.config.AgentBinaryPath,
			BootstrapBinaryPath: s.config.BootstrapBinaryPath,
			UserEntrypoint:      config.Entrypoint,
			UserCmd:             config.Cmd,
			BindLinks:           bindLinks,
		}
		repo, repoErr := s.overlayECRRepo()
		if repoErr != nil {
			return nil, &api.ServerError{Message: repoErr.Error()}
		}
		destRef := repo + ":" + OverlayContentTag(spec)
		var builder core.CloudBuildService
		if s.images != nil {
			builder = s.images.BuildService
		}
		overlay, buildErr := BuildAndPushOverlayImage(s.ctx(), spec, destRef, builder)
		if buildErr != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("overlay build failed for %q: %v", base, buildErr)}
		}
		imageURI = overlay.ImageURI
	}

	// Create Lambda function. Architectures is the operator-configured
	// `Config.Architecture` value — sockerless reports this same value
	// (Docker-style) via `docker info` so clients pull single-arch
	// images that actually match the Lambda runtime.
	arch := lambdatypes.ArchitectureX8664
	if strings.EqualFold(s.config.Architecture, "arm64") {
		arch = lambdatypes.ArchitectureArm64
	}
	createInput := &awslambda.CreateFunctionInput{
		FunctionName:  aws.String(funcName),
		Role:          aws.String(s.config.RoleARN),
		PackageType:   lambdatypes.PackageTypeImage,
		Architectures: []lambdatypes.Architecture{arch},
		Code: &lambdatypes.FunctionCode{
			ImageUri: aws.String(imageURI),
		},
		MemorySize: aws.Int32(int32(s.config.MemorySize)),
		Timeout:    aws.Int32(int32(s.config.Timeout)),
		Tags:       func() map[string]string { m := tags.AsMap(); m["sockerless-image"] = config.Image; return m }(),
	}

	if len(envVars) > 0 {
		createInput.Environment = &lambdatypes.Environment{
			Variables: envVars,
		}
	}

	// Add VPC config if subnets are specified
	if len(s.config.SubnetIDs) > 0 {
		createInput.VpcConfig = &lambdatypes.VpcConfig{
			SubnetIds:        s.config.SubnetIDs,
			SecurityGroupIds: s.config.SecurityGroupIDs,
		}
	}

	// Attach the resolved FileSystemConfig (already computed above so
	// the bind-link symlinks could be baked into the overlay image).
	// `SOCKERLESS_LAMBDA_BIND_LINKS` env is still emitted as a fallback
	// for runtime-flexible deployments (sim mode, where the overlay
	// path doesn't fire); the in-Lambda bootstrap is idempotent —
	// finding pre-existing symlinks is a no-op. See
	// `specs/CLOUD_RESOURCE_MAPPING.md` § "Lambda bind-mount translation".
	if len(fsConfigs) > 0 {
		createInput.FileSystemConfigs = fsConfigs
	}
	if len(bindLinks) > 0 {
		if createInput.Environment == nil {
			createInput.Environment = &lambdatypes.Environment{Variables: envVars}
		}
		createInput.Environment.Variables["SOCKERLESS_LAMBDA_BIND_LINKS"] = strings.Join(bindLinks, ",")
	}

	// We DON'T propagate Cmd/Entrypoint/WorkingDir to Lambda's
	// ImageConfig — the overlay-inject path bakes
	// `sockerless-lambda-bootstrap` as the ENTRYPOINT (it owns the
	// Lambda Runtime API loop) and handles the user's argv + workdir
	// via env vars (`SOCKERLESS_USER_ENTRYPOINT/CMD/WORKDIR`). Setting
	// `ImageConfig.WorkingDirectory` here would make Lambda's runtime
	// chdir BEFORE the bootstrap runs — and BIND_LINKS-targeted paths
	// like `/__w/<repo>` only exist as symlinks created by the
	// bootstrap, so Lambda's pre-bootstrap chdir fails with
	// `Runtime.InvalidWorkingDir`. The user's workdir is honoured by
	// the bootstrap when it spawns the user subprocess (or in
	// `execEnvelope.Workdir` for Path B execs).
	if config.WorkingDir != "" {
		if createInput.Environment == nil {
			createInput.Environment = &lambdatypes.Environment{Variables: envVars}
		}
		createInput.Environment.Variables["SOCKERLESS_USER_WORKDIR"] = config.WorkingDir
	}

	result, err := s.aws.Lambda.CreateFunction(s.ctx(), createInput)
	if err != nil {
		s.Logger.Error().Err(err).Str("function", funcName).Msg("failed to create Lambda function")
		return nil, awscommon.MapAWSError(err, "function", funcName)
	}

	functionARN := aws.ToString(result.FunctionArn)

	s.PendingCreates.Put(id, container)

	s.Lambda.Put(id, LambdaState{
		FunctionName: funcName,
		FunctionARN:  functionARN,
		OpenStdin:    config.OpenStdin && config.AttachStdin,
	})

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "lambda",
		ResourceType: "function",
		ResourceID:   functionARN,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": container.Image, "name": container.Name, "functionName": funcName},
	})

	s.EmitEvent("container", "create", id, map[string]string{
		"name":  strings.TrimPrefix(name, "/"),
		"image": config.Image,
	})

	return &api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	}, nil
}

// ContainerStart starts a Lambda function invocation for the container.
func (s *Server) ContainerStart(ref string) error {
	// Resolve from PendingCreates (containers between create and start)
	c, ok := s.PendingCreates.Get(ref)
	if !ok {
		// Try name/short-ID match in PendingCreates
		for _, pc := range s.PendingCreates.List() {
			if pc.Name == ref || pc.Name == "/"+ref || (len(ref) >= 3 && strings.HasPrefix(pc.ID, ref)) {
				c = pc
				ok = true
				break
			}
		}
	}
	if !ok {
		// CloudState fallback: gitlab-runner's docker-executor reuses
		// the same container ID across stages (/start cycle 2+).
		// PendingCreates is dropped after cycle 1's deferred Invoke,
		// so subsequent /start calls without this fallback would 404
		// even though the Lambda function still exists. Mirror of the
		// same pattern in `backends/ecs/backend_impl.go::ContainerStart`.
		resolved, found := s.ResolveContainerAuto(context.Background(), ref)
		if !found {
			return &api.NotFoundError{Resource: "container", ID: ref}
		}
		c = resolved
		// Restore the container to PendingCreates so the rest of the
		// start flow finds it. Dropped again at end of the deferred
		// Invoke goroutine.
		s.PendingCreates.Put(c.ID, c)
	}
	id := c.ID

	if c.State.Running {
		return &api.NotModifiedError{}
	}

	// Multi-container pods are not supported by FaaS backends
	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod && len(pod.ContainerIDs) > 1 {
		return &api.InvalidParameterError{
			Message: "multi-container pods are not supported by the lambda backend",
		}
	}

	lambdaState, _ := s.resolveLambdaState(s.ctx(), id)

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Remove from PendingCreates now that the function is being invoked.
	s.PendingCreates.Delete(id)

	// gitlab-runner / `docker run -i` pattern: the container will
	// receive its actual command via stdin on the hijacked attach
	// connection. The Docker SDK's standard sequence is /create →
	// /start → /attach, so /start often arrives BEFORE /attach has
	// registered the stdin pipe. The Invoke goroutine polls
	// `stdinPipes` for a few seconds (covers the /start→/attach
	// gap) so it can wait for stdin EOF before calling Invoke
	// rather than racing in with an empty `{}` payload — bash
	// reading `{}` as a command was the source of the
	// predefined-helper "Unhandled" Lambda errors when OpenStdin
	// was set but the goroutine fired before /attach registered.
	// Only bake stdin into Invoke Payload when:
	//   1. lambdaState.OpenStdin is set
	//   2. an attach pipe registers within the polling window
	//
	// Predefined helpers (gitlab-runner) go through this same path:
	// each stage gets a fresh `lambda.Invoke` with the stage's stdin
	// script as the Payload — analogous to the per-stage Fargate task
	// flow on ECS (Phase 114). Cross-stage state lives on EFS via the
	// shared volume mounts gitlab-runner sets up itself, not on a
	// long-lived Lambda execution.

	// Block synchronously until the function is Active — clients
	// (especially CI runners) typically issue `docker exec` immediately
	// after `docker start` returns, and Invoke against a Pending
	// function fails with ResourceConflictException. The runner-on-
	// Lambda path (`specs/CLOUD_RESOURCE_MAPPING.md` § Lambda exec
	// semantics — Path B) routes each `docker exec` through a fresh
	// `lambda.Invoke`, so the function MUST be Active before /start
	// returns.
	//
	// Lambda has a brief eventual-consistency window after CreateFunction
	// where GetFunction can return 404. Tolerate that for a few seconds
	// before handing off to the V2 waiter, which then enforces
	// State=Active (image pull, ENI attach for VPC).
	visibilityDeadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(visibilityDeadline) {
		_, gerr := s.aws.Lambda.GetFunction(s.ctx(), &awslambda.GetFunctionInput{
			FunctionName: aws.String(lambdaState.FunctionName),
		})
		if gerr == nil {
			break
		}
		if !strings.Contains(gerr.Error(), "ResourceNotFoundException") {
			break
		}
		time.Sleep(2 * time.Second)
	}
	waiter := awslambda.NewFunctionActiveV2Waiter(s.aws.Lambda)
	if werr := waiter.Wait(s.ctx(), &awslambda.GetFunctionInput{
		FunctionName: aws.String(lambdaState.FunctionName),
	}, 5*time.Minute); werr != nil {
		s.Logger.Error().Err(werr).Str("function", lambdaState.FunctionName).Msg("Lambda function did not become Active")
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
		return &api.ServerError{Message: fmt.Sprintf("lambda function %s did not become Active: %v", lambdaState.FunctionName, werr)}
	}

	// Function is Active. The "main Invoke" only fires when there is a
	// concrete payload to deliver (gitlab-runner stdin-piped script).
	// The exec-driven model — `docker create` then per-step `docker
	// exec` — does NOT auto-invoke a stay-alive entrypoint like
	// `tail -f /dev/null` because Lambda has no equivalent primitive
	// (Lambda functions are invoke-on-demand, not "long-running"). The
	// container stays "Running" in CloudState as long as the function
	// exists, regardless of whether any invocation is in flight. Each
	// `docker exec` fires its own concurrent `lambda.Invoke`. See
	// `specs/CLOUD_RESOURCE_MAPPING.md` § Lambda exec semantics.
	// Containers with no stdin pipe (e.g. gitlab-runner's volume-
	// permission helper, which runs `chown -R` baked as Cmd at
	// /create time and never opens an /attach for stdin) still need
	// the function to actually be invoked — without this branch the
	// function is created+Active but never runs anything, and the
	// caller's `docker wait` hangs. Treat it as an empty-payload
	// Invoke so the bootstrap runs the user Cmd directly.
	go func() {
		// Resolve the attach pipe with a polling window. Docker SDK
		// clients (gitlab-runner included) typically issue /create →
		// /start → /attach in that order, so when /start returns the
		// pipe may not be registered yet. Poll for up to 5 s; if no
		// pipe shows up by then assume this isn't an OpenStdin caller
		// after all (or it crashed before attaching) and fall through
		// to an empty-payload Invoke.
		var stdinP *stdinPipe
		if lambdaState.OpenStdin {
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if v, ok := s.stdinPipes.Load(id); ok {
					if pipe := v.(*stdinPipe); pipe.IsOpen() {
						stdinP = pipe
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
			}
			if stdinP == nil {
				s.Logger.Warn().Str("container", id[:12]).Msg("lambda-stdin: OpenStdin set but no attach pipe registered within 5s — invoking with empty payload")
			}
		}

		var invokePayload []byte
		if stdinP != nil {
			// Wait up to 30 s for caller to half-close the hijacked
			// attach connection. gitlab-runner's docker-executor flow
			// streams the script then signals EOF via CloseWrite();
			// log-streaming attaches (no stdin write) hit the timeout
			// and proceed with whatever's buffered. Without the timeout
			// this goroutine waits forever, the WaitCh stays open, and
			// the caller's `docker wait` hangs.
			select {
			case <-stdinP.Done():
			case <-time.After(30 * time.Second):
				s.Logger.Info().Str("container", id[:12]).Int("buffered_bytes", len(stdinP.Bytes())).Msg("lambda-stdin: pipe wait timeout — proceeding with buffered bytes")
			}
			scriptBytes := stdinP.Bytes()
			s.stdinPipes.Delete(id)

			// Lambda's Invoke expects a JSON Payload. gitlab-runner's
			// docker-executor sends a raw bash script via /attach
			// stdin — we wrap it as a Path-B exec envelope
			// (`{"sockerless":{"exec":{"argv":["sh","-c","<script>"]}}}`)
			// so the in-Lambda bootstrap parses it as a docker-exec
			// dispatch, runs the script, and returns
			// `{"sockerlessExecResult":...}`. Without the wrapping
			// Lambda's API rejects the raw `#!/usr/bin/env bash`
			// header as invalid JSON.
			if len(scriptBytes) > 0 {
				envelope := execEnvelopeRequest{}
				envelope.Sockerless.Exec = execEnvelopeExec{
					Argv: []string{"sh", "-c", string(scriptBytes)},
				}
				if p, err := json.Marshal(envelope); err == nil {
					invokePayload = p
				} else {
					s.Logger.Error().Err(err).Msg("lambda-stdin: marshal exec envelope failed")
					if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
						close(ch.(chan struct{}))
					}
					return
				}
			}
		}

		result, err := s.aws.Lambda.Invoke(s.ctx(), &awslambda.InvokeInput{
			FunctionName: aws.String(lambdaState.FunctionName),
			Payload:      invokePayload,
			LogType:      lambdatypes.LogTypeTail,
		})

		inv := core.InvocationResult{FinishedAt: time.Now()}
		switch {
		case err != nil:
			s.Logger.Error().Err(err).Str("function", lambdaState.FunctionName).Msg("Lambda invocation failed")
			inv.ExitCode = 1
			inv.Error = err.Error()
		case result.FunctionError != nil:
			fnErr := aws.ToString(result.FunctionError)
			payloadPreview := string(result.Payload)
			if len(payloadPreview) > 4096 {
				payloadPreview = payloadPreview[:4096] + "...(truncated)"
			}
			// LogResult is base64-encoded last 4KB of the function's stderr.
			// Lambda returns it inline when LogType=Tail is set on Invoke,
			// avoiding a round-trip to CloudWatch when the function dies
			// before the log group propagates.
			var logTail string
			if result.LogResult != nil {
				if decoded, derr := base64.StdEncoding.DecodeString(aws.ToString(result.LogResult)); derr == nil {
					logTail = string(decoded)
				}
			}
			s.Logger.Warn().
				Str("error", fnErr).
				Str("function", lambdaState.FunctionName).
				Str("payload", payloadPreview).
				Str("log_tail", logTail).
				Msg("Lambda function returned error")
			inv.ExitCode = 1
			inv.Error = fnErr
			if len(result.Payload) > 0 {
				s.Store.LogBuffers.Store(id, result.Payload)
			}
		default:
			if len(result.Payload) > 0 && string(result.Payload) != "{}" {
				s.Store.LogBuffers.Store(id, result.Payload)
			}
		}
		s.Store.PutInvocationResult(id, inv)
		s.persistInvocationResultToTags(s.ctx(), lambdaState.FunctionARN, inv)

		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
	}()

	return nil
}

// ContainerStop stops a running Lambda container. AWS Lambda exposes no
// "cancel invoke" API and UpdateFunctionConfiguration only applies to
// future invocations — an in-flight invoke cannot be aborted from the
// control plane. Stop therefore does three things, in order:
// 1. Clamps the function timeout to the minimum (1s) so any subsequent
// invocations of this container are short-lived. Best-effort — a
// failure here is logged but non-fatal (the invocation may already
// be finishing and the function may already be gone).
// 2. Requests the reverse agent (if connected) to exit, which causes
// the agent-as-handler Lambda invocation to return immediately.
// This is the only path that actually cuts short an in-flight
// invocation. Containers without the bundled agent will keep
// running until natural completion or the 15-min AWS hard cap.
// 3. Closes the local wait channel so `docker wait` unblocks.
// Exit code 137 matches Docker's convention for force-stopped containers.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	s.StopHealthCheck(id)

	// cloud-fallback lookup so stop works post-restart.
	if lambdaState, ok := s.resolveLambdaState(s.ctx(), id); ok && lambdaState.FunctionName != "" {
		_, err := s.aws.Lambda.UpdateFunctionConfiguration(s.ctx(),
			&awslambda.UpdateFunctionConfigurationInput{
				FunctionName: aws.String(lambdaState.FunctionName),
				Timeout:      aws.Int32(1),
			},
		)
		if err != nil {
			s.Logger.Debug().Err(err).Str("function", lambdaState.FunctionName).
				Msg("UpdateFunctionConfiguration(Timeout=1) failed during stop")
		}
	}

	s.disconnectReverseAgent(id)

	// Record the stop outcome so CloudState reports the container as
	// exited with code 137 (SIGKILL equivalent) even though Lambda has
	// no invocation-cancel API.
	s.Store.PutInvocationResult(id, core.InvocationResult{ExitCode: 137})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": "137", "name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerKill kills a container with the given signal. Lambda delivers
// no POSIX signals to invocations; termination follows the same path as
// ContainerStop (clamp future timeout, disconnect reverse agent, close
// wait channel). The supplied signal is reflected only in the reported
// exit code via SignalToExitCode.
func (s *Server) ContainerKill(ref string, signal string) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		}
	}

	s.StopHealthCheck(id)

	exitCode := core.SignalToExitCode(signal)

	// cloud-fallback lookup so kill works post-restart.
	lambdaState, _ := s.resolveLambdaState(s.ctx(), id)
	if lambdaState.FunctionName != "" {
		_, err := s.aws.Lambda.UpdateFunctionConfiguration(s.ctx(),
			&awslambda.UpdateFunctionConfigurationInput{
				FunctionName: aws.String(lambdaState.FunctionName),
				Timeout:      aws.Int32(1),
			},
		)
		if err != nil {
			s.Logger.Debug().Err(err).Str("function", lambdaState.FunctionName).
				Msg("UpdateFunctionConfiguration(Timeout=1) failed during kill")
		}
	}

	s.disconnectReverseAgent(id)

	// Record the kill outcome so CloudState reports the container as
	// exited with the signal-derived code.
	s.Store.PutInvocationResult(id, core.InvocationResult{ExitCode: exitCode})

	s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", exitCode), "name": strings.TrimPrefix(c.Name, "/")})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	return nil
}

// ContainerRemove removes a container and its associated Lambda resources.
func (s *Server) ContainerRemove(ref string, force bool) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		// Also check PendingCreates (container created but never started)
		if pc, pcOK := s.PendingCreates.Get(ref); pcOK {
			c = pc
			ok = true
		}
	}
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if c.State.Running && !force {
		return &api.ConflictError{
			Message: fmt.Sprintf("You cannot remove a running container %s. Stop the container before attempting removal or force remove", id[:12]),
		}
	}

	if c.State.Running {
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
	}

	s.StopHealthCheck(id)

	// Delete Lambda function (best-effort)
	lambdaState, _ := s.resolveLambdaState(s.ctx(), id)
	if lambdaState.FunctionName != "" {
		_, _ = s.aws.Lambda.DeleteFunction(s.ctx(), &awslambda.DeleteFunctionInput{
			FunctionName: aws.String(lambdaState.FunctionName),
		})
	}

	if lambdaState.FunctionARN != "" {
		s.Registry.MarkCleanedUp(lambdaState.FunctionARN)
	}

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod {
		s.Store.Pods.RemoveContainer(pod.ID, id)
	}

	// Clean up network associations
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, id)
		}
	}

	s.PendingCreates.Delete(id)
	s.Lambda.Delete(id)
	// Unblock any deferred-stdin Invoke goroutine waiting on the pipe
	// so it can exit cleanly without firing a phantom invocation.
	if v, ok := s.stdinPipes.LoadAndDelete(id); ok {
		_ = v.(*stdinPipe).Close()
	}
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.Store.LogBuffers.Delete(id)
	s.Store.StagingDirs.Delete(id)
	s.Store.DeleteInvocationResult(id)
	if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(id); ok {
		for _, d := range dirs.([]string) {
			os.RemoveAll(d)
		}
	}
	for _, eid := range c.ExecIDs {
		s.Store.Execs.Delete(eid)
	}

	s.EmitEvent("container", "destroy", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerLogs streams container logs from CloudWatch. The log stream
// is resolved lazily on each fetch: Lambda creates the stream when the
// function is invoked for the first time, so if ContainerLogs is called
// before the invocation has produced output the stream lookup would
// return empty. In follow mode the fetch closure keeps checking until
// the stream appears.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	fetch := s.buildCloudWatchFetcher(ref)
	return core.StreamCloudLogs(s.BaseServer, ref, opts, fetch, core.StreamCloudLogsOptions{
		CheckLogBuffers: true,
	})
}

// buildCloudWatchFetcher returns a CloudLogFetchFunc closure that reads
// log events for the given container's Lambda function. Shared by
// ContainerLogs and ContainerAttach. The log group is resolved once;
// the stream name is resolved lazily because Lambda creates the stream
// only after the first invocation produces output.
func (s *Server) buildCloudWatchFetcher(ref string) core.CloudLogFetchFunc {
	var logGroupName *string
	if id, ok := s.ResolveContainerIDAuto(context.Background(), ref); ok {
		lambdaState, _ := s.resolveLambdaState(s.ctx(), id)
		if lambdaState.FunctionName != "" {
			group := fmt.Sprintf("/aws/lambda/%s", lambdaState.FunctionName)
			logGroupName = &group
		}
	}

	resolveStream := func() *string {
		if logGroupName == nil {
			return nil
		}
		out, err := s.aws.CloudWatch.DescribeLogStreams(s.ctx(), &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: logGroupName,
			OrderBy:      "LastEventTime",
			Descending:   aws.Bool(true),
			Limit:        aws.Int32(1),
		})
		if err != nil || len(out.LogStreams) == 0 {
			return nil
		}
		return out.LogStreams[0].LogStreamName
	}

	var cachedStream *string

	return func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		if logGroupName == nil {
			return nil, nil, nil
		}
		if cachedStream == nil {
			cachedStream = resolveStream()
			if cachedStream == nil {
				return nil, cursor, nil
			}
		}

		input := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  logGroupName,
			LogStreamName: cachedStream,
			StartFromHead: aws.Bool(true),
		}

		if cursor != nil {
			input.NextToken = cursor.(*string)
		} else {
			input.StartFromHead = aws.Bool(params.CloudLogTailInt32() == nil)
			if limit := params.CloudLogTailInt32(); limit != nil {
				input.Limit = limit
			}
			if ms := params.SinceMillis(); ms != nil {
				input.StartTime = ms
			}
			if ms := params.UntilMillis(); ms != nil {
				input.EndTime = ms
			}
		}

		result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
		if err != nil {
			return nil, cursor, err
		}

		var entries []core.CloudLogEntry
		for _, event := range result.Events {
			if event.Message == nil {
				continue
			}
			var ts time.Time
			if event.Timestamp != nil {
				ts = time.UnixMilli(*event.Timestamp)
			}
			entries = append(entries, core.CloudLogEntry{Timestamp: ts, Message: *event.Message})
		}
		return entries, result.NextForwardToken, nil
	}
}

// ContainerRestart stops and then starts a container.
func (s *Server) ContainerRestart(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if c.State.Running {
		s.StopHealthCheck(id)

		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}

	// Re-add to PendingCreates so ContainerStart can find it
	c.State.Status = "created"
	c.State.Running = false
	c.State.Pid = 0
	c.State.StartedAt = "0001-01-01T00:00:00Z"
	c.RestartCount++
	s.PendingCreates.Put(id, c)

	// Start the container directly via typed method
	if err := s.ContainerStart(id); err != nil {
		return err
	}

	s.EmitEvent("container", "restart", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerPrune removes all stopped containers.
// In the stateless model, only PendingCreates (never-started) containers are local.
// Lambda functions that have already run are cleaned up via ContainerRemove.
func (s *Server) ContainerPrune(filters map[string][]string) (*api.ContainerPruneResponse, error) {
	labelFilters := filters["label"]
	untilFilters := filters["until"]
	var deleted []string
	var spaceReclaimed uint64

	// Check PendingCreates for containers that were created but never started
	for _, c := range s.PendingCreates.List() {
		// PendingCreates containers are in "created" state — treat as pruneable
		if len(labelFilters) > 0 && !core.MatchLabels(c.Config.Labels, labelFilters) {
			continue
		}
		if len(untilFilters) > 0 && !core.MatchUntil(c.Created, untilFilters) {
			continue
		}
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		// Clean up Lambda cloud resources
		lambdaState, _ := s.resolveLambdaState(s.ctx(), c.ID)
		if lambdaState.FunctionName != "" {
			_, _ = s.aws.Lambda.DeleteFunction(s.ctx(), &awslambda.DeleteFunctionInput{
				FunctionName: aws.String(lambdaState.FunctionName),
			})
		}
		if lambdaState.FunctionARN != "" {
			s.Registry.MarkCleanedUp(lambdaState.FunctionARN)
		}

		s.StopHealthCheck(c.ID)

		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, c.ID)
			}
		}
		if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
			s.Store.Pods.RemoveContainer(pod.ID, c.ID)
		}
		s.PendingCreates.Delete(c.ID)
		s.Lambda.Delete(c.ID)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
			close(ch.(chan struct{}))
		}
		s.Store.LogBuffers.Delete(c.ID)
		s.Store.StagingDirs.Delete(c.ID)
		if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(c.ID); ok {
			for _, d := range dirs.([]string) {
				os.RemoveAll(d)
			}
		}
		for _, eid := range c.ExecIDs {
			s.Store.Execs.Delete(eid)
		}
		s.EmitEvent("container", "destroy", c.ID, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
		deleted = append(deleted, c.ID)
	}
	if deleted == nil {
		deleted = []string{}
	}
	return &api.ContainerPruneResponse{
		ContainersDeleted: deleted,
		SpaceReclaimed:    spaceReclaimed,
	}, nil
}

// ContainerPause sends SIGSTOP to the user subprocess via the reverse-
// agent.
func (s *Server) ContainerPause(ref string) error {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return core.MapPauseErr(core.RunContainerPauseViaAgent(s.reverseAgents, cid))
}

// ContainerUnpause sends SIGCONT to the user subprocess via the
// reverse-agent.
func (s *Server) ContainerUnpause(ref string) error {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return core.MapPauseErr(core.RunContainerUnpauseViaAgent(s.reverseAgents, cid))
}

// ImagePull pulls an image, using ECR cloud auth when available.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
}

// ImageTag tags an image and syncs the new tag to ECR.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	return s.images.Tag(source, repo, tag)
}

// ImageRemove removes an image and syncs the removal to ECR.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	return s.images.Remove(name, force, prune)
}

// ImageLoad loads an image from a tar archive.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return s.images.Load(r)
}

// ImageBuild delegates to the shared ImageManager.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	return s.images.Build(opts, buildContext)
}

// AuthLogin validates login credentials.
// For ECR registries, logs a warning that credentials should be obtained via
// `aws ecr get-login-password`. For all other registries, delegates to BaseServer.
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	if strings.HasSuffix(req.ServerAddress, ".amazonaws.com") &&
		strings.Contains(req.ServerAddress, ".dkr.ecr.") {
		// ECR registry — store credentials via BaseServer but warn that
		// ECR auth tokens should be obtained via `aws ecr get-login-password`.
		s.Logger.Warn().
			Str("registry", req.ServerAddress).
			Msg("ECR login: credentials stored locally; use `aws ecr get-login-password` for production")
		return s.BaseServer.AuthLogin(req)
	}
	return s.BaseServer.AuthLogin(req)
}

// Info returns system information with Lambda-appropriate values.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}
	info.KernelVersion = "5.10.0-aws-lambda"
	info.OperatingSystem = "AWS Lambda"
	return info, nil
}

// ContainerAttach bridges stdin/stdout/stderr to the bootstrap process
// inside the Lambda invocation container via the reverse-agent
// WebSocket when a session is registered. When no agent is registered
// and the caller doesn't need stdin (read-only attach — `docker
// attach` without -i, or `docker run` follow-mode), fall back to
// streaming CloudWatch as the attached output. Interactive attach
// without an agent has no native Lambda surface, so it stays
// NotImplementedError.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); hasAgent {
		return s.BaseServer.ContainerAttach(id, opts)
	}
	if opts.Stdin {
		return nil, &api.NotImplementedError{Message: "interactive docker attach requires a reverse-agent bootstrap inside the Lambda container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	return core.AttachViaCloudLogs(s.BaseServer, id, opts, s.buildCloudWatchFetcher(id))
}

// ContainerExport streams a tar archive of the Lambda container's
// rootfs via the reverse-agent. Buffered in memory; see
// core.RunContainerExportViaAgent for the size caveat.
func (s *Server) ContainerExport(id string) (io.ReadCloser, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	rc, err := core.RunContainerExportViaAgent(s.reverseAgents, cid)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker export requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("export via reverse-agent: %v", err)}
	}
	return rc, nil
}

// ContainerCommit builds a new image from the invocation container's
// rootfs via the reverse-agent and stores it in the image cache so
// `docker push` can sync it to ECR. Gated behind EnableCommit because
// the result wraps the whole rootfs as a single layer (no diff against
// the original image — sockerless can't read it from the Lambda
// backend host).
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if req.Container == "" {
		return nil, &api.InvalidParameterError{Message: "container query parameter is required"}
	}
	if !s.config.EnableCommit {
		return nil, &api.NotImplementedError{Message: "docker commit on Lambda is gated — set SOCKERLESS_ENABLE_COMMIT=1 (the agent-driven commit captures the whole rootfs as a single layer)"}
	}
	return core.CommitContainerRequestViaAgent(s.BaseServer, s.reverseAgents, req)
}

// ImagePush pushes an image, syncing to ECR when applicable.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	return s.images.Push(name, tag, auth)
}
