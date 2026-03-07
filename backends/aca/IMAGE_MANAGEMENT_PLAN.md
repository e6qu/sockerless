# ACA Backend: Image Management Plan

## 1. Current State

### 1.1 Image Methods — Current Implementation

| Method | Implementation | Location | Notes |
|--------|---------------|----------|-------|
| `ImagePull` | ACA-custom | `backend_impl.go` | Fetches real image config from registry (ACR, Docker Hub), stores synthetic image metadata in-memory |
| `ImageInspect` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory lookup. Works correctly. |
| `ImageLoad` | ACA-custom (NotImplemented) | `backend_impl.go` | Returns `NotImplementedError` |
| `ImageTag` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory tag aliasing. Works correctly. |
| `ImageList` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory listing with container count. Works correctly. |
| `ImageRemove` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory removal with container-in-use checks. Works correctly. |
| `ImageHistory` | Delegate to BaseServer | `backend_delegates_gen.go` | Synthetic layer history. Works correctly. |
| `ImagePrune` | Delegate to BaseServer | `backend_delegates_gen.go` | In-memory prune with label/dangling filters. Works correctly. |
| `ImageBuild` | Delegate to BaseServer | `backend_delegates_gen.go` | Synthetic Dockerfile parsing, stores image metadata in-memory. No cloud build. |
| `ImagePush` | Delegate to BaseServer | `backend_delegates_gen.go` | Synthetic progress output. Does not actually push to any registry. |
| `ImageSave` | Delegate to BaseServer | `backend_delegates_gen.go` | Creates tar archive with config JSON + manifest. Works correctly. |
| `ImageSearch` | Delegate to BaseServer | `backend_delegates_gen.go` | Searches in-memory images by name. Works correctly. |
| `AuthLogin` | ACA-custom | `backend_impl_pods.go` | ACR warning + BaseServer delegation |

### 1.2 What Works Today

- **ImagePull**: Already cloud-aware. Uses `fetchImageConfig()` in `registry.go` to pull real OCI image configs from Docker Hub and ACR registries. Authenticates to ACR via `azidentity.DefaultAzureCredential`. Stores image metadata with full config (Env, Cmd, Entrypoint, WorkingDir, etc.) so ContainerCreate can merge it correctly.

- **ContainerCreate → Image reference**: When creating an ACA Job, the container spec directly references `config.Image` (e.g., `alpine:latest`, `myregistry.azurecr.io/myapp:v1`). The ACA platform pulls the image from the registry at job execution time. The backend does not need to pre-push images to ACR for them to be used.

- **In-memory metadata operations**: ImageInspect, ImageList, ImageTag, ImageRemove, ImageHistory, ImagePrune, ImageSave, ImageSearch all work on the in-memory image store. They provide Docker API compatibility for tooling that inspects images after pulling.

### 1.3 What's Synthetic/Broken

- **ImageBuild**: Uses BaseServer's synthetic Dockerfile parser. Parses FROM, ENV, CMD, ENTRYPOINT, COPY, etc., but does not build real layers. The resulting image ID is stored in-memory only. If the built image is referenced in ContainerCreate, ACA will try to pull it from a registry and fail (the image only exists in-memory).

- **ImagePush**: Produces synthetic "Pushed" progress output but does not actually push anything to a registry. The image exists only in-memory.

- **ImageLoad**: Returns NotImplementedError. Cannot load tar archives.

- **ImagePull image size**: Stored as `Size: 0` and `VirtualSize: 0`. BaseServer stores a deterministic size derived from the ref hash.

- **ImagePull RepoDigests**: Stored as empty `[]string{}`. BaseServer populates a synthetic digest.

- **ImagePull RootFS**: Stored as `{Type: "layers"}` with no Layers. BaseServer adds a synthetic layer hash.

- **ImagePull GraphDriver**: Not populated. BaseServer adds synthetic overlay2 data.

### 1.4 ACR Simulator Capabilities

The Azure simulator (`simulators/azure/acr.go`, ~509 lines) implements the OCI Distribution API:

- **ARM management**: Create/Get/Delete registries, check name availability, list replications
- **OCI Distribution v2**: GET/HEAD/PUT manifests, GET/HEAD blobs, blob uploads (POST init, PATCH chunked, PUT/GET finalize), manifest content-type tracking
- **Storage model**: In-memory `StateStore` for manifests (keyed by `repo:ref`), blobs (keyed by `repo@digest`), uploads (keyed by UUID)
- **No Tags List API** (`GET /v2/{name}/tags/list`) -- would need to be added for ImageSearch against ACR
- **No Catalog API** (`GET /v2/_catalog`) -- would need to be added for listing all repositories
- **No ACR Tasks/Build API** -- no ARM endpoints for `Microsoft.ContainerRegistry/registries/scheduleRun` or task management

---

## 2. ACR Integration Design

### 2.1 Config Changes

Add an optional ACR configuration field to `Config`:

```go
type Config struct {
    // ... existing fields ...
    ACRName    string // Optional: ACR registry name (e.g., "myregistry")
    ACRServer  string // Derived: login server (e.g., "myregistry.azurecr.io")
}
```

Environment variable: `SOCKERLESS_ACA_ACR_NAME`. If set, `ACRServer` is derived as `strings.ToLower(acrName) + ".azurecr.io"`.

When `ACRName` is empty, all image methods continue with current behavior (in-memory metadata + synthetic operations). When set, ImagePush and ImageBuild can proxy to ACR.

### 2.2 New Azure SDK Client

Add `armcontainerregistry.RegistriesClient` to `AzureClients`:

```go
type AzureClients struct {
    Jobs       *armappcontainers.JobsClient
    Executions *armappcontainers.JobsExecutionsClient
    Logs       *azquery.LogsClient
    Registries *armcontainerregistry.RegistriesClient // NEW: for ACR Tasks (ImageBuild)
}
```

Package: `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry`

For OCI operations (push, pull, tag), use direct HTTP calls to the registry's OCI Distribution API rather than the ARM SDK -- this matches what `registry.go` already does for fetching image configs.

### 2.3 Authentication

ACR authentication is already implemented in `registry.go`:
- `getACRToken()` uses `azidentity.DefaultAzureCredential` to get a Bearer token scoped to the ACR registry
- For the simulator, `fakeCredential` returns a dummy token

No changes needed for auth.

---

## 3. Image Lifecycle

### 3.1 Image Creation Flow

```
User builds/pulls image
  --> Stored in-memory with full metadata
  --> When ACR is configured and ImagePush is called:
      --> Push layers + manifest to ACR via OCI Distribution API
  --> ContainerCreate references the image
  --> buildJobSpec sets container.Image = config.Image
  --> ACA Job execution pulls the image from its original registry
```

Key insight: ACA Jobs pull images themselves at execution time. The backend does not need to re-push images to ACR unless the user explicitly wants images in ACR (e.g., for ImageBuild results or for private registry images the ACA environment can't reach).

### 3.2 Image Storage

Images live in two places:
1. **In-memory store** (`s.Store.Images`): Full `api.Image` metadata for Docker API responses
2. **Container registry** (ACR or original): Actual layers and manifests, pulled by ACA at execution time

The in-memory store is the source of truth for the Docker API. The registry is the source of truth for actual container execution.

---

## 4. Method-by-Method Implementation Plan

### 4.1 ImagePull (KEEP CURRENT + MINOR IMPROVEMENTS)

**Current behavior**: Fetches real image config from the registry via `fetchImageConfig()`, creates synthetic metadata in the in-memory store.

**Proposed improvements**:
- Add `RepoDigests` field (derive from manifest digest if fetchImageConfig fetches it)
- Add `RootFS.Layers` with a synthetic layer hash (for consistency with BaseServer)
- Add `GraphDriver` data (for consistency with BaseServer)
- Populate `Size` with a deterministic value from ref hash (matching BaseServer pattern)

**Priority**: Low. Current implementation is functional and correct for the primary use case.

### 4.2 ImageInspect (KEEP DELEGATE)

BaseServer implementation is correct. Returns the image from in-memory store. No cloud-specific behavior needed.

### 4.3 ImageLoad (KEEP NotImplementedError)

ACA cannot load arbitrary tar archives as images. The proper workflow is to push images to a registry (ACR, Docker Hub, etc.) and reference them by tag.

**Alternative (future)**: Parse the tar, extract manifest.json and config, store metadata in-memory, and optionally push layers+manifest to ACR. This would enable `docker load` + `docker run` workflows. Low priority.

### 4.4 ImageTag (KEEP DELEGATE)

BaseServer's in-memory tag aliasing is sufficient. Tags are local metadata -- the user should use `docker push` (ImagePush) to push the new tag to a registry.

**Enhancement (future)**: If ACR is configured, also create the tag in ACR via OCI Distribution API (PUT manifest with new tag reference pointing to same digest). Low priority.

### 4.5 ImageList (KEEP DELEGATE)

BaseServer implementation is correct. Returns images from in-memory store with container counts. No need to query ACR for this.

### 4.6 ImageRemove (KEEP DELEGATE)

BaseServer implementation handles untagging vs. deletion, force removal, container-in-use checks. In-memory only is correct.

**Enhancement (future)**: If ACR is configured, also delete the manifest from ACR. Requires `DELETE /v2/{name}/manifests/{digest}`. Low priority and potentially dangerous (may affect other clients).

### 4.7 ImageHistory (KEEP DELEGATE)

BaseServer creates a synthetic history with FROM base image + current layer. Correct for simulation purposes.

### 4.8 ImagePrune (KEEP DELEGATE)

BaseServer handles dangling/label filters and space reclamation. In-memory is correct.

### 4.9 ImageBuild (UPGRADE: ACR Tasks)

**Current**: Delegates to BaseServer synthetic Dockerfile parser. Creates in-memory image only.

**Problem**: Built images cannot be used by ACA Jobs because they don't exist in any registry.

**Proposed implementation** (when `ACRName` is configured):

1. Accept build context tar + Dockerfile as today
2. Use ACR Quick Build (ACR Tasks) to build the image in the cloud:
   - `armcontainerregistry.RegistriesClient.BeginScheduleRun()` with `DockerBuildRequest`
   - Upload build context to ACR as an OCI blob (or point to a source archive URL)
   - ACR builds the Dockerfile server-side and stores the result in ACR
3. Tag the result as `{acrServer}/{image}:{tag}`
4. Store metadata in-memory (same as today)
5. Stream build progress from ACR build logs

**Fallback** (when `ACRName` is not configured):
- Continue using BaseServer synthetic build
- Log a warning that built images won't be usable in ACA Jobs

**ACR Simulator changes needed**:
- Add `POST .../registries/{name}/scheduleRun` ARM endpoint
- Accept `DockerBuildRequest` with embedded context
- Execute a synthetic build (parse Dockerfile, create manifest+config)
- Store result as manifests/blobs in the ACR OCI store

**Priority**: Medium. This is the most impactful improvement. Without it, `docker build` + `docker run` workflows fail silently (ACA pulls a nonexistent image).

**Estimated effort**: ~300-400 lines backend, ~200 lines simulator

### 4.10 ImagePush (UPGRADE: ACR Push)

**Current**: Delegates to BaseServer synthetic progress.

**Proposed implementation** (when `ACRName` is configured, or when pushing to any registry with auth):

1. Resolve image from in-memory store
2. If target registry is ACR and matches configured `ACRServer`:
   - Push a minimal OCI manifest + config blob to ACR via OCI Distribution API
   - Use the existing `registry.go` auth mechanism for ACR tokens
   - Stream real progress
3. If target is another registry:
   - Keep synthetic progress (we don't have real layers to push)

**For ACR push specifically**:
- We don't have real layers (image was pulled as metadata only)
- Push a minimal "empty" layer + config blob + manifest
- The important thing is that the manifest exists in ACR with the right tag so that ACA Jobs can resolve it

**Alternative approach**: For images pulled from Docker Hub, use ACR Import instead of push:
- `POST .../registries/{name}/importImage` with source image reference
- ACR copies the image server-side (no need to download/upload layers)
- Much more efficient

**Priority**: Medium. Needed for `docker tag myimage myacr.azurecr.io/myimage && docker push` workflows.

**Estimated effort**: ~150-200 lines backend, ~50 lines simulator (import endpoint)

### 4.11 ImageSave (KEEP DELEGATE)

BaseServer creates a valid tar archive with config JSON and manifest.json. Works for `docker save` workflows. No cloud integration needed.

### 4.12 ImageSearch (KEEP DELEGATE + FUTURE ACR CATALOG)

**Current**: Searches in-memory images only.

**Future enhancement**: Query ACR catalog API (`GET /v2/_catalog`) and tags list API (`GET /v2/{name}/tags/list`) to search images in ACR. Would require:
- ACR simulator: Add `_catalog` and `tags/list` endpoints
- Backend: Query ACR if `ACRName` is configured, merge with in-memory results

**Priority**: Low. In-memory search is adequate for most workflows.

---

## 5. Caching Strategy

### 5.1 Image Config Cache

Already implemented in `backends/core/registry.go`:
- `imageConfigCache` is a `sync.RWMutex`-protected `map[string]*api.ContainerConfig`
- `FetchImageConfig()` checks cache before making HTTP requests
- Cache is process-lifetime (no TTL, no eviction)

ACA's `registry.go` `fetchImageConfig()` does NOT use this cache. It makes fresh HTTP requests every time.

**Proposed fix**: Use `core.FetchImageConfig()` in ACA's ImagePull instead of the custom `fetchImageConfig()`, or add caching to the ACA version. The core version already handles Docker Hub, has graceful fallback, and caches results.

### 5.2 Avoiding Re-Pulls

ACA's ImagePull already checks `s.Store.ResolveImage(ref)` indirectly via `StoreImageWithAliases` overwriting. But it does NOT short-circuit like BaseServer does:

```go
// BaseServer does this (ACA does not):
if _, exists := s.Store.ResolveImage(ref); exists {
    return "Image is up to date" progress
}
```

**Proposed fix**: Add the same early-return check to ACA's ImagePull. If the image is already in-memory, return "up to date" progress without re-fetching config from the registry.

### 5.3 ACR Image Existence Check

When `ACRName` is configured and ImagePull is called for a public image:
1. Check in-memory store first (fast path)
2. If not found, check if image exists in ACR (`HEAD /v2/{repo}/manifests/{tag}`)
3. If in ACR, create in-memory metadata from ACR manifest/config
4. If not in ACR, fetch from original registry and optionally import to ACR

This avoids redundant imports and speeds up repeated pulls.

---

## 6. ACA Job Image Resolution

### 6.1 How ACA Jobs Pull Images

The `buildContainerSpec()` in `jobspec.go` sets:
```go
Container{
    Image: ptr(config.Image),  // e.g., "alpine:latest" or "myacr.azurecr.io/app:v1"
}
```

ACA Job execution pulls images from the specified registry. For public images (Docker Hub), this works out of the box. For private registries, ACA needs credentials configured via:
- Managed Identity on the ACA Environment (for ACR)
- Registry credentials in the Job configuration

### 6.2 Registry Credentials in Job Spec

Currently `buildJobSpec()` does not set registry credentials. For ACR images, ACA Environments typically use a system-assigned managed identity.

**Proposed enhancement**: If `ACRName` is configured, add `Registries` to the job spec:
```go
Configuration: &armappcontainers.JobConfiguration{
    Registries: []*armappcontainers.RegistryCredentials{{
        Server:   ptr(s.config.ACRServer),
        Identity: ptr("system"),
    }},
}
```

This ensures ACA Jobs can pull from the configured ACR using the environment's managed identity.

### 6.3 Image Rewriting

When `ACRName` is configured and `ImageBuild` produces an image, the resulting image should be tagged as `{acrServer}/{name}:{tag}` in the in-memory store. When `ContainerCreate` references this image, `buildContainerSpec` will use the ACR-qualified name, and ACA will pull it from ACR.

No automatic image rewriting should happen for `ImagePull` -- if the user pulls `alpine:latest`, the container should reference `alpine:latest` (public registry) rather than being rewritten to `myacr.azurecr.io/alpine:latest`.

---

## 7. ACR Simulator Enhancements Needed

For full image management support, the ACR simulator (`simulators/azure/acr.go`) needs:

1. **Tags List API**: `GET /v2/{name}/tags/list` -- returns list of tags for a repository
2. **Catalog API**: `GET /v2/_catalog` -- returns list of repositories
3. **Delete Manifest**: `DELETE /v2/{name}/manifests/{digest}` -- for ImageRemove ACR cleanup
4. **ACR Import Image**: `POST .../registries/{name}/importImage` -- for efficient cross-registry import
5. **ACR Quick Build** (optional): `POST .../registries/{name}/scheduleRun` -- for cloud-side ImageBuild

Items 1-3 are straightforward OCI Distribution spec additions (~50-80 lines each). Item 4 is an ARM endpoint (~100 lines). Item 5 is the most complex (~200-300 lines).

---

## 8. Implementation Phases

### Phase A: ImagePull Polish (Low effort, no breaking changes)
- Add early-return for already-pulled images
- Populate Size, RepoDigests, RootFS.Layers, GraphDriver for parity with BaseServer
- Consider switching to `core.FetchImageConfig()` for caching

### Phase B: ACR Config + Job Spec Credentials (Low effort)
- Add `ACRName`/`ACRServer` to Config
- Add `SOCKERLESS_ACA_ACR_NAME` env var
- Add `Registries` to `buildJobSpec()` when ACRName is set

### Phase C: ImagePush to ACR (Medium effort)
- Implement OCI Distribution push (POST upload init, PATCH data, PUT finalize, PUT manifest)
- Stream real progress
- Add ACR simulator: Tags List API, Catalog API

### Phase D: ImageBuild via ACR Tasks (Medium-High effort)
- Implement `RegistriesClient.BeginScheduleRun()` with `DockerBuildRequest`
- Add ACR simulator: `scheduleRun` endpoint with synthetic build
- Tag result in ACR, store metadata in-memory
- Stream build progress

### Phase E: ImageLoad via ACR Import (Low effort, optional)
- Parse tar archive for manifest.json and config
- Store metadata in-memory
- If ACR configured, push to ACR (or use import API)

### Recommended Order: A -> B -> C -> D -> E

---

## 9. E2E Test Compatibility

### 9.1 Current E2E Tests (integration_test.go)

The ACA integration tests use Docker SDK:
- `ImagePull("alpine:latest", ...)` -- works today
- `ContainerCreate` with `Image: "alpine:latest"` -- works today
- No `ImageBuild`, `ImagePush`, `ImageLoad`, `ImageSave` tests

### 9.2 Non-Breaking Guarantees

All proposed changes are additive:
- Phase A only improves metadata completeness (more fields populated)
- Phase B adds optional config and job spec fields
- Phase C adds real behavior behind a feature flag (`ACRName` must be set)
- Phase D adds real behavior behind a feature flag (`ACRName` must be set)

When `ACRName` is not configured, all behavior remains identical to current.

### 9.3 New Tests Needed

- Unit test: ImagePull early-return when image exists
- Unit test: ImagePull metadata completeness (Size, RepoDigests, RootFS, GraphDriver)
- Integration test: ImageBuild + ContainerCreate (with ACR Tasks, when Phase D is implemented)
- Integration test: ImagePush to ACR (when Phase C is implemented)
- ACR simulator test: Tags List, Catalog, Delete Manifest endpoints

---

## 10. Azure Functions Backend Differences

See `backends/azure-functions/IMAGE_MANAGEMENT_PLAN.md` for AZF-specific notes. Key differences:

- AZF uses `LinuxFxVersion: "DOCKER|" + config.Image` in the Function App site config (set at ContainerCreate time, not ContainerStart)
- AZF has its own `Registry` config field (`SOCKERLESS_AZF_REGISTRY`) for `DOCKER_REGISTRY_SERVER_URL`
- AZF does not have agent-based image injection; images must be pre-built
- AZF already returns NotImplementedError for ImageBuild, ImagePush, ImageLoad
- AZF's ImagePull uses `core.FetchImageConfig()` (the core cached version), while ACA uses its own `fetchImageConfig()` with direct ACR auth
