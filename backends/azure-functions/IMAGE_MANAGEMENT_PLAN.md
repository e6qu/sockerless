# Azure Functions Backend: Image Management Plan

## 1. Overview

Azure Functions is a FaaS platform that runs pre-built container images. Unlike ACA (which launches ACA Jobs at ContainerStart), Azure Functions creates the Function App resource at ContainerCreate time with the image reference baked into `LinuxFxVersion`. This fundamentally constrains image management:

- Images must exist in a registry before ContainerCreate
- There is no "build in the cloud" step (unlike ACA which could use ACR Tasks)
- Image changes require re-creating the Function App

This plan documents the current state, confirms which methods should remain as-is, and identifies minor improvements.

---

## 2. Current State

### 2.1 Image Methods -- Current Implementation

| Method | Implementation | Location | Notes |
|--------|---------------|----------|-------|
| `ImagePull` | AZF-custom | `backend_impl.go` | Uses `core.FetchImageConfig()` (cached), creates in-memory metadata |
| `ImageInspect` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory lookup |
| `ImageLoad` | AZF-custom (NotImplemented) | `backend_impl.go` | Returns `NotImplementedError` |
| `ImageTag` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory tag aliasing |
| `ImageList` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory listing |
| `ImageRemove` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory removal |
| `ImageHistory` | Delegate to BaseServer | `backend_delegates_gen.go` | Synthetic history |
| `ImagePrune` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory prune |
| `ImageBuild` | AZF-custom (NotImplemented) | `backend_impl.go` | Returns `NotImplementedError` -- "push pre-built images to ACR" |
| `ImagePush` | AZF-custom (NotImplemented) | `backend_impl.go` | Returns `NotImplementedError` -- "push images directly to ACR" |
| `ImageSave` | Delegate to BaseServer | `backend_delegates_gen.go` | Tar archive from in-memory metadata |
| `ImageSearch` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory search |
| `AuthLogin` | Delegate to BaseServer | `backend_delegates_gen.go` | Always succeeds |

### 2.2 Key Architectural Differences from ACA

1. **Image reference is set at ContainerCreate, not ContainerStart**:
   - ACA: `ContainerCreate` stores metadata, `ContainerStart` calls `buildJobSpec()` with `config.Image`
   - AZF: `ContainerCreate` calls `WebApps.BeginCreateOrUpdate()` with `LinuxFxVersion: "DOCKER|" + config.Image`

2. **Registry config is a separate field**:
   - AZF has `Config.Registry` (`SOCKERLESS_AZF_REGISTRY`) which sets `DOCKER_REGISTRY_SERVER_URL` on the Function App
   - ACA has no equivalent (relies on ACA Environment's managed identity for ACR)

3. **Image config fetching uses core library**:
   - AZF: `core.FetchImageConfig(ref, "")` -- uses the shared cached implementation
   - ACA: Custom `s.fetchImageConfig(ref, auth)` with direct ACR credential acquisition

4. **No agent-based builds**: AZF containers are ephemeral function executions. There is no mechanism to run a build process inside a Function App.

---

## 3. Method-by-Method Analysis

### 3.1 ImagePull (KEEP CURRENT)

**Current behavior**: Uses `core.FetchImageConfig()` to fetch real config from registries (Docker Hub, etc.), creates in-memory metadata with SHA256 image ID.

**Assessment**: This is correct and sufficient. The image config (Env, Cmd, Entrypoint, WorkingDir, Labels) is used by ContainerCreate to populate Function App settings.

**Minor improvements** (matching ACA plan Phase A):
- Populate `Size` with a deterministic value from ref hash
- Populate `RepoDigests` with synthetic digest
- Populate `RootFS.Layers` with synthetic layer hash
- Populate `GraphDriver` with synthetic overlay2 data
- Add early-return when image already exists in-memory

### 3.2 ImageInspect (KEEP DELEGATE)

BaseServer in-memory lookup is correct.

### 3.3 ImageLoad (KEEP NotImplementedError)

Azure Functions requires images to be in a registry accessible via `LinuxFxVersion`. Loading tar archives has no meaningful semantics.

### 3.4 ImageTag (KEEP DELEGATE)

In-memory tag aliasing is sufficient. Tags are local metadata.

### 3.5 ImageList (KEEP DELEGATE)

In-memory listing is correct.

### 3.6 ImageRemove (KEEP DELEGATE)

In-memory removal with container-in-use checks is correct.

### 3.7 ImageHistory (KEEP DELEGATE)

Synthetic history is sufficient.

### 3.8 ImagePrune (KEEP DELEGATE)

In-memory prune with filters is correct.

### 3.9 ImageBuild (KEEP NotImplementedError)

Azure Functions requires pre-built container images. Building images is outside the scope of a FaaS backend. The error message correctly directs users to push pre-built images to ACR.

**Future consideration**: If an ACR name were configured, the ACA backend's ACR Tasks approach could theoretically be shared. However, this adds complexity for minimal benefit -- users should build images before deploying to Azure Functions.

### 3.10 ImagePush (KEEP NotImplementedError)

Same rationale as ImageBuild. Users should push images directly to their registry.

**Future consideration**: If a `Registry` is configured and points to ACR, a push implementation could proxy to the ACR OCI Distribution API. However, the in-memory image has no real layers, so this would be a minimal/empty push.

### 3.11 ImageSave (KEEP DELEGATE)

BaseServer tar archive creation works correctly.

### 3.12 ImageSearch (KEEP DELEGATE)

In-memory search is sufficient.

### 3.13 AuthLogin (KEEP DELEGATE)

BaseServer always-success is acceptable. AZF uses `DOCKER_REGISTRY_SERVER_URL` for registry auth, not Docker CLI login.

**Enhancement (matching ACA)**: Override to log a warning for ACR registries (matching what ACA does in `backend_impl_pods.go`). Low priority.

---

## 4. Proposed Changes (Minimal)

### Phase A: ImagePull Polish

Same improvements as the ACA plan Phase A:

1. **Early-return for existing images**:
```go
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
    // ... tag normalization ...
    if _, exists := s.Store.ResolveImage(ref); exists {
        // Return "Image is up to date" progress
        pr, pw := io.Pipe()
        go func() {
            defer pw.Close()
            json.NewEncoder(pw).Encode(map[string]string{
                "status": "Status: Image is up to date for " + ref,
            })
        }()
        return pr, nil
    }
    // ... rest of current implementation ...
}
```

2. **Metadata completeness**: Add Size, RepoDigests, RootFS.Layers, GraphDriver to the created image (matching BaseServer output format).

### No Other Changes Needed

The remaining methods are either correctly implemented (NotImplementedError for unsupported operations) or correctly delegated to BaseServer. Azure Functions' FaaS model means there is no viable cloud-side image management to integrate.

---

## 5. Differences from ACA Image Management

| Aspect | ACA | Azure Functions |
|--------|-----|-----------------|
| Image set at | ContainerStart (job spec) | ContainerCreate (site config) |
| Registry config | `ACRName` (proposed) | `Registry` (existing) |
| Image config fetch | Custom `fetchImageConfig()` | `core.FetchImageConfig()` |
| ACR auth | `azidentity.DefaultAzureCredential` | `DOCKER_REGISTRY_SERVER_URL` app setting |
| ImageBuild | Upgradeable to ACR Tasks | NotImplementedError (correct for FaaS) |
| ImagePush | Upgradeable to OCI push | NotImplementedError (correct for FaaS) |
| ImageLoad | NotImplementedError | NotImplementedError |
| Multi-container | Supported (ACA Jobs with sidecars) | Not supported (single function) |

---

## 6. E2E Test Compatibility

### Current Tests

The AZF integration tests (`integration_test.go`) use:
- `ImagePull("alpine:latest", ...)` -- works today
- `ContainerCreate` with `Image: "alpine:latest"` -- works today
- No ImageBuild, ImagePush, ImageLoad tests

### Non-Breaking Guarantee

All proposed changes (Phase A) are additive metadata improvements. No existing behavior changes. The early-return optimization for existing images matches Docker's real behavior more closely.

---

## 7. Recommendation

Azure Functions image management is essentially complete in its current form. The NotImplementedError returns for ImageBuild, ImagePush, and ImageLoad are correct design decisions for a FaaS backend. The only actionable improvement is Phase A (ImagePull polish), which is low effort and improves Docker API fidelity.

**Priority**: Low. Focus image management efforts on the ACA backend where ACR Tasks integration would unlock `docker build` + `docker run` workflows.
