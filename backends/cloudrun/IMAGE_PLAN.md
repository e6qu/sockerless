# GCP Unified Image Management Plan (CloudRun + GCF)

This plan implements the architecture described in `backends/IMAGE_ARCHITECTURE.md` for the GCP cloud (CloudRun and CloudRun Functions backends).

---

## 1. ARAuthProvider

### 1.1 Location

**File:** `backends/cloudrun/image_auth.go` (~80 lines)

This file lives in the `cloudrun` package. The GCF backend imports and reuses it (see Section 3).

### 1.2 Struct Definition

```go
package cloudrun

import (
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/sockerless/api"
    core "github.com/sockerless/backend-core"
    "golang.org/x/oauth2/google"
    "context"
)

// ARAuthProvider implements core.AuthProvider for GCP Artifact Registry and GCR.
type ARAuthProvider struct {
    // Ctx is used for ADC credential lookups.
    Ctx context.Context
}
```

### 1.3 Method Implementations

#### `GetToken(registry string) (string, error)`

Returns a Bearer token for GCP registries via Application Default Credentials. Returns `("", nil)` for non-GCP registries (fall-through to core's Www-Authenticate token exchange).

```go
func (a *ARAuthProvider) GetToken(registry string) (string, error) {
    if !a.IsCloudRegistry(registry) {
        return "", nil // not a GCP registry -- let core handle auth
    }
    creds, err := google.FindDefaultCredentials(a.Ctx, "https://www.googleapis.com/auth/cloud-platform")
    if err != nil {
        return "", fmt.Errorf("find default credentials: %w", err)
    }
    token, err := creds.TokenSource.Token()
    if err != nil {
        return "", fmt.Errorf("get token: %w", err)
    }
    return "Bearer " + token.AccessToken, nil
}
```

This replaces both:
- `cloudrun.Server.getARToken()` (registry.go:155-165)
- `gcf.Server.getARToken()` (registry.go:40-50)

#### `IsCloudRegistry(registry string) bool`

Matches `*.gcr.io` and `*-docker.pkg.dev` patterns. Equivalent to the existing `core.IsGCPRegistry()` helper.

```go
func (a *ARAuthProvider) IsCloudRegistry(registry string) bool {
    return strings.HasSuffix(registry, ".gcr.io") || strings.HasSuffix(registry, "-docker.pkg.dev")
}
```

#### `OnPush(img api.Image, registry, repo, tag string) error`

Pushes a synthetic image to AR/GCR via the OCI Distribution API using `core.OCIPush`. This replaces the inline OCI push logic in `cloudrun/backend_impl.go:1311-1362` and `gcf/backend_impl.go:972-1023`.

```go
func (a *ARAuthProvider) OnPush(img api.Image, registry, repo, tag string) error {
    token, err := a.GetToken(registry)
    if err != nil {
        return fmt.Errorf("get token for push: %w", err)
    }
    _, err = core.OCIPush(core.OCIPushOptions{
        Registry:   registry,
        Repository: repo,
        Tag:        tag,
        AuthToken:  token,
    })
    return err
}
```

#### `OnTag(img api.Image, registry, repo, newTag string) error`

Re-PUTs the manifest with a new tag via OCI. This replaces the inline goroutine logic in `cloudrun/backend_impl.go:1366-1400`.

```go
func (a *ARAuthProvider) OnTag(img api.Image, registry, repo, newTag string) error {
    token, err := a.GetToken(registry)
    if err != nil {
        return fmt.Errorf("get token for tag: %w", err)
    }
    _, err = core.OCIPush(core.OCIPushOptions{
        Registry:   registry,
        Repository: repo,
        Tag:        newTag,
        AuthToken:  token,
    })
    return err
}
```

#### `OnRemove(registry, repo string, tags []string) error`

DELETEs the manifest from AR/GCR via the OCI Distribution API. This replaces the inline goroutine logic in `cloudrun/backend_impl.go:1404-1453`.

```go
func (a *ARAuthProvider) OnRemove(registry, repo string, tags []string) error {
    token, err := a.GetToken(registry)
    if err != nil {
        return fmt.Errorf("get token for remove: %w", err)
    }
    client := &http.Client{Timeout: 30 * time.Second}
    for _, tag := range tags {
        deleteURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
        req, err := http.NewRequest(http.MethodDelete, deleteURL, nil)
        if err != nil {
            return fmt.Errorf("create delete request: %w", err)
        }
        core.SetOCIAuth(req, token) // needs to be exported (see Section 5.1)
        resp, err := client.Do(req)
        if err != nil {
            return fmt.Errorf("delete manifest: %w", err)
        }
        resp.Body.Close()
        if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
            return fmt.Errorf("delete manifest returned %d", resp.StatusCode)
        }
    }
    return nil
}
```

---

## 2. Core ImageManager (created separately, referenced here for context)

Per `IMAGE_ARCHITECTURE.md`, `backends/core/image_manager.go` defines:

```go
type AuthProvider interface {
    GetToken(registry string) (string, error)
    IsCloudRegistry(registry string) bool
    OnPush(img api.Image, registry, repo, tag string) error
    OnTag(img api.Image, registry, repo, newTag string) error
    OnRemove(registry, repo string, tags []string) error
}

type ImageManager struct {
    Auth   AuthProvider  // nil = no cloud integration
    Store  *Store
    Logger *slog.Logger
}
```

The `ImageManager` wraps the existing `BaseServer` image method logic and adds hooks for cloud-specific operations via `AuthProvider`. The 12 methods it provides (with signatures matching `api.Backend`):

| ImageManager Method | Core Logic Source | Cloud Hook |
|---|---|---|
| `Pull(ref, auth)` | `BaseServer.ImagePull` + `core.FetchImageConfig` (443-line registry.go) | `Auth.GetToken()` for registry auth |
| `Inspect(name)` | `BaseServer.ImageInspect` | none |
| `Load(r)` | `BaseServer.ImageLoad` | none |
| `Tag(source, repo, tag)` | `BaseServer.ImageTag` | `Auth.OnTag()` (best-effort, goroutine) |
| `List(opts)` | `BaseServer.ImageList` | none |
| `Remove(name, force, prune)` | `BaseServer.ImageRemove` | `Auth.OnRemove()` (best-effort, goroutine) |
| `History(name)` | `BaseServer.ImageHistory` | none |
| `Prune(filters)` | `BaseServer.ImagePrune` | none |
| `Build(opts, ctx)` | `BaseServer.ImageBuild` | none |
| `Push(name, tag, auth)` | `BaseServer.ImagePush` | `Auth.OnPush()` (replaces synthetic for cloud registries) |
| `Save(names)` | `BaseServer.ImageSave` | none |
| `Search(term, limit, filters)` | `BaseServer.ImageSearch` | none |

`AuthLogin` and `ContainerCommit` remain on the backend directly (not in ImageManager). These are not image-manager methods and are not counted in the 12 above.

---

## 3. CloudRun Wiring

### 3.1 Server Struct Change

Add `images *core.ImageManager` field to `Server`:

```go
// In backends/cloudrun/server.go
type Server struct {
    *core.BaseServer
    config       Config
    gcp          *GCPClients
    ipCounter    atomic.Int32
    images       *core.ImageManager  // NEW

    CloudRun     *core.StateStore[CloudRunState]
    NetworkState *core.StateStore[NetworkState]
    VolumeState  *core.StateStore[VolumeState]
}
```

### 3.2 NewServer Wiring

Create `ImageManager` with `ARAuthProvider` in `NewServer()`:

```go
func NewServer(config Config, gcpClients *GCPClients, logger zerolog.Logger) *Server {
    s := &Server{
        config:       config,
        gcp:          gcpClients,
        // ... existing fields ...
    }

    s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{...}, logger)
    s.SetSelf(s)

    // Wire up image manager with GCP auth
    s.images = core.NewImageManager(
        &ARAuthProvider{Ctx: context.Background()},
        s.Store,
        logger,
    )

    // ... rest of existing NewServer ...
    return s
}
```

### 3.3 Image Method Delegation (12 methods)

All 12 image methods on CloudRun `Server` become one-line delegates to `s.images`:

```go
// In backends/cloudrun/backend_impl.go (replacing current implementations)

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

**AuthLogin and ContainerCommit** stay as-is on `Server` (AuthLogin has GCR detection warning logic; ContainerCommit returns NotImplementedError). These are not image-manager methods.

### 3.4 What Moves Out of `backend_impl.go`

The following methods are replaced by one-line delegates:
- `ImagePull` (lines 944-1003, ~60 lines) -- replaced by `s.images.Pull()`
- `ImageLoad` (lines 1007-1009, ~3 lines) -- replaced by `s.images.Load()`
- `ImagePush` (lines 1311-1362, ~52 lines) -- replaced by `s.images.Push()`
- `ImageTag` (lines 1366-1400, ~35 lines) -- replaced by `s.images.Tag()`
- `ImageRemove` (lines 1404-1453, ~50 lines) -- replaced by `s.images.Remove()`

The following move from `backend_delegates_gen.go` to inline delegates in `backend_impl.go`:
- `ImageBuild`, `ImageHistory`, `ImageInspect`, `ImageList`, `ImagePrune`, `ImageSave`, `ImageSearch`

These 7 delegates change from `s.BaseServer.ImageX()` to `s.images.X()`.

### 3.5 What Stays in `backend_delegates_gen.go`

All non-image delegates remain unchanged (Container, Network, Pod, Volume, System, Exec methods).

---

## 4. GCF (Cloud Run Functions) Sharing

### 4.1 Sharing Approach

GCF imports `ARAuthProvider` from the `cloudrun` package. Both backends are in the same Go module (`github.com/sockerless/backend-cloudrun` and `github.com/sockerless/backend-cloudrun-functions`), but they are separate modules.

**Option A (preferred): Shared `image_auth.go` package**

Move `ARAuthProvider` to a shared package that both can import. Since both already import `github.com/sockerless/backend-core`, the cleanest approach is to keep `ARAuthProvider` in the cloudrun package and have GCF depend on the cloudrun module.

However, adding a module dependency from GCF to CloudRun may be undesirable. Instead:

**Option B: Duplicate ARAuthProvider in GCF (~80 lines)**

Create `backends/cloudrun-functions/image_auth.go` as a copy. The struct is tiny (~80 lines) and has no dependencies on cloudrun-specific types. The only dependency is `golang.org/x/oauth2/google` (already imported by GCF) and `core.OCIPush` (already imported).

**Option C (recommended): Extract to a shared GCP auth package**

Create `backends/gcp/image_auth.go` in a new `github.com/sockerless/backend-gcp` module that both cloudrun and GCF import:

```
backends/gcp/
    go.mod              # module github.com/sockerless/backend-gcp
    image_auth.go       # ARAuthProvider struct (~80 lines)
```

Both `cloudrun/go.mod` and `cloudrun-functions/go.mod` add:
```
require github.com/sockerless/backend-gcp v0.0.0
replace github.com/sockerless/backend-gcp => ../gcp
```

**Decision**: Use **Option B** (duplicate). The struct is ~80 lines with zero cloudrun-specific types. Duplication avoids a new module, a new go.work entry, and cross-module coupling. Both copies are identical and can be validated by a linter or test.

### 4.2 GCF Server Struct Change

```go
// In backends/cloudrun-functions/server.go
type Server struct {
    *core.BaseServer
    config    Config
    gcp       *GCPClients
    ipCounter atomic.Int32
    images    *core.ImageManager  // NEW

    GCF *core.StateStore[GCFState]
}
```

### 4.3 GCF NewServer Wiring

```go
func NewServer(config Config, gcpClients *GCPClients, logger zerolog.Logger) *Server {
    s := &Server{...}
    s.BaseServer = core.NewBaseServer(...)
    s.SetSelf(s)

    s.images = core.NewImageManager(
        &ARAuthProvider{Ctx: context.Background()},
        s.Store,
        logger,
    )

    // ...
    return s
}
```

### 4.4 GCF FaaS Overrides

GCF overrides `ImageBuild` with `NotImplementedError` (FaaS backends cannot build images). This is handled by the GCF `Server` method taking precedence over the `ImageManager` delegate:

```go
// In backends/cloudrun-functions/backend_impl.go (KEEP existing)
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
    return nil, &api.NotImplementedError{
        Message: "Cloud Run Functions backend does not support image build; push pre-built images to Artifact Registry",
    }
}
```

All other image methods delegate to `s.images`:

```go
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
    return s.images.Pull(ref, auth)
}

func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
    return s.images.Load(r)
}

func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
    return s.images.Push(name, tag, auth)
}

// ImageTag, ImageList, ImageRemove, ImageHistory, ImagePrune, ImageSave, ImageSearch
// all delegate to s.images.X()
```

### 4.5 GCF Delegates Migration

Move from `backend_delegates_gen.go` to `backend_impl.go`:
- `ImageHistory`, `ImageInspect`, `ImageList`, `ImagePrune`, `ImageRemove`, `ImageSave`, `ImageSearch`, `ImageTag`

Change from `s.BaseServer.ImageX()` to `s.images.X()`.

---

## 5. What Gets Deleted

### 5.1 Files Deleted Entirely

| File | Lines | Reason |
|------|-------|--------|
| `backends/cloudrun/registry.go` | 185 | `fetchImageConfig()`, `parseImageRef()`, `getARToken()`, `getDockerHubToken()` all replaced by `core.ImageManager` + `ARAuthProvider` |
| `backends/cloudrun-functions/registry.go` | 50 | `parseImageRef()`, `getARToken()` replaced by `ARAuthProvider` + `core.parseImageRef()` |
| `backends/cloudrun/IMAGE_MANAGEMENT_PLAN.md` | 500 | Replaced by this file |
| `backends/cloudrun-functions/IMAGE_MANAGEMENT_PLAN.md` | 148 | Replaced by this file |

### 5.2 Code Removed from Existing Files

| File | What | Lines Removed |
|------|------|---------------|
| `backends/cloudrun/backend_impl.go` | `ImagePull` (full), `ImagePush` (full), `ImageTag` (full), `ImageRemove` (full), `ImageLoad` (just body change) | ~200 lines |
| `backends/cloudrun/backend_delegates_gen.go` | `ImageBuild`, `ImageHistory`, `ImageInspect`, `ImageList`, `ImagePrune`, `ImageSave`, `ImageSearch` delegates | ~28 lines |
| `backends/cloudrun-functions/backend_impl.go` | `ImagePull` (full), `ImagePush` (full), `ImageLoad` (full) | ~120 lines |
| `backends/cloudrun-functions/backend_delegates_gen.go` | `ImageHistory`, `ImageInspect`, `ImageList`, `ImagePrune`, `ImageRemove`, `ImageSave`, `ImageSearch`, `ImageTag` delegates | ~32 lines |

### 5.3 Estimated Impact

- **Deleted**: ~185 + 50 + 200 + 28 + 120 + 32 = **~615 lines**
- **Added**: ~80 (ARAuthProvider in cloudrun) + ~80 (ARAuthProvider copy in GCF) + ~50 (one-liner delegates in cloudrun) + ~50 (one-liner delegates in GCF) = **~260 lines**
- **Net reduction**: ~355 lines from GCP backends

---

## 6. What Gets Created

### 6.1 New Files

| File | Package | Contents | Lines |
|------|---------|----------|-------|
| `backends/cloudrun/image_auth.go` | `cloudrun` | `ARAuthProvider` struct + 5 methods | ~80 |
| `backends/cloudrun-functions/image_auth.go` | `gcf` | `ARAuthProvider` struct + 5 methods (identical copy) | ~80 |

### 6.2 Required Changes in Core (prerequisite)

These changes are made as part of the core `ImageManager` work (not GCP-specific), but are required:

1. **`backends/core/image_manager.go`** (NEW): `AuthProvider` interface + `ImageManager` struct + 12 methods + constructor
2. **`backends/core/registry.go`**: Must support pre-authenticated tokens (see Review Notes Issue 2). Currently `FetchImageConfig(ref, basicAuth)` always does Www-Authenticate token exchange; a cloud Bearer token cannot be passed through the existing `basicAuth` parameter without corruption. The core ImageManager must add a code path that accepts a ready-to-use auth token and passes it directly to `registryGet()`.
3. **`backends/core/oci_push.go`**: Either export `setOCIAuth` as `SetOCIAuth` or inline the auth-header logic in `ARAuthProvider.OnRemove()` (see Review Notes Issue 6 — recommend inline).
4. **`backends/core/registry.go`**: The `parseImageRef` function is already package-private; `ImageManager` uses it internally. No export needed since `ImageManager.Pull()` calls it internally.

### 6.3 Contents Outline: `image_auth.go`

```go
package cloudrun // (or gcf for the copy)

import (
    "context"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/sockerless/api"
    core "github.com/sockerless/backend-core"
    "golang.org/x/oauth2/google"
)

// ARAuthProvider implements core.AuthProvider for GCP Artifact Registry and GCR.
type ARAuthProvider struct {
    Ctx context.Context
}

func (a *ARAuthProvider) GetToken(registry string) (string, error) { ... }
func (a *ARAuthProvider) IsCloudRegistry(registry string) bool { ... }
func (a *ARAuthProvider) OnPush(img api.Image, registry, repo, tag string) error { ... }
func (a *ARAuthProvider) OnTag(img api.Image, registry, repo, newTag string) error { ... }
func (a *ARAuthProvider) OnRemove(registry, repo string, tags []string) error { ... }
```

---

## 7. Migration Path (Step-by-Step)

### Step 1: Create Core ImageManager (prerequisite, not GCP-specific)

1. Create `backends/core/image_manager.go` with `AuthProvider` interface and `ImageManager` struct
2. Move image method logic from `BaseServer` into `ImageManager` methods
3. `BaseServer` creates a default `ImageManager{Auth: nil, Store: s.Store, Logger: s.Logger}` and its image methods delegate to it
4. Export `setOCIAuth` as `SetOCIAuth` in `oci_push.go`
5. Verify all existing tests pass (BaseServer behavior unchanged)

### Step 1b: Add OCI Manifest DELETE to GCP Simulator (prerequisite)

1. Add `case http.MethodDelete:` to `handleOCIManifest()` in `simulators/gcp/artifactregistry.go`
2. Delete the manifest from the state store, return 202 Accepted
3. This is required for `ARAuthProvider.OnRemove()` to work in integration tests
4. ~10 lines of code

### Step 2: Create ARAuthProvider in CloudRun

1. Create `backends/cloudrun/image_auth.go` with `ARAuthProvider`
2. Unit test: verify `IsCloudRegistry` matches expected patterns
3. Verify compilation

### Step 3: Wire CloudRun ImageManager

1. Add `images *core.ImageManager` to `Server` struct in `server.go`
2. Initialize in `NewServer()` with `ARAuthProvider{Ctx: context.Background()}`
3. Replace all image method implementations in `backend_impl.go` with one-liner `s.images.X()` delegates
4. Remove image delegates from `backend_delegates_gen.go` (they move to `backend_impl.go` as `s.images.X()` calls)
5. Run CloudRun integration tests -- verify all pass

### Step 4: Delete CloudRun registry.go

1. Delete `backends/cloudrun/registry.go` (185 lines)
2. Verify compilation (no references to `fetchImageConfig`, `parseImageRef`, `getARToken`, `getDockerHubToken` remain)
3. Run CloudRun integration tests

### Step 5: Create ARAuthProvider in GCF (copy)

1. Create `backends/cloudrun-functions/image_auth.go` (copy of cloudrun's, package name `gcf`)
2. Verify compilation

### Step 6: Wire GCF ImageManager

1. Add `images *core.ImageManager` to `Server` struct in `server.go`
2. Initialize in `NewServer()` with `ARAuthProvider{Ctx: context.Background()}`
3. Replace image method implementations in `backend_impl.go`:
   - `ImagePull`: change from inline implementation to `s.images.Pull()`
   - `ImagePush`: change from inline OCI push to `s.images.Push()`
   - `ImageLoad`: change from `s.BaseServer.ImageLoad()` to `s.images.Load()`
   - `ImageBuild`: KEEP as `NotImplementedError` (FaaS override)
4. Move image delegates from `backend_delegates_gen.go` to `backend_impl.go` as `s.images.X()` calls
5. Run GCF integration tests

### Step 7: Delete GCF registry.go

1. Delete `backends/cloudrun-functions/registry.go` (50 lines)
2. Verify compilation
3. Run GCF integration tests

### Step 8: Delete Old Plans

1. Delete `backends/cloudrun/IMAGE_MANAGEMENT_PLAN.md`
2. Delete `backends/cloudrun-functions/IMAGE_MANAGEMENT_PLAN.md`

### Step 9: Full Test Validation

1. Run `tests/` e2e tests (TestImageBuild, TestImagePull, TestImageInspect, TestImageTag)
2. Run CloudRun SDK tests (`simulators/gcp/sdk-tests/`)
3. Run GCF integration tests
4. Run `sim-test-all` if applicable

---

## 8. Test Impact

### 8.1 Existing Tests -- No Breakage Expected

| Test Suite | Impact | Reason |
|---|---|---|
| `tests/images_test.go` (TestImagePull, TestImageInspect, TestImageTag) | None | These run against BaseServer, which uses `ImageManager{Auth: nil}` -- identical behavior |
| `tests/system_test.go` (TestImageBuild) | None | ImageBuild delegates through ImageManager to same BaseServer logic |
| CloudRun integration tests | None | `ImageManager.Pull()` uses same `core.FetchImageConfig()` + `core.StoreImageWithAliases()` logic |
| GCF integration tests | None | Same as above; ImageBuild still returns NotImplementedError |
| GCP SDK tests (sdk-tests/) | None | These test the GCP simulator, not the backend image methods |

### 8.2 Behavioral Changes

1. **CloudRun ImagePull**: Currently uses its own `fetchImageConfig()` (185-line registry.go). After migration, uses `core.FetchImageConfig()` (443-line registry.go) which is **superior** -- handles manifest lists, parses Www-Authenticate, has caching. Net improvement.

2. **GCF ImagePull**: Currently uses `core.FetchImageConfig()` with ADC auth. After migration, `ImageManager.Pull()` calls `Auth.GetToken()` and passes it to `core.FetchImageConfig()`. Identical behavior but cleaner.

3. **CloudRun ImagePush/ImageTag/ImageRemove**: Currently have inline AR sync logic with goroutines. After migration, `ImageManager` calls `Auth.OnPush()`/`OnTag()`/`OnRemove()` in goroutines with error logging. Identical behavior.

4. **GCF ImagePush**: Currently uses the same inline OCI push pattern. After migration, delegates to `ImageManager.Push()` which calls `Auth.OnPush()`. Identical behavior.

5. **GCF ImageTag** (NEW behavior): Currently GCF delegates `ImageTag` to `BaseServer.ImageTag()` with NO AR sync. After migration, `ImageManager.Tag()` calls `Auth.OnTag()`, which syncs the new tag to AR. This is a new capability for GCF — consistent with CloudRun's existing behavior. Net improvement but should be noted as a behavioral change.

6. **GCF ImageRemove** (NEW behavior): Currently GCF delegates `ImageRemove` to `BaseServer.ImageRemove()` with NO AR sync. After migration, `ImageManager.Remove()` calls `Auth.OnRemove()`, which deletes from AR. Same situation as ImageTag — new capability, consistent with CloudRun.

### 8.3 New Tests to Add (optional, during implementation)

- Unit test for `ARAuthProvider.IsCloudRegistry()` -- verify `us-docker.pkg.dev`, `gcr.io`, `us.gcr.io` match; `docker.io`, `ecr.amazonaws.com` do not
- Integration test: push to AR simulator, pull back, verify round-trip (if AR simulator is configured in test harness)

---

## 9. Dependency Graph

```
Step 1 (core ImageManager)  ----+-- PREREQUISITE, blocks everything
Step 1b (GCP sim DELETE)  ------+-- PREREQUISITE for OnRemove testing
    |
    v
Step 2 (ARAuthProvider in cloudrun)
    |
    v
Step 3 (Wire CloudRun)  --------> Step 4 (Delete cloudrun/registry.go)
    |
    v
Step 5 (ARAuthProvider copy in GCF)
    |
    v
Step 6 (Wire GCF)  ------------> Step 7 (Delete gcf/registry.go)
    |
    v
Step 8 (Delete old plans)
    |
    v
Step 9 (Full test validation)
```

Steps 2-4 (CloudRun) and Steps 5-7 (GCF) could be parallelized after Step 1, but sequential ordering is safer.

---

## 10. Open Questions

1. **`core.IsGCPRegistry()` redundancy**: After `ARAuthProvider.IsCloudRegistry()` exists, `core.IsGCPRegistry()` in `oci_push.go:237-239` becomes redundant. Keep it for backward compat or remove? **Recommendation**: Keep it -- it's 3 lines and other packages may use it.

2. **`core.SetOCIAuth()` export**: Currently `setOCIAuth` is unexported. `ARAuthProvider.OnRemove()` needs it. Alternative: inline the auth header logic (4 lines). **Recommendation**: Export it -- it's a clean utility function.

3. **`ARAuthProvider.Ctx` vs parameter**: Should `GetToken` take a `context.Context` parameter instead of storing it? The `AuthProvider` interface in `IMAGE_ARCHITECTURE.md` does not include context. **Recommendation**: Keep `Ctx` on struct -- simpler interface, and the context is always `context.Background()` in practice.

---

## Review Notes

Added during review on 2026-03-07. Issues found and their resolutions:

### Issue 1: Method Count — "14" Should Be "12"

The plan repeatedly says "14 image methods" (Sections 2, 3.3 heading, 3.4) but `api.Backend` defines exactly **12** image methods: ImagePull, ImageInspect, ImageLoad, ImageTag, ImageList, ImageRemove, ImageHistory, ImagePrune, ImageBuild, ImagePush, ImageSave, ImageSearch. The architecture doc (`IMAGE_ARCHITECTURE.md` line 90) also says 14 but lists 12. The GCP plan's Section 3.3 code block correctly shows 12 one-liner delegates, so the code is right — only the prose count is wrong.

**Fix**: All references to "14" image methods in this plan should read "12". The architecture doc has the same error but is out of scope for this plan.

### Issue 2: `core.FetchImageConfig` Auth Threading — Bearer Token Incompatibility

This is the most significant design gap. `core.FetchImageConfig(ref, basicAuth)` passes `basicAuth` to `getRegistryToken()`, which does:

```go
req.Header.Set("Authorization", "Basic "+basicAuth)
```

If `ARAuthProvider.GetToken()` returns `"Bearer <token>"` and `ImageManager.Pull()` passes that to `FetchImageConfig(ref, "Bearer <token>")`, the result is `Authorization: Basic Bearer <token>` — a malformed header.

This affects ALL cloud plans (AWS returns `"Basic <b64>"`, GCP returns `"Bearer <token>"`). The core `ImageManager` implementation must solve this by one of:

1. **Refactor `FetchImageConfig` to accept a pre-authenticated token** — add a new parameter or option that bypasses `getRegistryToken()` entirely when a cloud token is already available.
2. **Add a `FetchImageConfigWithToken(ref, token string)` variant** — skips the Www-Authenticate dance, passes `token` directly to `registryGet()` as the bearer token for manifest/blob fetches.
3. **Have `ImageManager.Pull()` call lower-level functions** — call `parseImageRef` + `getConfigDigest` + `getConfigBlob` directly with the cloud-provided token, bypassing `FetchImageConfig`'s auth layer.

Option 2 is cleanest. This is a **core ImageManager prerequisite** (Step 1), not GCP-specific. The plan's Section 8.2 item 1 ("uses `core.FetchImageConfig()`") is correct in intent but must note this refactoring requirement.

**Impact on this plan**: The `ARAuthProvider.GetToken()` return value is correct (`"Bearer " + token.AccessToken`). The fix is entirely in core's `ImageManager.Pull()` implementation. No changes needed to `ARAuthProvider`.

### Issue 3: GCP Simulator Does Not Support OCI Manifest DELETE

`ARAuthProvider.OnRemove()` sends `DELETE /v2/{name}/manifests/{tag}` to the registry. However, `simulators/gcp/artifactregistry.go`'s `handleOCIManifest()` (line 284) only handles `GET` and `PUT` — the `default` case returns 405 Method Not Allowed.

**Fix required in simulator**: Add `case http.MethodDelete:` to `handleOCIManifest()` that removes the manifest from the state store and returns 202 Accepted. This is a small change (~10 lines) but must be done before `OnRemove` can be tested against the simulator.

**Impact on this plan**: Add a note to Step 9 (Full Test Validation) that the simulator needs the DELETE handler added. Alternatively, add a new Step 3.5 or pre-step: "Add OCI manifest DELETE support to GCP simulator."

### Issue 4: Cross-Cloud Consistency — ARAuthProvider vs ECRAuthProvider

The interface implementations are consistent:

| Method | ECRAuthProvider | ARAuthProvider |
|--------|----------------|----------------|
| `GetToken` | Returns `"Basic " + token` | Returns `"Bearer " + token` |
| `IsCloudRegistry` | String match on `.amazonaws.com` + `.dkr.ecr.` | String match on `.gcr.io` + `-docker.pkg.dev` |
| `OnPush` | ECR SDK (`CreateRepository` + `PutImage`) | `core.OCIPush` |
| `OnTag` | Delegates to `OnPush` | `core.OCIPush` (same as OnPush) |
| `OnRemove` | ECR SDK (`BatchDeleteImage`) | OCI HTTP DELETE |

Both implement all 5 `AuthProvider` methods. The struct shapes differ appropriately (ECR needs SDK client; AR needs `context.Context`). The ECS plan stores context as `Ctx func() context.Context` while the GCP plan stores it as `Ctx context.Context` — this is fine since GCP ADC only needs a context for the initial credential lookup.

One minor inconsistency: the ECS plan uses direct struct initialization (`&core.ImageManager{...}`) while this plan uses a constructor (`core.NewImageManager(...)`). The constructor doesn't exist yet; whichever pattern core adopts, both plans should align. **Recommendation**: Use direct struct init to match the ECS plan, or note that `NewImageManager` must be created.

### Issue 5: Token Refresh / Expiry

Neither the GCP nor AWS plans address token caching or refresh. `ARAuthProvider.GetToken()` calls `google.FindDefaultCredentials` + `TokenSource.Token()` on every invocation. For `Pull` this is fine (one call per pull), but `OnRemove` with many tags would call `GetToken()` once per the method (acceptable).

However, the `oauth2.TokenSource` from ADC handles refresh internally — `Token()` returns a cached token until expiry, then auto-refreshes. So there is no token refresh bug here. The plan is correct as written.

**One improvement**: The `ARAuthProvider` could cache the `TokenSource` instead of calling `FindDefaultCredentials` each time (which re-reads the credentials file). Consider storing `ts oauth2.TokenSource` on the struct, initialized once in the constructor:

```go
type ARAuthProvider struct {
    ts oauth2.TokenSource
}
```

This is an optimization, not a correctness issue.

### Issue 6: `core.SetOCIAuth` Export

The plan correctly identifies that `setOCIAuth` needs to be exported as `SetOCIAuth` (Section 5.1, 6.2, Open Question 2). This is used in `OnRemove` for the DELETE request auth header. The ECS plan does NOT need this export (it uses ECR SDK, not OCI HTTP). This is a GCP-specific core change.

**Alternative** (as the plan notes): Inline the 4-line auth header logic in `OnRemove()` instead of exporting. Since the logic is trivial and only used in one place outside core, inlining may be cleaner than polluting core's public API. **Recommendation**: Inline it. The pattern is:

```go
if strings.HasPrefix(token, "Bearer ") || strings.HasPrefix(token, "Basic ") {
    req.Header.Set("Authorization", token)
} else {
    req.Header.Set("Authorization", "Bearer "+token)
}
```

This matches what the current `cloudrun/backend_impl.go:1430-1434` already does.

### Issue 7: Missing GCF `ImageTag` Override Discussion

GCF's `backend_delegates_gen.go` currently delegates `ImageTag` to `s.BaseServer.ImageTag()`. After migration, it would delegate to `s.images.Tag()`. But unlike CloudRun, GCF currently does NOT sync tags to AR (there is no `ImageTag` override in GCF's `backend_impl.go`). After migration, `ImageManager.Tag()` will call `Auth.OnTag()` — meaning GCF will START syncing tags to AR, which is a behavioral change.

This is arguably an improvement (consistency with CloudRun), but it should be called out in Section 8.2 as a behavioral change, not listed as "None."

### Summary of Required Fixes Before Implementation

1. **Core prerequisite**: `FetchImageConfig` must support pre-authenticated tokens (Issue 2)
2. **Simulator prerequisite**: Add OCI manifest DELETE to GCP simulator (Issue 3)
3. **Prose fix**: Change "14" to "12" image methods throughout (Issue 1)
4. **Documentation**: Note the GCF ImageTag behavioral change (Issue 7)
5. **Decision**: Inline `setOCIAuth` logic vs export (Issue 6) — recommend inline
