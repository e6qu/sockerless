# Cloud Run Functions (GCF) Backend: Image Management Plan

## 1. Overview

This document covers GCF-specific image management. It references the Cloud Run backend's `IMAGE_MANAGEMENT_PLAN.md` for shared design details and focuses on differences unique to the FaaS execution model.

## 2. Current State

### 2.1 Image Method Inventory

| Method | Location | Behavior | Status |
|--------|----------|----------|--------|
| ImagePull | `backend_impl.go` | Cloud-native: fetches real config via `core.FetchImageConfig()`, stores in-memory | Working |
| ImageInspect | `backend_delegates_gen.go` | Delegates to BaseServer | Working |
| ImageLoad | `backend_impl.go` | Returns `NotImplementedError` | Intentional |
| ImageTag | `backend_delegates_gen.go` | Delegates to BaseServer | Working |
| ImageList | `backend_delegates_gen.go` | Delegates to BaseServer | Working |
| ImageRemove | `backend_delegates_gen.go` | Delegates to BaseServer | Working |
| ImageHistory | `backend_delegates_gen.go` | Delegates to BaseServer | Working |
| ImagePrune | `backend_delegates_gen.go` | Delegates to BaseServer | Working |
| ImageBuild | `backend_impl.go` | Returns `NotImplementedError` | Intentional |
| ImagePush | `backend_impl.go` | Returns `NotImplementedError` | Intentional |
| ImageSave | `backend_delegates_gen.go` | Delegates to BaseServer | Working |
| ImageSearch | `backend_delegates_gen.go` | Delegates to BaseServer | Working |
| AuthLogin | `backend_delegates_gen.go` | Delegates to BaseServer | Working |
| ContainerCommit | `backend_impl.go` | Returns `NotImplementedError` | Intentional |

### 2.2 Key Differences from Cloud Run

1. **ImagePull uses `core.FetchImageConfig()`** (shared helper) instead of the Cloud Run backend's private `fetchImageConfig()` method. The core helper has its own caching (`imageConfigCache`) but does not support GCP ADC authentication -- it uses anonymous access or basic auth.

2. **ImageBuild returns NotImplementedError** (Cloud Run delegates to BaseServer's synthetic build). GCF explicitly blocks builds because Cloud Run Functions require pre-built container images.

3. **ImagePush returns NotImplementedError** -- same as Cloud Run's current behavior.

4. **No `registry.go`** -- GCF has no private registry helper. It relies on `core.FetchImageConfig()` in `backends/core/registry.go`.

5. **No ADC token support** for image config fetching -- `core.FetchImageConfig()` does not call `google.FindDefaultCredentials()`. Private AR/GCR images will fail to fetch config (graceful fallback to synthetic config).

## 3. GCF-Specific Design Decisions

### 3.1 ImageBuild -- Keep NotImplementedError

Cloud Run Functions 2nd gen run as Cloud Run services under the hood, but the function deployment model expects pre-built images. The `CreateFunction` API specifies the image via `BuildConfig`. Building images is not a function-backend concern.

**Rationale:** The GCF backend creates functions at `ContainerCreate` time, passing the image reference to the Functions API. The image must already exist in a registry. Users should build images separately (via `docker build` against a different backend, or via `gcloud builds submit`).

### 3.2 ImagePush -- Implement OCI Push (Same as Cloud Run)

GCF should support `docker push` for the same reason Cloud Run does: users need to push images to AR before they can reference them in function creation.

**Implementation:** Share the OCI push logic with Cloud Run. Extract the push helpers into a shared GCP package or duplicate the ~100 lines of push logic.

**Option A (preferred): Shared helper in `backends/core/`**
```go
// backends/core/oci_push.go
func OCIPushImage(img api.Image, registryURL, repo, tag, token string) io.ReadCloser
```

**Option B: Copy push logic into GCF backend_impl.go**

With Option A, both Cloud Run and GCF call `core.OCIPushImage()` with their respective auth tokens.

### 3.3 ImageLoad -- Delegate to BaseServer

Same as Cloud Run: remove the NotImplementedError and delegate to BaseServer. The in-memory image becomes available for `ContainerCreate` references.

```go
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
    return s.BaseServer.ImageLoad(r)
}
```

### 3.4 ImagePull -- Add ADC Support

The GCF backend should support pulling from private AR/GCR registries using ADC, similar to what Cloud Run does via `fetchImageConfig()`.

**Proposed:** Add a GCF-specific `ImagePull` that mirrors Cloud Run's approach:
1. Parse image ref into (registry, repo, tag)
2. If registry is AR/GCR, get token via ADC
3. Fetch manifest + config blob via OCI API
4. Store image in-memory

Alternatively, enhance `core.FetchImageConfig()` to accept an optional token parameter. The GCF backend would call `getARToken()` and pass it.

**Current state of `core.FetchImageConfig()`:**
```go
func FetchImageConfig(ref string, basicAuth ...string) (*api.ContainerConfig, error)
```

The function already accepts an optional auth parameter. The GCF ImagePull passes empty auth:
```go
if realConfig, _ := core.FetchImageConfig(ref, ""); realConfig != nil {
```

**Enhancement:** GCF ImagePull should obtain an ADC Bearer token for AR/GCR registries and pass it as the auth parameter.

### 3.5 AuthLogin -- Add GCR/AR Detection

The GCF backend currently delegates AuthLogin directly to BaseServer without any GCR/AR detection (unlike Cloud Run, which logs a warning). This is a minor gap.

**Proposed:** Add a GCF-specific AuthLogin that mirrors Cloud Run's:
```go
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
    addr := req.ServerAddress
    if strings.HasSuffix(addr, ".gcr.io") || strings.HasSuffix(addr, "-docker.pkg.dev") {
        s.Logger.Warn().Str("registry", addr).
            Msg("GCR/AR login: use gcloud auth configure-docker for production")
    }
    return s.BaseServer.AuthLogin(req)
}
```

## 4. Implementation Phases

### Phase 1: Low-Risk (2 changes)
1. **ImageLoad**: Remove NotImplementedError, delegate to BaseServer
2. **AuthLogin**: Add GCR/AR detection warning

### Phase 2: Registry Integration (2 changes)
3. **ImagePull**: Add ADC token for AR/GCR registries (enhance auth parameter)
4. **ImagePush**: Implement OCI push (shared with Cloud Run or duplicated)

### Phase 3: No Change
- **ImageBuild**: Keep NotImplementedError (FaaS paradigm)
- **ContainerCommit**: Keep NotImplementedError (no filesystem)
- **All other delegates**: Keep BaseServer delegation

## 5. Comparison: Cloud Run vs GCF Image Handling

| Aspect | Cloud Run | GCF |
|--------|-----------|-----|
| ImagePull auth | Private `fetchImageConfig()` with ADC | `core.FetchImageConfig()` (no ADC) |
| ImageBuild | BaseServer delegate (synthetic) | NotImplementedError |
| ImagePush | NotImplementedError -> OCI push | NotImplementedError -> OCI push |
| ImageLoad | NotImplementedError -> delegate | NotImplementedError -> delegate |
| ContainerCommit | NotImplementedError | NotImplementedError |
| Image at deploy time | Cloud Run Job spec references image | `CreateFunction` BuildConfig references image |
| Registry interaction | Direct OCI calls from backend | Direct OCI calls from backend |
| Default registry | `{region}-docker.pkg.dev/{project}/...` | `{region}-docker.pkg.dev/{project}/...` |

## 6. E2E Test Compatibility

All existing GCF integration tests (`backends/cloudrun-functions/integration_test.go`, `arithmetic_integration_test.go`) call `ImagePull` before container operations. No existing tests call ImagePush, ImageLoad, or ImageBuild. The proposed changes are purely additive and will not break existing tests.

New tests should be added for:
- `TestImageLoad` (verify it no longer returns NotImplementedError)
- `TestImagePush` (verify OCI push flow, once implemented)
