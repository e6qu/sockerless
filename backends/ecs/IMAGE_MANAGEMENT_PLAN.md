# ECS Backend: Image Management Plan

## 1. Current State

### ECR Simulator (simulators/aws/ecr.go)
The AWS simulator already supports 12 ECR API actions:
- **CreateRepository** / **DescribeRepositories** / **DeleteRepository** -- full CRUD
- **GetAuthorizationToken** -- returns base64 `AWS:password` token
- **PutImage** -- stores image manifest + tags, generates digest, indexes by tag and digest
- **BatchGetImage** -- retrieves images by tag or digest
- **BatchCheckLayerAvailability** -- always returns AVAILABLE (layer uploads are no-ops)
- **PutLifecyclePolicy** / **GetLifecyclePolicy** / **DeleteLifecyclePolicy** -- full CRUD
- **ListTagsForResource** / **TagResource** -- stub implementations

The simulator does NOT implement the Docker Registry V2 HTTP API (no `/v2/` endpoints). It only implements the ECR JSON-RPC API via `X-Amz-Target` headers.

### ECS Backend Image Methods

| Method | Current Impl | Behavior |
|--------|-------------|----------|
| **ImagePull** | Custom (`backend_impl.go:890`) | Fetches image config from real registry via V2 API (`registry.go`). Falls back to synthetic if fetch fails. Stores in-memory via `StoreImageWithAliases`. ECR auth via `getECRToken()` (ECR SDK). |
| **ImageLoad** | Custom (`backend_impl.go:955`) | Returns `NotImplementedError` |
| **ImageBuild** | Delegate to BaseServer | Synthetic build: parses Dockerfile, generates synthetic image, returns JSON progress stream |
| **ImagePush** | Delegate to BaseServer | Synthetic push: returns fake progress without pushing anywhere |
| **ImageInspect** | Delegate to BaseServer | Returns in-memory image data |
| **ImageList** | Delegate to BaseServer | Lists in-memory images with filter support |
| **ImageTag** | Delegate to BaseServer | Creates alias in in-memory store |
| **ImageRemove** | Delegate to BaseServer | Removes from in-memory store |
| **ImageHistory** | Delegate to BaseServer | Returns synthetic single-layer history |
| **ImagePrune** | Delegate to BaseServer | Removes dangling images from in-memory store |
| **ImageSave** | Delegate to BaseServer | Exports tar archive of image metadata (no layers) |
| **ImageSearch** | Delegate to BaseServer | Searches in-memory images by term |

### AWS SDK Clients Available
`AWSClients` already includes `ECR *ecr.Client` -- used by `fetchImageConfig` for auth tokens.

### E2E Test Expectations
- `TestImagePull` -- expects streaming progress output, non-zero bytes
- `TestImageInspect` -- expects non-empty ID, RepoTags, Os, Architecture
- `TestImageTag` -- expects tagged image to be inspectable by new tag
- `TestImageBuild` -- **expects 200 status**, streaming JSON with `stream` and `aux` messages, image inspectable with correct entrypoint afterward

**Critical constraint**: `TestImageBuild` runs against all backends via the frontend. The ECS `ImageBuild` currently delegates to BaseServer which returns 200 with synthetic output. If we change this to `NotImplementedError`, the e2e test will break.

### Integration Tests (sim-test-ecs)
The ECS integration tests (`integration_test.go`) only use `ImagePull` (alpine:latest) as a prerequisite for container lifecycle tests. No dedicated image management tests exist.

---

## 2. Design Principles

1. **In-memory store is the source of truth** for the Docker API layer. ECR is a backing store for persistence and cross-instance sharing.
2. **ECR integration is optional** -- when the simulator is unreachable or ECR calls fail, fall back to synthetic behavior (current default).
3. **No breaking changes to e2e tests** -- `ImageBuild` must continue returning 200.
4. **Lazy ECR repo creation** -- auto-create ECR repositories when needed (pull, push, build).
5. **ECR URI format**: `{registryId}.dkr.ecr.{region}.amazonaws.com/{repo}:{tag}`

---

## 3. ECR Integration Architecture

### Image Naming Convention

When a user does `docker pull alpine:latest`, the ECS backend should:
1. Store the image in-memory as `alpine:latest` (for Docker API compatibility)
2. Optionally mirror to ECR as `{registryId}.dkr.ecr.{region}.amazonaws.com/sockerless/alpine:latest`

The `sockerless/` prefix namespace avoids collisions with user-managed ECR repositories.

### ECR Repository Naming
```
sockerless/{normalized-image-name}
```
Examples:
- `alpine` -> `sockerless/alpine`
- `library/nginx` -> `sockerless/nginx`
- `myuser/myapp` -> `sockerless/myuser/myapp`
- `gcr.io/project/image` -> `sockerless/gcr.io/project/image`

### Caching Strategy

The in-memory store (`s.Store.Images`) already functions as a cache. The flow:

1. **ImagePull**: Check in-memory store first. If not found (or force-pull), fetch config from registry. Store in-memory. Optionally record in ECR via `PutImage` for persistence.
2. **ContainerCreate**: Resolves image from in-memory store. If not found, returns error (user must pull first -- standard Docker behavior).
3. **ContainerStart**: ECS task definition references the original image URI (e.g., `alpine:latest` or ECR URI). Fargate/ECS pulls the actual image layers from the original registry.

**Key insight**: The ECS backend does not need to proxy image layers through ECR. Fargate pulls images directly from their source registries. ECR integration is only needed when:
- The user explicitly pushes to ECR
- Building images that need to be stored somewhere Fargate can pull
- Persisting image metadata across backend restarts

---

## 4. Per-Method Implementation Plan

### 4.1 ImagePull (KEEP CURRENT + MINOR ENHANCEMENT)

**Current**: Fetches image config from registry V2 API. Stores in-memory. Returns progress stream.

**Enhancement**: After storing in-memory, optionally record the image reference in ECR via `PutImage` with a synthetic manifest. This enables cross-instance image discovery.

**Implementation**:
```
1. Parse ref, add :latest if needed
2. Fetch image config from registry (existing fetchImageConfig)
3. Generate image ID, create api.Image
4. Store via StoreImageWithAliases (existing)
5. (Optional) Best-effort ECR PutImage to record the ref
6. Return progress stream (existing)
```

**Priority**: Low -- current implementation is fully functional. ECR recording is a future enhancement.

**No changes needed for e2e test compatibility.**

### 4.2 ImageInspect (KEEP DELEGATE)

**Current**: Delegates to BaseServer. Returns in-memory image data.

**Why adequate**: Image data is populated by ImagePull/ImageBuild/ImageLoad. The in-memory store has all needed fields. No ECR query needed.

**No changes needed.**

### 4.3 ImageLoad (KEEP CURRENT)

**Current**: Returns `NotImplementedError`.

**Why correct**: `docker load` imports a tar archive of image layers. The ECS backend has nowhere to store actual layers -- Fargate pulls from registries. Loading a tar into memory without actual layer storage would be misleading.

**Alternative considered**: Accept the tar, extract the manifest to get image metadata, store synthetic image in-memory. This would make `docker save | docker load` round-trip work for metadata but not for actual layers.

**Recommendation**: Keep `NotImplementedError`. Users should push images to ECR and pull from there.

**No changes needed.**

### 4.4 ImageTag (KEEP DELEGATE)

**Current**: Delegates to BaseServer. Creates alias in in-memory store.

**Why adequate**: Tagging is a metadata operation. The in-memory store correctly maintains tag aliases. When a container is created with the new tag, it resolves to the same image config.

**Future enhancement**: If ECR integration is added, also call `PutImage` with the new tag to persist the alias.

**No changes needed.**

### 4.5 ImageList (KEEP DELEGATE)

**Current**: Delegates to BaseServer. Lists in-memory images with filter support.

**Why adequate**: All images that matter are in the in-memory store (populated by pull/build). ECR may have additional images, but listing them would be confusing (they might not be usable without a pull).

**Future enhancement**: Add an option to merge ECR repository listing with in-memory images. Would require `ecr:DescribeRepositories` + `ecr:DescribeImages`.

**No changes needed.**

### 4.6 ImageRemove (KEEP DELEGATE)

**Current**: Delegates to BaseServer. Removes from in-memory store.

**Why adequate**: Removing from in-memory prevents the image from being used in new containers. ECR images are independent.

**Future enhancement**: Also call `ecr:BatchDeleteImage` to remove from ECR when the image URI matches the ECR registry.

**No changes needed.**

### 4.7 ImageHistory (KEEP DELEGATE)

**Current**: Delegates to BaseServer. Returns synthetic single-layer history.

**Why adequate**: Real image layer history is not available without pulling actual layers. The synthetic response satisfies Docker API clients.

**No changes needed.**

### 4.8 ImagePrune (KEEP DELEGATE)

**Current**: Delegates to BaseServer. Removes dangling images from in-memory store.

**Why adequate**: Prune is a cleanup operation. In-memory cleanup is sufficient. ECR has its own lifecycle policies for cleanup.

**No changes needed.**

### 4.9 ImageBuild (KEEP DELEGATE -- CRITICAL)

**Current**: Delegates to BaseServer. Parses Dockerfile, generates synthetic image, returns JSON progress stream with `stream` and `aux` messages. Returns 200.

**Why it MUST stay this way**: `TestImageBuild` in `tests/system_test.go` expects:
- HTTP 200 response
- JSON lines with `stream` messages (step output)
- JSON line with `aux` message (image ID)
- Image inspectable afterward with correct entrypoint

Returning `NotImplementedError` (501) would break this test.

**The synthetic build is actually the right approach for a simulator.** Real Docker also doesn't need BuildKit for simple Dockerfiles. The BaseServer's Dockerfile parser handles FROM, ENV, CMD, ENTRYPOINT, COPY, WORKDIR, USER, LABEL, EXPOSE, HEALTHCHECK, SHELL, STOPSIGNAL, VOLUME, and ARG.

**Future CodeBuild integration**: For production use (not simulator testing), a CodeBuild-based build could be added behind a feature flag:
1. Upload build context to S3
2. Start CodeBuild project that runs `docker build` and pushes to ECR
3. Stream CodeBuild logs back as Docker build progress
4. On completion, record the ECR image in the in-memory store

This would be a new feature, not a replacement for the synthetic build.

**No changes needed.**

### 4.10 ImagePush (KEEP DELEGATE or ENHANCE)

**Current**: Delegates to BaseServer. Returns synthetic "pushed" progress without actually pushing.

**Two options**:

**Option A (Keep delegate)**: The synthetic push satisfies API clients. The image was already pulled from its source registry, and Fargate can pull from that same source. No real push needed.

**Option B (ECR push)**: When the target is an ECR registry, use ECR APIs to record the image:
1. Parse the push target to determine if it's ECR
2. Ensure the ECR repository exists (`CreateRepository`, ignore `RepositoryAlreadyExistsException`)
3. Call `PutImage` with a synthetic manifest
4. Return progress stream

**Recommendation**: Keep delegate for now. The synthetic push is functionally correct for the simulator use case. ECR push can be added as an enhancement when cross-instance image sharing is needed.

**Note**: The IMPLEMENTATION_PLAN.md says ImagePush returns `NotImplementedError`, but `backend_delegates_gen.go` shows it delegates to BaseServer which returns 200. The delegate is correct.

### 4.11 ImageSave (KEEP DELEGATE)

**Current**: Delegates to BaseServer. Exports tar archive containing image metadata JSON and manifest.

**Why adequate**: The tar contains enough metadata for `docker load` to recreate the image on a real Docker daemon. For the simulator, this is the expected behavior.

**No changes needed.**

### 4.12 ImageSearch (KEEP DELEGATE)

**Current**: Delegates to BaseServer. Searches in-memory images by term.

**Why adequate**: Docker Hub search API is separate from registry operations. The in-memory search covers locally-known images. ECR doesn't have a search API (only describe/list).

**No changes needed.**

---

## 5. Lambda Specifics

Lambda uses ECR image URIs directly in `CreateFunction`:
```go
Code: &lambdatypes.FunctionCode{
    ImageUri: aws.String(config.Image),
}
```

This means:
- **Lambda requires images to be in ECR** (or Docker Hub via Lambda's built-in pull)
- Lambda's `ImagePull` stores metadata in-memory but the actual image must be accessible to the Lambda service
- Lambda's `ImageBuild` correctly returns `NotImplementedError` -- users must pre-build and push to ECR
- Lambda's `ImagePush` correctly returns `NotImplementedError` -- users must push to ECR directly

See `backends/lambda/IMAGE_MANAGEMENT_PLAN.md` for Lambda-specific details.

---

## 6. BuildKit Considerations

### What's Feasible via CodeBuild
- Multi-stage builds
- Build arguments
- Build caching (CodeBuild supports Docker layer caching)
- Private base images (CodeBuild can authenticate to ECR)
- Build secrets (CodeBuild has Secrets Manager integration)

### What Should Stay Synthetic
- Simple single-stage Dockerfiles (BaseServer handles these well)
- Simulator testing (no real cloud resources needed)
- Development workflows (fast feedback, no CodeBuild wait time)

### Recommendation
Keep synthetic builds as default. Add CodeBuild integration as an opt-in feature behind `SOCKERLESS_BUILD_BACKEND=codebuild` for production deployments that need real image artifacts.

---

## 7. Implementation Phases

### Phase 1: No Changes (Current Sprint)
All 12 image methods work correctly for the simulator use case:
- ImagePull: fetches real config from registry, stores in-memory
- ImageBuild: synthetic build, returns 200
- 10 others: delegate to BaseServer, all functional

### Phase 2: ECR Recording (Future)
Add best-effort ECR `PutImage` calls to persist image references:
- After ImagePull: record pulled ref in ECR
- After ImageBuild: record built image in ECR
- After ImageTag: record new tag in ECR
- ECR failures are non-fatal (log warning, continue)

### Phase 3: ECR Push Integration (Future)
Override ImagePush to actually push to ECR when target is an ECR registry:
- Auto-create repository if needed
- Call `PutImage` with image manifest
- Return real progress stream

### Phase 4: CodeBuild Integration (Future, Optional)
Add opt-in real builds via CodeBuild:
- Upload build context to S3
- Start CodeBuild project
- Stream logs as Docker build progress
- Push result to ECR
- Record in in-memory store

---

## 8. Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| TestImageBuild breaks | High -- blocks CI | Keep BaseServer delegation (returns 200) |
| ECR calls fail | Low -- fallback to in-memory | All ECR calls are best-effort, non-fatal |
| Registry V2 fetch fails | Low -- fallback to synthetic config | Already handled in fetchImageConfig |
| Image not in ECR for Fargate pull | Medium -- container start fails | Fargate pulls from original registry, not ECR |

---

## 9. Summary

The ECS backend's current image management is well-suited for the simulator use case:

1. **ImagePull** already fetches real config from registries (including ECR auth)
2. **ImageBuild** works correctly via BaseServer's synthetic Dockerfile parser
3. **All other methods** work correctly via BaseServer delegation
4. **No changes are needed** for current test compatibility
5. **ECR integration** is a future enhancement for production deployments, not a simulator requirement

The key architectural insight is that **Fargate pulls images from their source registries**, not from the backend's in-memory store. The in-memory store is only used for Docker API metadata operations (inspect, list, tag, remove). This means the backend doesn't need to proxy image layers -- it only needs image config metadata, which it already has.
