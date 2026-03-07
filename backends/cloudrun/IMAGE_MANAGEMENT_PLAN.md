# Cloud Run Backend: Image Management Plan

## 1. Current State

### 1.1 Image Method Inventory

| Method | Location | Behavior | Status |
|--------|----------|----------|--------|
| ImagePull | `backend_impl.go` | Cloud-native: fetches real image config from registry (AR, GCR, Docker Hub) via OCI Distribution API, stores in-memory with synthetic image ID | Working |
| ImageInspect | `backend_delegates_gen.go` | Delegates to BaseServer: looks up in-memory store | Working |
| ImageLoad | `backend_impl.go` | Returns `NotImplementedError` | Intentional |
| ImageTag | `backend_delegates_gen.go` | Delegates to BaseServer: in-memory tag manipulation | Working |
| ImageList | `backend_delegates_gen.go` | Delegates to BaseServer: lists in-memory images | Working |
| ImageRemove | `backend_delegates_gen.go` | Delegates to BaseServer: removes from in-memory store | Working |
| ImageHistory | `backend_delegates_gen.go` | Delegates to BaseServer: synthetic history from in-memory image | Working |
| ImagePrune | `backend_delegates_gen.go` | Delegates to BaseServer: prunes unused in-memory images | Working |
| ImageBuild | `backend_delegates_gen.go` | Delegates to BaseServer: parses Dockerfile, creates synthetic image | Working (synthetic) |
| ImagePush | `backend_impl.go` | Returns `NotImplementedError` | Intentional |
| ImageSave | `backend_delegates_gen.go` | Delegates to BaseServer: exports in-memory images as tar | Working (synthetic) |
| ImageSearch | `backend_delegates_gen.go` | Delegates to BaseServer: searches in-memory store | Working |
| AuthLogin | `backend_impl.go` | Cloud-native: detects GCR/AR addresses, logs warning, delegates to BaseServer | Working |
| ContainerCommit | `backend_impl.go` | Returns `NotImplementedError` | Intentional |

### 1.2 What Works

- **ImagePull** is the most cloud-aware method. It uses `fetchImageConfig()` in `registry.go` to contact real registries (Artifact Registry, GCR, Docker Hub) via the OCI Distribution API (GET manifest, GET config blob). It retrieves ENV, CMD, ENTRYPOINT, WorkingDir, Labels, etc. from the real image config. On failure, it falls back to a synthetic config.
- **All in-memory operations** (Inspect, List, Tag, Remove, History, Prune, Search, Save) work correctly through BaseServer. Images pulled or built are stored in `s.Store.Images` and can be queried.
- **ImageBuild** via BaseServer parses Dockerfiles (FROM, ENV, CMD, ENTRYPOINT, COPY, WORKDIR, LABEL, EXPOSE, USER, HEALTHCHECK, SHELL, STOPSIGNAL, VOLUME, ARG) and creates synthetic images. This is sufficient for CI/CD workflows that `docker build` and then `docker run`.
- **AuthLogin** correctly detects GCR/AR registry addresses.

### 1.3 What's Synthetic

- **ImagePull** stores images with `Size: 0` and no real layers. The image ID is a deterministic SHA-256 of the ref string, not the actual content digest.
- **ImageBuild** does not execute RUN instructions or produce real layers. It parses the Dockerfile and creates a metadata-only image.
- **ImageSave/ImageLoad** work with in-memory representations, not real OCI/Docker tar archives.

### 1.4 What's Broken or Missing

- **ImagePush** always returns `NotImplementedError`. Users cannot `docker push` through the backend.
- **ImageLoad** always returns `NotImplementedError`. Users cannot `docker load` a tarball.
- **ContainerCommit** always returns `NotImplementedError`. No way to create images from containers.

### 1.5 Artifact Registry Simulator

The GCP simulator (`simulators/gcp/artifactregistry.go`, 449 lines) provides:

**GCP Artifact Registry Management API (v1):**
- `POST /v1/projects/{p}/locations/{l}/repositories` -- Create repository (with LRO)
- `GET /v1/projects/{p}/locations/{l}/repositories/{r}` -- Get repository
- `GET /v1/projects/{p}/locations/{l}/repositories` -- List repositories
- `DELETE /v1/projects/{p}/locations/{l}/repositories/{r}` -- Delete repository (with LRO)
- `GET /v1/projects/{p}/locations/{l}/repositories/{r}/dockerImages` -- List docker images
- IAM get/set on repositories (GET and POST variants)

**OCI Distribution API (under /v2/):**
- `GET /v2/` -- API version check
- `GET /v2/{name}/manifests/{ref}` -- Get manifest
- `PUT /v2/{name}/manifests/{ref}` -- Put manifest (registers DockerImage entry)
- `GET /v2/{name}/blobs/{digest}` -- Get blob
- `HEAD /v2/{name}/blobs/{digest}` -- Check blob existence
- `POST /v2/{name}/blobs/uploads/` -- Initiate blob upload
- `PUT /v2/{name}/blobs/uploads/{uuid}?digest=...` -- Complete blob upload

The simulator stores manifests, blobs, and uploads in `sim.StateStore`. When a manifest is PUT, `registerDockerImageFromManifest()` creates a `DockerImage` entry that appears in the AR listing API.

---

## 2. Design: Artifact Registry Integration

### 2.1 Architecture Overview

```
docker pull nginx          docker push myrepo/app:v1     docker build -t app .
       |                          |                             |
  ImagePull()               ImagePush()                  ImageBuild()
       |                          |                             |
  [fetch config               [push layers+manifest         [parse Dockerfile,
   from real registry           to AR via OCI]               create synthetic image,
   OR from AR sim]                                           optionally submit to
       |                          |                          Cloud Build]
  [store in-memory]          [store in-memory]                  |
                                                          [store in-memory]
```

### 2.2 ImagePull Enhancement

**Current behavior** is already good. The `fetchImageConfig()` method in `registry.go` supports:
- Artifact Registry (`*-docker.pkg.dev`) with ADC token
- GCR (`*.gcr.io`) with ADC token
- Docker Hub (anonymous token via `auth.docker.io`)
- Any registry (with auth header from `docker pull --auth`)

**Proposed enhancement -- AR-aware pull with simulator support:**

When the backend is running against the GCP simulator (i.e., `EndpointURL` is set), `fetchImageConfig()` should route AR/GCR image requests through the simulator's OCI endpoint instead of the real registry. This enables end-to-end testing of `push` then `pull` flows.

```go
// In fetchImageConfig(), when EndpointURL is set and registry matches AR/GCR:
// 1. Replace registry URL with simulator URL for OCI requests
// 2. Skip ADC token -- simulator doesn't require auth
```

**No other changes needed** for ImagePull. The in-memory storage with real config metadata is the right design for a simulator backend.

### 2.3 ImagePush Implementation

**Proposed behavior**: Push image layers and manifest to Artifact Registry via the OCI Distribution API.

```
ImagePush(name, tag, auth) -> io.ReadCloser
  1. Resolve image from in-memory store (404 if not found)
  2. Determine target registry from image name:
     - If name contains AR/GCR host -> use that registry
     - Otherwise -> use default AR repo: {region}-docker.pkg.dev/{project}/sockerless
  3. Build a synthetic OCI manifest (since we don't have real layers)
  4. Push config blob: POST /v2/{name}/blobs/uploads/ + PUT
  5. Push (empty) layer blob: POST /v2/{name}/blobs/uploads/ + PUT
  6. Push manifest: PUT /v2/{name}/manifests/{tag}
  7. Stream progress JSON to caller
```

**Implementation notes:**
- When running against the simulator, the OCI endpoint is the simulator URL
- When running against real GCP, use ADC for authentication
- The push is "synthetic" in the sense that we push metadata, not real layer data. This is consistent with how the whole backend works.
- This enables the `docker push` -> `docker pull` round-trip, which is important for CI/CD workflows.

**Config addition:**
```go
type Config struct {
    // ... existing fields ...
    DefaultARRepo string // Default AR repo, e.g., "us-docker.pkg.dev/myproject/docker"
}
```

### 2.4 ImageBuild Enhancement

**Phase 1 (keep current):** BaseServer's synthetic Dockerfile parser is sufficient. It handles FROM, ENV, CMD, ENTRYPOINT, COPY, WORKDIR, LABEL, EXPOSE, USER, HEALTHCHECK, SHELL, STOPSIGNAL, VOLUME, ARG. This covers the vast majority of CI/CD use cases.

**Phase 2 (future, Cloud Build integration):** Submit builds to Cloud Build for real image builds.

```
ImageBuild(opts, context) -> io.ReadCloser
  1. Extract build context tar to temp dir
  2. Upload context to GCS bucket: gs://sockerless-builds-{project}/build-{uuid}/
  3. Submit Cloud Build request:
     POST /v1/projects/{project}/builds
     {
       "source": {"storageSource": {"bucket": "...", "object": "..."}},
       "steps": [{"name": "gcr.io/cloud-builders/docker", "args": ["build", "-t", tag, "."]}],
       "images": [tag]
     }
  4. Poll for completion (LRO)
  5. On success: register image in in-memory store with real digest
  6. Stream build logs as progress JSON
```

**Prerequisites for Phase 2:**
- Add Cloud Build simulator to `simulators/gcp/cloudbuild.go`
- Add `cloudbuild.NewClient` to `GCPClients`
- Add GCS upload support (bucket already in `GCPClients.Storage`)

**Recommendation:** Defer Phase 2. The synthetic build is adequate for the simulator use case. Cloud Build integration is a stretch goal that adds significant complexity (new simulator, GCS uploads, LRO polling) for marginal gain.

### 2.5 ImageLoad Enhancement

**Current:** Returns `NotImplementedError`.

**Proposed:** Implement by delegating to BaseServer. The BaseServer's `ImageLoad()` already parses Docker-format tar archives, extracts repo tags and image config, and stores images in-memory. There's no cloud-specific reason to block this.

```go
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
    return s.BaseServer.ImageLoad(r)
}
```

This change moves ImageLoad from `backend_impl.go` (NotImplementedError) to `backend_delegates_gen.go` (BaseServer delegate). The image becomes available for `docker run` commands.

---

## 3. Concrete Implementation Per Method

### 3.1 ImagePull -- Keep Current (Minor Enhancement)

**File:** `backend_impl.go` (existing)

**Behavior:** Fetch real image config from registry, store synthetic image in-memory.

**Enhancement:** When `EndpointURL` is set, route AR/GCR registry requests through the simulator's OCI endpoint. Add to `fetchImageConfig()`:

```go
if s.config.EndpointURL != "" && (strings.HasSuffix(registry, ".gcr.io") || strings.HasSuffix(registry, "-docker.pkg.dev")) {
    // Use simulator's OCI endpoint
    manifestURL = fmt.Sprintf("%s/v2/%s/manifests/%s", s.config.EndpointURL, repo, tag)
    // No auth needed for simulator
    token = ""
}
```

**E2E impact:** None -- existing tests pass. Enhancement enables new push-then-pull tests.

### 3.2 ImageInspect -- Keep Delegate

**File:** `backend_delegates_gen.go` (existing)

**Behavior:** `s.BaseServer.ImageInspect(name)` -- looks up in-memory store, returns `*api.Image`.

**Rationale:** In-memory lookup is correct. The image metadata was populated by ImagePull with real config data.

### 3.3 ImageLoad -- Change from NotImplemented to Delegate

**File:** Remove override from `backend_impl.go`, add to `backend_delegates_gen.go`.

**Behavior:** `s.BaseServer.ImageLoad(r)` -- parses tar archive, stores image in-memory.

**Rationale:** BaseServer already handles Docker-format tar parsing. No cloud-specific reason to block.

**E2E impact:** Enables `docker save | docker load` round-trips. No existing tests break.

### 3.4 ImageTag -- Keep Delegate

**File:** `backend_delegates_gen.go` (existing)

**Behavior:** `s.BaseServer.ImageTag(source, repo, tag)` -- adds tag to in-memory image.

**Rationale:** Purely in-memory operation. Works correctly.

### 3.5 ImageList -- Keep Delegate

**File:** `backend_delegates_gen.go` (existing)

**Behavior:** `s.BaseServer.ImageList(opts)` -- lists images from in-memory store with filter support.

**Rationale:** In-memory listing is correct and supports all Docker filter syntax.

### 3.6 ImageRemove -- Keep Delegate

**File:** `backend_delegates_gen.go` (existing)

**Behavior:** `s.BaseServer.ImageRemove(name, force, prune)` -- removes from in-memory store.

**Rationale:** In-memory removal is correct. Does not affect AR (images pushed to AR remain there).

### 3.7 ImageHistory -- Keep Delegate

**File:** `backend_delegates_gen.go` (existing)

**Behavior:** `s.BaseServer.ImageHistory(name)` -- synthetic history from in-memory image.

**Rationale:** Synthetic history is adequate. Real layer history would require parsing the AR manifest chain.

### 3.8 ImagePrune -- Keep Delegate

**File:** `backend_delegates_gen.go` (existing)

**Behavior:** `s.BaseServer.ImagePrune(filters)` -- removes unused in-memory images.

**Rationale:** In-memory pruning is correct. Does not affect AR.

### 3.9 ImageBuild -- Keep Delegate (Phase 1)

**File:** `backend_delegates_gen.go` (existing)

**Behavior:** `s.BaseServer.ImageBuild(opts, context)` -- parses Dockerfile, creates synthetic image.

**Rationale:** Synthetic build is adequate for CI/CD workflows. Cloud Build integration (Phase 2) is a future enhancement.

### 3.10 ImagePush -- New Cloud-Native Implementation

**File:** `backend_impl.go` (replace current NotImplementedError)

**Behavior:** Push image to AR via OCI Distribution API.

```go
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
    img, ok := s.Store.ResolveImage(name)
    if !ok {
        return nil, &api.NotFoundError{Resource: "image", ID: name}
    }
    if tag == "" {
        tag = "latest"
    }

    // Determine registry URL
    registry, repo := s.resolveRegistryForPush(name)

    // Get auth token
    token, err := s.getRegistryToken(registry, auth)
    if err != nil {
        return nil, &api.ServerError{Message: "registry auth failed: " + err.Error()}
    }

    pr, pw := io.Pipe()
    go func() {
        defer pw.Close()
        enc := json.NewEncoder(pw)

        // 1. Push config blob
        configJSON, _ := json.Marshal(map[string]any{
            "architecture": img.Architecture,
            "os":           img.Os,
            "config":       img.Config,
            "rootfs":       img.RootFS,
        })
        configDigest := "sha256:" + sha256hex(configJSON)
        enc.Encode(map[string]string{"status": "Preparing", "id": tag})

        if err := s.pushBlob(registry, repo, configDigest, configJSON, token); err != nil {
            enc.Encode(map[string]string{"error": err.Error()})
            return
        }

        // 2. Push empty layer blob
        emptyLayer := []byte{}
        layerDigest := "sha256:" + sha256hex(emptyLayer)
        if err := s.pushBlob(registry, repo, layerDigest, emptyLayer, token); err != nil {
            enc.Encode(map[string]string{"error": err.Error()})
            return
        }

        // 3. Push manifest
        manifest := buildOCIManifest(configDigest, len(configJSON), layerDigest, len(emptyLayer))
        if err := s.pushManifest(registry, repo, tag, manifest, token); err != nil {
            enc.Encode(map[string]string{"error": err.Error()})
            return
        }

        enc.Encode(map[string]string{"status": "Pushed", "id": tag})
        enc.Encode(map[string]string{"status": tag + ": digest: " + configDigest})
    }()

    return pr, nil
}
```

**Helper methods needed:**
- `resolveRegistryForPush(name) (registryURL, repo)` -- extracts registry from image name, uses simulator URL when `EndpointURL` is set
- `getRegistryToken(registry, auth)` -- gets token (ADC for AR, auth header for others)
- `pushBlob(registry, repo, digest, data, token)` -- POST initiate + PUT complete
- `pushManifest(registry, repo, tag, manifest, token)` -- PUT manifest
- `buildOCIManifest(configDigest, configSize, layerDigest, layerSize)` -- builds OCI manifest JSON

**E2E impact:** New functionality. Enables `docker push` flow. Existing tests unaffected (they never call ImagePush).

### 3.11 ImageSave -- Keep Delegate

**File:** `backend_delegates_gen.go` (existing)

**Behavior:** `s.BaseServer.ImageSave(names)` -- exports images as Docker-format tar.

**Rationale:** In-memory export is correct.

### 3.12 ImageSearch -- Keep Delegate

**File:** `backend_delegates_gen.go` (existing)

**Behavior:** `s.BaseServer.ImageSearch(term, limit, filters)` -- searches in-memory store.

**Rationale:** In-memory search is adequate. Could be enhanced to search AR in the future.

### 3.13 AuthLogin -- Keep Current

**File:** `backend_impl.go` (existing)

**Behavior:** Detects GCR/AR addresses, logs warning, delegates to BaseServer.

**Rationale:** Already cloud-aware.

### 3.14 ContainerCommit -- Keep Current

**File:** `backend_impl.go` (existing)

**Behavior:** Returns `NotImplementedError`.

**Rationale:** Creating images from Cloud Run container state is not feasible -- there's no filesystem access.

---

## 4. Caching Strategy

### 4.1 Current Caching

- **In-memory store**: `s.Store.Images` holds all pulled/built images. BaseServer's `ImagePull` checks `s.Store.ResolveImage(ref)` first and returns "up to date" if found.
- **Core image config cache**: `backends/core/registry.go` has `imageConfigCache` (sync.RWMutex + map) that caches `FetchImageConfig()` results globally.
- **Cloud Run's ImagePull**: Does NOT check the in-memory store first -- it always calls `fetchImageConfig()` then stores. This means re-pulling always succeeds but always contacts the registry.

### 4.2 Proposed Caching Enhancement

Add the same early-return pattern that BaseServer uses:

```go
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
    // ... normalize ref ...

    // Check if already pulled
    if _, exists := s.Store.ResolveImage(ref); exists {
        // Return "up to date" without hitting the registry
        pr, pw := io.Pipe()
        go func() {
            defer pw.Close()
            json.NewEncoder(pw).Encode(map[string]string{
                "status": fmt.Sprintf("Status: Image is up to date for %s", ref),
            })
        }()
        return pr, nil
    }

    // ... existing fetch logic ...
}
```

This avoids redundant registry calls when the same image is pulled multiple times (common in CI/CD pipelines).

---

## 5. E2E Test Compatibility

### 5.1 Existing Tests That Touch Images

**`tests/images_test.go`:**
- `TestImagePull` -- pulls `alpine`, checks progress output. Will continue to work.
- `TestImageInspect` -- pulls `alpine`, inspects. Will continue to work.
- `TestImageTag` -- pulls `alpine`, tags, inspects. Will continue to work.

**`backends/cloudrun/integration_test.go`:**
- Multiple tests call `dockerClient.ImagePull(ctx, "alpine:latest", ...)` before creating containers. Will continue to work.

**No existing tests call:**
- `ImagePush` (was NotImplementedError)
- `ImageLoad` (was NotImplementedError)
- `ImageBuild` (uses BaseServer delegate)
- `ImageSearch`, `ImageSave`, `ImagePrune`

### 5.2 New Tests to Add

After implementing ImagePush and ImageLoad changes:

```go
func TestImagePushToAR(t *testing.T) {
    // Pull an image, tag it for AR, push it
    pullImage(t, "alpine:latest")
    dockerClient.ImageTag(ctx, "alpine:latest", "us-docker.pkg.dev/test-project/docker/alpine:pushed")
    rc, err := dockerClient.ImagePush(ctx, "us-docker.pkg.dev/test-project/docker/alpine", image.PushOptions{})
    // Verify progress output, no error
}

func TestImagePushThenPull(t *testing.T) {
    // Push, remove local, pull back
    // Verifies round-trip through AR simulator OCI endpoint
}

func TestImageLoad(t *testing.T) {
    // Save an image, load it back under a new tag
    // Verifies ImageLoad no longer returns NotImplementedError
}
```

---

## 6. Implementation Phases

### Phase 1: Low-Risk Improvements (3 changes)
1. **ImageLoad**: Remove NotImplementedError, delegate to BaseServer (~1 line change)
2. **ImagePull caching**: Add early-return for already-pulled images (~10 lines)
3. **ImagePull simulator routing**: Route AR/GCR requests through simulator when EndpointURL is set (~15 lines in `registry.go`)

### Phase 2: ImagePush via OCI Distribution (~150 lines)
4. **ImagePush**: Replace NotImplementedError with OCI push logic
5. **Helper methods**: `resolveRegistryForPush`, `pushBlob`, `pushManifest`
6. **Config**: Add `DefaultARRepo` field

### Phase 3: Cloud Build Integration (Stretch Goal, ~300+ lines)
7. **Cloud Build simulator**: New file `simulators/gcp/cloudbuild.go`
8. **ImageBuild override**: Submit to Cloud Build instead of synthetic parse
9. **GCS context upload**: Upload build context tar to GCS
10. **GCPClients**: Add `CloudBuild *cloudbuild.Client`

**Recommendation:** Implement Phases 1 and 2. Defer Phase 3 unless Cloud Build is a user requirement.

---

## 7. Summary Table

| Method | Current | Proposed | Change |
|--------|---------|----------|--------|
| ImagePull | Cloud-native (registry fetch) | Add caching + simulator routing | Minor enhancement |
| ImageInspect | BaseServer delegate | Keep | None |
| ImageLoad | NotImplementedError | BaseServer delegate | Unblock |
| ImageTag | BaseServer delegate | Keep | None |
| ImageList | BaseServer delegate | Keep | None |
| ImageRemove | BaseServer delegate | Keep | None |
| ImageHistory | BaseServer delegate | Keep | None |
| ImagePrune | BaseServer delegate | Keep | None |
| ImageBuild | BaseServer delegate | Keep (Phase 1) / Cloud Build (Phase 3) | None / Future |
| ImagePush | NotImplementedError | OCI push to AR | New implementation |
| ImageSave | BaseServer delegate | Keep | None |
| ImageSearch | BaseServer delegate | Keep | None |
| AuthLogin | Cloud-native (AR detection) | Keep | None |
| ContainerCommit | NotImplementedError | Keep | None |
