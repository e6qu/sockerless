package core

import (
	"io"

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

// ImageManager handles all 12 image methods, delegating base logic to BaseServer
// and adding cloud-specific auth and sync via an AuthProvider.
//
// Cloud backends create an ImageManager with their cloud's AuthProvider and delegate
// all image methods to it. This ensures per-cloud unified behavior regardless of
// which backend (container vs FaaS) is used.
type ImageManager struct {
	Base   *BaseServer    // base implementation for in-memory operations
	Auth   AuthProvider   // cloud-specific auth and sync (nil = no cloud integration)
	Logger zerolog.Logger
}

// Pull pulls an image, using cloud auth if available.
func (m *ImageManager) Pull(ref string, auth string) (io.ReadCloser, error) {
	if m.Auth != nil {
		registry, _, _ := ParseImageRef(ref)
		if auth == "" && m.Auth.IsCloudRegistry(registry) {
			if token, err := m.Auth.GetToken(registry); err == nil {
				auth = token
			} else {
				m.Logger.Warn().Err(err).Str("registry", registry).Msg("cloud auth failed for pull, proceeding without")
			}
		}
	}

	// Fetch real metadata from registry (anonymous auth for public images).
	// Do not pass Docker client auth (X-Registry-Auth) — it's
	// base64-encoded JSON that breaks Docker Hub's token endpoint.
	var meta *ImageMetadataResult
	if realMeta, err := FetchImageMetadata(ref); err == nil && realMeta != nil {
		meta = realMeta
	}

	// Delegate to BaseServer for the actual pull (creates in-memory image + progress stream).
	// Pass metadata so ImagePull can use real data instead of synthetics.
	result, err := m.Base.ImagePullWithMetadata(ref, auth, meta)
	if err != nil {
		return nil, err
	}

	// Sync to cloud registry (non-fatal)
	if m.Auth != nil {
		registry, repo, tag := ParseImageRef(ref)
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

	// Sync to cloud registry (non-fatal)
	if m.Auth != nil {
		registry, repo, _ := ParseImageRef(name)
		if m.Auth.IsCloudRegistry(registry) {
			if err := m.Auth.OnPush(img.ID, registry, repo, tag); err != nil {
				m.Logger.Warn().Err(err).Str("name", name).Str("tag", tag).Msg("cloud push failed")
			}
		}
	}

	// Return progress stream via BaseServer
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
		registry, repoPath, newTag := ParseImageRef(ref)
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
				registry, repo, imgTag := ParseImageRef(repoTag)
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

	// Sync removal to cloud (non-fatal)
	for _, ref := range cloudRefs {
		if err := m.Auth.OnRemove(ref.registry, ref.repo, ref.tags); err != nil {
			m.Logger.Warn().Err(err).Str("repo", ref.repo).Msg("cloud remove sync failed")
		}
	}

	return result, nil
}

// Build delegates to BaseServer's synthetic Dockerfile parser.
func (m *ImageManager) Build(opts api.ImageBuildOptions, ctx io.Reader) (io.ReadCloser, error) {
	return m.Base.ImageBuild(opts, ctx)
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

// ParseImageRef splits an image reference into registry, repository, and tag.
// Exported for use by AuthProvider implementations and backends.
func ParseImageRef(ref string) (registry, repo, tag string) {
	rc := parseImageRef(ref)
	return rc.Registry, rc.Repository, rc.Tag
}
