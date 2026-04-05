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
func NewGCPBuildService(ctx context.Context, project, bucket, arRepo string, logger zerolog.Logger) (*GCPBuildService, error) {
	if project == "" || bucket == "" {
		return nil, nil
	}

	cb, err := cloudbuild.NewClient(ctx)
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

	// Add secret environment variables
	var secretEnvs []string
	for k := range opts.Secrets {
		secretEnvs = append(secretEnvs, k)
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
		Steps:  steps,
		Images: []string{imageRef},
	}

	_ = secretEnvs // TODO: wire secretEnvs into build steps when Secret Manager integration is ready

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
