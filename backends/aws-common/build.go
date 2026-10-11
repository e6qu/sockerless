package awscommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Compile-time check.
var _ core.CloudBuildService = (*CodeBuildService)(nil)

// CodeBuildService builds Docker images using AWS CodeBuild.
type CodeBuildService struct {
	codebuild *codebuild.Client
	s3        *s3.Client
	logs      *cloudwatchlogs.Client // reads a failed build's log for its error
	project   string                 // CodeBuild project name
	bucket    string                 // S3 bucket for context upload
	ecrRepo   string                 // ECR repo prefix for output images
	region    string
	ecrAuth   *ECRAuthProvider // for AssembleMultiArchManifest token mint
	logger    zerolog.Logger
}

// NewCodeBuildService creates a CodeBuild-backed build service.
// Returns nil if project or bucket are empty (not configured).
func NewCodeBuildService(cb *codebuild.Client, s3c *s3.Client, logs *cloudwatchlogs.Client, project, bucket, ecrRepo, region string, ecrAuth *ECRAuthProvider, logger zerolog.Logger) *CodeBuildService {
	if project == "" || bucket == "" {
		return nil
	}
	return &CodeBuildService{
		codebuild: cb,
		s3:        s3c,
		logs:      logs,
		project:   project,
		bucket:    bucket,
		ecrRepo:   ecrRepo,
		region:    region,
		ecrAuth:   ecrAuth,
		logger:    logger,
	}
}

func (s *CodeBuildService) Available() bool {
	return s.project != "" && s.bucket != ""
}

// AssembleMultiArchManifest delegates to the universal helper, with
// the ECR bearer token (`ECRAuthProvider.GetToken` returns
// `Basic <b64>`; the OCI distribution v2 endpoints accept either
// `Basic` or `Bearer` for this token, but the universal helper sets
// `Authorization: Bearer <token>` — pass the raw base64 portion so
// ECR's manifest endpoint authenticates correctly).
func (s *CodeBuildService) AssembleMultiArchManifest(ctx context.Context, opts core.MultiArchManifestOptions) error {
	if s.ecrAuth == nil {
		return fmt.Errorf("AssembleMultiArchManifest: ECRAuthProvider not configured")
	}
	return core.AssembleMultiArchManifest(ctx, opts, func(registryAndRepo string) (string, error) {
		registry, repo, _ := strings.Cut(registryAndRepo, "/")
		raw, err := s.ecrAuth.GetToken(registry, repo, core.ActionPull, core.ActionPush)
		if err != nil {
			return "", err
		}
		// ECR returns "Basic <b64>"; strip the prefix so the
		// universal helper's "Authorization: Bearer <token>" wraps
		// the base64 portion only. ECR accepts both Basic and Bearer
		// against the same base64 credential.
		return strings.TrimPrefix(raw, "Basic "), nil
	})
}

func (s *CodeBuildService) Build(ctx context.Context, opts core.CloudBuildOptions) (*core.CloudBuildResult, error) {
	start := time.Now()

	// Upload context tar to S3
	var contextBuf bytes.Buffer
	if _, err := io.Copy(&contextBuf, opts.ContextTar); err != nil {
		return nil, fmt.Errorf("read build context: %w", err)
	}

	objectKey := fmt.Sprintf("build-context/%d.tar.gz", time.Now().UnixNano())
	_, err := s.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader(contextBuf.Bytes()),
	})
	if err != nil {
		return nil, fmt.Errorf("upload context to S3: %w", err)
	}

	// Determine target image reference. A fully-qualified ref in
	// `Tags[0]` (anything containing a slash, i.e. `<host>/<repo>:tag`)
	// is used as-is — callers needing a different ECR repo than the
	// service-default `s.ecrRepo` (e.g. Lambda's overlay-inject path
	// pushing to `sockerless-live-lambda-overlay`) pass the full ref.
	// Otherwise the legacy `s.ecrRepo:tag` shape applies.
	var imageRef string
	if len(opts.Tags) > 0 && strings.Contains(opts.Tags[0], "/") {
		imageRef = opts.Tags[0]
	} else {
		tag := "latest"
		if len(opts.Tags) > 0 {
			tag = opts.Tags[0]
			if idx := strings.LastIndex(tag, ":"); idx >= 0 {
				tag = tag[idx+1:]
			}
		}
		imageRef = fmt.Sprintf("%s:%s", s.ecrRepo, tag)
	}

	// Build the docker build command
	dockerCmd := fmt.Sprintf("docker build -f %s", opts.Dockerfile)
	if opts.Dockerfile == "" {
		dockerCmd = "docker build -f Dockerfile"
	}
	for k, v := range opts.BuildArgs {
		dockerCmd += fmt.Sprintf(" --build-arg %s=%s", k, v)
	}
	for k, v := range opts.Labels {
		dockerCmd += fmt.Sprintf(" --label %s=%s", k, v)
	}
	if opts.Target != "" {
		dockerCmd += " --target " + opts.Target
	}
	if opts.NoCache {
		dockerCmd += " --no-cache"
	}
	if opts.Platform != "" {
		dockerCmd += " --platform " + opts.Platform
	}
	for _, cf := range opts.CacheFrom {
		dockerCmd += " --cache-from " + cf
	}
	dockerCmd += fmt.Sprintf(" -t %s .", imageRef)

	// Build environment variables (includes secrets)
	envVars := []cbtypes.EnvironmentVariable{
		{Name: aws.String("IMAGE_REF"), Value: aws.String(imageRef), Type: cbtypes.EnvironmentVariableTypePlaintext},
	}
	for k, v := range opts.Secrets {
		if strings.HasPrefix(v, "aws:secretsmanager:") {
			envVars = append(envVars, cbtypes.EnvironmentVariable{
				Name:  aws.String(k),
				Value: aws.String(strings.TrimPrefix(v, "aws:secretsmanager:")),
				Type:  cbtypes.EnvironmentVariableTypeSecretsManager,
			})
		} else if strings.HasPrefix(v, "aws:ssm:") {
			envVars = append(envVars, cbtypes.EnvironmentVariable{
				Name:  aws.String(k),
				Value: aws.String(strings.TrimPrefix(v, "aws:ssm:")),
				Type:  cbtypes.EnvironmentVariableTypeParameterStore,
			})
		} else {
			envVars = append(envVars, cbtypes.EnvironmentVariable{
				Name:  aws.String(k),
				Value: aws.String(v),
				Type:  cbtypes.EnvironmentVariableTypePlaintext,
			})
		}
	}

	// Buildspec. Login URL is derived from `imageRef` (the resolved
	// destination) rather than `s.ecrRepo` so callers passing a fully-
	// qualified Tags[0] log in to the right registry.
	//
	// The build runs the project as provisioned — a NO_SOURCE project
	// whose buildspec this override replaces — and fetches its own
	// context: the tarball is copied out of the build bucket and
	// extracted in pre_build, so the Dockerfile + binaries land where
	// `docker build` expects them. A source override is not used: an
	// Amazon S3 source is a ZIP archive CodeBuild unpacks itself, and the
	// context is a gzipped tarball.
	loginRegistry := strings.Split(imageRef, "/")[0]
	buildspec := fmt.Sprintf(`version: 0.2
phases:
  pre_build:
    commands:
      - aws s3 cp "s3://%s/%s" /tmp/context.tar.gz
      - mkdir -p /tmp/build-context
      - tar -xzf /tmp/context.tar.gz -C /tmp/build-context
      - aws ecr get-login-password --region %s | docker login --username AWS --password-stdin %s
  build:
    commands:
      - cd /tmp/build-context && %s
  post_build:
    commands:
      - docker push %s
`, s.bucket, objectKey, s.region, loginRegistry, dockerCmd, imageRef)

	// Start build
	buildResult, err := s.codebuild.StartBuild(ctx, &codebuild.StartBuildInput{
		ProjectName:                  aws.String(s.project),
		BuildspecOverride:            aws.String(buildspec),
		EnvironmentVariablesOverride: envVars,
	})
	if err != nil {
		return nil, fmt.Errorf("start CodeBuild: %w", err)
	}

	buildID := aws.ToString(buildResult.Build.Id)
	s.logger.Info().Str("buildID", buildID).Str("image", imageRef).Msg("CodeBuild started")

	// Poll for completion
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}

		status, err := s.codebuild.BatchGetBuilds(ctx, &codebuild.BatchGetBuildsInput{
			Ids: []string{buildID},
		})
		if err != nil {
			return nil, fmt.Errorf("poll CodeBuild: %w", err)
		}
		if len(status.Builds) == 0 {
			continue
		}

		build := status.Builds[0]
		switch build.BuildStatus {
		case cbtypes.StatusTypeSucceeded:
			s.logger.Info().Str("buildID", buildID).Dur("duration", time.Since(start)).Msg("CodeBuild succeeded")
			var logStream string
			if build.Logs != nil {
				logStream = aws.ToString(build.Logs.DeepLink)
			}
			return &core.CloudBuildResult{
				ImageRef:  imageRef,
				ImageID:   "", // Fetched by caller via FetchImageMetadata
				Duration:  time.Since(start),
				LogStream: logStream,
			}, nil
		case cbtypes.StatusTypeFailed, cbtypes.StatusTypeFault, cbtypes.StatusTypeStopped, cbtypes.StatusTypeTimedOut:
			reason := string(build.BuildStatus)
			if build.Phases != nil {
				for _, p := range build.Phases {
					if p.PhaseStatus == cbtypes.StatusTypeFailed && len(p.Contexts) > 0 {
						reason = aws.ToString(p.Contexts[0].Message)
						break
					}
				}
			}
			return nil, fmt.Errorf("CodeBuild %s: %s%s", build.BuildStatus, reason, s.buildLogTail(ctx, build))
		}
		// Still in progress
	}
}

// buildLogLines is how much of a failed build's log its error carries: the
// end of the log, where the failing command and its output are.
const buildLogLines = 40

// buildLogTail returns the last lines of a build's Amazon CloudWatch Logs
// stream, formatted to follow the build's error, so a failed build reports
// what failed and not only that a command exited non-zero. A build with no
// log location, or a log that cannot be read, says so in place of the lines.
func (s *CodeBuildService) buildLogTail(ctx context.Context, build cbtypes.Build) string {
	if build.Logs == nil || aws.ToString(build.Logs.GroupName) == "" || aws.ToString(build.Logs.StreamName) == "" {
		return "\n(the build reports no log location)"
	}
	group, stream := aws.ToString(build.Logs.GroupName), aws.ToString(build.Logs.StreamName)
	out, err := s.logs.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(group),
		LogStreamName: aws.String(stream),
		StartFromHead: aws.Bool(false),
		Limit:         aws.Int32(buildLogLines),
	})
	if err != nil {
		return fmt.Sprintf("\n(read build log %s/%s: %v)", group, stream, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nbuild log %s/%s (last %d lines):", group, stream, buildLogLines)
	for _, e := range out.Events {
		b.WriteString("\n  ")
		b.WriteString(strings.TrimRight(aws.ToString(e.Message), "\n"))
	}
	return b.String()
}
