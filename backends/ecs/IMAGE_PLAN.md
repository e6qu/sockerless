# AWS Unified Image Management Plan (ECS + Lambda)

This plan implements the global architecture from `backends/IMAGE_ARCHITECTURE.md` for the AWS cloud (ECS and Lambda backends). It creates an `ECRAuthProvider` shared by both backends, wires each backend to use `core.ImageManager`, and deletes all duplicated registry/image code.

---

## 1. ECRAuthProvider Definition

**File**: `backends/ecs/image_auth.go` (new, ~80 lines)

```go
package ecs

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// ECRAuthProvider implements core.AuthProvider for AWS ECR.
type ECRAuthProvider struct {
	ECR    *ecr.Client
	Logger zerolog.Logger // note: IMAGE_ARCHITECTURE.md says *slog.Logger but project uses zerolog
	Ctx    func() context.Context // returns context.Background(); avoids storing context
}

// GetToken returns a raw base64-encoded auth token from ECR GetAuthorizationToken.
// The token is base64("user:password") — no "Basic " prefix. The ImageManager is
// responsible for adding the appropriate scheme prefix when passing to FetchImageConfig.
// Returns ("", nil) for non-ECR registries (fallthrough to anonymous/Docker Hub auth).
func (p *ECRAuthProvider) GetToken(registry string) (string, error) {
	if !p.IsCloudRegistry(registry) {
		return "", nil
	}
	result, err := p.ECR.GetAuthorizationToken(p.Ctx(), &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", fmt.Errorf("ECR GetAuthorizationToken: %w", err)
	}
	if len(result.AuthorizationData) == 0 {
		return "", fmt.Errorf("ECR returned no authorization data")
	}
	// Token is base64-encoded "user:password" — return raw, no scheme prefix
	token := aws.ToString(result.AuthorizationData[0].AuthorizationToken)
	return token, nil
}

// IsCloudRegistry returns true if the registry matches *.dkr.ecr.*.amazonaws.com.
func (p *ECRAuthProvider) IsCloudRegistry(registry string) bool {
	return strings.HasSuffix(registry, ".amazonaws.com") && strings.Contains(registry, ".dkr.ecr.")
}

// OnPush is called after a successful in-memory push. Creates the ECR repository
// (ignoring AlreadyExists) and calls PutImage with a synthetic manifest.
func (p *ECRAuthProvider) OnPush(img api.Image, registry, repo, tag string) error {
	_, err := p.ECR.CreateRepository(p.Ctx(), &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repo),
	})
	if err != nil && !isECRAlreadyExistsError(err) {
		return fmt.Errorf("ECR CreateRepository(%s): %w", repo, err)
	}
	manifest := fmt.Sprintf(
		`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"digest":"%s"}}`,
		img.ID,
	)
	_, err = p.ECR.PutImage(p.Ctx(), &ecr.PutImageInput{
		RepositoryName: aws.String(repo),
		ImageManifest:  aws.String(manifest),
		ImageTag:       aws.String(tag),
	})
	if err != nil {
		return fmt.Errorf("ECR PutImage(%s:%s): %w", repo, tag, err)
	}
	return nil
}

// OnTag is called after a successful in-memory tag. Records the new tag in ECR
// via PutImage (same manifest, new tag). Equivalent to OnPush.
func (p *ECRAuthProvider) OnTag(img api.Image, registry, repo, newTag string) error {
	return p.OnPush(img, registry, repo, newTag)
}

// OnRemove is called after a successful in-memory remove. Calls BatchDeleteImage
// to remove the specified tags from the ECR repository.
func (p *ECRAuthProvider) OnRemove(registry, repo string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	imageIds := make([]ecrtypes.ImageIdentifier, len(tags))
	for i, t := range tags {
		imageIds[i] = ecrtypes.ImageIdentifier{ImageTag: aws.String(t)}
	}
	_, err := p.ECR.BatchDeleteImage(p.Ctx(), &ecr.BatchDeleteImageInput{
		RepositoryName: aws.String(repo),
		ImageIds:       imageIds,
	})
	if err != nil {
		return fmt.Errorf("ECR BatchDeleteImage(%s, %v): %w", repo, tags, err)
	}
	return nil
}

// isECRAlreadyExistsError checks for RepositoryAlreadyExistsException.
func isECRAlreadyExistsError(err error) bool {
	return strings.Contains(err.Error(), "RepositoryAlreadyExistsException")
}
```

### ECR SDK Calls Summary

| Method | ECR SDK Call | Input | Error Handling |
|--------|-------------|-------|---------------|
| `GetToken` | `ecr.GetAuthorizationToken` | `{}` (no params) | Return error; caller falls back to anonymous. Returns raw base64 token (no scheme prefix). |
| `IsCloudRegistry` | None (string match) | Registry hostname | N/A |
| `OnPush` | `ecr.CreateRepository` then `ecr.PutImage` | Repo name; synthetic manifest + tag | Log + ignore (non-fatal) |
| `OnTag` | Same as OnPush | Same as OnPush | Log + ignore (non-fatal) |
| `OnRemove` | `ecr.BatchDeleteImage` | Repo name + `[]ImageIdentifier` with tags | Log + ignore (non-fatal) |

---

## 2. Core ImageManager (prerequisite -- defined in IMAGE_ARCHITECTURE.md)

The `core.ImageManager` struct must exist in `backends/core/image_manager.go` before this plan executes. It provides all 12 `api.Backend` image methods as methods on `ImageManager`, delegating to the existing `BaseServer` logic but calling `AuthProvider` hooks at the right points.

The 12 image methods on `api.Backend` are:
1. `ImagePull(ref, auth string) (io.ReadCloser, error)`
2. `ImageInspect(name string) (*api.Image, error)`
3. `ImageLoad(r io.Reader) (io.ReadCloser, error)`
4. `ImageTag(source, repo, tag string) error`
5. `ImageList(opts api.ImageListOptions) ([]*api.ImageSummary, error)`
6. `ImageRemove(name string, force, prune bool) ([]*api.ImageDeleteResponse, error)`
7. `ImageHistory(name string) ([]*api.ImageHistoryEntry, error)`
8. `ImagePrune(filters map[string][]string) (*api.ImagePruneResponse, error)`
9. `ImageBuild(opts api.ImageBuildOptions, ctx io.Reader) (io.ReadCloser, error)`
10. `ImagePush(name, tag, auth string) (io.ReadCloser, error)`
11. `ImageSave(names []string) (io.ReadCloser, error)`
12. `ImageSearch(term string, limit int, filters map[string][]string) ([]*api.ImageSearchResult, error)`

Note: `AuthLogin` (the 13th image-adjacent method) is NOT part of `ImageManager`. Both ECS and Lambda already have `AuthLogin` implementations (ECS delegates to BaseServer, Lambda has a custom override that warns about ECR). These remain unchanged.

The `ImageManager` calls `AuthProvider.GetToken()` in `Pull` to get ECR auth, calls `AuthProvider.OnPush()` after `Pull`/`Push`/`Tag`, and calls `AuthProvider.OnRemove()` after `Remove`. Methods 2, 5, 7, 8, 9, 11, 12 are pure in-memory operations that do not touch `AuthProvider` at all.

### Auth Threading: FetchImageConfig + ECR Token

**Critical detail**: `core.FetchImageConfig(ref, basicAuth ...string)` passes `basicAuth[0]` to `getRegistryToken()`, which sets `req.Header.Set("Authorization", "Basic "+basicAuth)`. But `ECRAuthProvider.GetToken()` returns `"Basic " + token` (already prefixed). If `ImageManager.Pull()` passes the full `GetToken()` result to `FetchImageConfig`, the Authorization header would be set to `"Basic Basic <token>"` — double-prefixed.

**Resolution**: `ImageManager.Pull()` must strip the `"Basic "` prefix before passing to `FetchImageConfig()`. Alternatively, `GetToken()` should return the raw base64 token without any scheme prefix, and `ImageManager` adds the appropriate prefix when calling `FetchImageConfig`. The cleanest approach is for `GetToken()` to return the raw token (no "Basic " / "Bearer " prefix) and let `ImageManager` or `FetchImageConfig` handle scheme prefixing.

However, this affects the `AuthProvider` interface contract across all clouds. The simplest fix specific to `FetchImageConfig` is: `ImageManager.Pull()` should call `core.FetchImageConfig(ref, strings.TrimPrefix(token, "Basic "))` when the token has a "Basic " prefix. The `ImageManager` implementation must handle this.

### ECS recordImageInECR: All Images vs Cloud-Only

**Critical detail**: The current ECS `ImagePull` calls `recordImageInECR()` for ALL pulled images (line 1086: `_ = registry // We record all pulled images for local persistence`), not just ECR-sourced ones. The `ImageManager` architecture calls `AuthProvider.OnPush()` only when `IsCloudRegistry()` returns true. This is a **behavioral change**: after migration, pulling `alpine:latest` will NOT be recorded in ECR.

**Resolution**: This is arguably the correct behavior — recording Docker Hub images in ECR was unnecessary overhead. The plan accepts this behavioral change. If preserving the old behavior is required, `ECRAuthProvider.IsCloudRegistry()` could be made to return `true` for all registries, but that would be semantically wrong. The `ImageManager` should instead have a separate `OnPull` hook (or the `ImageManager.Pull()` implementation should always call `OnPush` after pull regardless of registry). **Decision**: Accept the behavioral change. Non-ECR images do not need ECR persistence.

---

## 3. ECS Backend Wiring

### 3.1 Add `images` field to Server

**File**: `backends/ecs/server.go`

Add a `images *core.ImageManager` field to the `Server` struct:

```go
type Server struct {
	*core.BaseServer
	config       Config
	aws          *AWSClients
	images       *core.ImageManager   // NEW
	ECS          *core.StateStore[ECSState]
	NetworkState *core.StateStore[NetworkState]
	VolumeState  *core.StateStore[VolumeState]
	ipCounter    atomic.Int32
}
```

### 3.2 Initialize ImageManager in NewServer

In `NewServer()`, after `s.BaseServer` is created (so `s.Store` and `s.Logger` are available):

```go
s.images = &core.ImageManager{
	Auth:   &ECRAuthProvider{
		ECR:    awsClients.ECR,
		Logger: logger,
		Ctx:    s.ctx,
	},
	Store:  s.Store,
	Logger: logger,
}
```

### 3.3 Replace 5 custom image methods with one-liner delegates

All 5 custom image methods in `backend_impl.go` (lines 893-1122) become one-liner delegates:

**In `backend_impl.go`**, delete:
- `ImagePull` (lines 893-958, 66 lines)
- `ImagePush` (lines 963-1010, 48 lines)
- `ImageTag` (lines 1013-1035, 23 lines)
- `ImageRemove` (lines 1038-1078, 41 lines)
- `recordImageInECR` (lines 1082-1107, 26 lines)
- `isECRRegistry` (lines 1110-1112, 3 lines)
- `isECRAlreadyExistsError` (lines 1114-1117, 4 lines)
- `ImageLoad` (lines 1120-1122, 3 lines)

Total: ~214 lines deleted from `backend_impl.go`.

**Replace with** (in `backend_impl.go` or move to delegates file):

```go
func (s *Server) ImagePull(ref, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
}
func (s *Server) ImagePush(name, tag, auth string) (io.ReadCloser, error) {
	return s.images.Push(name, tag, auth)
}
func (s *Server) ImageTag(source, repo, tag string) error {
	return s.images.Tag(source, repo, tag)
}
func (s *Server) ImageRemove(name string, force, prune bool) ([]*api.ImageDeleteResponse, error) {
	return s.images.Remove(name, force, prune)
}
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{Message: "image load is not supported by ECS backend"}
}
```

### 3.4 Update delegates file

**File**: `backends/ecs/backend_delegates_gen.go`

The following 7 methods already delegate to `BaseServer` and should be changed to delegate to `s.images`:

| Method | Current delegate target | New delegate target |
|--------|------------------------|---------------------|
| `ImageBuild` | `s.BaseServer.ImageBuild` | `s.images.Build` |
| `ImageHistory` | `s.BaseServer.ImageHistory` | `s.images.History` |
| `ImageInspect` | `s.BaseServer.ImageInspect` | `s.images.Inspect` |
| `ImageList` | `s.BaseServer.ImageList` | `s.images.List` |
| `ImagePrune` | `s.BaseServer.ImagePrune` | `s.images.Prune` |
| `ImageSave` | `s.BaseServer.ImageSave` | `s.images.Save` |
| `ImageSearch` | `s.BaseServer.ImageSearch` | `s.images.Search` |

Note: Since `ImageManager` methods ultimately call the same `Store` methods that `BaseServer` does, this is functionally equivalent. The benefit is that all 12 image methods go through `ImageManager`, making the flow uniform.

**Important**: `ImageManager.Build()` must have access to `BaseServer`'s synthetic Dockerfile parser logic (tar extraction, `FROM`/`ENTRYPOINT`/`CMD`/`ENV`/`LABEL` parsing). This means `ImageManager` either needs a reference to `BaseServer` or the Dockerfile parser must be extracted into a standalone function in `core/`. The core ImageManager prerequisite task must address this — `ImageManager` cannot be a pure `Store`-only struct if it needs to support `Build`.

### 3.5 Delete registry.go

**File**: `backends/ecs/registry.go` (196 lines) -- DELETE ENTIRELY.

Functions deleted:
- `fetchImageConfig()` (lines 17-128, 112 lines) -- replaced by `core.FetchImageConfig()` called inside `ImageManager.Pull()`
- `parseImageRef()` (lines 131-161, 31 lines) -- replaced by `core.parseImageRef()` (already exists, unexported; ImageManager uses it internally)
- `getECRToken()` (lines 164-176, 13 lines) -- replaced by `ECRAuthProvider.GetToken()`
- `getDockerHubToken()` (lines 179-195, 17 lines) -- handled inside core's `getRegistryToken()` via Www-Authenticate flow

---

## 4. Lambda Backend Wiring

### 4.1 Lambda reuses ECRAuthProvider

The `ECRAuthProvider` type is defined in `package ecs`. Lambda cannot import it directly because `backends/lambda/` and `backends/ecs/` are separate Go modules that don't import each other.

**Two options**:

**Option A (Recommended): Duplicate ECRAuthProvider in Lambda (~30 lines)**

Create `backends/lambda/image_auth.go` with a minimal copy of `ECRAuthProvider`. This is acceptable because:
- It's ~30 lines (just `GetToken` + `IsCloudRegistry`; Lambda doesn't need `OnPush`/`OnTag`/`OnRemove`)
- Lambda's `OnPush`/`OnTag`/`OnRemove` can be no-ops (return nil) since Lambda doesn't do ECR recording
- Avoids adding a new shared module or import cycle

```go
package lambda

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// ECRAuthProvider implements core.AuthProvider for AWS ECR (Lambda variant).
// Lambda only needs GetToken for ImagePull auth. OnPush/OnTag/OnRemove are
// no-ops because Lambda doesn't record images in ECR.
type ECRAuthProvider struct {
	ECR    *ecr.Client
	Logger zerolog.Logger // note: IMAGE_ARCHITECTURE.md says *slog.Logger but project uses zerolog
	Ctx    func() context.Context
}

func (p *ECRAuthProvider) GetToken(registry string) (string, error) {
	if !p.IsCloudRegistry(registry) {
		return "", nil
	}
	result, err := p.ECR.GetAuthorizationToken(p.Ctx(), &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", fmt.Errorf("ECR GetAuthorizationToken: %w", err)
	}
	if len(result.AuthorizationData) == 0 {
		return "", fmt.Errorf("ECR returned no authorization data")
	}
	// Return raw base64 token, no scheme prefix (matches ECS variant)
	token := aws.ToString(result.AuthorizationData[0].AuthorizationToken)
	return token, nil
}

func (p *ECRAuthProvider) IsCloudRegistry(registry string) bool {
	return strings.HasSuffix(registry, ".amazonaws.com") && strings.Contains(registry, ".dkr.ecr.")
}

func (p *ECRAuthProvider) OnPush(img api.Image, registry, repo, tag string) error  { return nil }
func (p *ECRAuthProvider) OnTag(img api.Image, registry, repo, newTag string) error { return nil }
func (p *ECRAuthProvider) OnRemove(registry, repo string, tags []string) error      { return nil }
```

**Option B: Extract to shared package**

Create `backends/aws-common/` with `ECRAuthProvider`. Both ECS and Lambda import it. This adds a new Go module and more wiring. Not recommended unless a third AWS backend appears.

### 4.2 Add `images` field to Lambda Server

**File**: `backends/lambda/server.go`

```go
type Server struct {
	*core.BaseServer
	config    Config
	aws       *AWSClients
	images    *core.ImageManager   // NEW
	Lambda    *core.StateStore[LambdaState]
	ipCounter atomic.Int32
}
```

Initialize in `NewServer()`:

```go
s.images = &core.ImageManager{
	Auth:   &ECRAuthProvider{
		ECR:    awsClients.ECR,
		Logger: logger,
		Ctx:    s.ctx,
	},
	Store:  s.Store,
	Logger: logger,
}
```

### 4.3 Replace Lambda image methods

**In `backend_impl.go`**, delete:
- `ImagePull` (lines 815-895, 81 lines)
- `ImageLoad` (lines 898-900, 3 lines)
- `ImageBuild` (lines 903-908, 6 lines)
- `ImagePush` (lines 981-985, 5 lines)
- `getECRToken` (lines 988-1000, 13 lines)
- `isECRImage` (lines 1003-1011, 9 lines)

Total: ~117 lines deleted from Lambda `backend_impl.go`.

**Replace with**:

```go
func (s *Server) ImagePull(ref, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
}

func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{Message: "image load is not supported by Lambda backend"}
}

func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{
		Message: "Lambda backend does not support image build; push pre-built images to ECR and use the ECR image URI",
	}
}

func (s *Server) ImagePush(name, tag, auth string) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{
		Message: "Lambda backend does not support image push; push images directly to ECR",
	}
}
```

### 4.4 Update Lambda delegates file

**File**: `backends/lambda/backend_delegates_gen.go`

Change the following 7 methods from `s.BaseServer.Image*` to `s.images.*`:

| Method | New delegate |
|--------|-------------|
| `ImageHistory` | `s.images.History` |
| `ImageInspect` | `s.images.Inspect` |
| `ImageList` | `s.images.List` |
| `ImagePrune` | `s.images.Prune` |
| `ImageRemove` | `s.images.Remove` |
| `ImageSave` | `s.images.Save` |
| `ImageSearch` | `s.images.Search` |

---

## 5. What Gets Deleted

| File | Lines | Reason |
|------|-------|--------|
| `backends/ecs/registry.go` | 196 | Entire file. Replaced by core's `FetchImageConfig()` + `ECRAuthProvider.GetToken()` |
| `backends/ecs/IMAGE_MANAGEMENT_PLAN.md` | 333 | Replaced by this plan |
| `backends/lambda/IMAGE_MANAGEMENT_PLAN.md` | 149 | Replaced by this plan |
| ECS `backend_impl.go` image methods | ~214 | `ImagePull`, `ImagePush`, `ImageTag`, `ImageRemove`, `ImageLoad`, `recordImageInECR`, `isECRRegistry`, `isECRAlreadyExistsError` |
| Lambda `backend_impl.go` image methods | ~117 | `ImagePull`, `ImageLoad`, `ImageBuild`, `ImagePush`, `getECRToken`, `isECRImage` |
| **Total deleted** | **~1009** | |

## 6. What Gets Created

| File | Est. Lines | Contents |
|------|-----------|----------|
| `backends/ecs/image_auth.go` | ~80 | `ECRAuthProvider` struct with all 5 `AuthProvider` methods + `isECRAlreadyExistsError` helper |
| `backends/lambda/image_auth.go` | ~40 | Lambda variant of `ECRAuthProvider` (GetToken + IsCloudRegistry; no-op OnPush/OnTag/OnRemove) |
| ECS `backend_impl.go` replacements | ~20 | 5 one-liner image method delegates to `s.images.*` |
| Lambda `backend_impl.go` replacements | ~16 | 4 methods (1 delegate + 3 NotImplementedError) |
| ECS `server.go` changes | ~8 | `images` field + initialization in `NewServer()` |
| Lambda `server.go` changes | ~8 | `images` field + initialization in `NewServer()` |
| **Total created** | **~172** | |

**Net reduction**: ~837 lines.

---

## 7. Migration Path (Step-by-Step)

Execute in this exact order. Each step must compile and pass tests before proceeding.

### Step 1: Core ImageManager (prerequisite)

This is a separate task (applies to all clouds). Create `backends/core/image_manager.go` with:
- `AuthProvider` interface (5 methods)
- `ImageManager` struct (Auth, Store, Logger fields)
- 12 image methods that refactor existing `BaseServer` logic

Verify: `BaseServer` still works (it creates `ImageManager{Auth: nil}` internally or continues using its own methods).

### Step 2: ECS ECRAuthProvider

1. Create `backends/ecs/image_auth.go` with `ECRAuthProvider` as defined in Section 1.
2. Verify it compiles: `cd backends/ecs && go build ./...`

### Step 3: Wire ECS ImageManager

1. Add `images *core.ImageManager` field to `Server` struct in `server.go`.
2. Initialize `s.images` in `NewServer()` after `s.BaseServer` is created.
3. Replace the 5 custom image methods in `backend_impl.go` with one-liner delegates.
4. Update 7 image delegates in `backend_delegates_gen.go` to call `s.images.*`.
5. Delete `backends/ecs/registry.go`.
6. Remove now-unused imports from `backend_impl.go` (`crypto/sha256`, `ecrtypes` if no other usage remains).
7. Verify: `cd backends/ecs && go build ./...` and `go vet ./...`.

### Step 4: Lambda ECRAuthProvider

1. Create `backends/lambda/image_auth.go` with Lambda variant of `ECRAuthProvider`.
2. Verify it compiles.

### Step 5: Wire Lambda ImageManager

1. Add `images *core.ImageManager` field to `Server` struct in `server.go`.
2. Initialize `s.images` in `NewServer()`.
3. Replace 6 custom image methods in `backend_impl.go` with 4 methods (1 delegate + 3 NotImplementedError).
4. Update 7 image delegates in `backend_delegates_gen.go` to call `s.images.*`.
5. Remove now-unused imports (`crypto/sha256`, `ecr` SDK if fully replaced).
6. Verify: `cd backends/lambda && go build ./...` and `go vet ./...`.

### Step 6: Delete old plan files

1. Delete `backends/ecs/IMAGE_MANAGEMENT_PLAN.md`.
2. Delete `backends/lambda/IMAGE_MANAGEMENT_PLAN.md`.

### Step 7: Run all tests

1. `make sim-test-all` -- all 75 backend tests must pass.
2. `make test-core` -- all 302 core tests must pass.
3. E2E tests (`tests/system_test.go`) -- `TestImageBuild` must return 200.
4. ECS integration tests -- image pull must still work.
5. Lambda integration tests -- image pull must still work.

---

## 8. Test Impact

### Tests that must not break

| Test Suite | Key Tests | Risk |
|-----------|-----------|------|
| `tests/system_test.go` | `TestImageBuild` (expects 200) | Low -- ECS delegates ImageBuild to `s.images.Build` which calls BaseServer's synthetic builder |
| ECS integration (`sim-test-ecs`) | Uses `ImagePull("alpine:latest")` as prerequisite | Low -- `ImageManager.Pull` calls `core.FetchImageConfig` (same as current ECS `fetchImageConfig` but better) |
| Lambda integration (`sim-test-lambda`) | Uses `ImagePull` for function images | Low -- `ImageManager.Pull` now gets ECR auth via `ECRAuthProvider.GetToken` (improvement over current code which only had it for ECR refs) |
| Core tests (302) | All image-related tests | None -- `BaseServer` methods unchanged or refactored into `ImageManager` with `Auth: nil` |
| sim-test-all (75) | All backends | Low -- only image method routing changes |

### Tests that might need updating

1. **ECS integration tests**: If any test directly creates `Server{}` (not via `NewServer()`), it will need `s.images` initialized. Check for test files that construct `Server` structs directly.

2. **Core ImageManager tests**: New tests should be added for `ImageManager` with a mock `AuthProvider` to verify:
   - `Pull` calls `GetToken` for cloud registries
   - `Push` calls `OnPush` for cloud registries
   - `Tag` calls `OnTag` for cloud registries
   - `Remove` calls `OnRemove` for cloud registries
   - All `On*` errors are logged but not returned

### What does NOT need tests

- `ECRAuthProvider` methods are thin SDK wrappers -- tested via integration tests against the ECR simulator.
- `isECRAlreadyExistsError` is a 1-line string check -- covered by existing integration tests.

---

## 9. Key Behavioral Differences from Current Code

### ECS ImagePull

**Current**: Uses `s.fetchImageConfig()` from `registry.go` (custom OCI client, no manifest list support, no caching). Falls back to synthetic config on failure. Then calls `s.recordImageInECR()` to persist ALL pulled images in ECR (including Docker Hub images).

**New**: Uses `core.FetchImageConfig()` (superior: handles manifest lists, Www-Authenticate token exchange, in-memory caching). Falls back to synthetic config on failure. Then calls `ECRAuthProvider.OnPush()` to persist ONLY ECR-sourced images in ECR.

**Behavioral changes**:
1. Better image config fetching (manifest list support, caching).
2. **ECR recording narrowed**: Only ECR-sourced images are recorded in ECR (via `IsCloudRegistry` check). Previously ALL images were recorded. This is the correct behavior — recording Docker Hub images in ECR was unnecessary.

### ECS ImagePush

**Current**: Resolves image, checks if target is ECR via `isECRRegistry()`, calls `CreateRepository` + `PutImage` for ECR targets, always returns synthetic progress stream.

**New**: `ImageManager.Push()` does the same: resolves image, calls `AuthProvider.OnPush()` for cloud registries, returns synthetic progress stream.

**Behavioral change**: None. `OnPush` errors are logged (non-fatal), same as current.

### Lambda ImagePull

**Current**: Uses `core.FetchImageConfig(ref, auth)`. For ECR images, calls `s.getECRToken()` to get auth. Does NOT record in ECR.

**New**: `ImageManager.Pull()` calls `ECRAuthProvider.GetToken()` for ECR images (same behavior), then `core.FetchImageConfig()`. `ECRAuthProvider.OnPush()` is a no-op for Lambda, so no ECR recording (same behavior).

**Behavioral change**: None.

### Lambda ImageBuild / ImagePush / ImageLoad

**Current**: Returns `NotImplementedError`.

**New**: Still returns `NotImplementedError` -- these are overridden on `Server`, not delegated to `ImageManager`.

**Behavioral change**: None.

---

## 10. Import Path Considerations

- `core.ImageManager` is in `github.com/sockerless/backend-core` (imported as `core`).
- `core.AuthProvider` interface is in the same package.
- `ECRAuthProvider` uses `github.com/aws/aws-sdk-go-v2/service/ecr` -- already in both ECS and Lambda `go.mod`.
- `ecrtypes` (`github.com/aws/aws-sdk-go-v2/service/ecr/types`) is only needed in ECS's `ECRAuthProvider` (for `ecrtypes.ImageIdentifier` in `OnRemove`). Lambda's no-op variant doesn't need it.
- No new Go module dependencies are added.

---

## 11. Dependency on Core ImageManager

This plan **cannot be executed** until `backends/core/image_manager.go` exists with the `AuthProvider` interface and `ImageManager` struct. That is a separate task defined in `backends/IMAGE_ARCHITECTURE.md` Section "Implementation Order", Step 1.

The core ImageManager task should:
1. Define `AuthProvider` interface (5 methods as shown in IMAGE_ARCHITECTURE.md)
2. Create `ImageManager` struct with `Auth AuthProvider`, `Store *Store`, `Logger zerolog.Logger`
3. Move the 12 image method implementations from `BaseServer` into `ImageManager`
4. Have `BaseServer` create its own `ImageManager{Auth: nil}` and delegate to it (or keep its methods and have `ImageManager` call `Store` methods directly)
5. Ensure `core.FetchImageConfig()` is used for registry fetching (it already is for Lambda; ECS's custom `fetchImageConfig` is inferior and gets deleted)

Once that exists, Steps 2-7 of this plan can be executed independently of the GCP and Azure plans.

---

## Review Notes

This section documents issues found during review and the changes made to fix them.

### Issue 1: Auth Token Double-Prefix Bug (CRITICAL)

**Problem**: `core.FetchImageConfig(ref, basicAuth ...string)` passes `basicAuth[0]` to `getRegistryToken()`, which sets `req.Header.Set("Authorization", "Basic "+basicAuth)`. The original plan had `GetToken()` returning `"Basic " + token`, which would result in `"Basic Basic <base64>"` — a malformed Authorization header that would fail all ECR registry requests.

**Fix**: Changed `GetToken()` in both ECS and Lambda `ECRAuthProvider` to return the raw base64 token without any scheme prefix. `ImageManager.Pull()` passes this raw token to `FetchImageConfig()`, which adds the `"Basic "` prefix internally. This aligns with the `FetchImageConfig` API contract where `basicAuth` is a raw credential, not a prefixed header value.

**Implication for AuthProvider interface**: The `GetToken()` contract must be clearly documented: return the raw credential value (base64 for Basic, JWT for Bearer), NOT the full `Authorization` header value. The core `ImageManager` implementation must handle scheme prefixing. This affects the GCP and Azure plans too (they use Bearer tokens).

### Issue 2: ECR Recording Behavioral Change (MEDIUM)

**Problem**: Current ECS `ImagePull` calls `recordImageInECR()` for ALL pulled images (including Docker Hub images like `alpine:latest`), not just ECR-sourced ones. The `ImageManager` architecture only calls `OnPush` when `IsCloudRegistry()` returns true, meaning non-ECR images will no longer be recorded in ECR after migration.

**Fix**: Documented this as an accepted behavioral change in Section 9 ("Key Behavioral Differences"). Recording Docker Hub images in ECR was unnecessary overhead and arguably incorrect behavior. No code change needed — the narrower behavior is preferable.

### Issue 3: AuthLogin Not Covered by ImageManager (MINOR)

**Problem**: The global architecture (`IMAGE_ARCHITECTURE.md`) claims "14 image methods" but there are only 12 image methods on `api.Backend`. `AuthLogin` is image-adjacent but not an image method. The plan correctly lists 12 methods but should explicitly note that `AuthLogin` is out of scope.

**Fix**: Added a note in Section 2 clarifying that `AuthLogin` is NOT part of `ImageManager` and remains handled by existing ECS/Lambda implementations (ECS delegates to BaseServer, Lambda has a custom override).

### Issue 4: Logger Type Mismatch (MINOR)

**Problem**: `IMAGE_ARCHITECTURE.md` specifies `Logger *slog.Logger` on `ImageManager`, but the entire project uses `zerolog.Logger` (as seen in `BaseServer`, ECS `Server`, Lambda `Server`). The plan code snippets correctly use `zerolog.Logger` but this inconsistency with the global architecture should be noted.

**Fix**: Added inline comments on the `Logger` field noting the discrepancy with `IMAGE_ARCHITECTURE.md`. The global architecture doc should be updated to use `zerolog.Logger` when the core `ImageManager` is implemented.

### Issue 5: ImageManager.Build() Needs Dockerfile Parser Access (MEDIUM)

**Problem**: The plan says ECS delegates `ImageBuild` to `s.images.Build`, but `ImageManager` as described (with only `Auth`, `Store`, `Logger` fields) has no access to `BaseServer`'s synthetic Dockerfile parser. The `TestImageBuild` e2e test sends a tar with a Dockerfile and expects the image to have the parsed `ENTRYPOINT` — this requires the tar extraction and Dockerfile parsing logic that lives in `BaseServer.ImageBuild()`.

**Fix**: Added a note in Section 3.4 that `ImageManager` must either hold a reference to `BaseServer` or the Dockerfile parser must be extracted into a standalone core function. This is a constraint on the core `ImageManager` prerequisite task.

### Issue 6: Line Count Verification

**Verified line counts against actual code**:
- ECS `registry.go`: 197 lines (plan says 196) — close enough, includes trailing newline
- ECS `backend_impl.go` image methods (lines 891-1122): ~232 lines (plan says ~214) — the plan's count is slightly low because it excludes blank lines and comments between methods
- Lambda `backend_impl.go` image methods (lines 813-1011): ~199 lines but some are `AuthLogin` (not deleted) — plan says ~117 for the 6 items to delete, which is reasonable (81+3+6+5+13+9 = 117)
- Net reduction estimate of ~837 lines is plausible

### Issue 7: Completeness Check

**All 12 image methods accounted for both backends**:

| Method | ECS Source | ECS Plan | Lambda Source | Lambda Plan |
|--------|-----------|----------|--------------|-------------|
| ImagePull | backend_impl.go | s.images.Pull | backend_impl.go | s.images.Pull |
| ImageInspect | delegates (BaseServer) | s.images.Inspect | delegates (BaseServer) | s.images.Inspect |
| ImageLoad | backend_impl.go | NotImplementedError | backend_impl.go | NotImplementedError |
| ImageTag | backend_impl.go | s.images.Tag | delegates (BaseServer) | s.images.Tag |
| ImageList | delegates (BaseServer) | s.images.List | delegates (BaseServer) | s.images.List |
| ImageRemove | backend_impl.go | s.images.Remove | delegates (BaseServer) | s.images.Remove |
| ImageHistory | delegates (BaseServer) | s.images.History | delegates (BaseServer) | s.images.History |
| ImagePrune | delegates (BaseServer) | s.images.Prune | delegates (BaseServer) | s.images.Prune |
| ImageBuild | delegates (BaseServer) | s.images.Build | backend_impl.go | NotImplementedError |
| ImagePush | backend_impl.go | s.images.Push | backend_impl.go | NotImplementedError |
| ImageSave | delegates (BaseServer) | s.images.Save | delegates (BaseServer) | s.images.Save |
| ImageSearch | delegates (BaseServer) | s.images.Search | delegates (BaseServer) | s.images.Search |
| AuthLogin | delegates (BaseServer) | unchanged | backend_impl.go (custom) | unchanged |

### Issue 8: Migration Path Feasibility

The step-by-step migration path is sound. Each step compiles independently. The critical dependency is Step 1 (core `ImageManager`), which is a separate task. Steps 2-7 can proceed in order once Step 1 is complete. The plan correctly identifies that ECS and Lambda can be migrated independently of GCP/Azure.

One risk not mentioned: if `ImageManager` is wired into `BaseServer` (Step 1), and `BaseServer.ImagePull` etc. start going through `ImageManager{Auth: nil}`, then the self-dispatch pattern (`s.self.ImagePull`) may need updating. Currently `BaseServer` methods are called directly via embedding. After migration, `s.self.ImagePull()` on ECS would call `ECS.ImagePull()` which calls `s.images.Pull()`. This should work correctly with the existing self-dispatch pattern since ECS `Server` overrides `ImagePull` on its own type.
