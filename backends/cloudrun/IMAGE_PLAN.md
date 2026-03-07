# GCP Unified Image Management Plan (CloudRun + GCF)

**STATUS: FULLY IMPLEMENTED.** All steps below have been completed. Both CloudRun and GCF backends now use the unified `core.ImageManager` with `ARAuthProvider` for all 12 image methods. The old `registry.go` files have been deleted from both backends.

This plan implemented the architecture described in `backends/IMAGE_ARCHITECTURE.md` for the GCP cloud (CloudRun and CloudRun Functions backends).

**Key constraint**: NO simulator changes. The GCP simulator already supports OCI push (blob uploads + manifest PUT). It does NOT support OCI manifest DELETE -- `OnRemove` handles this gracefully (log warning, skip) rather than requiring simulator changes.

---

## 1. ARAuthProvider

### 1.1 Location

**File:** `backends/cloudrun/image_auth.go` (~80 lines)

This file lives in the `cloudrun` package. The GCF backend has its own copy (see Section 4).

### 1.2 Struct Definition

```go
package cloudrun

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

Attempts to DELETE the manifest from AR/GCR via the OCI Distribution API. **Graceful degradation**: If the registry returns 405 Method Not Allowed (as the current GCP simulator does), the error is logged but not propagated -- the `ImageManager` treats all `OnRemove` errors as non-fatal.

The auth header logic is inlined (4 lines) rather than requiring `core.SetOCIAuth` to be exported.

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
        // Inline auth header logic (matches oci_push.go setOCIAuth pattern)
        if strings.HasPrefix(token, "Bearer ") || strings.HasPrefix(token, "Basic ") {
            req.Header.Set("Authorization", token)
        } else {
            req.Header.Set("Authorization", "Bearer "+token)
        }
        resp, err := client.Do(req)
        if err != nil {
            return fmt.Errorf("delete manifest: %w", err)
        }
        resp.Body.Close()
        // Accept 200, 202 (success), 404 (already gone), 405 (not implemented by registry)
        switch resp.StatusCode {
        case http.StatusOK, http.StatusAccepted, http.StatusNotFound:
            // success or already deleted
        case http.StatusMethodNotAllowed:
            // Registry does not support DELETE (e.g. simulator) -- not an error
            return nil
        default:
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
    Logger zerolog.Logger
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
| `Build(opts, ctx)` | `BaseServer.ImageBuild` (via BuildFunc) | none |
| `Push(name, tag, auth)` | `BaseServer.ImagePush` | `Auth.OnPush()` (replaces synthetic for cloud registries) |
| `Save(names)` | `BaseServer.ImageSave` | none |
| `Search(term, limit, filters)` | `BaseServer.ImageSearch` | none |

`AuthLogin` and `ContainerCommit` remain on the backend directly (not in ImageManager).

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
    s.images = &core.ImageManager{
        Auth:   &ARAuthProvider{Ctx: context.Background()},
        Store:  s.Store,
        Logger: logger,
    }

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

## 4. GCF (Cloud Run Functions) Wiring

### 4.1 Sharing Approach

CloudRun and GCF are **separate Go modules** (`github.com/sockerless/backend-cloudrun` and `github.com/sockerless/backend-cloudrun-functions`). Cross-module imports would add coupling. The `ARAuthProvider` is ~80 lines with zero cloudrun-specific types and depends only on `golang.org/x/oauth2/google` (already imported by GCF) and `core.OCIPush` (already imported).

**Decision**: Duplicate `ARAuthProvider` in GCF. The struct is small enough that duplication is preferable to a new shared module.

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

    s.images = &core.ImageManager{
        Auth:   &ARAuthProvider{Ctx: context.Background()},
        Store:  s.Store,
        Logger: logger,
    }

    // ...
    return s
}
```

### 4.4 GCF Image Method Delegation (12 methods)

GCF overrides `ImageBuild` with `NotImplementedError` (FaaS backends cannot build images). This method is NOT delegated to `s.images`:

```go
// In backends/cloudrun-functions/backend_impl.go (KEEP existing)
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
    return nil, &api.NotImplementedError{
        Message: "Cloud Run Functions backend does not support image build; push pre-built images to Artifact Registry",
    }
}
```

The remaining 11 image methods delegate to `s.images`:

```go
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
2. **`backends/core/registry.go`**: Must support pre-authenticated tokens (see Section 9, Issue 2). Currently `FetchImageConfig(ref, basicAuth)` passes `basicAuth` to `getRegistryToken()` which prepends `"Basic "`. A cloud Bearer token cannot be passed through the existing `basicAuth` parameter without corruption. The core ImageManager must add a code path that accepts a ready-to-use auth token and passes it directly to `registryGet()`.
3. **`backends/core/oci_push.go`**: No export needed. `ARAuthProvider.OnRemove()` inlines the 4-line auth-header logic rather than requiring `SetOCIAuth`.
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

## 7. Migration Path (Step-by-Step) — ALL COMPLETE

### Step 1: Create Core ImageManager (prerequisite, not GCP-specific) — DONE
### Step 2: Create ARAuthProvider in CloudRun — DONE
### Step 3: Wire CloudRun ImageManager — DONE
### Step 4: Delete CloudRun registry.go — DONE
### Step 5: Create ARAuthProvider in GCF (copy) — DONE
### Step 6: Wire GCF ImageManager — DONE
### Step 7: Delete GCF registry.go — DONE
### Step 8: Full Test Validation — DONE

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

2. **GCF ImagePull**: Currently uses `core.FetchImageConfig()` with ADC auth. After migration, `ImageManager.Pull()` calls `Auth.GetToken()` and passes it to `core.FetchImageConfigWithAuth()`. Identical behavior but cleaner.

3. **CloudRun ImagePush/ImageTag/ImageRemove**: Currently have inline AR sync logic with goroutines. After migration, `ImageManager` calls `Auth.OnPush()`/`OnTag()`/`OnRemove()` in goroutines with error logging. Identical behavior.

4. **GCF ImagePush**: Currently uses the same inline OCI push pattern. After migration, delegates to `ImageManager.Push()` which calls `Auth.OnPush()`. Identical behavior.

5. **GCF ImageTag** (NEW behavior): Currently GCF delegates `ImageTag` to `BaseServer.ImageTag()` with NO AR sync. After migration, `ImageManager.Tag()` calls `Auth.OnTag()`, which syncs the new tag to AR. This is a **behavioral change** -- GCF will start syncing tags to AR, consistent with CloudRun's existing behavior. Net improvement but must be noted.

6. **GCF ImageRemove** (NEW behavior): Currently GCF delegates `ImageRemove` to `BaseServer.ImageRemove()` with NO AR sync. After migration, `ImageManager.Remove()` calls `Auth.OnRemove()`. Same situation as ImageTag -- new capability, consistent with CloudRun. **Note**: Since the simulator returns 405 for DELETE, `OnRemove` will gracefully log and skip in simulator mode. In production against real AR, it will delete. This is correct behavior.

### 8.3 New Tests to Add (optional, during implementation)

- Unit test for `ARAuthProvider.IsCloudRegistry()` -- verify `us-docker.pkg.dev`, `gcr.io`, `us.gcr.io` match; `docker.io`, `ecr.amazonaws.com` do not
- Integration test: push to AR simulator, pull back, verify round-trip (if AR simulator is configured in test harness)

---

## 9. Design Notes (from verification)

### Issue 1: `core.FetchImageConfig` Auth Threading -- Bearer Token Incompatibility

`core.FetchImageConfig(ref, basicAuth)` passes `basicAuth` to `getRegistryToken()`, which does:

```go
req.Header.Set("Authorization", "Basic "+basicAuth)
```

If `ARAuthProvider.GetToken()` returns `"Bearer <token>"` and `ImageManager.Pull()` passes that to `FetchImageConfig(ref, "Bearer <token>")`, the result is `Authorization: Basic Bearer <token>` -- a malformed header.

**Resolution**: The core `ImageManager` implementation must solve this by adding `FetchImageConfigWithAuth(ref, authHeader string)` that bypasses `getRegistryToken()` entirely and passes the auth header directly to `registryGet()`. This is a **core prerequisite** (Step 1), not GCP-specific.

### Issue 2: OnRemove and Simulator DELETE Support

The GCP simulator's `handleOCIManifest()` only handles `GET` and `PUT` -- the `default` case returns 405 Method Not Allowed. Rather than requiring simulator changes, `ARAuthProvider.OnRemove()` handles 405 gracefully:

```go
case http.StatusMethodNotAllowed:
    // Registry does not support DELETE (e.g. simulator) -- not an error
    return nil
```

Additionally, the `ImageManager` treats ALL `OnRemove` errors as non-fatal (logged, never returned to caller). This double layer of protection ensures `ImageRemove` always succeeds locally even if the registry DELETE fails.

### Issue 3: Token Refresh / Expiry

`ARAuthProvider.GetToken()` calls `google.FindDefaultCredentials` + `TokenSource.Token()` on every invocation. The `oauth2.TokenSource` from ADC handles refresh internally -- `Token()` returns a cached token until expiry, then auto-refreshes. No token refresh bug exists.

**Optional optimization**: Cache the `TokenSource` on the struct:

```go
type ARAuthProvider struct {
    ts oauth2.TokenSource  // initialized once
}
```

This avoids re-reading the credentials file on each call. Not required for correctness.

### Issue 4: Inlining `setOCIAuth` vs Exporting

The plan inlines the 4-line auth header logic in `OnRemove()` rather than exporting `core.setOCIAuth` as `core.SetOCIAuth`. Rationale: the logic is trivial, used in only one place outside core, and inlining avoids polluting core's public API. The inlined pattern matches `cloudrun/backend_impl.go:1430-1434` exactly.

---

## 10. Dependency Graph — ALL COMPLETE

All steps executed sequentially as planned. Both `registry.go` files deleted.

---

## 11. Open Questions — RESOLVED

1. **`core.IsGCPRegistry()` redundancy**: Kept for backward compat (3 lines, other packages may use it).
2. **`ARAuthProvider.Ctx` vs parameter**: Kept `Ctx` on struct as recommended.
