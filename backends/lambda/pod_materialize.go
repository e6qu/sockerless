package lambda

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/sockerless/api"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// materializePodFunction collapses a multi-container pod into a single
// Lambda Function backed by a merged-rootfs overlay image. Called from
// ContainerStart on the LAST pod member's start (when PodDeferredStart
// returns shouldDefer=false and len(podContainers) > 1).
//
// Mirror of `backends/cloudrun-functions/pod_materialize.go::materializePodFunction`,
// adapted for Lambda's invocation model: Lambda functions stay warm
// across consecutive invokes, so the bootstrap pre-warms sidecars at
// init (in the cold-start path) rather than per-invoke.
//
// Per-member ContainerCreate already created throwaway single-container
// Functions; this materialisation deletes them atomically before
// creating the merged pod Function so the pod Function is the only
// cloud-side resource that maps back to any of the member containers.
//
// All members share the InvocationResult: the pod is one Function; the
// main member's stdout becomes the /response payload; `docker wait
// <any-member>` returns when the pod's Invoke completes.
func (s *Server) materializePodFunction(mainContainerID string, containers []api.Container, exitCh chan struct{}) error {
	ctx := s.ctx()

	pod, _ := s.Store.Pods.GetPodForContainer(mainContainerID)
	podName := ""
	if pod != nil {
		podName = pod.Name
	}

	podSpec := containersToPodOverlaySpec(s.config.AgentBinaryPath, s.config.BootstrapBinaryPath, podName, mainContainerID, containers)
	contentTag := PodOverlayContentTag(podSpec)

	// 1. Build + push the pod overlay image. Routes through CodeBuild
	//    when available (in-Lambda sockerless), local docker otherwise.
	repo, repoErr := s.overlayECRRepo()
	if repoErr != nil {
		return &api.ServerError{Message: repoErr.Error()}
	}
	destRef := repo + ":" + contentTag
	var builder core.CloudBuildService
	if s.images != nil {
		builder = s.images.BuildService
	}
	overlay, buildErr := buildAndPushPodOverlay(ctx, podSpec, destRef, builder)
	if buildErr != nil {
		return &api.ServerError{Message: fmt.Sprintf("pod overlay build failed for %q: %v", podName, buildErr)}
	}

	// 2. Atomically delete the per-member Functions created by ContainerCreate.
	//    Best-effort: delete failures log + proceed (orphan cleanup falls to
	//    the operator's sweep), so the pod-Function create is never blocked.
	for _, c := range containers {
		state, ok := s.resolveLambdaState(ctx, c.ID)
		if !ok || state.FunctionName == "" {
			continue
		}
		_, derr := s.aws.Lambda.DeleteFunction(ctx, &awslambda.DeleteFunctionInput{
			FunctionName: aws.String(state.FunctionName),
		})
		if derr != nil {
			s.Logger.Warn().Err(derr).Str("function", state.FunctionName).Msg("pre-pod cleanup: per-member Lambda delete failed")
			continue
		}
		s.Lambda.Delete(c.ID)
		s.Registry.MarkCleanedUp(state.FunctionARN)
	}

	// 3. Create the merged pod Lambda function. Tag with sockerless-pod=<name>
	//    so cloud_state can map back; sockerless-allocation holds the main
	//    container's short ID for pool semantics.
	arch := lambdatypes.ArchitectureX8664
	if strings.EqualFold(s.config.Architecture, "arm64") {
		arch = lambdatypes.ArchitectureArm64
	}
	funcName := fmt.Sprintf("skls-pod-%s-%s", contentTag, mainContainerID[:6])

	tags := core.TagSet{
		ContainerID: mainContainerID,
		Backend:     "lambda",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
		AutoRemove:  false,
	}
	awsTags := tags.AsMap()
	awsTags["sockerless-overlay-hash"] = contentTag
	awsTags["sockerless-allocation"] = shortAllocLabelLambda(mainContainerID)
	if podName != "" {
		awsTags["sockerless-pod"] = sanitizePodTagValue(podName)
	}

	envVars := map[string]string{
		"SOCKERLESS_CONTAINER_ID": mainContainerID,
		"SOCKERLESS_POD_NAME":     podName,
	}
	if manifest, err := EncodePodManifest(podSpec.Members); err == nil {
		envVars["SOCKERLESS_POD_CONTAINERS"] = manifest
	}
	if podSpec.MainName != "" {
		envVars["SOCKERLESS_POD_MAIN"] = podSpec.MainName
	}

	createInput := &awslambda.CreateFunctionInput{
		FunctionName:  aws.String(funcName),
		Role:          aws.String(s.config.RoleARN),
		PackageType:   lambdatypes.PackageTypeImage,
		Architectures: []lambdatypes.Architecture{arch},
		Code: &lambdatypes.FunctionCode{
			ImageUri: aws.String(overlay.ImageURI),
		},
		MemorySize:  aws.Int32(int32(s.config.MemorySize)),
		Timeout:     aws.Int32(int32(s.config.Timeout)),
		Tags:        awsTags,
		Environment: &lambdatypes.Environment{Variables: envVars},
	}
	if len(s.config.SubnetIDs) > 0 {
		createInput.VpcConfig = &lambdatypes.VpcConfig{
			SubnetIds:        s.config.SubnetIDs,
			SecurityGroupIds: s.config.SecurityGroupIDs,
		}
	}

	result, err := s.aws.Lambda.CreateFunction(ctx, createInput)
	if err != nil {
		return awscommon.MapAWSError(err, "function", funcName)
	}
	functionARN := aws.ToString(result.FunctionArn)

	// All pod members get the same LambdaState reference so subsequent
	// ContainerStart / ContainerWait / ContainerLogs against any
	// member resolves to the merged pod function.
	for _, c := range containers {
		s.Lambda.Put(c.ID, LambdaState{
			FunctionName: funcName,
			FunctionARN:  functionARN,
		})
	}
	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainContainerID,
		Backend:      "lambda",
		ResourceType: "function",
		ResourceID:   functionARN,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata: map[string]string{
			"functionName": funcName,
			"podName":      podName,
			"podMembers":   fmt.Sprintf("%d", len(containers)),
			"overlayHash":  contentTag,
			"role":         "pod-supervisor",
		},
	})
	for _, c := range containers {
		s.EmitEvent("container", "start", c.ID, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
			"pod":  podName,
		})
	}

	// 4. Wait for Active before invoking. Lambda image-mode pull +
	//    extraction can take ~30-60s on first deploy.
	waiter := awslambda.NewFunctionActiveV2Waiter(s.aws.Lambda)
	if werr := waiter.Wait(ctx, &awslambda.GetFunctionInput{
		FunctionName: aws.String(funcName),
	}, 5*time.Minute); werr != nil {
		s.Logger.Error().Err(werr).Str("function", funcName).Msg("pod Lambda did not become Active")
		for _, c := range containers {
			if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
				close(ch.(chan struct{}))
			}
		}
		return &api.ServerError{Message: fmt.Sprintf("pod lambda %s did not become Active: %v", funcName, werr)}
	}

	// 5. Invoke the pod Function. Result fan-out: every pod member
	//    receives the same exit code + stdout buffer.
	go s.invokePodFunction(ctx, funcName, functionARN, containers, exitCh)
	return nil
}

// invokePodFunction performs the lambda.Invoke against the pod Function
// and fans the InvocationResult out to every member container. Each
// member's WaitCh is closed so `docker wait <any-member>` unblocks.
func (s *Server) invokePodFunction(ctx context.Context, funcName, functionARN string, containers []api.Container, mainExitCh chan struct{}) {
	result, err := s.aws.Lambda.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName: aws.String(funcName),
		Payload:      []byte("{}"),
		LogType:      lambdatypes.LogTypeTail,
	})

	inv := core.InvocationResult{FinishedAt: time.Now()}
	switch {
	case err != nil:
		s.Logger.Error().Err(err).Str("function", funcName).Msg("pod Lambda invocation failed")
		inv.ExitCode = 1
		inv.Error = err.Error()
	case result.FunctionError != nil:
		fnErr := aws.ToString(result.FunctionError)
		var logTail string
		if result.LogResult != nil {
			if decoded, derr := base64.StdEncoding.DecodeString(aws.ToString(result.LogResult)); derr == nil {
				logTail = string(decoded)
			}
		}
		s.Logger.Warn().Str("error", fnErr).Str("function", funcName).Str("log_tail", logTail).Msg("pod Lambda returned error")
		inv.ExitCode = 1
		inv.Error = fnErr
		if len(result.Payload) > 0 {
			for _, c := range containers {
				s.Store.LogBuffers.Store(c.ID, result.Payload)
			}
		}
	default:
		if len(result.Payload) > 0 && string(result.Payload) != "{}" {
			for _, c := range containers {
				s.Store.LogBuffers.Store(c.ID, result.Payload)
			}
		}
	}
	for _, c := range containers {
		s.Store.PutInvocationResult(c.ID, inv)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
			close(ch.(chan struct{}))
		}
	}
	// mainExitCh is also drained by the LoadAndDelete loop above.
	_ = mainExitCh
	// Persist the result to function tags so post-restart cloud_state
	// queries can recover the exit code without an in-memory cache.
	s.persistInvocationResultToTags(ctx, functionARN, inv)
}

// buildAndPushPodOverlay routes the pod overlay build through
// `core.CloudBuildService` (CodeBuild path — preferred when sockerless
// runs in-Lambda and has no docker daemon) or local docker. Mirrors
// `BuildAndPushOverlayImage` but for the pod variant.
func buildAndPushPodOverlay(ctx context.Context, spec PodOverlaySpec, destRef string, builder core.CloudBuildService) (*OverlayBuildResult, error) {
	if destRef == "" {
		return nil, fmt.Errorf("buildAndPushPodOverlay: destRef is required")
	}
	if builder != nil && builder.Available() {
		contextTar, err := TarPodOverlayContext(spec)
		if err != nil {
			return nil, fmt.Errorf("tar pod overlay context: %w", err)
		}
		result, err := builder.Build(ctx, core.CloudBuildOptions{
			Dockerfile: "Dockerfile",
			ContextTar: newReadSeeker(contextTar),
			Tags:       []string{destRef},
			Platform:   "linux/amd64",
		})
		if err != nil {
			return nil, fmt.Errorf("cloud build pod overlay %s: %w", destRef, err)
		}
		return &OverlayBuildResult{ImageURI: result.ImageRef}, nil
	}
	// Local-docker fallback: render the Dockerfile + stage agent +
	// bootstrap binaries in a temp build context and shell out.
	// Implementation deliberately reuses the single-container helper
	// shape in BuildAndPushOverlayImage so behaviour is consistent;
	// the pod variant just renders a different Dockerfile.
	return buildPodOverlayViaLocalDocker(ctx, spec, destRef)
}

// containersToPodOverlaySpec converts the live container set into the
// build-time PodOverlaySpec the renderer consumes. Member order is
// preserved; the main is identified by mainID.
func containersToPodOverlaySpec(agentPath, bootstrapPath, podName, mainID string, containers []api.Container) PodOverlaySpec {
	members := make([]PodMemberSpec, 0, len(containers))
	mainName := ""
	for _, c := range containers {
		name := strings.TrimPrefix(c.Name, "/")
		if name == "" {
			name = c.ID[:12]
		}
		name = sanitizePodMemberName(name)
		members = append(members, PodMemberSpec{
			Name:         name,
			ContainerID:  c.ID,
			BaseImageRef: c.Config.Image,
			Entrypoint:   c.Config.Entrypoint,
			Cmd:          c.Config.Cmd,
			Workdir:      c.Config.WorkingDir,
			Env:          c.Config.Env,
		})
		if c.ID == mainID {
			mainName = name
		}
	}
	return PodOverlaySpec{
		PodName:             podName,
		MainName:            mainName,
		AgentBinaryPath:     agentPath,
		BootstrapBinaryPath: bootstrapPath,
		Members:             members,
	}
}

// sanitizePodMemberName lowercases and strips characters outside
// [a-z0-9-]. Safe for chroot subdir + AWS tag fragment.
func sanitizePodMemberName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-':
			b.WriteRune(r)
		case r == '_', r == '.':
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		out = "x"
	}
	return out
}

// sanitizePodTagValue applies the AWS tag-value charset (loose: letters,
// digits, plus -_./=+:@ and space) but we restrict to [a-z0-9_-] for
// cross-cloud consistency with the gcf path. Empty result leads to the
// caller dropping the tag.
func sanitizePodTagValue(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// newReadSeeker wraps a byte slice as a *bytes.Reader for the
// CloudBuildOptions.ContextTar field, which expects an io.ReadSeeker.
func newReadSeeker(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}

// buildPodOverlayViaLocalDocker is the local-docker fallback for the
// pod overlay build. Mirrors `buildOverlayViaLocalDocker` (single-
// container variant) but renders the pod Dockerfile and stages both
// agent + bootstrap binaries into the build context.
func buildPodOverlayViaLocalDocker(ctx context.Context, spec PodOverlaySpec, destRef string) (*OverlayBuildResult, error) {
	dockerfile, err := RenderPodOverlayDockerfile(spec)
	if err != nil {
		return nil, fmt.Errorf("render pod Dockerfile: %w", err)
	}
	buildDir, err := os.MkdirTemp("", "sockerless-pod-overlay-")
	if err != nil {
		return nil, fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(buildDir)

	if err := os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return nil, fmt.Errorf("write Dockerfile: %w", err)
	}
	if err := copyFile(spec.AgentBinaryPath, filepath.Join(buildDir, spec.AgentBinaryPath)); err != nil {
		return nil, fmt.Errorf("stage agent binary: %w", err)
	}
	if err := copyFile(spec.BootstrapBinaryPath, filepath.Join(buildDir, spec.BootstrapBinaryPath)); err != nil {
		return nil, fmt.Errorf("stage bootstrap binary: %w", err)
	}

	buildCmd := exec.CommandContext(ctx, "docker", "build", "-t", destRef, buildDir)
	buildCmd.Stdout = os.Stderr
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker build %s: %w", destRef, err)
	}
	pushCmd := exec.CommandContext(ctx, "docker", "push", destRef)
	pushCmd.Stdout = os.Stderr
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker push %s: %w", destRef, err)
	}
	return &OverlayBuildResult{ImageURI: destRef}, nil
}
