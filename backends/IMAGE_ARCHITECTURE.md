# Unified Image Management Architecture

## Problem Statement

Image management code is heavily duplicated across cloud backends:

### Current Duplication

1. **`registry.go` copied 3 times** (ECS ~196, CloudRun ~185, ACA ~184 lines):
   - `fetchImageConfig()` -- identical logic, only auth differs
   - `parseImageRef()` -- identical in all 3 + GCF (4 copies total)
   - `getDockerHubToken()` -- identical in all 3

2. **Core's `registry.go` (443 lines) is superior but ignored**:
   - Handles manifest lists (multi-arch images)
   - Parses `Www-Authenticate` headers for token exchange
   - Has in-memory caching
   - Only used by Lambda and GCF via `core.FetchImageConfig()`

3. **OCI push scattered**:
   - Core has `oci_push.go` (239 lines) -- only GCP backends use it
   - ACA has its own OCI push in `backend_impl_images.go` (~150 lines)
   - ECS uses ECR SDK (PutImage) instead of OCI Distribution API

4. **Image method implementations duplicated per-backend**:
   - Each backend has its own ImagePull, ImagePush, ImageTag, ImageRemove
   - FaaS backends (Lambda, GCF, AZF) have near-identical patterns
   - Container backends (ECS, CloudRun, ACA) have near-identical patterns

### What Should Exist

One **per-cloud image manager** that both backends in that cloud share, built on top of core's OCI client.

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

## Implementation Order

### Step 1: Core `image_manager.go` (BLOCKING -- all clouds depend on this)
- Create `AuthProvider` interface and `ImageManager` struct
- Add `FetchImageConfigWithAuth(ref, authHeader string)` -- passes auth header verbatim,
  bypassing `getRegistryToken()` for cloud registries that provide their own token
- Move image method logic from `backend_impl.go` / `backend_impl_ext.go` into `ImageManager`
- `ImageManager.Pull()` uses `Auth.GetToken()` for cloud registries, falls back to core's
  token exchange for Docker Hub
- `ImageManager.Build()` delegates to `BuildFunc` (injected by caller)
- `BaseServer.InitDrivers()` creates `ImageManager{Auth: nil, BuildFunc: s.imageBuild}`
- `BaseServer` image methods delegate to `s.Images.Method()`
- All existing core tests must pass

### Step 2: Per-cloud (parallelizable after Step 1)

**AWS**: ECRAuthProvider in `ecs/image_auth.go` + `lambda/image_auth.go`.
Wire both backends, delete `ecs/registry.go` and scattered methods.

**GCP**: ARAuthProvider in `cloudrun/image_auth.go` + `cloudrun-functions/image_auth.go`.
Wire both backends, delete `cloudrun/registry.go` and `cloudrun-functions/registry.go`.
See `backends/cloudrun/IMAGE_PLAN.md` for the detailed GCP plan.

**Azure**: ACRAuthProvider in `aca/image_auth.go` + `azure-functions/image_auth.go`.
Wire both backends, delete `aca/registry.go` and `aca/backend_impl_images.go`.

### Step 3: Cleanup
Delete old plan files, update `IMPLEMENTATION_PLAN.md` files, verify coverage checker.

---

## Constraints

- **E2E tests**: `TestImageBuild` expects HTTP 200 -- `ImageManager.Build()` must use BaseServer's synthetic Dockerfile parser.
- **No new Go modules**: AuthProviders live in existing backend packages, not new modules.
- **core has no cloud SDK deps**: `AuthProvider` is an interface -- cloud SDKs only imported in backend packages.
- **Non-fatal cloud ops**: All `OnPush`/`OnTag`/`OnRemove` failures are logged, never returned as errors.
- **Backward compat**: `BaseServer.ImagePull()` etc. still work for the Docker backend and tests that create `BaseServer` directly.
- **No simulator changes**: The architecture works against existing simulator APIs. `OnRemove` implementations handle missing DELETE support (405) gracefully. No changes to `simulators/` are required.
