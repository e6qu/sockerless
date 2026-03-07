# Unified Image Management Architecture

## Problem Statement

Image management code is heavily duplicated across cloud backends:

### Current Duplication

1. **`registry.go` copied 3 times** (ECS ~196, CloudRun ~185, ACA ~184 lines):
   - `fetchImageConfig()` — identical logic, only auth differs
   - `parseImageRef()` — identical in all 3 + GCF (4 copies total)
   - `getDockerHubToken()` — identical in all 3

2. **Core's `registry.go` (443 lines) is superior but ignored**:
   - Handles manifest lists (multi-arch images)
   - Parses `Www-Authenticate` headers for token exchange
   - Has in-memory caching
   - Only used by Lambda and GCF via `core.FetchImageConfig()`

3. **OCI push scattered**:
   - Core has `oci_push.go` (239 lines) — only GCP backends use it
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
                        ┌──────────────────────────┐
                        │     backends/core/        │
                        │                          │
                        │  registry.go   (OCI pull) │
                        │  oci_push.go   (OCI push) │
                        │  image_manager.go (NEW)   │
                        │                          │
                        │  AuthProvider interface   │
                        │  ImageManager struct      │
                        └──────────┬───────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                     │
    ┌─────────▼──────┐  ┌─────────▼──────┐  ┌──────────▼─────┐
    │  AWS auth impl │  │  GCP auth impl │  │  Azure auth    │
    │  (in ecs/)     │  │  (in cloudrun/)│  │  (in aca/)     │
    │                │  │                │  │                │
    │ ECRAuthProvider│  │ ARAuthProvider │  │ ACRAuthProvider│
    │ + ECR SDK ops  │  │                │  │                │
    └───────┬────────┘  └───────┬────────┘  └───────┬────────┘
            │                   │                    │
      ┌─────┴────┐       ┌─────┴────┐        ┌─────┴────┐
      │ECS│Lambda│       │CR │ GCF  │        │ACA│ AZF  │
      └──────────┘       └──────────┘        └──────────┘
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
    // Non-fatal — errors are logged, not returned.
    OnPush(img api.Image, registry, repo, tag string) error

    // OnTag is called after a successful in-memory tag to sync to the cloud registry.
    // Non-fatal.
    OnTag(img api.Image, registry, repo, newTag string) error

    // OnRemove is called after a successful in-memory remove to sync to the cloud registry.
    // Non-fatal.
    OnRemove(registry, repo string, tags []string) error
}

// ImageManager handles all 12 image methods using core OCI client + cloud AuthProvider.
// AuthLogin is NOT part of ImageManager — backends handle it independently.
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

1. **Auth token semantics**: `GetToken()` must return the **raw token** without scheme prefix.
   `ImageManager.Pull()` handles adding the appropriate prefix:
   - For ECR: token is base64 `user:password`, sent as `Basic <token>`
   - For AR/ACR: token is an OAuth access token, sent as `Bearer <token>`
   The `ImageManager` knows which to use because `IsCloudRegistry()` tells it the provider type.
   Alternatively, `GetToken()` returns a full `Authorization` header value and `ImageManager` passes it through verbatim.

2. **`FetchImageConfig` refactor**: Current `core.FetchImageConfig(ref, basicAuth)` prepends
   `"Basic "` internally. `ImageManager.Pull()` must NOT use this function directly for cloud
   registries. Instead, it should call the lower-level `fetchConfigFromRegistry()` with the
   cloud auth token pre-set, or add a new `FetchImageConfigWithAuth(ref, authHeader string)`
   that passes the Authorization header verbatim.

3. **`BuildFunc` for ImageBuild**: `ImageManager` cannot import BaseServer's Dockerfile parser
   (circular dep risk). Instead, `BuildFunc` is injected at creation time. BaseServer sets it
   to its synthetic build logic. FaaS backends set it to return `NotImplementedError`.

4. **Separate Go modules**: Each backend is a separate `go.mod`. FaaS backends in the same
   cloud (Lambda, GCF, AZF) CANNOT import from their container counterpart (ECS, CloudRun, ACA).
   Each must duplicate the ~80-line AuthProvider implementation.

### Per-Cloud AuthProvider

Each cloud implements `AuthProvider` in ~30-50 lines:

**AWS** (`backends/ecs/image_auth.go` + `backends/lambda/image_auth.go`):
- `IsCloudRegistry`: matches `*.dkr.ecr.*.amazonaws.com`
- `GetToken`: calls `ecr.GetAuthorizationToken`, returns raw base64 token (no "Basic " prefix)
- `OnPush`: calls `ecr.CreateRepository` + `ecr.PutImage`
- `OnTag`: calls `ecr.PutImage` with new tag
- `OnRemove`: calls `ecr.BatchDeleteImage`

**GCP** (`backends/cloudrun/image_auth.go`):
- `IsCloudRegistry`: matches `*.gcr.io`, `*-docker.pkg.dev`
- `GetToken`: calls `google.FindDefaultCredentials`, returns Bearer token
- `OnPush`: calls `core.OCIPush`
- `OnTag`: re-PUTs manifest with new tag via OCI
- `OnRemove`: DELETEs manifest via OCI

**Azure** (`backends/aca/image_auth.go`):
- `IsCloudRegistry`: matches `*.azurecr.io`
- `GetToken`: calls `azidentity.DefaultAzureCredential.GetToken`, returns Bearer token
- `OnPush`: calls `core.OCIPush`
- `OnTag`: re-PUTs manifest with new tag via OCI
- `OnRemove`: DELETEs manifest via OCI

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
// ... same for all 14 methods
```

Lambda has its own `ECRAuthProvider` (separate Go module, ~40 lines).
GCF has its own `ARAuthProvider` (separate Go module, ~80 lines).
AZF has its own `ACRAuthProvider` (separate Go module, ~80 lines).

### FaaS Overrides

FaaS backends (Lambda, GCF, AZF) need some methods to return `NotImplementedError`:
- `ImageBuild` — no build capability
- `ImageLoad` — Lambda/GCF may want to block this (or allow for metadata-only)

This is handled by a `FaaSImageManager` wrapper or by the backend overriding specific methods after delegation.

---

## What Gets Deleted

| File | Lines | Reason |
|------|-------|--------|
| `backends/ecs/registry.go` | 196 | Replaced by core ImageManager + ECRAuthProvider |
| `backends/cloudrun/registry.go` | 185 | Replaced by core ImageManager + ARAuthProvider |
| `backends/aca/registry.go` | 184 | Replaced by core ImageManager + ACRAuthProvider |
| `backends/cloudrun-functions/registry.go` | 50 | Uses CloudRun's ARAuthProvider |
| `backends/aca/backend_impl_images.go` | 359 | OCI push moved to core, auth to ACRAuthProvider |
| Duplicate `parseImageRef()` | 4 copies | Use core's `parseImageRef()` |
| Duplicate `getDockerHubToken()` | 3 copies | Handled inside core's ImageManager |
| Scattered image methods in `backend_impl.go` | ~400 lines total | Replaced by `s.images.Method()` one-liners |

**Estimated deletion**: ~1,200+ lines of duplicated code.
**Estimated addition**: ~500 lines (ImageManager + 6 AuthProviders, one per backend module).

### Simulator Prerequisites

OCI manifest DELETE is needed for `OnRemove`:
- `simulators/gcp/artifactregistry.go` — add DELETE handler to `handleOCIManifest()` (~15 lines)
- `simulators/azure/acr.go` — add DELETE handler to manifest routing (~15 lines)
- `simulators/aws/ecr.go` — uses ECR SDK (BatchDeleteImage), already supported

---

## Implementation Order

### Step 0: Simulator prerequisites
Add OCI manifest DELETE handlers to GCP and Azure simulators (~30 lines total).

### Step 1: Core `image_manager.go` (BLOCKING — all clouds depend on this)
- Create `AuthProvider` interface and `ImageManager` struct
- Add `FetchImageConfigWithAuth(ref, authHeader string)` — passes auth header verbatim
- Move image method logic from `backend_impl.go` / `backend_impl_ext.go` into `ImageManager`
- `ImageManager.Pull()` uses `Auth.GetToken()` for cloud registries, falls back to core's token exchange for Docker Hub
- `ImageManager.Build()` delegates to `BuildFunc` (injected by caller)
- Export `SetOCIAuth()` (currently unexported `setOCIAuth`)
- `BaseServer.InitDrivers()` creates `ImageManager{Auth: nil, BuildFunc: s.imageBuild}`
- `BaseServer` image methods delegate to `s.Images.Method()`
- All existing core tests must pass

### Step 2: Per-cloud (parallelizable after Step 1)

**AWS**: ECRAuthProvider in `ecs/image_auth.go` + `lambda/image_auth.go`.
Wire both backends, delete `ecs/registry.go` and scattered methods.

**GCP**: ARAuthProvider in `cloudrun/image_auth.go` + `cloudrun-functions/image_auth.go`.
Wire both backends, delete `cloudrun/registry.go` and `cloudrun-functions/registry.go`.

**Azure**: ACRAuthProvider in `aca/image_auth.go` + `azure-functions/image_auth.go`.
Wire both backends, delete `aca/registry.go` and `aca/backend_impl_images.go`.

### Step 3: Cleanup
Delete old `IMAGE_MANAGEMENT_PLAN.md` files, update `IMPLEMENTATION_PLAN.md` files, verify coverage checker.

---

## Constraints

- **E2E tests**: `TestImageBuild` expects HTTP 200 — `ImageManager.Build()` must use BaseServer's synthetic Dockerfile parser.
- **No new Go modules**: AuthProviders live in existing backend packages, not new modules.
- **core has no cloud SDK deps**: `AuthProvider` is an interface — cloud SDKs only imported in backend packages.
- **Non-fatal cloud ops**: All `OnPush`/`OnTag`/`OnRemove` failures are logged, never returned as errors.
- **Backward compat**: `BaseServer.ImagePull()` etc. still work for the Docker backend and tests that create `BaseServer` directly.
