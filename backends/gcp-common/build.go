package gcpcommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"cloud.google.com/go/storage"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/option"
)

// Compile-time check.
var _ core.CloudBuildService = (*GCPBuildService)(nil)

// GCPBuildService builds Docker images using Google Cloud Build.
type GCPBuildService struct {
	cloudbuild *cloudbuild.Client
	gcs        *storage.Client
	project    string
	bucket     string // GCS bucket for context upload
	arRepo     string // Artifact Registry repo prefix
	logger     zerolog.Logger
}

// NewGCPBuildService creates a Cloud Build-backed build service.
// Returns nil if project or bucket are empty.
//
// endpointURL is a single configuration knob that routes SDK requests:
// empty → Google's default discovery endpoint; non-empty → the
// supplied URL. The build service does not know or care what's at
// the other end of the URL — could be a regional endpoint, a
// private-service-connect address, a custom proxy, or anything that
// speaks the Cloud Build REST API.
//
// Always uses the REST variant of the Cloud Build client because REST
// works against any HTTPS endpoint with the same wire format, while
// `cloudbuild.NewClient` (gRPC) requires `googleapis.com`-shaped HTTP/2
// gRPC service exposure. Auth is always real ADC; targets that don't
// validate ignore it.
//
// Storage uses the standard `cloud.google.com/go/storage` client. The
// SDK's native `STORAGE_EMULATOR_HOST` env var routes storage requests
// at a non-default host when set — operators set that env var on the
// backend process if they need to (the env-var name is Google's, not
// a comment on what's at the other end). The build service makes no
// env-var side effects of its own.
func NewGCPBuildService(ctx context.Context, project, bucket, arRepo, endpointURL string, logger zerolog.Logger) (*GCPBuildService, error) {
	if project == "" || bucket == "" {
		return nil, nil
	}

	var cbOpts []option.ClientOption
	if endpointURL != "" {
		cbOpts = append(cbOpts, option.WithEndpoint(endpointURL))
	}

	cb, err := cloudbuild.NewRESTClient(ctx, cbOpts...)
	if err != nil {
		return nil, fmt.Errorf("create Cloud Build client: %w", err)
	}

	gcs, err := storage.NewClient(ctx)
	if err != nil {
		cb.Close()
		return nil, fmt.Errorf("create GCS client: %w", err)
	}

	return &GCPBuildService{
		cloudbuild: cb,
		gcs:        gcs,
		project:    project,
		bucket:     bucket,
		arRepo:     arRepo,
		logger:     logger,
	}, nil
}

func (s *GCPBuildService) Available() bool {
	return s.project != "" && s.bucket != ""
}

func (s *GCPBuildService) Build(ctx context.Context, opts core.CloudBuildOptions) (*core.CloudBuildResult, error) {
	start := time.Now()

	// Upload context to GCS
	var contextBuf bytes.Buffer
	if _, err := io.Copy(&contextBuf, opts.ContextTar); err != nil {
		return nil, fmt.Errorf("read build context: %w", err)
	}

	objectName := fmt.Sprintf("build-context/%d.tar.gz", time.Now().UnixNano())
	writer := s.gcs.Bucket(s.bucket).Object(objectName).NewWriter(ctx)
	if _, err := writer.Write(contextBuf.Bytes()); err != nil {
		return nil, fmt.Errorf("upload context to GCS: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close GCS writer: %w", err)
	}

	// Determine target image reference
	tag := "latest"
	if len(opts.Tags) > 0 {
		tag = opts.Tags[0]
	}
	imageRef := tag
	if !strings.Contains(imageRef, "/") && s.arRepo != "" {
		imageRef = fmt.Sprintf("%s/%s", s.arRepo, tag)
	}

	// Build docker build args
	dockerArgs := []string{"build", "-f", opts.Dockerfile}
	if opts.Dockerfile == "" {
		dockerArgs = []string{"build", "-f", "Dockerfile"}
	}
	for k, v := range opts.BuildArgs {
		dockerArgs = append(dockerArgs, "--build-arg", k+"="+v)
	}
	for k, v := range opts.Labels {
		dockerArgs = append(dockerArgs, "--label", k+"="+v)
	}
	if opts.Target != "" {
		dockerArgs = append(dockerArgs, "--target", opts.Target)
	}
	if opts.NoCache {
		dockerArgs = append(dockerArgs, "--no-cache")
	}
	if opts.Platform != "" {
		dockerArgs = append(dockerArgs, "--platform", opts.Platform)
	}
	for _, cf := range opts.CacheFrom {
		dockerArgs = append(dockerArgs, "--cache-from", cf)
	}
	dockerArgs = append(dockerArgs, "-t", imageRef, ".")

	// Build steps
	steps := []*cloudbuildpb.BuildStep{
		{
			Name: "gcr.io/cloud-builders/docker",
			Args: dockerArgs,
		},
		{
			Name: "gcr.io/cloud-builders/docker",
			Args: []string{"push", imageRef},
		},
	}

	// Wire secret env vars through to Cloud Build via
	// availableSecrets.secretManager + per-step secretEnv. `opts.Secrets`
	// maps env-var-name → Secret Manager resource reference
	// (`projects/P/secrets/S/versions/V`). Each entry becomes an
	// AvailableSecrets.SecretManager binding, and each step gets the
	// env name listed in its SecretEnv so Cloud Build's runtime exposes
	// the resolved payload to the step process.
	var availableSecrets *cloudbuildpb.Secrets
	if len(opts.Secrets) > 0 {
		secretEnvs := make([]string, 0, len(opts.Secrets))
		sm := make([]*cloudbuildpb.SecretManagerSecret, 0, len(opts.Secrets))
		for envName, versionRef := range opts.Secrets {
			secretEnvs = append(secretEnvs, envName)
			sm = append(sm, &cloudbuildpb.SecretManagerSecret{
				VersionName: versionRef,
				Env:         envName,
			})
		}
		availableSecrets = &cloudbuildpb.Secrets{SecretManager: sm}
		for _, step := range steps {
			step.SecretEnv = append(step.SecretEnv, secretEnvs...)
		}
	}

	build := &cloudbuildpb.Build{
		Source: &cloudbuildpb.Source{
			Source: &cloudbuildpb.Source_StorageSource{
				StorageSource: &cloudbuildpb.StorageSource{
					Bucket: s.bucket,
					Object: objectName,
				},
			},
		},
		Steps:            steps,
		Images:           []string{imageRef},
		AvailableSecrets: availableSecrets,
	}

	// Submit build
	op, err := s.cloudbuild.CreateBuild(ctx, &cloudbuildpb.CreateBuildRequest{
		ProjectId: s.project,
		Build:     build,
	})
	if err != nil {
		return nil, fmt.Errorf("create Cloud Build: %w", err)
	}

	s.logger.Info().Str("project", s.project).Str("image", imageRef).Msg("Cloud Build started")

	// Wait for completion
	result, err := op.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("Cloud Build failed: %w", err)
	}

	if result.Status != cloudbuildpb.Build_SUCCESS {
		return nil, fmt.Errorf("Cloud Build %s: %s", result.Status, result.StatusDetail)
	}

	s.logger.Info().Str("image", imageRef).Dur("duration", time.Since(start)).Msg("Cloud Build succeeded")

	return &core.CloudBuildResult{
		ImageRef:  imageRef,
		ImageID:   "",
		Duration:  time.Since(start),
		LogStream: result.LogUrl,
	}, nil
}
