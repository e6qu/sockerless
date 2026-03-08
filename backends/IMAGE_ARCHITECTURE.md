# Unified Image Management Architecture

## Overview

Image management is unified via `core.ImageManager` + `core.AuthProvider` interface. Three shared modules (`aws-common`, `gcp-common`, `azure-common`) each implement `AuthProvider`, shared by both backends in that cloud. ~2000 lines of prior duplication eliminated. For current backend structure, see [backends/README.md](README.md).

---

## Target Architecture

```
                        +----------------------------+
                        |     backends/core/          |
                        |                            |
                        |  registry.go   (OCI pull)  |
                        |  oci_push.go   (OCI push)  |
                        |  image_manager.go (NEW)    |
                        |                            |
                        |  AuthProvider interface    |
                        |  ImageManager struct       |
                        +-------------+--------------+
                                      |
              +-----------------------+----------------------+
              |                       |                      |
    +---------v--------+  +-----------v------+  +------------v-----+
    |  AWS auth impl   |  |  GCP auth impl   |  |  Azure auth      |
    |  (in ecs/)       |  |  (in cloudrun/)  |  |  (in aca/)       |
    |                  |  |                  |  |                  |
    | ECRAuthProvider  |  | ARAuthProvider   |  | ACRAuthProvider  |
    | + ECR SDK ops    |  |                  |  |                  |
    +--------+---------+  +--------+---------+  +--------+---------+
             |                     |                      |
       +-----+----+         +-----+----+           +-----+----+
       |ECS|Lambda|         |CR | GCF  |           |ACA| AZF  |
       +----------+         +----------+           +----------+
```

### Core: `image_manager.go` (NEW)

```go
// AuthProvider provides cloud-specific registry authentication.
type AuthProvider interface {
    // GetToken returns an auth token for the given registry.
    // Returns ("", nil) if no cloud auth is available (fall through to Docker Hub/anonymous).
    GetToken(registry string) (string, error)

    // IsCloudRegistry returns true if the registry belongs to this cloud provider.
    IsCloudRegistry(registry string) bool

    // OnPush is called after a successful in-memory push to sync to the cloud registry.
    // Non-fatal -- errors are logged, not returned.
    OnPush(img api.Image, registry, repo, tag string) error

    // OnTag is called after a successful in-memory tag to sync to the cloud registry.
    // Non-fatal.
    OnTag(img api.Image, registry, repo, newTag string) error

    // OnRemove is called after a successful in-memory remove to sync to the cloud registry.
    // Non-fatal. Implementations must handle graceful degradation (e.g., registry
    // returns 405 for DELETE) by returning nil or a non-fatal error.
    OnRemove(registry, repo string, tags []string) error
}

// ImageManager handles all 12 image methods using core OCI client + cloud AuthProvider.
// AuthLogin is NOT part of ImageManager -- backends handle it independently.
type ImageManager struct {
    Auth      AuthProvider    // nil = no cloud integration (pure in-memory)
    Store     *Store
    Logger    zerolog.Logger
    BuildFunc func(opts api.ImageBuildOptions, ctx io.Reader) (io.ReadCloser, error)
    // BuildFunc is set to BaseServer.imageBuild by default (synthetic Dockerfile parser).
    // Backends that want NotImplementedError for Build override this.
}

// All 12 api.Backend image methods as methods on ImageManager:
func (m *ImageManager) Pull(ref, auth string) (io.ReadCloser, error)
func (m *ImageManager) Inspect(name string) (*api.Image, error)
func (m *ImageManager) Load(r io.Reader) (io.ReadCloser, error)
func (m *ImageManager) Tag(source, repo, tag string) error
func (m *ImageManager) List(opts api.ImageListOptions) ([]*api.ImageSummary, error)
func (m *ImageManager) Remove(name string, force, prune bool) ([]*api.ImageDeleteResponse, error)
func (m *ImageManager) History(name string) ([]*api.ImageHistoryEntry, error)
func (m *ImageManager) Prune(filters map[string][]string) (*api.ImagePruneResponse, error)
func (m *ImageManager) Build(opts api.ImageBuildOptions, ctx io.Reader) (io.ReadCloser, error)
func (m *ImageManager) Push(name, tag, auth string) (io.ReadCloser, error)
func (m *ImageManager) Save(names []string) (io.ReadCloser, error)
func (m *ImageManager) Search(term string, limit int, filters map[string][]string) ([]*api.ImageSearchResult, error)
```

### Critical Design Notes (from verification)

1. **Auth token semantics**: `GetToken()` must return the **full Authorization header value** including scheme prefix.
   - For ECR: returns `"Basic <base64>"` (base64 of `user:password`)
   - For AR/ACR: returns `"Bearer <access_token>"`
   - `ImageManager.Pull()` passes the token to `FetchImageConfigWithAuth()` which sets it as the Authorization header verbatim.
   - Returns `("", nil)` for non-cloud registries, causing `ImageManager` to fall through to core's `Www-Authenticate` token exchange.

2. **`FetchImageConfig` refactor**: Current `core.FetchImageConfig(ref, basicAuth)` prepends
   `"Basic "` internally. `ImageManager.Pull()` must NOT use this function directly for cloud
   registries. Instead, add `FetchImageConfigWithAuth(ref, authHeader string)` that bypasses
   `getRegistryToken()` entirely and passes the Authorization header verbatim to `registryGet()`.

3. **`BuildFunc` for ImageBuild**: `ImageManager` cannot import BaseServer's Dockerfile parser
   (circular dep risk). Instead, `BuildFunc` is injected at creation time. BaseServer sets it
   to its synthetic build logic. FaaS backends set it to return `NotImplementedError`.

4. **Separate Go modules**: Each backend is a separate `go.mod`. FaaS backends in the same
   cloud (Lambda, GCF, AZF) CANNOT import from their container counterpart (ECS, CloudRun, ACA).
   Each must duplicate the ~80-line AuthProvider implementation.

5. **No simulator changes required**: All `OnRemove` implementations must be resilient to
   missing APIs. AWS uses ECR SDK (`BatchDeleteImage`) which is already supported by the
   simulator. GCP and Azure use OCI manifest DELETE -- if the simulator returns 405 Method
   Not Allowed, `OnRemove` returns nil (non-fatal). The `ImageManager` also treats all
   `OnRemove` errors as non-fatal (logged, never returned to caller). This double layer
   of protection ensures `ImageRemove` always succeeds locally.

### Per-Cloud AuthProvider

Each cloud implements `AuthProvider` in ~80 lines:

**AWS** (`backends/ecs/image_auth.go` + `backends/lambda/image_auth.go`):
- `IsCloudRegistry`: matches `*.dkr.ecr.*.amazonaws.com`
- `GetToken`: calls `ecr.GetAuthorizationToken`, returns `"Basic " + raw_base64_token`
- `OnPush`: calls `ecr.CreateRepository` + `ecr.PutImage` (ECR SDK, not OCI)
- `OnTag`: calls `ecr.PutImage` with new tag
- `OnRemove`: calls `ecr.BatchDeleteImage` (already supported by simulator)

**GCP** (`backends/cloudrun/image_auth.go` + `backends/cloudrun-functions/image_auth.go`):
- `IsCloudRegistry`: matches `*.gcr.io`, `*-docker.pkg.dev`
- `GetToken`: calls `google.FindDefaultCredentials`, returns `"Bearer " + access_token`
- `OnPush`: calls `core.OCIPush`
- `OnTag`: re-PUTs manifest with new tag via `core.OCIPush`
- `OnRemove`: DELETEs manifest via OCI HTTP -- handles 405 gracefully (returns nil)

**Azure** (`backends/aca/image_auth.go` + `backends/azure-functions/image_auth.go`):
- `IsCloudRegistry`: matches `*.azurecr.io`
- `GetToken`: calls `azidentity.DefaultAzureCredential.GetToken`, returns `"Bearer " + token`
- `OnPush`: calls `core.OCIPush`
- `OnTag`: re-PUTs manifest with new tag via `core.OCIPush`
- `OnRemove`: DELETEs manifest via OCI HTTP -- handles 405 gracefully (returns nil)

### Backend Wiring

Each backend creates an `ImageManager` and delegates:

```go
// In ECS server.go:
func NewServer(...) *Server {
    s := &Server{...}
    s.images = &core.ImageManager{
        Auth:   &ECRAuthProvider{ecr: s.aws.ECR, logger: s.Logger},
        Store:  s.Store,
        Logger: s.Logger,
    }
    return s
}

// In ECS backend_impl.go:
func (s *Server) ImagePull(ref, auth string) (io.ReadCloser, error) {
    return s.images.Pull(ref, auth)
}
// ... same for all 12 methods
```

Lambda has its own `ECRAuthProvider` (separate Go module, ~80 lines).
GCF has its own `ARAuthProvider` (separate Go module, ~80 lines).
AZF has its own `ACRAuthProvider` (separate Go module, ~80 lines).

### FaaS Overrides

FaaS backends (Lambda, GCF, AZF) need some methods to return `NotImplementedError`:
- `ImageBuild` -- no build capability

This is handled by the FaaS backend keeping its own `ImageBuild` method that returns
`NotImplementedError` directly, while delegating the other 11 methods to `s.images`.

---

## What Gets Deleted

| File | Lines | Reason |
|------|-------|--------|
| `backends/ecs/registry.go` | 196 | Replaced by core ImageManager + ECRAuthProvider |
| `backends/cloudrun/registry.go` | 185 | Replaced by core ImageManager + ARAuthProvider |
| `backends/aca/registry.go` | 184 | Replaced by core ImageManager + ACRAuthProvider |
| `backends/cloudrun-functions/registry.go` | 50 | Uses GCF's own ARAuthProvider copy |
| `backends/aca/backend_impl_images.go` | 359 | OCI push moved to core, auth to ACRAuthProvider |
| Duplicate `parseImageRef()` | 4 copies | Use core's `parseImageRef()` |
| Duplicate `getDockerHubToken()` | 3 copies | Handled inside core's ImageManager |
| Scattered image methods in `backend_impl.go` | ~400 lines total | Replaced by `s.images.Method()` one-liners |

**Estimated deletion**: ~1,200+ lines of duplicated code.
**Estimated addition**: ~500 lines (ImageManager + 6 AuthProviders, one per backend module).

---

## Implementation Status — ALL COMPLETE

All steps implemented in PR #100. Auth providers consolidated into 3 shared modules
(`backends/aws-common/`, `backends/gcp-common/`, `backends/azure-common/`), each used by
both backends in that cloud. All 6 cloud backends delegate image methods to `s.images.*`.

## Design Constraints (preserved for reference)

- **E2E tests**: `TestImageBuild` expects HTTP 200 — `ImageManager.Build()` delegates to BaseServer's synthetic Dockerfile parser.
- **Shared cloud modules**: AuthProviders live in `*-common` modules, shared by both backends in each cloud.
- **core has no cloud SDK deps**: `AuthProvider` is an interface — cloud SDKs only imported in `*-common` and backend packages.
- **Non-fatal cloud ops**: All `OnPush`/`OnTag`/`OnRemove` failures are logged, never returned as errors.
- **Backward compat**: `BaseServer.ImagePull()` etc. still work for the Docker backend and tests that create `BaseServer` directly.
- **No simulator changes**: `OnRemove` implementations handle missing DELETE support (405) gracefully.
