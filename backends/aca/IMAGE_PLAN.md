# Azure Unified Image Management Plan (ACA + AZF)

This plan implements the global architecture from `backends/IMAGE_ARCHITECTURE.md` for the Azure cloud: both ACA and Azure Functions backends delegate all image methods to `core.ImageManager` via an `ACRAuthProvider`.

**Key constraint**: No simulator changes. The ACR simulator is used as-is.

---

## 1. ACRAuthProvider

### Struct Definition

**File**: `backends/aca/image_auth.go` (~120 lines)

```go
package aca

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
    "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
    "github.com/rs/zerolog"
    "github.com/sockerless/api"
    core "github.com/sockerless/backend-core"
)

// ACRAuthProvider implements core.AuthProvider for Azure Container Registry.
type ACRAuthProvider struct {
    Logger zerolog.Logger
}
```

### Method Signatures

```go
// IsCloudRegistry returns true for *.azurecr.io registries.
func (a *ACRAuthProvider) IsCloudRegistry(registry string) bool

// GetToken returns a full Authorization header value ("Bearer <token>")
// for ACR registries. Returns ("", nil) for non-ACR registries.
func (a *ACRAuthProvider) GetToken(registry string) (string, error)

// OnPush pushes a synthetic image manifest + blobs to ACR via OCI Distribution API.
// Uses core.OCIPush. Non-fatal -- errors are logged by ImageManager, not returned to client.
func (a *ACRAuthProvider) OnPush(img api.Image, registry, repo, tag string) error

// OnTag re-PUTs the existing manifest with a new tag reference in ACR.
// GET manifest by digest -> PUT manifest with new tag.
func (a *ACRAuthProvider) OnTag(img api.Image, registry, repo, newTag string) error

// OnRemove attempts to delete the manifest from ACR via OCI Distribution API.
// The ACR simulator does NOT support DELETE /v2/{name}/manifests/{digest}.
// This method logs a warning and returns nil when DELETE returns a non-success status.
// On real ACR, DELETE works normally. Errors are always non-fatal.
func (a *ACRAuthProvider) OnRemove(registry, repo string, tags []string) error
```

### Implementation Details

**`IsCloudRegistry`**: `return strings.HasSuffix(registry, ".azurecr.io")`

**`GetToken`**:
- Returns `("", nil)` for non-ACR registries (signals ImageManager to use core's Www-Authenticate token exchange for Docker Hub, etc.)
- For ACR: uses `azidentity.NewDefaultAzureCredential` + `cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"https://<registry>/.default"}})`.
- Returns the full header value: `"Bearer " + token.Token`.

**`OnPush`**:
- Calls `a.GetToken(registry)` for auth.
- Delegates to `core.OCIPush(core.OCIPushOptions{...})`.
- This replaces ACA's current `pushToACR()` + `uploadBlob()` (~130 lines in `backend_impl_images.go`).

**`OnTag`**:
- GET manifest by source digest from ACR.
- PUT the same manifest bytes with the new tag as reference.
- Uses `core.SetOCIAuth()` (exported from `oci_push.go`) to set auth headers.

**`OnRemove`**:
- For each tag: HEAD to get `Docker-Content-Digest`, then DELETE by digest.
- **Graceful degradation**: If DELETE returns a non-success status (e.g., 404 or 405 from the simulator which lacks a DELETE handler), log a warning and continue to the next tag. The method always returns `nil` -- OnRemove errors are non-fatal by design.
- On real ACR, DELETE works and returns 202.

### Why ACRAuthProvider Lives in `backends/aca/`

The `aca` package already imports `azidentity` and `azcore/policy`. Placing ACRAuthProvider here avoids adding cloud SDK deps to `backends/core/` (which must remain cloud-agnostic).

---

## 2. Separate Go Modules: ACA and AZF

ACA and AZF are **separate Go modules**:
- ACA: `module github.com/sockerless/backend-aca` (`backends/aca/go.mod`)
- AZF: `module github.com/sockerless/backend-azf` (`backends/azure-functions/go.mod`)

AZF has **zero dependency** on ACA today. Adding a cross-module import would require `go.mod` changes and couples two independent backends.

### Decision: Duplicate ACRAuthProvider in AZF (~120 lines)

Following the GCP precedent (CloudRun + GCF both have their own `ARAuthProvider`), AZF gets its own copy of `ACRAuthProvider` in `backends/azure-functions/image_auth.go`. Both copies import only:
- `azidentity` (already in AZF's `go.mod`)
- `azcore/policy` (already in AZF's `go.mod`)
- `backend-core` (already in AZF's `go.mod`)
- `api` (already in AZF's `go.mod`)

No new module dependencies needed.

---

## 3. Core ImageManager Wiring

The `core.ImageManager` struct (defined in `IMAGE_ARCHITECTURE.md`) handles all 12 image methods. Each Azure backend creates one and delegates.

### How ACA Creates ImageManager

In `backends/aca/server.go`, `NewServer()`:

```go
type Server struct {
    *core.BaseServer
    config    Config
    azure     *AzureClients
    ipCounter atomic.Int32
    images    *core.ImageManager  // NEW

    ACA          *core.StateStore[ACAState]
    NetworkState *core.StateStore[NetworkState]
    VolumeState  *core.StateStore[VolumeState]
}

func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
    s := &Server{...}
    // ... existing BaseServer creation ...

    s.images = &core.ImageManager{
        Auth:   &ACRAuthProvider{Logger: logger},
        Store:  s.Store,
        Logger: logger,
    }
    // BuildFunc is set by ImageManager to use BaseServer's synthetic Dockerfile parser
    // (injected via Store reference, not direct BaseServer import).

    return s
}
```

### How AZF Creates ImageManager

In `backends/azure-functions/server.go`, `NewServer()`:

```go
type Server struct {
    *core.BaseServer
    config    Config
    azure     *AzureClients
    ipCounter atomic.Int32
    images    *core.ImageManager  // NEW

    AZF *core.StateStore[AZFState]
}

func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
    s := &Server{...}
    // ... existing BaseServer creation ...

    s.images = &core.ImageManager{
        Auth:   &ACRAuthProvider{Logger: logger},  // AZF's own copy
        Store:  s.Store,
        Logger: logger,
    }

    return s
}
```

---

## 4. Method Coverage: All 12 Image Methods + AuthLogin

### ACA Method Table

| # | Method | Current Location | Current Lines | New Implementation |
|---|--------|-----------------|---------------|-------------------|
| 1 | `ImagePull` | `backend_impl.go` (custom) | ~80 | `return s.images.Pull(ref, auth)` |
| 2 | `ImageInspect` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Inspect(name)` |
| 3 | `ImageLoad` | `backend_impl.go` (BaseServer delegate) | 3 | `return s.images.Load(r)` |
| 4 | `ImageTag` | `backend_impl_images.go` (custom ACR sync) | ~70 | `return s.images.Tag(source, repo, tag)` |
| 5 | `ImageList` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.List(opts)` |
| 6 | `ImageRemove` | `backend_impl_images.go` (custom ACR sync) | ~75 | `return s.images.Remove(name, force, prune)` |
| 7 | `ImageHistory` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.History(name)` |
| 8 | `ImagePrune` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Prune(filters)` |
| 9 | `ImageBuild` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Build(opts, context)` |
| 10 | `ImagePush` | `backend_impl_images.go` (custom ACR push) | ~57 | `return s.images.Push(name, tag, auth)` |
| 11 | `ImageSave` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Save(names)` |
| 12 | `ImageSearch` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Search(term, limit, filters)` |
| -- | `AuthLogin` | `backend_impl_pods.go` (ACR warning + BaseServer) | 9 | Keep on Server (NOT in ImageManager) |

**AuthLogin** stays on the `Server` directly, matching ECS/GCP precedent. It is not part of `ImageManager`. The ACR warning log remains in the `AuthLogin` method on `Server`.

### AZF Method Table

| # | Method | Current Location | Current Lines | New Implementation |
|---|--------|-----------------|---------------|-------------------|
| 1 | `ImagePull` | `backend_impl.go` (custom) | ~90 | `return s.images.Pull(ref, auth)` |
| 2 | `ImageInspect` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Inspect(name)` |
| 3 | `ImageLoad` | `backend_impl.go` (BaseServer delegate) | 3 | `return s.images.Load(r)` |
| 4 | `ImageTag` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Tag(source, repo, tag)` |
| 5 | `ImageList` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.List(opts)` |
| 6 | `ImageRemove` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Remove(name, force, prune)` |
| 7 | `ImageHistory` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.History(name)` |
| 8 | `ImagePrune` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Prune(filters)` |
| 9 | `ImageBuild` | `backend_impl.go` (NotImplementedError) | 5 | **Override**: return `NotImplementedError` |
| 10 | `ImagePush` | `backend_impl.go` (NotImplementedError) | 5 | **Override**: return `NotImplementedError` |
| 11 | `ImageSave` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Save(names)` |
| 12 | `ImageSearch` | `backend_delegates_gen.go` (BaseServer) | 3 | `return s.images.Search(term, limit, filters)` |
| -- | `AuthLogin` | `backend_impl.go` (ACR warning + BaseServer) | 9 | Keep on Server (NOT in ImageManager) |

**AZF FaaS overrides**: `ImageBuild` and `ImagePush` stay as `NotImplementedError` returns on the `Server` struct. They do NOT delegate to `s.images`. All other 10 methods delegate.

---

## 5. What Gets Deleted

### Files Deleted Entirely

| File | Lines | Reason |
|------|-------|--------|
| `backends/aca/registry.go` | 184 | `fetchImageConfig()`, `parseImageRef()`, `getDockerHubToken()`, `getACRToken()` -- replaced by `core.ImageManager` + `ACRAuthProvider.GetToken()` |
| `backends/aca/backend_impl_images.go` | 359 | `ImagePush`, `ImageTag`, `ImageRemove`, `pushToACR`, `uploadBlob`, `setAuthHeader` -- replaced by `ImageManager` + `ACRAuthProvider.OnPush/OnTag/OnRemove` |

### Functions/Methods Removed from Existing Files

| File | Function | Lines | Reason |
|------|----------|-------|--------|
| `backends/aca/backend_impl.go` | `ImagePull` | ~80 | Replaced by `s.images.Pull()` one-liner |
| `backends/aca/backend_impl.go` | `ImageLoad` | 3 | Replaced by `s.images.Load()` one-liner |
| `backends/azure-functions/backend_impl.go` | `ImagePull` | ~90 | Replaced by `s.images.Pull()` one-liner |
| `backends/azure-functions/backend_impl.go` | `ImageBuild` | 5 | Stays as override, but moves to `backend_impl_images.go` |
| `backends/azure-functions/backend_impl.go` | `ImagePush` | 5 | Stays as override, but moves to `backend_impl_images.go` |
| `backends/azure-functions/backend_impl.go` | `ImageLoad` | 3 | Replaced by `s.images.Load()` one-liner |

### Delegates Removed from `backend_delegates_gen.go`

- **ACA**: `ImageBuild`, `ImageHistory`, `ImageInspect`, `ImageList`, `ImagePrune`, `ImageSave`, `ImageSearch` (7 delegates)
- **AZF**: `ImageHistory`, `ImageInspect`, `ImageList`, `ImagePrune`, `ImageRemove`, `ImageSave`, `ImageSearch`, `ImageTag` (8 delegates)

### Total Deletion Estimate

- **~543 lines deleted** from ACA (184 `registry.go` + 359 `backend_impl_images.go`)
- **~90 lines deleted** from AZF (`ImagePull` body)
- **15 delegates removed** from `backend_delegates_gen.go` files

---

## 6. What Gets Created

### New File: `backends/aca/image_auth.go` (~120 lines)

`ACRAuthProvider` struct implementing `core.AuthProvider` with 5 methods:
- `IsCloudRegistry`, `GetToken`, `OnPush`, `OnTag`, `OnRemove`

### New File: `backends/azure-functions/image_auth.go` (~120 lines)

Duplicate of ACA's `ACRAuthProvider` (separate Go module, cannot import).

### New File: `backends/aca/backend_impl_images.go` (~35 lines, replaces old 359-line file)

12 one-liner delegates to `s.images`:

```go
package aca

import (
    "io"
    "github.com/sockerless/api"
)

func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
    return s.images.Pull(ref, auth)
}
func (s *Server) ImageInspect(name string) (*api.Image, error) {
    return s.images.Inspect(name)
}
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
    return s.images.Load(r)
}
func (s *Server) ImageTag(source string, repo string, tag string) error {
    return s.images.Tag(source, repo, tag)
}
func (s *Server) ImageList(opts api.ImageListOptions) ([]*api.ImageSummary, error) {
    return s.images.List(opts)
}
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
    return s.images.Remove(name, force, prune)
}
func (s *Server) ImageHistory(name string) ([]*api.ImageHistoryEntry, error) {
    return s.images.History(name)
}
func (s *Server) ImagePrune(filters map[string][]string) (*api.ImagePruneResponse, error) {
    return s.images.Prune(filters)
}
func (s *Server) ImageBuild(opts api.ImageBuildOptions, context io.Reader) (io.ReadCloser, error) {
    return s.images.Build(opts, context)
}
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
    return s.images.Push(name, tag, auth)
}
func (s *Server) ImageSave(names []string) (io.ReadCloser, error) {
    return s.images.Save(names)
}
func (s *Server) ImageSearch(term string, limit int, filters map[string][]string) ([]*api.ImageSearchResult, error) {
    return s.images.Search(term, limit, filters)
}
```

### New File: `backends/azure-functions/backend_impl_images.go` (~45 lines)

10 one-liner delegates to `s.images` + 2 `NotImplementedError` overrides:

```go
package azf

import (
    "io"
    "github.com/sockerless/api"
)

func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
    return s.images.Pull(ref, auth)
}
func (s *Server) ImageInspect(name string) (*api.Image, error) {
    return s.images.Inspect(name)
}
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
    return s.images.Load(r)
}
func (s *Server) ImageTag(source string, repo string, tag string) error {
    return s.images.Tag(source, repo, tag)
}
func (s *Server) ImageList(opts api.ImageListOptions) ([]*api.ImageSummary, error) {
    return s.images.List(opts)
}
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
    return s.images.Remove(name, force, prune)
}
func (s *Server) ImageHistory(name string) ([]*api.ImageHistoryEntry, error) {
    return s.images.History(name)
}
func (s *Server) ImagePrune(filters map[string][]string) (*api.ImagePruneResponse, error) {
    return s.images.Prune(filters)
}
func (s *Server) ImageSave(names []string) (io.ReadCloser, error) {
    return s.images.Save(names)
}
func (s *Server) ImageSearch(term string, limit int, filters map[string][]string) ([]*api.ImageSearchResult, error) {
    return s.images.Search(term, limit, filters)
}

// ImageBuild is not supported by the Azure Functions backend.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
    return nil, &api.NotImplementedError{
        Message: "Azure Functions backend does not support image build; push pre-built images to Azure Container Registry",
    }
}

// ImagePush is not supported by the Azure Functions backend.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
    return nil, &api.NotImplementedError{
        Message: "Azure Functions backend does not support image push; push images directly to Azure Container Registry",
    }
}
```

### Modified: `backends/core/oci_push.go`

Export `setOCIAuth` as `SetOCIAuth` (rename function + update 4 internal call sites). This is needed by `ACRAuthProvider.OnTag()` and `ACRAuthProvider.OnRemove()` to set auth headers on outgoing OCI requests.

### Modified: `backends/aca/server.go`

- Add `images *core.ImageManager` field to `Server` struct.
- Initialize in `NewServer()`.

### Modified: `backends/azure-functions/server.go`

- Add `images *core.ImageManager` field to `Server` struct.
- Initialize in `NewServer()`.

---

## 7. Migration Path (Step-by-Step)

### Step 1: Core ImageManager (prerequisite, not Azure-specific)

1. Create `backends/core/image_manager.go` with `AuthProvider` interface and `ImageManager` struct
2. Move image method logic from `BaseServer` into `ImageManager` methods
3. `BaseServer` creates `ImageManager{Auth: nil}` and delegates its own image methods to it
4. Export `SetOCIAuth` in `oci_push.go`
5. Add `FetchImageConfigWithAuth(ref, authHeader string)` to handle Bearer vs Basic token scheme mismatch (see Critical Issues below)
6. Run core tests -- must all pass

### Step 2: ACRAuthProvider

1. Create `backends/aca/image_auth.go` with `ACRAuthProvider`
2. Create `backends/azure-functions/image_auth.go` with duplicate `ACRAuthProvider`
3. Unit test: `IsCloudRegistry` returns true for `*.azurecr.io`, false for `docker.io`

### Step 3: Wire ACA

1. Add `images *core.ImageManager` to ACA `Server` struct
2. Initialize in `NewServer()` with `&ACRAuthProvider{Logger: logger}`
3. Replace old `backend_impl_images.go` (359 lines) with new 12-method delegate file (~35 lines)
4. Delete `registry.go` (184 lines)
5. Remove `ImagePull` and `ImageLoad` from `backend_impl.go`
6. Keep `AuthLogin` in `backend_impl_pods.go` (NOT moved to ImageManager)
7. Remove 7 image delegates from `backend_delegates_gen.go`
8. Run ACA tests -- must all pass

### Step 4: Wire AZF

1. Add `images *core.ImageManager` to AZF `Server` struct
2. Initialize in `NewServer()` with `&ACRAuthProvider{Logger: logger}`
3. Create `backend_impl_images.go` with 10 delegates + 2 `NotImplementedError` overrides
4. Remove `ImagePull`, `ImageBuild`, `ImagePush`, `ImageLoad` from `backend_impl.go`
5. Keep `AuthLogin` in `backend_impl.go` (NOT moved to ImageManager)
6. Remove 8 image delegates from `backend_delegates_gen.go`
7. Run AZF tests -- must all pass

### Step 5: Cleanup

1. Run full test suite (`sim-test-all`, e2e tests)
2. Verify no remaining references to deleted functions

---

## 8. Critical Design Issues

### Issue 1: `FetchImageConfig` Auth Token Scheme Mismatch

`core.FetchImageConfig(ref, basicAuth)` internally calls `getRegistryToken()` which sets `"Basic " + basicAuth`. But `ACRAuthProvider.GetToken()` returns `"Bearer " + token`. Passing a Bearer token through the `basicAuth` parameter would produce `"Basic Bearer xxx"` -- broken.

**Resolution**: `ImageManager.Pull()` must handle this in one of two ways:
1. For cloud registries (where `Auth.IsCloudRegistry()` returns true): bypass `FetchImageConfig` entirely and call the lower-level `fetchConfigFromRegistry()` with the cloud auth token, or use a new `FetchImageConfigWithAuth(ref, authHeader string)` that passes the Authorization header verbatim.
2. For non-cloud registries: use existing `FetchImageConfig()` with the standard Www-Authenticate flow.

This is a **core ImageManager design decision** resolved in Step 1, not Azure-specific.

### Issue 2: ACR Simulator Lacks DELETE Manifest Handler

The ACR simulator (`simulators/azure/acr.go`) has no `DELETE /v2/{name}/manifests/{digest}` handler. When `OnRemove` sends a DELETE request, the simulator returns a non-matching route error (likely 404 or 405).

**Handling**: `OnRemove` treats all errors as non-fatal. It logs a warning and returns `nil`. Tests pass because:
- The in-memory image removal (done by `ImageManager.Remove()` before calling `OnRemove`) always succeeds.
- `OnRemove` failures are logged but never propagated to the client.
- Real ACR supports DELETE and returns 202.

**No simulator changes needed.** The graceful degradation is by design.

### Issue 3: ACR Simulator Has No Www-Authenticate Challenge

The ACR simulator returns 200 directly for all OCI endpoints (no 401 + `Www-Authenticate`). This means `core.FetchImageConfig()` succeeds without auth on the simulator, even if the auth wiring is broken.

**Impact**: Simulator-based tests do not exercise the auth path. This is acceptable because:
- The auth path is tested against real ACR in production.
- The `ACRAuthProvider` implementation is trivial (`azidentity.NewDefaultAzureCredential` + `GetToken`).
- The `ImageManager` auth integration is tested at the core level with mock AuthProviders.

### Issue 4: ImagePull Metadata Preservation

ACA's current `ImagePull` generates detailed metadata: `RepoDigests`, `RootFS` with synthetic layer hash, `GraphDriver` with overlay2 paths, `Metadata.LastTagTime`, deterministic `Size` via FNV hash. The core `BaseServer.ImagePull` generates identical metadata. When `ImageManager.Pull()` absorbs this logic, it must preserve all these fields. This is verified during Step 1 (core tests must pass).

---

## 9. Dependency Graph

```
Step 1: core/image_manager.go  (no Azure deps, no simulator changes)
    |
    v
Step 2: aca/image_auth.go + azf/image_auth.go  (ACRAuthProvider, duplicated)
    |
    +--> Step 3: aca/server.go + backend_impl_images.go  (wire ACA)
    |
    +--> Step 4: azf/server.go + backend_impl_images.go  (wire AZF)
    |
    v
Step 5: Cleanup (verify all tests pass)
```

Steps 3 and 4 are independent (can be done in parallel or either order). Step 2 depends on Step 1. Step 5 depends on Steps 3+4.

---

## 10. Test Impact

### Tests That Must Continue Passing

| Test Suite | Key Tests | Risk |
|-----------|-----------|------|
| `tests/system_test.go` | `TestImageBuild`, `TestImagePull` | Low -- `ImageManager.Build()` uses same BaseServer logic |
| ACA SDK tests (`simulators/azure/sdk-tests/`) | All image-related | Low -- behavior unchanged |
| AZF SDK tests (`simulators/azure/sdk-tests/`) | All image-related | Low -- behavior unchanged |
| Core unit tests (`backends/core/`) | `BaseServer` image tests | Low -- `BaseServer` delegates to `ImageManager{Auth: nil}` |
| `sim-test-all` (75 tests) | All 6 backends | Low -- all use same `ImageManager` |

### Behavioral Invariants

1. **ACA ImagePull**: Still fetches real image config from registries. `ImageManager.Pull()` calls `core.FetchImageConfig()` (cached) with auth from `ACRAuthProvider.GetToken()` for ACR registries. Same result, better code.
2. **ACA ImagePush to ACR**: Still pushes synthetic manifest+config to ACR via OCI Distribution API. `ImageManager.Push()` calls `ACRAuthProvider.OnPush()` which calls `core.OCIPush()`. Same result.
3. **ACA ImagePush to non-ACR**: Falls through to BaseServer synthetic progress. Same result.
4. **ACA ImageTag to ACR**: Still syncs tag via GET manifest + PUT with new tag. `ImageManager.Tag()` calls `ACRAuthProvider.OnTag()`. Same result.
5. **ACA ImageRemove from ACR**: In-memory removal always succeeds. `ACRAuthProvider.OnRemove()` attempts DELETE but handles failure gracefully (logs warning). On simulator, DELETE silently fails. On real ACR, DELETE succeeds.
6. **AZF ImageBuild/ImagePush**: Still return `NotImplementedError`. Overridden at the `Server` level, not delegated to ImageManager.
7. **AZF ImagePull**: Still uses `core.FetchImageConfig()`. Same result.
8. **AuthLogin ACR warning**: Still logged for `*.azurecr.io` registries. Stays on `Server.AuthLogin()`, not in ImageManager.
