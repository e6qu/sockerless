package gcpcommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"cloud.google.com/go/storage"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
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
	// tokenSource is the GCE-metadata token source used when the service
	// points at a non-default endpoint (a simulator or a private-service
	// proxy) that enforces a bearer. Nil in real-cloud mode, where the AR
	// manifest push and Cloud Build calls use Application Default
	// Credentials directly.
	tokenSource oauth2.TokenSource
	logger      zerolog.Logger
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
// gRPC service exposure.
//
// Auth is a GCE-metadata bearer, obtained the same way a workload does
// on real GCE. When endpointURL is set, that endpoint's data plane
// enforces the bearer, so both the Cloud Build client and the Storage
// client present a real, endpoint-signed token: `GCE_METADATA_HOST` is
// pointed at the endpoint host (Google's own coordinate for a
// non-default metadata server; the simulator serves `/computeMetadata/*`
// on the same port) and `google.ComputeTokenSource` mints the token.
// When endpointURL is empty (real cloud), no token source is wired and
// the clients use Application Default Credentials directly.
//
// Storage uses the standard `cloud.google.com/go/storage` client. Its
// native `STORAGE_EMULATOR_HOST` env var routes storage requests at a
// non-default host and, in that mode, hard-forces WithoutAuthentication;
// to still present a bearer against an enforcing endpoint the Storage
// client is handed an *http.Client whose transport injects the same
// metadata token.
func NewGCPBuildService(ctx context.Context, project, bucket, arRepo, endpointURL string, logger zerolog.Logger) (*GCPBuildService, error) {
	if project == "" || bucket == "" {
		return nil, nil
	}

	var cbOpts []option.ClientOption
	var tokenSource oauth2.TokenSource
	var storageOpts []option.ClientOption
	if endpointURL != "" {
		if host, err := urlHost(endpointURL); err == nil {
			_ = os.Setenv("GCE_METADATA_HOST", host)
			_ = os.Setenv("STORAGE_EMULATOR_HOST", host)
		}
		tokenSource = google.ComputeTokenSource("")
		cbOpts = append(cbOpts,
			option.WithEndpoint(endpointURL),
			option.WithTokenSource(tokenSource),
		)
		storageOpts = append(storageOpts, option.WithHTTPClient(oauth2.NewClient(ctx, tokenSource)))
	}

	cb, err := cloudbuild.NewRESTClient(ctx, cbOpts...)
	if err != nil {
		return nil, fmt.Errorf("create Cloud Build client: %w", err)
	}

	gcs, err := storage.NewClient(ctx, storageOpts...)
	if err != nil {
		cb.Close()
		return nil, fmt.Errorf("create GCS client: %w", err)
	}

	return &GCPBuildService{
		cloudbuild:  cb,
		gcs:         gcs,
		tokenSource: tokenSource,
		project:     project,
		bucket:      bucket,
		arRepo:      arRepo,
		logger:      logger,
	}, nil
}

// urlHost returns "host:port" from a URL, or an error if malformed. Used
// to point GCE_METADATA_HOST / STORAGE_EMULATOR_HOST at a non-default
// endpoint host.
func urlHost(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

func (s *GCPBuildService) Available() bool {
	return s.project != "" && s.bucket != ""
}

// AssembleMultiArchManifest delegates to the universal helper, with the
// Artifact Registry bearer token for the auth callback. AR honours the
// standard OCI distribution v2 PUT /v2/<repo>/manifests/<tag> request
// with a Docker manifest-list / OCI image-index media type.
//
// The bearer comes from the same source as the other clients: the
// GCE-metadata token source when pointed at an enforcing endpoint
// (simulator / proxy), or Application Default Credentials in real-cloud
// mode. Selecting the source by the endpoint coordinate — not by a
// runtime probe — keeps the AR push token consistent with the token the
// Cloud Build and Storage clients present.
func (s *GCPBuildService) AssembleMultiArchManifest(ctx context.Context, opts core.MultiArchManifestOptions) error {
	return core.AssembleMultiArchManifest(ctx, opts, func(_ string) (string, error) {
		ts := s.tokenSource
		if ts == nil {
			creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
			if err != nil {
				return "", fmt.Errorf("ADC: %w", err)
			}
			ts = creds.TokenSource
		}
		tok, err := ts.Token()
		if err != nil {
			return "", fmt.Errorf("AR bearer token: %w", err)
		}
		return tok.AccessToken, nil
	})
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
		_ = writer.Close()
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
