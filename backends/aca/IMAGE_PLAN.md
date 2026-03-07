# Azure Unified Image Management Plan (ACA + AZF)

This plan implements the global architecture from `backends/IMAGE_ARCHITECTURE.md` for the Azure cloud: both ACA and Azure Functions backends share a single `ACRAuthProvider` and delegate all image methods to `core.ImageManager`.

---

## 1. ACRAuthProvider

**File**: `backends/aca/image_auth.go` (~80 lines)

**Import path**: `github.com/sockerless/aca` (AZF imports this package)

```go
package aca

import (
    "bytes"
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

// IsCloudRegistry returns true if the registry is an Azure Container Registry.
func (a *ACRAuthProvider) IsCloudRegistry(registry string) bool {
    return strings.HasSuffix(registry, ".azurecr.io")
}

// GetToken returns a Bearer token for an ACR registry.
// Returns ("", nil) for non-ACR registries (falls through to core token exchange).
func (a *ACRAuthProvider) GetToken(registry string) (string, error) {
    if !a.IsCloudRegistry(registry) {
        return "", nil
    }
    cred, err := azidentity.NewDefaultAzureCredential(nil)
    if err != nil {
        return "", err
    }
    scope := fmt.Sprintf("https://%s/.default", registry)
    token, err := cred.GetToken(context.Background(), policy.TokenRequestOptions{
        Scopes: []string{scope},
    })
    if err != nil {
        return "", err
    }
    return "Bearer " + token.Token, nil
}

// OnPush pushes a synthetic image manifest + blobs to ACR via OCI Distribution API.
func (a *ACRAuthProvider) OnPush(img api.Image, registry, repo, tag string) error {
    token, err := a.GetToken(registry)
    if err != nil {
        return fmt.Errorf("ACR auth: %w", err)
    }
    _, err = core.OCIPush(core.OCIPushOptions{
        Registry:   registry,
        Repository: repo,
        Tag:        tag,
        AuthToken:  token,
    })
    return err
}

// OnTag re-PUTs the existing manifest with a new tag reference in ACR.
func (a *ACRAuthProvider) OnTag(img api.Image, registry, repo, newTag string) error {
    token, err := a.GetToken(registry)
    if err != nil {
        return err
    }
    // GET existing manifest by digest
    srcDigest := strings.TrimPrefix(img.ID, "sha256:")
    manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/sha256:%s", registry, repo, srcDigest)
    req, _ := http.NewRequest("GET", manifestURL, nil)
    req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
    core.SetOCIAuth(req, token)

    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        return fmt.Errorf("manifest GET failed: %d", resp.StatusCode)
    }

    manifestData, _ := io.ReadAll(resp.Body)
    contentType := resp.Header.Get("Content-Type")
    if contentType == "" {
        contentType = "application/vnd.docker.distribution.manifest.v2+json"
    }

    // PUT manifest with new tag
    putURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, newTag)
    putReq, _ := http.NewRequest("PUT", putURL, bytes.NewReader(manifestData))
    putReq.Header.Set("Content-Type", contentType)
    core.SetOCIAuth(putReq, token)

    putResp, err := client.Do(putReq)
    if err != nil {
        return err
    }
    putResp.Body.Close()
    if putResp.StatusCode != 201 && putResp.StatusCode != 200 {
        return fmt.Errorf("manifest PUT failed: %d", putResp.StatusCode)
    }
    return nil
}

// OnRemove deletes the manifest from ACR via OCI Distribution API.
func (a *ACRAuthProvider) OnRemove(registry, repo string, tags []string) error {
    token, err := a.GetToken(registry)
    if err != nil {
        return err
    }
    client := &http.Client{Timeout: 30 * time.Second}

    for _, tag := range tags {
        // HEAD to get digest
        headURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
        headReq, _ := http.NewRequest("HEAD", headURL, nil)
        headReq.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
        core.SetOCIAuth(headReq, token)

        headResp, err := client.Do(headReq)
        if err != nil {
            continue
        }
        headResp.Body.Close()
        if headResp.StatusCode != 200 {
            continue
        }

        digest := headResp.Header.Get("Docker-Content-Digest")
        if digest == "" {
            digest = tag
        }

        // DELETE manifest
        delURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, digest)
        delReq, _ := http.NewRequest("DELETE", delURL, nil)
        core.SetOCIAuth(delReq, token)

        delResp, err := client.Do(delReq)
        if err != nil {
            continue
        }
        delResp.Body.Close()
    }
    return nil
}
```

### Key Design Decisions

1. **`ACRAuthProvider` lives in `backends/aca/`** because that package already imports `azidentity`. AZF imports `ACRAuthProvider` from the `aca` package.

2. **`GetToken` returns `("", nil)` for non-ACR registries** -- this signals to `ImageManager` to fall through to core's standard `Www-Authenticate` token exchange (Docker Hub, etc.).

3. **`OnPush` uses `core.OCIPush`** -- the existing core helper handles blob uploads + manifest PUT. This replaces ACA's custom `pushToACR()` + `uploadBlob()` (~130 lines).

4. **`OnTag` does GET manifest + PUT with new tag** -- identical pattern to current `ImageTag` in `backend_impl_images.go`, but as a standalone method.

5. **`OnRemove` iterates tags, HEAD + DELETE** -- same pattern as current `ImageRemove` in `backend_impl_images.go`.

### Prerequisite: Export `setOCIAuth`

`core.setOCIAuth` (in `oci_push.go`) is currently unexported. Must be exported as `core.SetOCIAuth` (or we add a `core.OCIRegistryRequest` helper). The `OnTag` and `OnRemove` methods need to set auth headers on outgoing requests.

---

## 2. Core ImageManager Wiring

The `core.ImageManager` struct is created as specified in `IMAGE_ARCHITECTURE.md`. It wraps core's existing `BaseServer` image methods plus `AuthProvider` hooks.

### How ACA Creates ImageManager

In `backends/aca/server.go`, `NewServer()` adds:

```go
func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
    s := &Server{...}
    // ... existing BaseServer creation ...

    s.images = &core.ImageManager{
        Auth:   &ACRAuthProvider{Logger: logger},
        Store:  s.Store,
        Logger: logger,
    }

    // ... rest of NewServer ...
}
```

New field on `Server`:

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
```

### ACA Method Delegation (14 image methods + AuthLogin)

After wiring, ACA's image methods become one-liners. Some are already delegates in `backend_delegates_gen.go`; others need to be moved from custom implementations to `s.images.Method()` calls.

| # | Method | Current Location | New Implementation |
|---|--------|-----------------|-------------------|
| 1 | `ImagePull` | `backend_impl.go` (custom, 80 lines) | `return s.images.Pull(ref, auth)` |
| 2 | `ImageInspect` | `backend_delegates_gen.go` (BaseServer) | `return s.images.Inspect(name)` |
| 3 | `ImageLoad` | `backend_impl.go` (delegates to BaseServer) | `return s.images.Load(r)` |
| 4 | `ImageTag` | `backend_impl_images.go` (custom, 70 lines) | `return s.images.Tag(source, repo, tag)` |
| 5 | `ImageList` | `backend_delegates_gen.go` (BaseServer) | `return s.images.List(opts)` |
| 6 | `ImageRemove` | `backend_impl_images.go` (custom, 75 lines) | `return s.images.Remove(name, force, prune)` |
| 7 | `ImageHistory` | `backend_delegates_gen.go` (BaseServer) | `return s.images.History(name)` |
| 8 | `ImagePrune` | `backend_delegates_gen.go` (BaseServer) | `return s.images.Prune(filters)` |
| 9 | `ImageBuild` | `backend_delegates_gen.go` (BaseServer) | `return s.images.Build(opts, context)` |
| 10 | `ImagePush` | `backend_impl_images.go` (custom, 57 lines) | `return s.images.Push(name, tag, auth)` |
| 11 | `ImageSave` | `backend_delegates_gen.go` (BaseServer) | `return s.images.Save(names)` |
| 12 | `ImageSearch` | `backend_delegates_gen.go` (BaseServer) | `return s.images.Search(term, limit, filters)` |
| 13 | `AuthLogin` | `backend_impl_pods.go` (custom, 9 lines) | `return s.images.AuthLogin(req)` or keep custom (ACR warning) |

**AuthLogin special case**: The ACR warning log in `AuthLogin` can either move into `ImageManager.AuthLogin()` (checking `Auth.IsCloudRegistry()`) or stay as a thin wrapper. The cleaner approach is to have `ImageManager.AuthLogin()` log the warning when `Auth.IsCloudRegistry(req.ServerAddress)` is true, then both ACA and AZF get it automatically.

---

## 3. How AZF Shares ACRAuthProvider

### Import, Not Duplicate

AZF imports `ACRAuthProvider` from the `aca` package:

```go
// backends/azure-functions/server.go
import (
    "github.com/sockerless/aca"
)

func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
    s := &Server{...}
    // ... existing BaseServer creation ...

    s.images = &core.ImageManager{
        Auth:   &aca.ACRAuthProvider{Logger: logger},
        Store:  s.Store,
        Logger: logger,
    }
    // ...
}
```

### Go Module Dependency

Both `aca` and `azure-functions` are in the same Go module (`backends/` or top-level `go.mod` -- verify). If they are separate modules, `ACRAuthProvider` should instead be placed in a shared `backends/azure/` package or in `backends/core/` behind an interface. Since `IMAGE_ARCHITECTURE.md` says "No new Go modules", and both backends are in the same module, the direct import works.

**Fallback**: If circular import issues arise (unlikely since AZF importing ACA is one-directional), move `ACRAuthProvider` to a new file `backends/core/auth_azure.go` behind a build tag or just as a plain struct. But this would require `core` to import `azidentity` -- violating the "core has no cloud SDK deps" constraint. So the import from `aca` is the right approach.

### AZF FaaS Overrides

AZF needs `NotImplementedError` for `ImageBuild` and `ImagePush`. Two approaches:

**Option A (preferred)**: AZF overrides just those methods:

```go
// backends/azure-functions/backend_impl.go

func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
    return nil, &api.NotImplementedError{
        Message: "Azure Functions backend does not support image build; push pre-built images to Azure Container Registry",
    }
}

func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
    return nil, &api.NotImplementedError{
        Message: "Azure Functions backend does not support image push; push images directly to Azure Container Registry",
    }
}

// All other 12 image methods delegate to s.images:
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
    return s.images.Pull(ref, auth)
}
// ... etc
```

**Option B**: `ImageManager` supports a `FaaSMode bool` flag that returns `NotImplementedError` for Build/Push. Less flexible, not recommended.

### AZF AuthLogin ACR Detection

AZF currently has a custom `AuthLogin` that logs a warning for ACR registries. This merges into `ImageManager.AuthLogin()` which checks `Auth.IsCloudRegistry()`. Both ACA and AZF get the warning automatically.

### AZF ImageLoad

AZF currently delegates `ImageLoad` to `BaseServer`. With `ImageManager`, it becomes `s.images.Load(r)` -- same behavior (BaseServer's tar parsing), no change.

---

## 4. What Gets Deleted

### Files Deleted Entirely

| File | Lines | Reason |
|------|-------|--------|
| `backends/aca/registry.go` | 184 | `fetchImageConfig()`, `parseImageRef()`, `getDockerHubToken()`, `getACRToken()` -- all replaced by `core.ImageManager` + `ACRAuthProvider.GetToken()` |
| `backends/aca/backend_impl_images.go` | 359 | `ImagePush`, `ImageTag`, `ImageRemove`, `pushToACR`, `uploadBlob`, `setAuthHeader` -- all replaced by `ImageManager` + `ACRAuthProvider.OnPush/OnTag/OnRemove` |
| `backends/aca/IMAGE_MANAGEMENT_PLAN.md` | 415 | Replaced by this plan |
| `backends/azure-functions/IMAGE_MANAGEMENT_PLAN.md` | 192 | Replaced by this plan |

### Functions/Methods Removed from Existing Files

| File | Function | Lines | Reason |
|------|----------|-------|--------|
| `backends/aca/backend_impl.go` | `ImagePull` | ~80 | Replaced by `s.images.Pull()` one-liner |
| `backends/aca/backend_impl.go` | `ImageLoad` | ~3 | Replaced by `s.images.Load()` one-liner |
| `backends/aca/backend_impl_pods.go` | `AuthLogin` | ~9 | Warning moves into `ImageManager.AuthLogin()` |
| `backends/azure-functions/backend_impl.go` | `ImagePull` | ~90 | Replaced by `s.images.Pull()` one-liner |
| `backends/azure-functions/backend_impl.go` | `ImageLoad` | ~3 | Replaced by `s.images.Load()` one-liner |
| `backends/azure-functions/backend_impl.go` | `AuthLogin` | ~9 | Warning moves into `ImageManager.AuthLogin()` |

### Delegates Removed from `backend_delegates_gen.go`

Both ACA and AZF `backend_delegates_gen.go` will have image delegates removed (they'll be replaced by explicit `s.images.Method()` calls in `backend_impl.go` or a new `backend_impl_images.go` one-liner file):

- ACA: `ImageBuild`, `ImageHistory`, `ImageInspect`, `ImageList`, `ImagePrune`, `ImageSave`, `ImageSearch` (7 delegates)
- AZF: `ImageHistory`, `ImageInspect`, `ImageList`, `ImagePrune`, `ImageRemove`, `ImageSave`, `ImageSearch`, `ImageTag` (8 delegates)

### Total Deletion Estimate

- **~543 lines deleted** from ACA (184 + 359)
- **~102 lines deleted** from AZF (ImagePull 90 + ImageLoad 3 + AuthLogin 9)
- **~15 delegates removed** from `backend_delegates_gen.go` files
- **~607 lines of old plan docs** deleted

---

## 5. What Gets Created

### New File: `backends/aca/image_auth.go` (~130 lines)

Contains `ACRAuthProvider` struct implementing `core.AuthProvider`:
- `IsCloudRegistry(registry string) bool`
- `GetToken(registry string) (string, error)`
- `OnPush(img api.Image, registry, repo, tag string) error`
- `OnTag(img api.Image, registry, repo, newTag string) error`
- `OnRemove(registry, repo string, tags []string) error`

### New File: `backends/core/image_manager.go` (~400 lines)

Contains `AuthProvider` interface and `ImageManager` struct with all 14 image methods + `AuthLogin`. This is created as part of the global architecture work (see `IMAGE_ARCHITECTURE.md`), not Azure-specific. Methods:
- `Pull`, `Inspect`, `Load`, `Tag`, `List`, `Remove`, `History`, `Prune`, `Build`, `Push`, `Save`, `Search`, `AuthLogin`

### Modified File: `backends/aca/server.go`

- Add `images *core.ImageManager` field to `Server` struct
- Initialize in `NewServer()`

### Modified File: `backends/azure-functions/server.go`

- Add `images *core.ImageManager` field to `Server` struct
- Add import of `github.com/sockerless/aca` for `ACRAuthProvider`
- Initialize in `NewServer()`

### New File: `backends/aca/backend_impl_images.go` (~40 lines, replaces old 359-line file)

14 one-liner delegates to `s.images`:

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
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
    return s.images.AuthLogin(req)
}
```

### New File: `backends/azure-functions/backend_impl_images.go` (~50 lines)

Same as ACA, but `ImageBuild` and `ImagePush` return `NotImplementedError`:

```go
package azf

// ... 12 one-liner delegates to s.images ...

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

### Modified File: `backends/core/oci_push.go`

Export `setOCIAuth` as `SetOCIAuth` (rename, 1 line change + update internal callers).

---

## 6. Migration Path (Step-by-Step)

### Step 1: Core ImageManager (prerequisite, not Azure-specific)

1. Create `backends/core/image_manager.go` with `AuthProvider` interface and `ImageManager` struct
2. Move image method logic from `BaseServer` into `ImageManager` methods
3. `BaseServer` creates `ImageManager{Auth: nil}` and delegates its own image methods to it
4. Export `SetOCIAuth` in `oci_push.go`
5. Run core tests -- must all pass

### Step 2: ACRAuthProvider

1. Create `backends/aca/image_auth.go` with `ACRAuthProvider`
2. Unit test: `IsCloudRegistry` returns true for `*.azurecr.io`, false for `docker.io`
3. Integration test: `GetToken` with simulator's fake credential (already works via `azidentity.NewDefaultAzureCredential`)

### Step 3: Wire ACA

1. Add `images *core.ImageManager` to ACA `Server` struct
2. Initialize in `NewServer()` with `&ACRAuthProvider{Logger: logger}`
3. Create new `backend_impl_images.go` with 14 one-liners
4. Delete old `backend_impl_images.go` (359 lines)
5. Delete `registry.go` (184 lines)
6. Remove `ImagePull` and `ImageLoad` from `backend_impl.go`
7. Remove `AuthLogin` from `backend_impl_pods.go`
8. Remove image delegates from `backend_delegates_gen.go`
9. Run ACA tests -- must all pass

### Step 4: Wire AZF

1. Add `images *core.ImageManager` to AZF `Server` struct
2. Add import of `aca.ACRAuthProvider`
3. Initialize in `NewServer()` with `&aca.ACRAuthProvider{Logger: logger}`
4. Create `backend_impl_images.go` with 12 delegates + 2 `NotImplementedError` overrides
5. Remove `ImagePull`, `ImageBuild`, `ImagePush`, `ImageLoad`, `AuthLogin` from `backend_impl.go`
6. Remove image delegates from `backend_delegates_gen.go`
7. Run AZF tests -- must all pass

### Step 5: Cleanup

1. Delete `backends/aca/IMAGE_MANAGEMENT_PLAN.md`
2. Delete `backends/azure-functions/IMAGE_MANAGEMENT_PLAN.md`
3. Run full test suite (`sim-test-all`, e2e tests)
4. Verify no remaining references to deleted functions

---

## 7. Test Impact

### Tests That Must Continue Passing

| Test Suite | Key Tests | Risk |
|-----------|-----------|------|
| `tests/system_test.go` | `TestImageBuild`, `TestImagePull` | Low -- `ImageManager.Build()` uses same BaseServer logic |
| ACA SDK tests (`simulators/azure/sdk-tests/`) | All image-related | Low -- behavior unchanged |
| AZF SDK tests (`simulators/azure/sdk-tests/`) | All image-related | Low -- behavior unchanged |
| Core unit tests (`backends/core/`) | `BaseServer` image tests | Low -- `BaseServer` delegates to `ImageManager{Auth: nil}` |
| `sim-test-all` (75 tests) | All 6 backends | Low -- all use same `ImageManager` |

### No New Tests Required for Azure

The existing test suites cover all image methods. The refactoring is purely structural (moving code, not changing behavior). The only new test surface is `ACRAuthProvider.IsCloudRegistry()` which is trivially testable.

### Behavioral Invariants

1. **ACA ImagePull**: Still fetches real image config from registries. `ImageManager.Pull()` calls `core.FetchImageConfig()` (cached) with auth from `ACRAuthProvider.GetToken()`. Same result, better code.
2. **ACA ImagePush to ACR**: Still pushes synthetic manifest+config to ACR via OCI Distribution API. `ImageManager.Push()` calls `ACRAuthProvider.OnPush()` which calls `core.OCIPush()`. Same result.
3. **ACA ImagePush to non-ACR**: Falls through to BaseServer synthetic progress. Same result.
4. **ACA ImageTag to ACR**: Still syncs tag via GET manifest + PUT with new tag. `ImageManager.Tag()` calls `ACRAuthProvider.OnTag()`. Same result.
5. **ACA ImageRemove from ACR**: Still deletes manifest. `ImageManager.Remove()` calls `ACRAuthProvider.OnRemove()`. Same result.
6. **AZF ImageBuild/ImagePush**: Still return `NotImplementedError`. Overridden at the `Server` level.
7. **AZF ImagePull**: Still uses `core.FetchImageConfig()`. Same result.
8. **AuthLogin ACR warning**: Still logged for `*.azurecr.io` registries. Now handled in `ImageManager.AuthLogin()`.

### ACR Simulator

No changes needed. The ACR simulator (`simulators/azure/acr.go`) already supports all OCI Distribution operations used by `ACRAuthProvider`:
- `GET/HEAD /v2/{name}/manifests/{ref}` -- used by `OnTag`, `OnRemove`
- `PUT /v2/{name}/manifests/{ref}` -- used by `OnTag`
- `POST /v2/{name}/blobs/uploads/` + `PUT` -- used by `OnPush` via `core.OCIPush`
- `HEAD /v2/{name}/blobs/{digest}` -- used by `OnPush` via `core.OCIPush`

No `DELETE /v2/{name}/manifests/{digest}` handler exists in the ACR simulator. This needs to be added (~15 lines) for `OnRemove` to work against the simulator. Currently `OnRemove` errors are non-fatal (logged, not returned), so tests pass even without it, but adding it is correct.

---

## 8. Dependency Graph

```
Step 1: core/image_manager.go  (no Azure deps)
    |
    v
Step 2: aca/image_auth.go  (ACRAuthProvider)
    |
    +--> Step 3: aca/server.go + backend_impl_images.go  (wire ACA)
    |
    +--> Step 4: azf/server.go + backend_impl_images.go  (wire AZF, imports aca)
    |
    v
Step 5: Cleanup (delete old plans, old files)
```

Steps 3 and 4 are independent of each other (can be done in parallel or either order). Step 2 depends on Step 1. Step 5 depends on Steps 3+4.

---

## Review Notes

The following issues were identified during review and must be resolved before or during implementation.

### Issue 1 (CRITICAL): ACA and AZF Are Separate Go Modules

The plan states "both backends are in the same Go module" (Section 3, paragraph 2) and shows AZF importing `github.com/sockerless/aca`. This is **wrong**. They are separate Go modules:

- ACA: `module github.com/sockerless/backend-aca` (in `backends/aca/go.mod`)
- AZF: `module github.com/sockerless/backend-azf` (in `backends/azure-functions/go.mod`)

AZF currently has **zero** dependency on the ACA module. Adding one requires:
1. Adding `require github.com/sockerless/backend-aca v0.0.0` to `backends/azure-functions/go.mod`
2. Adding `replace github.com/sockerless/backend-aca => ../aca` to `backends/azure-functions/go.mod`
3. Verifying the `go.work` file already includes both modules (it likely does, but must be checked)
4. The import path would be `github.com/sockerless/backend-aca`, not `github.com/sockerless/aca`

**Alternative (recommended, matching GCP precedent)**: The GCP plan (CloudRun + GCF) faced the identical problem and chose **Option B: duplicate the AuthProvider** (~80 lines). Since `ACRAuthProvider` is ~130 lines with no ACA-specific type dependencies, duplicating it in `backends/azure-functions/image_auth.go` avoids the cross-module coupling. Both copies import only `azidentity`, `azcore/policy`, and `core` -- all of which AZF already depends on.

**Decision required**: Duplicate (recommended) or cross-module import (requires go.mod changes).

### Issue 2 (CRITICAL): `core.FetchImageConfig` Auth Token Scheme Mismatch

`core.FetchImageConfig(ref string, basicAuth ...string)` passes `basicAuth[0]` to `getRegistryToken()`, which sets:
```go
req.Header.Set("Authorization", "Basic "+basicAuth)
```

But `ACRAuthProvider.GetToken()` returns `"Bearer " + token.Token`. If `ImageManager.Pull()` passes this Bearer token as the `basicAuth` parameter, the Authorization header becomes `"Basic Bearer xxx"` -- double-prefixed and wrong scheme.

The ECS plan (Section 2, "Auth Threading") identified this exact issue and proposed that `GetToken()` should return the raw token without any scheme prefix, letting `ImageManager` handle prefixing.

**Resolution**: The `ImageManager.Pull()` implementation must handle this. Options:
1. `GetToken()` returns the raw token (no `"Bearer "` prefix). `ImageManager` adds the correct prefix based on context. This changes the `AuthProvider` contract and affects all 3 clouds.
2. `ImageManager.Pull()` does not pass the cloud token through `FetchImageConfig`'s `basicAuth` parameter. Instead, it uses a different code path for cloud registries (direct HTTP with the Bearer token) vs non-cloud registries (`FetchImageConfig` with Www-Authenticate flow).
3. Refactor `FetchImageConfig` to accept a full `Authorization` header value (not just a basic auth credential).

This is a **core ImageManager design decision**, not Azure-specific. The ACA plan must acknowledge this dependency instead of assuming `ImageManager.Pull()` "just works" with `ACRAuthProvider.GetToken()`.

### Issue 3: ACR Simulator Has No `Www-Authenticate` Challenge

`core.FetchImageConfig()` discovers auth by hitting the manifest endpoint and expecting a 401 response with a `Www-Authenticate` header (see `registry.go:150-177`). The ACR simulator (`simulators/azure/acr.go`) does NOT return 401 or set `Www-Authenticate` -- it returns 200 directly for all OCI endpoints.

**Impact**: For simulator-based testing, `FetchImageConfig` will succeed without auth (gets 200 on the first attempt, skips token exchange entirely). This means tests will pass even if the auth wiring is broken. For real ACR, which returns 401 with proper `Www-Authenticate` headers, the flow would work IF Issue 2 is resolved.

**Recommendation**: Add a note that ACR simulator testing does not exercise the auth path. Consider optionally adding `Www-Authenticate` challenge support to the ACR simulator for more realistic testing.

### Issue 4: Missing DELETE Manifest Handler in ACR Simulator

The plan correctly notes (Section 7, ACR Simulator) that no `DELETE /v2/{name}/manifests/{digest}` handler exists. However, this is not listed as a migration prerequisite in Section 6 (Migration Path).

**Fix**: Add a Step 1.5 or prerequisite to Step 3: "Add DELETE manifest handler to ACR simulator (`simulators/azure/acr.go`, ~15 lines). Without this, `OnRemove` silently fails against the simulator." While `OnRemove` errors are non-fatal, having the handler enables proper integration testing.

### Issue 5: Method Count and AuthLogin Inconsistency

The plan title says "14 image methods" and the Section 2 table lists 13 entries (rows 1-13, including AuthLogin). The global architecture (`IMAGE_ARCHITECTURE.md`) defines 12 methods on `ImageManager` -- AuthLogin is NOT included. The ECS plan (Section 2) explicitly states: "Note: `AuthLogin` (the 13th image-adjacent method) is NOT part of `ImageManager`."

The ACA plan should decide:
- If AuthLogin is in `ImageManager`: all 3 clouds must agree, and the `AuthProvider` interface needs an auth-related hook.
- If AuthLogin stays on `Server` (matching ECS/GCP precedent): the plan should list 12 image methods delegated to `ImageManager`, with AuthLogin handled separately.

**Recommendation**: Match the ECS/GCP precedent -- keep AuthLogin on the `Server` directly (it's a thin wrapper around `BaseServer.AuthLogin` with an ACR warning log). Update the plan to say "12 image methods" delegated to `ImageManager`, plus AuthLogin handled separately.

### Issue 6: Missing `context` Import in GetToken Code Sample

The `GetToken` method uses `context.Background()` but the import block on line 16-31 does not include `"context"`. This is a minor code sample error.

### Issue 7: `ImageManager.Logger` Type

The global architecture shows `Logger *slog.Logger` on `ImageManager`, but all backends (ACA, AZF, ECS, CloudRun) use `zerolog.Logger`. The plan shows `Logger: logger` where `logger` is `zerolog.Logger`. The core `ImageManager` must use whichever logger the project standardizes on. This is a core design decision that affects all plans equally.

### Issue 8: ACA ImagePull Metadata Preservation

ACA's current `ImagePull` (lines 863-941 of `backend_impl.go`) adds detailed metadata to the stored image:
- `RepoDigests` with computed digest
- `RootFS` with a synthetic layer hash
- `GraphDriver` with overlay2 paths
- `Metadata.LastTagTime`
- Deterministic `Size` via FNV hash

The plan's Section 7 states `ImageManager.Pull()` preserves this behavior, but `ImageManager` is a generic component shared across all backends. If `ImageManager.Pull()` includes this metadata polish (as it should, since all backends currently do the same thing), the behavior is preserved. If not, ACA would need to override `ImagePull` instead of delegating, defeating the purpose.

**Action**: Verify during core ImageManager implementation that `Pull()` includes the full metadata polish (RepoDigests, RootFS, GraphDriver, Metadata). All 6 backends currently generate these fields identically.

### Issue 9: Cross-Cloud AuthProvider Interface Compatibility

All three cloud plans use the same `AuthProvider` interface from `IMAGE_ARCHITECTURE.md`:
```go
type AuthProvider interface {
    GetToken(registry string) (string, error)
    IsCloudRegistry(registry string) bool
    OnPush(img api.Image, registry, repo, tag string) error
    OnTag(img api.Image, registry, repo, newTag string) error
    OnRemove(registry, repo string, tags []string) error
}
```

The method signatures are consistent across ECS, CloudRun, and ACA plans. The `GetToken` return value semantics differ:
- ECS: returns raw base64 (no scheme prefix)
- CloudRun: returns `"Bearer " + token`
- ACA: returns `"Bearer " + token`

This inconsistency must be resolved in the core `ImageManager` implementation (see Issue 2). The `AuthProvider` contract must document whether `GetToken` returns a raw token or a prefixed value.

### Issue 10: `OnRemove` Status Code Handling

The current ACA `ImageRemove` (line 351 of `backend_impl_images.go`) checks for status codes `202` or `200`. The plan's `OnRemove` method (line 153-163) accepts any non-error response silently (returns nil on errors via `continue`). The GCP `OnRemove` plan checks for `202`, `200`, or `404`. These should be aligned.

### Summary of Required Changes Before Implementation

| Priority | Issue | Action |
|----------|-------|--------|
| CRITICAL | Separate Go modules | Choose: duplicate ACRAuthProvider in AZF (recommended) or add cross-module dependency with go.mod changes |
| CRITICAL | FetchImageConfig auth mismatch | Core ImageManager must handle Bearer vs Basic token schemes; ACA plan must not assume it "just works" |
| HIGH | Missing DELETE handler in ACR simulator | Add as prerequisite step before Step 3 |
| MEDIUM | Method count (12 vs 14) | Align with ECS/GCP: 12 ImageManager methods + AuthLogin on Server |
| LOW | Missing context import | Fix code sample |
| LOW | Logger type | Core decision, not Azure-specific |
| LOW | Metadata preservation | Verify during core ImageManager implementation |
