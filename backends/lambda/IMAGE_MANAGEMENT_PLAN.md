# Lambda Backend: Image Management Plan

## 1. Current State

### Lambda Image Methods

| Method | Current Impl | Behavior |
|--------|-------------|----------|
| **ImagePull** | Custom (`backend_impl.go:813`) | Generates synthetic image, tries `core.FetchImageConfig` for real config. Stores in-memory. Returns progress stream. |
| **ImageLoad** | Custom (`backend_impl.go:887`) | Returns `NotImplementedError` |
| **ImageBuild** | Custom (`backend_impl.go:893`) | Returns `NotImplementedError` |
| **ImagePush** | Custom (`backend_impl.go:970`) | Returns `NotImplementedError` |
| **ImageInspect** | Delegate to BaseServer | Returns in-memory image data |
| **ImageList** | Delegate to BaseServer | Lists in-memory images |
| **ImageTag** | Delegate to BaseServer | Creates alias in in-memory store |
| **ImageRemove** | Delegate to BaseServer | Removes from in-memory store |
| **ImageHistory** | Delegate to BaseServer | Synthetic single-layer history |
| **ImagePrune** | Delegate to BaseServer | Removes dangling images |
| **ImageSave** | Delegate to BaseServer | Exports tar with image metadata |
| **ImageSearch** | Delegate to BaseServer | Searches in-memory images |

### AWS SDK Clients Available
`AWSClients` has `Lambda *lambda.Client` and `CloudWatch *cloudwatchlogs.Client`. **No ECR client.**

### Key Difference from ECS

Lambda's `ContainerCreate` passes the image reference directly to the Lambda service:
```go
Code: &lambdatypes.FunctionCode{
    ImageUri: aws.String(config.Image),
}
```

Lambda resolves and pulls the image internally. The backend never needs to transfer image layers. The in-memory image store exists solely for Docker API metadata (inspect, list, etc.).

---

## 2. Lambda-Specific Constraints

### Image Sources for Lambda Functions
Lambda supports container images from:
1. **ECR** (primary): `{acctId}.dkr.ecr.{region}.amazonaws.com/{repo}:{tag}`
2. **ECR Public**: `public.ecr.aws/{alias}/{repo}:{tag}`

In the **simulator**, Lambda accepts any `ImageUri` string -- it doesn't validate that the image exists in ECR. This means:
- `docker pull alpine && docker run alpine echo hello` works in the simulator
- In production, the image must be in ECR

### No Real Image Manipulation
Lambda functions are immutable once created. You cannot:
- Load images into Lambda (`ImageLoad` -> `NotImplementedError`)
- Build images on Lambda (`ImageBuild` -> `NotImplementedError`)
- Push images from Lambda (`ImagePush` -> `NotImplementedError`)
- Export container filesystem (`ContainerExport` -> `NotImplementedError`)
- Commit container to image (`ContainerCommit` -> `NotImplementedError`)

These are all correctly returning `NotImplementedError`.

---

## 3. Per-Method Assessment

### 3.1 ImagePull (ADEQUATE)

**Current behavior**: Uses `core.FetchImageConfig` (shared helper) to try fetching real config from the registry. Falls back to synthetic config with just the image name. Stores in-memory.

**Difference from ECS**: The ECS backend has its own `fetchImageConfig` in `registry.go` with ECR-specific auth (`getECRToken`). The Lambda backend uses the core helper which does not have ECR auth.

**Gap**: If a user does `docker pull {ecrUri}` on the Lambda backend, the config fetch may fail because `core.FetchImageConfig` doesn't have ECR credentials. The image will still be stored with a synthetic config (Image field set, no Env/Cmd/Entrypoint).

**Recommendation**: Low priority. The synthetic config is usually sufficient because Lambda's `CreateFunction` sets `ImageConfig.EntryPoint`/`Command` from the container config, and users typically set these explicitly.

**Future enhancement**: Add ECR client to Lambda's `AWSClients` and use it for auth when pulling ECR images.

### 3.2 ImageLoad (CORRECT)

Returns `NotImplementedError`. Lambda cannot accept arbitrary image tars.

### 3.3 ImageBuild (CORRECT)

Returns `NotImplementedError` with message directing users to push pre-built images to ECR.

**Note**: Unlike the ECS backend, Lambda does NOT delegate to BaseServer for ImageBuild. This is intentional -- a synthetic build that stores an image in-memory would be misleading because Lambda needs images in ECR.

**E2E test impact**: `TestImageBuild` sends a build request to the **frontend**, which routes to whatever backend is configured. When Lambda is the backend, this will return 501. This is acceptable because:
- The e2e test suite runs against the default backend (core/in-process), not Lambda specifically
- Lambda's integration tests (`sim-test-lambda`) don't test ImageBuild
- Production Lambda users should use ECR directly

### 3.4 ImagePush (CORRECT)

Returns `NotImplementedError` directing users to push to ECR directly.

### 3.5 ImageInspect through ImageSearch (ADEQUATE)

All delegate to BaseServer. In-memory operations are sufficient for Lambda's use case.

---

## 4. Architecture Comparison: ECS vs Lambda

| Aspect | ECS | Lambda |
|--------|-----|--------|
| Image pull source | Fargate pulls from original registry | Lambda pulls from ECR (prod) or accepts any URI (sim) |
| ECR client | Yes (`ecr.Client` in AWSClients) | No -- would need to add |
| Registry auth | Custom `fetchImageConfig` with ECR auth | `core.FetchImageConfig` without ECR auth |
| ImageBuild | Delegates to BaseServer (returns 200) | Returns `NotImplementedError` (returns 501) |
| ImagePush | Delegates to BaseServer (synthetic 200) | Returns `NotImplementedError` (returns 501) |
| Image in task/function | Task def references `Image` field | CreateFunction references `ImageUri` |

---

## 5. Future Enhancements

### Phase 1: ECR Auth for ImagePull
Add `ecr.Client` to Lambda's `AWSClients`. When pulling an ECR image, use `GetAuthorizationToken` for auth. This brings Lambda's ImagePull to parity with ECS.

**Effort**: Low (add ECR client, replicate ECS `getECRToken` pattern).

### Phase 2: ECR Validation
Before `CreateFunction`, optionally validate that the image URI points to a valid ECR repository. This would catch configuration errors early instead of at function invocation time.

**Effort**: Low (add `BatchGetImage` call after `CreateFunction`).

### Phase 3: Integrated Build+Deploy
For production workflows, support a build pipeline:
1. Accept Dockerfile + context via ImageBuild
2. Submit to CodeBuild
3. Push result to ECR
4. Return ECR URI for use in ContainerCreate

This would be behind a feature flag and shares the CodeBuild integration planned for the ECS backend.

**Effort**: High (shared with ECS Phase 4).

---

## 6. Summary

Lambda's image management is more constrained than ECS because Lambda functions are immutable and require ECR images in production. The current implementation correctly:

1. **ImagePull**: Works for simulator use (synthetic config fallback)
2. **ImageBuild/Push/Load**: Correctly returns `NotImplementedError`
3. **Metadata operations** (inspect, list, tag, etc.): Correctly delegates to BaseServer

The main gap is ECR auth in ImagePull, which is low priority because:
- The simulator accepts any image URI
- Users can set Cmd/Entrypoint explicitly in ContainerCreate
- The synthetic config is sufficient for most test scenarios
