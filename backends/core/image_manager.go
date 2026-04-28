package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// AuthProvider provides cloud-specific registry authentication and lifecycle operations.
// Implemented per-cloud (AWS, GCP, Azure) in shared cloud modules.
type AuthProvider interface {
	// GetToken returns an auth token for the given registry.
	// The token includes the scheme prefix (e.g. "Basic <token>" or "Bearer <token>").
	// Returns ("", nil) if no cloud auth is available (fall through to anonymous/Www-Authenticate).
	GetToken(registry string) (string, error)

	// IsCloudRegistry returns true if the registry belongs to this cloud provider.
	IsCloudRegistry(registry string) bool

	// OnPush is called after a successful push to sync to the cloud registry.
	// Errors are non-fatal — implementations log warnings and return nil or an error
	// that the caller will log and discard.
	OnPush(imageID, registry, repo, tag string) error

	// OnTag is called after a successful tag to sync to the cloud registry.
	// Errors are non-fatal.
	OnTag(imageID, registry, repo, newTag string) error

	// OnRemove is called after a successful remove to sync to the cloud registry.
	// Errors are non-fatal. Implementations handle graceful degradation (e.g. 405 for DELETE).
	OnRemove(registry, repo string, tags []string) error
}

// CloudBuildService builds Docker images on cloud infrastructure.
// Implemented per-cloud in shared modules (aws-common, gcp-common, azure-common).
type CloudBuildService interface {
	// Build uploads context, executes the Dockerfile on cloud infra, and pushes the result.
	Build(ctx context.Context, opts CloudBuildOptions) (*CloudBuildResult, error)

	// Available returns true if the build service is configured and ready.
	Available() bool
}

// CloudBuildOptions configures a cloud build.
type CloudBuildOptions struct {
	Dockerfile string            // Path within context (default "Dockerfile")
	ContextTar io.Reader         // Build context tar stream
	Tags       []string          // Target image tags
	BuildArgs  map[string]string // --build-arg values
	Target     string            // Multi-stage --target
	NoCache    bool              // --no-cache
	Platform   string            // --platform (e.g. "linux/amd64")
	Labels     map[string]string // --label values
	CacheFrom  []string          // --cache-from refs
	CacheTo    []string          // --cache-to refs
	Secrets    map[string]string // --secret id=key (inline values or cloud secret ARNs)
}

// CloudBuildResult is returned after a successful cloud build.
type CloudBuildResult struct {
	ImageRef  string        // Full registry/repo:tag
	ImageID   string        // sha256:... config digest
	Duration  time.Duration // Build wall time
	LogStream string        // URL or ARN for build logs
}

// ecrBasicCredential extracts the base64-encoded credential portion
// from an ECRAuthProvider.GetToken response (`"Basic <b64>"`) so it
// can be passed to `FetchImageMetadata` as the basic-auth parameter
// (which expects raw base64 — `Basic ` is added by `getRegistryToken`
// downstream when it hits a token-exchange endpoint, or by
// `setRegistryAuth` for ECR's basic-auth-direct path). Empty input =
// empty output (anonymous).
func ecrBasicCredential(token string) string {
	if token == "" {
		return ""
	}
	if rest, ok := stripPrefix(token, "Basic "); ok {
		return rest
	}
	return token
}

// stripPrefix is a small utility to avoid pulling in a strings import
// solely for this helper.
func stripPrefix(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return s, false
}

// ImageManager handles all 12 image methods, delegating base logic to BaseServer
// and adding cloud-specific auth and sync via an AuthProvider.
type ImageManager struct {
	Base         *BaseServer       // base implementation for in-memory operations
	Auth         AuthProvider      // cloud-specific auth and sync (nil = no cloud integration)
	BuildService CloudBuildService // cloud build delegation (nil = local Dockerfile parse only)
	Logger       zerolog.Logger
}

// Pull pulls an image, using cloud auth if available.
func (m *ImageManager) Pull(ref string, auth string) (io.ReadCloser, error) {
	cloudAuthToken := ""
	if m.Auth != nil {
		registry, _, _ := splitImageRefRegistry(ref)
		if m.Auth.IsCloudRegistry(registry) {
			if token, err := m.Auth.GetToken(registry); err == nil {
				cloudAuthToken = token
				if auth == "" {
					auth = token
				}
			} else {
				m.Logger.Warn().Err(err).Str("registry", registry).Msg("cloud auth failed for pull, proceeding without")
			}
		}
	}

	// Fetch real metadata from registry. For ECR (and other Basic-auth
	// registries) we pass the pre-computed cloud auth token so the
	// manifest endpoint authenticates correctly — without it the
	// generic Bearer-flow code in `getRegistryToken` parses ECR's
	// `Basic realm=` challenge and either fails outright or POSTs the
	// token-exchange against the registry itself, neither of which is
	// the right shape. Docker client auth (X-Registry-Auth, base64-
	// encoded JSON) is still NOT passed for non-cloud registries — it
	// breaks Docker Hub's token endpoint. Registry failures propagate
	// as errors so the pull fails cleanly rather than producing a
	// synthetic image record.
	meta, err := FetchImageMetadata(ref, ecrBasicCredential(cloudAuthToken))
	if err != nil {
		return nil, err
	}

	result, err := m.Base.ImagePullWithMetadata(ref, auth, meta)
	if err != nil {
		return nil, err
	}

	// Sync to cloud registry (non-fatal)
	if m.Auth != nil {
		registry, repo, tag := splitImageRefRegistry(ref)
		if m.Auth.IsCloudRegistry(registry) {
			if img, ok := m.Base.Store.ResolveImage(ref); ok {
				if err := m.Auth.OnPush(img.ID, registry, repo, tag); err != nil {
					m.Logger.Warn().Err(err).Str("ref", ref).Msg("cloud sync after pull failed")
				}
			}
		}
	}

	return result, nil
}

// mergeImageConfig merges fetched config fields into the stored config.
func mergeImageConfig(dst *api.ContainerConfig, src *api.ContainerConfig) {
	if len(src.Env) > 0 {
		dst.Env = src.Env
	}
	if len(src.Cmd) > 0 {
		dst.Cmd = src.Cmd
	}
	if len(src.Entrypoint) > 0 {
		dst.Entrypoint = src.Entrypoint
	}
	if src.WorkingDir != "" {
		dst.WorkingDir = src.WorkingDir
	}
	if src.User != "" {
		dst.User = src.User
	}
	if len(src.Labels) > 0 {
		dst.Labels = src.Labels
	}
	if len(src.Volumes) > 0 {
		dst.Volumes = src.Volumes
	}
	if len(src.ExposedPorts) > 0 {
		dst.ExposedPorts = src.ExposedPorts
	}
}

// Push pushes an image, syncing to the cloud registry if applicable.
func (m *ImageManager) Push(name string, tag string, auth string) (io.ReadCloser, error) {
	img, ok := m.Base.Store.ResolveImage(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "image", ID: name}
	}

	if tag == "" {
		tag = "latest"
	}

	// Cloud-specific bookkeeping (e.g., ECR CreateRepository) before
	// the actual blob upload. OnPush is no-op for AR/ACR, lightweight
	// for ECR. Real OCI push happens below via BaseServer.ImagePush
	// which has access to the layer data through the local store.
	if m.Auth != nil {
		registry, repo, _ := splitImageRefRegistry(name)
		if m.Auth.IsCloudRegistry(registry) {
			if err := m.Auth.OnPush(img.ID, registry, repo, tag); err != nil {
				m.Logger.Warn().Err(err).Str("name", name).Str("tag", tag).Msg("cloud push pre-upload step failed")
				pr, pw := io.Pipe()
				go func() {
					enc := json.NewEncoder(pw)
					_ = enc.Encode(map[string]string{"status": "The push refers to repository [" + name + "]"})
					_ = enc.Encode(map[string]any{"errorDetail": map[string]string{"message": err.Error()}, "error": err.Error()})
					_ = pw.Close()
				}()
				return pr, nil
			}
			// Always use sockerless-managed credentials for our own
			// cloud registries — the user's Docker CLI may send an
			// `X-Registry-Auth` JSON that is either absent, stale, or
			// encoded as a Docker-style username/password wrapper that
			// OCIPush.SetOCIAuth can't interpret as a Basic token.
			// Fetching a fresh token here is authoritative.
			if token, err := m.Auth.GetToken(registry); err == nil {
				auth = token
			}
		}
	}

	return m.Base.ImagePush(name, tag, auth)
}

// Tag tags an image and syncs the new tag to the cloud registry.
func (m *ImageManager) Tag(source string, repo string, tag string) error {
	if err := m.Base.ImageTag(source, repo, tag); err != nil {
		return err
	}

	// Sync to cloud registry (non-fatal)
	if m.Auth != nil {
		ref := repo
		if tag != "" {
			ref = repo + ":" + tag
		}
		registry, repoPath, newTag := splitImageRefRegistry(ref)
		if m.Auth.IsCloudRegistry(registry) {
			if img, ok := m.Base.Store.ResolveImage(ref); ok {
				if err := m.Auth.OnTag(img.ID, registry, repoPath, newTag); err != nil {
					m.Logger.Warn().Err(err).Str("repo", repo).Str("tag", tag).Msg("cloud tag sync failed")
				}
			}
		}
	}

	return nil
}

// Remove removes an image and syncs the removal to the cloud registry.
func (m *ImageManager) Remove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	// Collect cloud refs before removal
	type cloudRef struct {
		registry string
		repo     string
		tags     []string
	}
	var cloudRefs []cloudRef

	if m.Auth != nil {
		if img, ok := m.Base.Store.ResolveImage(name); ok {
			refs := make(map[string]*cloudRef)
			for _, repoTag := range img.RepoTags {
				registry, repo, imgTag := splitImageRefRegistry(repoTag)
				if m.Auth.IsCloudRegistry(registry) {
					key := registry + "/" + repo
					if _, exists := refs[key]; !exists {
						refs[key] = &cloudRef{registry: registry, repo: repo}
					}
					refs[key].tags = append(refs[key].tags, imgTag)
				}
			}
			for _, ref := range refs {
				cloudRefs = append(cloudRefs, *ref)
			}
		}
	}

	result, err := m.Base.ImageRemove(name, force, prune)
	if err != nil {
		return nil, err
	}

	// Sync removal to cloud. Aggregate the cloud-side errors and
	// surface them — the local removal already succeeded, so we
	// return result *plus* the cloud error so callers see both
	// (the previous silent-warning behaviour left the operator
	// believing the image was gone from the cloud registry when it
	// might still be present).
	var cloudErrs []string
	for _, ref := range cloudRefs {
		if err := m.Auth.OnRemove(ref.registry, ref.repo, ref.tags); err != nil {
			cloudErrs = append(cloudErrs, fmt.Sprintf("%s: %v", ref.repo, err))
		}
	}
	if len(cloudErrs) > 0 {
		// We already removed locally; report the cloud-side failures
		// as a server error so the operator can rerun rmi to retry
		// or check the cloud-side state.
		return result, &api.ServerError{Message: "local image removed but cloud-registry sync failed: " + strings.Join(cloudErrs, "; ")}
	}

	return result, nil
}

// Build delegates to the configured cloud build service. When no
// cloud build service is configured (`m.BuildService == nil` or its
// `Available()` returns false) and the docker backend is not in use,
// `docker build` returns NotImplementedError — we don't silently fall
// back to a local Dockerfile parser that drops `RUN` steps as a no-op.
// The local-Dockerfile path remains for the docker backend only,
// gated by `SOCKERLESS_LOCAL_DOCKERFILE_BUILD=1` for cases where
// the operator deliberately wants the no-RUN parse-only mode (e.g.
// CI smoke tests of metadata-only images).
func (m *ImageManager) Build(opts api.ImageBuildOptions, ctxReader io.Reader) (io.ReadCloser, error) {
	if m.BuildService != nil && m.BuildService.Available() {
		// Buffer context so we can pass to cloud build service
		var contextBuf bytes.Buffer
		if _, err := io.Copy(&contextBuf, ctxReader); err != nil {
			return nil, err
		}

		// Convert build args
		buildArgs := make(map[string]string)
		for k, v := range opts.BuildArgs {
			if v != nil {
				buildArgs[k] = *v
			}
		}

		cloudOpts := CloudBuildOptions{
			Dockerfile: opts.Dockerfile,
			ContextTar: &contextBuf,
			Tags:       opts.Tags,
			BuildArgs:  buildArgs,
			Target:     opts.Target,
			NoCache:    opts.NoCache,
			Platform:   opts.Platform,
			Labels:     opts.Labels,
			CacheFrom:  opts.CacheFrom,
			CacheTo:    opts.CacheTo,
			Secrets:    opts.Secrets,
		}

		result, err := m.BuildService.Build(context.Background(), cloudOpts)
		if err != nil {
			m.Logger.Error().Err(err).Msg("cloud build failed")
			// Return error in stream format
			pr, pw := io.Pipe()
			go func() {
				enc := json.NewEncoder(pw)
				_ = enc.Encode(map[string]any{"errorDetail": map[string]string{"message": err.Error()}, "error": err.Error()})
				_ = pw.Close()
			}()
			return pr, nil
		}

		// Fetch the built image metadata from the cloud registry
		if result.ImageRef != "" {
			if meta, fetchErr := FetchImageMetadata(result.ImageRef); fetchErr == nil && meta != nil {
				_, _ = m.Base.ImagePullWithMetadata(result.ImageRef, "", meta)
			}
		}

		// Return success stream
		pr, pw := io.Pipe()
		go func() {
			enc := json.NewEncoder(pw)
			for _, tag := range opts.Tags {
				_ = enc.Encode(map[string]string{"stream": "Successfully tagged " + tag + "\n"})
			}
			if result.ImageID != "" {
				_ = enc.Encode(map[string]string{"stream": "Successfully built " + result.ImageID[:12] + "\n"})
			}
			_ = enc.Encode(map[string]string{"stream": "Cloud build completed in " + result.Duration.Round(time.Second).String() + "\n"})
			_ = pw.Close()
		}()
		return pr, nil
	}

	// No cloud build service configured. We don't silently route to
	// the local Dockerfile parser (which drops RUN steps as no-ops,
	// producing a "successful" build that doesn't match the user's
	// Dockerfile). Local parsing is enabled only when the operator
	// opts in via SOCKERLESS_LOCAL_DOCKERFILE_BUILD=1 — used by
	// docker-backend smoke tests where RUN isn't required.
	if os.Getenv("SOCKERLESS_LOCAL_DOCKERFILE_BUILD") == "1" {
		return m.Base.ImageBuild(opts, ctxReader)
	}
	return nil, &api.NotImplementedError{Message: "docker build requires a cloud build service (CodeBuild / Cloud Build / ACR Tasks); none is configured. Set SOCKERLESS_LOCAL_DOCKERFILE_BUILD=1 to opt in to the local parse-only path (no RUN execution; metadata-only)"}
}

// Inspect delegates to BaseServer.
func (m *ImageManager) Inspect(name string) (*api.Image, error) {
	return m.Base.ImageInspect(name)
}

// Load delegates to BaseServer.
func (m *ImageManager) Load(r io.Reader) (io.ReadCloser, error) {
	return m.Base.ImageLoad(r)
}

// List delegates to BaseServer.
func (m *ImageManager) List(opts api.ImageListOptions) ([]*api.ImageSummary, error) {
	return m.Base.ImageList(opts)
}

// History delegates to BaseServer.
func (m *ImageManager) History(name string) ([]*api.ImageHistoryEntry, error) {
	return m.Base.ImageHistory(name)
}

// Prune delegates to BaseServer.
func (m *ImageManager) Prune(filters map[string][]string) (*api.ImagePruneResponse, error) {
	return m.Base.ImagePrune(filters)
}

// Save delegates to BaseServer.
func (m *ImageManager) Save(names []string) (io.ReadCloser, error) {
	return m.Base.ImageSave(names)
}

// Search delegates to BaseServer.
func (m *ImageManager) Search(term string, limit int, filters map[string][]string) ([]*api.ImageSearchResult, error) {
	return m.Base.ImageSearch(term, limit, filters)
}

// splitImageRefRegistry splits a reference into the (registry, repo,
// tag) tuple used by `core.AuthProvider` and the image-manager
// internals — registry-scope identification rather than
// general-purpose parsing. The canonical parser is `ParseImageRef`
// returning a typed `ImageRef`; this helper unwraps the registryConfig
// computed by `parseImageRef` for callers that want the
// docker-hub-default rewrite (`docker.io` → `registry-1.docker.io`,
// bare names → `library/<name>`).
func splitImageRefRegistry(ref string) (registry, repo, tag string) {
	rc := parseImageRef(ref)
	return rc.Registry, rc.Repository, rc.Tag
}
