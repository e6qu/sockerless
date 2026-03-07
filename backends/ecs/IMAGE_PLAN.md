# AWS Unified Image Management Plan (ECS + Lambda) — IMPLEMENTED

This plan has been fully implemented. Both ECS and Lambda backends use `core.ImageManager` with `ECRAuthProvider` for unified image management.

**Status**: Complete. All 12 image methods delegate to `s.images` (ImageManager). ECR integration via `ECRAuthProvider` in `image_auth.go`. Old `registry.go` deleted. ~861 net lines removed.

**Key constraint**: No simulator changes. The plan works against the existing ECR simulator APIs (`GetAuthorizationToken`, `CreateRepository`, `PutImage`, `BatchDeleteImage`) as-is.

---

## 1. ECRAuthProvider Definition

**File**: `backends/ecs/image_auth.go` (new, ~80 lines)

```go
package ecs

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// ECRAuthProvider implements core.AuthProvider for AWS ECR.
type ECRAuthProvider struct {
	ECR    *ecr.Client
	Logger zerolog.Logger
	Ctx    func() context.Context // returns context.Background(); avoids storing context
}

// GetToken returns a raw base64-encoded auth token from ECR GetAuthorizationToken.
// The token is base64("user:password") -- NO "Basic " prefix. ImageManager passes
// this to FetchImageConfig as basicAuth, which internally does "Basic " + basicAuth
// in getRegistryToken.
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
	// Token is base64-encoded "user:password" -- return raw, no scheme prefix
	token := aws.ToString(result.AuthorizationData[0].AuthorizationToken)
	return token, nil
}

// IsCloudRegistry returns true if the registry matches *.dkr.ecr.*.amazonaws.com.
func (p *ECRAuthProvider) IsCloudRegistry(registry string) bool {
	return strings.HasSuffix(registry, ".amazonaws.com") && strings.Contains(registry, ".dkr.ecr.")
}

// OnPush is called after a successful in-memory push. Creates the ECR repository
// (ignoring AlreadyExists) and calls PutImage with a synthetic manifest.
// Uses ECR SDK, NOT OCI Distribution API -- no simulator changes needed.
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
// Uses ECR SDK BatchDeleteImage -- already supported by the ECR simulator.
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

| Method | ECR SDK Call | Notes |
|--------|-------------|-------|
| `GetToken` | `ecr.GetAuthorizationToken` | Returns raw base64 token. Simulator supports this. |
| `IsCloudRegistry` | None (string match) | N/A |
| `OnPush` | `ecr.CreateRepository` + `ecr.PutImage` | Both supported by simulator. Non-fatal. |
| `OnTag` | Same as OnPush | Same as OnPush. |
| `OnRemove` | `ecr.BatchDeleteImage` | Supported by simulator (`handleECRBatchDeleteImage`). Non-fatal. |

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

The `ImageManager` calls `AuthProvider.GetToken()` in `Pull` to get ECR auth, calls `AuthProvider.OnPush()` after `Push`/`Tag`, and calls `AuthProvider.OnRemove()` after `Remove`. Methods 2, 5, 7, 8, 9, 11, 12 are pure in-memory operations that do not touch `AuthProvider` at all.

### Auth Threading: FetchImageConfig + ECR Token

**How auth flows through the system today**:

1. Core's `FetchImageConfig(ref string, basicAuth ...string)` calls `getRegistryToken(rc, basicAuth[0])`.
2. `getRegistryToken` first tries an anonymous `GET /v2/.../manifests/tag`. If 401, it parses `Www-Authenticate`, builds a token URL, and sends `Authorization: Basic <basicAuth>` to the token endpoint.
3. The token endpoint returns a Bearer token, which is used for all subsequent registry requests.

**How ECR auth currently works in ECS**:
- `getECRToken()` calls `ecr.GetAuthorizationToken`, which returns a base64-encoded `user:password`.
- Returns `"Basic " + token` as a prefixed auth header.
- `fetchImageConfig()` sets `req.Header.Set("Authorization", token)` for manifest/blob requests.
- This bypasses the `Www-Authenticate` flow entirely -- auth is pre-baked.

**How ECR auth currently works in Lambda**:
- `getECRToken()` calls `ecr.GetAuthorizationToken`, returns `"Basic " + token`.
- Passes the full `"Basic ..." + token` string to `core.FetchImageConfig(ref, auth)`.
- `FetchImageConfig` passes `auth` to `getRegistryToken(rc, auth)`, which does
  `req.Header.Set("Authorization", "Basic " + auth)` -- resulting in `"Basic Basic ..."` (double-prefixed!).
- In practice this doesn't matter because Lambda only uses ECR against the simulator (which
  doesn't expose OCI v2 endpoints), so `FetchImageConfig` fails gracefully and synthetic config
  is used.

**Resolution for ImageManager**: `ECRAuthProvider.GetToken()` returns the **raw base64 token**
without any scheme prefix. `ImageManager.Pull()` passes this raw token to `FetchImageConfig(ref, rawToken)`.
Inside `getRegistryToken`, the token exchange request sends `"Basic " + rawToken` -- which is correct
(base64-encoded `user:password` with `Basic` scheme). Against real ECR (production), this works.
Against the simulator, `FetchImageConfig` fails because the ECR simulator has no OCI v2 endpoints,
and `ImageManager.Pull()` falls back to synthetic config (same behavior as today).

### ECR Simulator Limitation

The ECR simulator only exposes JSON-RPC SDK endpoints (e.g., `GetAuthorizationToken`, `PutImage`). It does NOT expose OCI Distribution API v2 endpoints (`/v2/.../manifests/...`). This means:

- `core.FetchImageConfig()` will always fail against simulated ECR images (no OCI v2 route to hit)
- This is the same behavior as today -- both ECS and Lambda fall back to synthetic config for simulator ECR images
- In production, real ECR exposes OCI v2 and `FetchImageConfig` works with the ECR auth token
- **No simulator changes needed** -- the graceful fallback is the correct behavior for testing

### ECS recordImageInECR: Behavioral Change

**Current**: ECS `ImagePull` calls `recordImageInECR()` for ALL pulled images (line 1086: `_ = registry // We record all pulled images for local persistence`), not just ECR-sourced ones.

**New**: `ImageManager` only calls `AuthProvider.OnPush()` when `IsCloudRegistry()` returns true for the image's registry. Pulling `alpine:latest` will NOT be recorded in ECR.

**Decision**: Accept this behavioral change. Recording Docker Hub images in ECR was unnecessary overhead and arguably incorrect. Non-ECR images do not need ECR persistence.

---

## 3. ECS Backend Wiring

### 3.1 Add `images` field to Server

**File**: `backends/ecs/server.go`

Add an `images *core.ImageManager` field to the `Server` struct:

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
	Auth: &ECRAuthProvider{
		ECR:    awsClients.ECR,
		Logger: logger,
		Ctx:    s.ctx,
	},
	Store:  s.Store,
	Logger: logger,
}
```

Note: `BuildFunc` is not set here because ECS delegates `ImageBuild` to `s.images.Build`,
which should use BaseServer's synthetic Dockerfile parser. The `ImageManager` prerequisite
task must ensure that `ImageManager.Build()` can work without a `BuildFunc` by either:
- Having `ImageManager` hold a reference to `BaseServer` for build logic, OR
- Extracting the Dockerfile parser into a standalone `core.SyntheticBuild()` function, OR
- Having `BaseServer` set `BuildFunc` when it creates its own `ImageManager` in `InitDrivers()`,
  and cloud backends inherit it through the shared `Store`.

The simplest approach: `BaseServer.InitDrivers()` creates an `ImageManager` with `BuildFunc`
pointing to the build logic. Cloud backends create their own `ImageManager` but set `BuildFunc`
to the same standalone function (e.g., `core.SyntheticBuild`).

### 3.3 Replace 5 custom image methods with one-liner delegates

All custom image methods in `backend_impl.go` (lines ~891-1122) are deleted and replaced:

**Delete from `backend_impl.go`**:
- `ImagePull` (lines 891-958, ~67 lines) -- uses `s.fetchImageConfig` + `recordImageInECR`
- `ImagePush` (lines 960-1010, ~51 lines) -- uses `parseImageRef` + `isECRRegistry` + ECR SDK
- `ImageTag` (lines 1012-1035, ~24 lines) -- delegates to BaseServer + `recordImageInECR`
- `ImageRemove` (lines 1037-1078, ~42 lines) -- collects ECR refs + BaseServer + `BatchDeleteImage`
- `recordImageInECR` (lines 1080-1107, ~28 lines) -- helper for ECR recording
- `isECRRegistry` (lines 1109-1112, ~4 lines) -- helper
- `isECRAlreadyExistsError` (lines 1114-1117, ~4 lines) -- helper (moved to `image_auth.go`)
- `ImageLoad` (lines 1119-1122, ~4 lines) -- NotImplementedError

Total: ~224 lines deleted from `backend_impl.go`.

**Replace with** (in `backend_impl.go`):

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

The following 7 methods currently delegate to `s.BaseServer` and should be changed to delegate to `s.images`:

| Method | Current delegate target | New delegate target |
|--------|------------------------|---------------------|
| `ImageBuild` | `s.BaseServer.ImageBuild` | `s.images.Build` |
| `ImageHistory` | `s.BaseServer.ImageHistory` | `s.images.History` |
| `ImageInspect` | `s.BaseServer.ImageInspect` | `s.images.Inspect` |
| `ImageList` | `s.BaseServer.ImageList` | `s.images.List` |
| `ImagePrune` | `s.BaseServer.ImagePrune` | `s.images.Prune` |
| `ImageSave` | `s.BaseServer.ImageSave` | `s.images.Save` |
| `ImageSearch` | `s.BaseServer.ImageSearch` | `s.images.Search` |

Since `ImageManager` methods ultimately call the same `Store` methods that `BaseServer` does, this is functionally equivalent. The benefit is that all 12 image methods go through `ImageManager`, making the flow uniform and enabling future enhancements (e.g., caching, metrics) in one place.

### 3.5 Delete registry.go

**File**: `backends/ecs/registry.go` (197 lines) -- DELETE ENTIRELY.

Functions deleted:
- `fetchImageConfig()` (lines 17-128, 112 lines) -- replaced by `core.FetchImageConfig()` called inside `ImageManager.Pull()`
- `parseImageRef()` (lines 131-161, 31 lines) -- replaced by `core.parseImageRef()` (already exists, unexported; ImageManager uses it internally)
- `getECRToken()` (lines 164-176, 13 lines) -- replaced by `ECRAuthProvider.GetToken()`
- `getDockerHubToken()` (lines 179-195, 17 lines) -- handled inside core's `getRegistryToken()` via `Www-Authenticate` flow

### 3.6 Remove unused imports from backend_impl.go

After deleting the image methods, the following imports in `backend_impl.go` may become unused:
- `"crypto/sha256"` -- only used by `ImagePull` for `sha256.Sum256([]byte(ref))`
- `ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"` -- only used by `ImageRemove` for `ecrtypes.ImageIdentifier`
- `"github.com/aws/aws-sdk-go-v2/service/ecr"` -- used by `ImagePush` for `ecr.CreateRepositoryInput` etc.

Check whether other (non-image) methods still use these imports before removing.

---

## 4. Lambda Backend Wiring

### 4.1 Lambda reuses ECRAuthProvider pattern

The `ECRAuthProvider` type is defined in `package ecs`. Lambda cannot import it directly because `backends/lambda/` and `backends/ecs/` are separate Go modules.

**Create `backends/lambda/image_auth.go`** (~30 lines) with a minimal variant:

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
// no-ops because Lambda doesn't record images in ECR -- it uses ECR images
// by reference (ImageUri in CreateFunction).
type ECRAuthProvider struct {
	ECR    *ecr.Client
	Logger zerolog.Logger
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
	// Return raw base64 token, no scheme prefix
	token := aws.ToString(result.AuthorizationData[0].AuthorizationToken)
	return token, nil
}

func (p *ECRAuthProvider) IsCloudRegistry(registry string) bool {
	return strings.HasSuffix(registry, ".amazonaws.com") && strings.Contains(registry, ".dkr.ecr.")
}

// OnPush/OnTag/OnRemove are no-ops for Lambda -- it doesn't record images in ECR.
func (p *ECRAuthProvider) OnPush(img api.Image, registry, repo, tag string) error  { return nil }
func (p *ECRAuthProvider) OnTag(img api.Image, registry, repo, newTag string) error { return nil }
func (p *ECRAuthProvider) OnRemove(registry, repo string, tags []string) error      { return nil }
```

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
	Auth: &ECRAuthProvider{
		ECR:    awsClients.ECR,
		Logger: logger,
		Ctx:    s.ctx,
	},
	Store:  s.Store,
	Logger: logger,
}
```

### 4.3 Replace Lambda image methods

**Delete from `backend_impl.go`**:
- `ImagePull` (lines 813-895, 83 lines) -- uses `core.FetchImageConfig` + `isECRImage` + `getECRToken`
- `ImageLoad` (lines 897-900, 4 lines) -- `NotImplementedError`
- `ImageBuild` (lines 902-908, 7 lines) -- `NotImplementedError`
- `ImagePush` (lines 979-985, 7 lines) -- `NotImplementedError`
- `getECRToken` (lines 987-1000, 14 lines) -- moved to `ECRAuthProvider.GetToken()`
- `isECRImage` (lines 1002-1011, 10 lines) -- moved to `ECRAuthProvider.IsCloudRegistry()`

Total: ~125 lines deleted from Lambda `backend_impl.go`.

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

Also: `ImageTag` currently delegates to `s.BaseServer.ImageTag` -- change to `s.images.Tag`.

### 4.5 Remove unused imports from Lambda backend_impl.go

After deleting the image methods:
- `"crypto/sha256"` -- only used by `ImagePull`
- `"github.com/aws/aws-sdk-go-v2/service/ecr"` -- only used by `getECRToken`

Check whether `AuthLogin` (which references `".amazonaws.com"` and `".dkr.ecr."`) still needs
any of these imports. `AuthLogin` uses only `strings` and `s.BaseServer.AuthLogin`, so the ECR
import can be removed.

---

## 5. What Gets Deleted

| File | Lines | Reason |
|------|-------|--------|
| `backends/ecs/registry.go` | 197 | Entire file. Replaced by core's `FetchImageConfig()` + `ECRAuthProvider.GetToken()` |
| `backends/ecs/IMAGE_MANAGEMENT_PLAN.md` | ~333 | Replaced by this plan |
| `backends/lambda/IMAGE_MANAGEMENT_PLAN.md` | ~149 | Replaced by this plan |
| ECS `backend_impl.go` image methods | ~224 | `ImagePull`, `ImagePush`, `ImageTag`, `ImageRemove`, `ImageLoad`, `recordImageInECR`, `isECRRegistry`, `isECRAlreadyExistsError` |
| Lambda `backend_impl.go` image methods | ~125 | `ImagePull`, `ImageLoad`, `ImageBuild`, `ImagePush`, `getECRToken`, `isECRImage` |
| **Total deleted** | **~1028** | |

## 6. What Gets Created

| File | Est. Lines | Contents |
|------|-----------|----------|
| `backends/ecs/image_auth.go` | ~80 | `ECRAuthProvider` struct with all 5 `AuthProvider` methods + `isECRAlreadyExistsError` helper |
| `backends/lambda/image_auth.go` | ~35 | Lambda variant of `ECRAuthProvider` (GetToken + IsCloudRegistry; no-op OnPush/OnTag/OnRemove) |
| ECS `backend_impl.go` replacements | ~20 | 5 one-liner image method delegates to `s.images.*` |
| Lambda `backend_impl.go` replacements | ~16 | 4 methods (1 delegate + 3 NotImplementedError) |
| ECS `server.go` changes | ~8 | `images` field + initialization in `NewServer()` |
| Lambda `server.go` changes | ~8 | `images` field + initialization in `NewServer()` |
| **Total created** | **~167** | |

**Net reduction**: ~861 lines.

---

## 7. Migration Path (Step-by-Step) — ALL COMPLETE

All steps executed and verified.

### Step 1: Core ImageManager (prerequisite) — DONE

This is a separate task (applies to all clouds). Create `backends/core/image_manager.go` with:
1. `AuthProvider` interface (5 methods: `GetToken`, `IsCloudRegistry`, `OnPush`, `OnTag`, `OnRemove`)
2. `ImageManager` struct (`Auth AuthProvider`, `Store *Store`, `Logger zerolog.Logger`, `BuildFunc`)
3. 12 image methods that refactor existing `BaseServer` logic
4. `BaseServer` creates its own `ImageManager{Auth: nil}` and delegates to it

Key implementation detail: `ImageManager.Pull()` must:
- Parse registry from ref using `parseImageRef()`
- If `Auth != nil`, call `Auth.GetToken(registry)` to get cloud auth token
- Call `FetchImageConfig(ref, rawToken)` -- token is raw (no scheme prefix), `getRegistryToken` adds `"Basic "` if needed
- If `FetchImageConfig` returns nil (graceful failure), use synthetic config
- Store image in `Store`
- If `Auth != nil && Auth.IsCloudRegistry(registry)`, call `Auth.OnPush(img, registry, repo, tag)` (non-fatal)

### Step 2: ECS ECRAuthProvider — DONE
Created `backends/ecs/image_auth.go` with `ECRAuthProvider` (GetToken, IsCloudRegistry, OnPush, OnTag, OnRemove).

### Step 3: Wire ECS ImageManager — DONE
Added `images *core.ImageManager` to Server, replaced custom image methods with delegates, updated `backend_delegates_gen.go`, deleted `registry.go`.

### Step 4: Lambda ECRAuthProvider — DONE
Created `backends/lambda/image_auth.go` with Lambda variant (OnPush/OnTag/OnRemove sync to ECR).

### Step 5: Wire Lambda ImageManager — DONE
Added `images *core.ImageManager` to Server, replaced custom image methods, updated `backend_delegates_gen.go`.

### Step 6: Delete old plan files — DONE

### Step 7: Run all tests — DONE

---

## 8. Test Impact

### Tests that must not break

| Test Suite | Key Tests | Risk |
|-----------|-----------|------|
| `tests/system_test.go` | `TestImageBuild` (expects 200) | Low -- ECS delegates to `s.images.Build` which uses BaseServer's synthetic builder |
| ECS integration | Uses `ImagePull("alpine:latest")` | Low -- `ImageManager.Pull` calls `core.FetchImageConfig` (same as current, but better: manifest list, caching) |
| Lambda integration | Uses `ImagePull` for function images | Low -- ECR auth via `ECRAuthProvider.GetToken` (same flow) |
| Core tests (302) | All image-related tests | None -- `BaseServer` methods refactored into `ImageManager` with `Auth: nil` |
| sim-test-all (75) | All backends | Low -- only image method routing changes |

### Tests that might need updating

1. **ECS integration tests**: If any test directly creates `Server{}` (not via `NewServer()`), it will need `s.images` initialized. Check for test files that construct `Server` structs directly.

2. **Core ImageManager tests**: New tests should be added for `ImageManager` with a mock `AuthProvider` to verify:
   - `Pull` calls `GetToken` for cloud registries
   - `Push` calls `OnPush` for cloud registries
   - `Tag` calls `OnTag` for cloud registries
   - `Remove` calls `OnRemove` for cloud registries
   - All `On*` errors are logged but not returned

---

## 9. Key Behavioral Differences from Current Code

### ECS ImagePull

**Current**: Uses `s.fetchImageConfig()` from `registry.go` (custom OCI client, no manifest list support, no caching). Falls back to synthetic config on failure. Then calls `s.recordImageInECR()` to persist ALL pulled images in ECR (including Docker Hub images).

**New**: Uses `core.FetchImageConfig()` (superior: handles manifest lists, `Www-Authenticate` token exchange, in-memory caching). Falls back to synthetic config on failure. Then calls `ECRAuthProvider.OnPush()` to persist ONLY ECR-sourced images in ECR (via `IsCloudRegistry` check).

**Behavioral changes**:
1. Better image config fetching (manifest list support, caching).
2. **ECR recording narrowed**: Only ECR-sourced images are recorded in ECR. Previously ALL images were recorded. This is correct behavior -- recording Docker Hub images in ECR was unnecessary overhead.

### ECS ImagePush

**Current**: Resolves image, checks if target is ECR via `isECRRegistry()`, calls `CreateRepository` + `PutImage` for ECR targets, always returns synthetic progress stream.

**New**: `ImageManager.Push()` does the same: resolves image, calls `AuthProvider.OnPush()` for cloud registries, returns synthetic progress stream.

**Behavioral change**: None. `OnPush` errors are logged (non-fatal), same as current.

### ECS ImageRemove

**Current**: Resolves image, collects ECR-tagged refs, delegates to `BaseServer.ImageRemove()`, then calls `BatchDeleteImage` for each ECR ref (best-effort).

**New**: `ImageManager.Remove()` resolves image, collects ECR-tagged refs, removes from in-memory store, then calls `AuthProvider.OnRemove(registry, repo, tags)` for each ECR registry (best-effort). `OnRemove` uses `BatchDeleteImage` (same SDK call, already supported by simulator).

**Behavioral change**: None.

### Lambda ImagePull

**Current**: Uses `core.FetchImageConfig(ref, auth)`. For ECR images, calls `s.getECRToken()` to get `"Basic " + token`. Does NOT record in ECR.

**New**: `ImageManager.Pull()` calls `ECRAuthProvider.GetToken()` for ECR images (returns raw token), passes to `core.FetchImageConfig()`. `ECRAuthProvider.OnPush()` is a no-op for Lambda, so no ECR recording (same behavior).

**Behavioral change**: Auth token is now passed without double `"Basic "` prefix. In practice this doesn't matter because the ECR simulator doesn't have OCI v2 endpoints, so `FetchImageConfig` fails gracefully regardless. In production, this is actually a bug fix.

### Lambda ImageBuild / ImagePush / ImageLoad

**Current**: Returns `NotImplementedError`.
**New**: Same -- these methods are overridden on `Server`, not delegated to `ImageManager`.
**Behavioral change**: None.

---

## 10. Import Path Considerations

- `core.ImageManager` is in `github.com/sockerless/backend-core` (imported as `core`).
- `core.AuthProvider` interface is in the same package.
- `ECRAuthProvider` uses `github.com/aws/aws-sdk-go-v2/service/ecr` -- already in both ECS and Lambda `go.mod`.
- `ecrtypes` (`github.com/aws/aws-sdk-go-v2/service/ecr/types`) is only needed in ECS's `ECRAuthProvider` (for `ecrtypes.ImageIdentifier` in `OnRemove`). Lambda's no-op variant doesn't need it.
- No new Go module dependencies are added.

---

## 11. Dependency on Core ImageManager — SATISFIED

Core `ImageManager` exists in `backends/core/image_manager.go` with the `AuthProvider` interface. Both ECS and Lambda backends are fully wired.

---

## 12. Method Coverage Matrix

All 12 image methods + AuthLogin accounted for both backends:

| Method | ECS Source | ECS After | Lambda Source | Lambda After |
|--------|-----------|-----------|--------------|--------------|
| ImagePull | backend_impl.go (custom) | `s.images.Pull` | backend_impl.go (custom) | `s.images.Pull` |
| ImageInspect | delegates (BaseServer) | `s.images.Inspect` | delegates (BaseServer) | `s.images.Inspect` |
| ImageLoad | backend_impl.go (NotImpl) | NotImplementedError | backend_impl.go (NotImpl) | NotImplementedError |
| ImageTag | backend_impl.go (custom) | `s.images.Tag` | delegates (BaseServer) | `s.images.Tag` |
| ImageList | delegates (BaseServer) | `s.images.List` | delegates (BaseServer) | `s.images.List` |
| ImageRemove | backend_impl.go (custom) | `s.images.Remove` | delegates (BaseServer) | `s.images.Remove` |
| ImageHistory | delegates (BaseServer) | `s.images.History` | delegates (BaseServer) | `s.images.History` |
| ImagePrune | delegates (BaseServer) | `s.images.Prune` | delegates (BaseServer) | `s.images.Prune` |
| ImageBuild | delegates (BaseServer) | `s.images.Build` | backend_impl.go (NotImpl) | NotImplementedError |
| ImagePush | backend_impl.go (custom) | `s.images.Push` | backend_impl.go (NotImpl) | NotImplementedError |
| ImageSave | delegates (BaseServer) | `s.images.Save` | delegates (BaseServer) | `s.images.Save` |
| ImageSearch | delegates (BaseServer) | `s.images.Search` | delegates (BaseServer) | `s.images.Search` |
| AuthLogin | delegates (BaseServer) | unchanged | backend_impl.go (custom) | unchanged |
