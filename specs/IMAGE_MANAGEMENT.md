# Image Management Specification

Image management is handled by `core.ImageManager` which wraps `BaseServer` image operations with cloud-specific registry authentication and synchronization.

## Architecture

```
Docker CLI → HTTP Handler → s.self.ImagePull/Push/Tag/Remove
                                ↓
                          ImageManager (cloud backends)
                            ├── AuthProvider.GetToken()     → cloud auth
                            ├── FetchImageMetadata()        → registry metadata
                            ├── BaseServer.ImagePullWithMetadata()  → store image
                            └── AuthProvider.OnPush/OnTag/OnRemove  → sync to cloud
```

Docker backend bypasses ImageManager entirely — delegates to local Docker daemon.

## ImageManager

Source: `backends/core/image_manager.go`

```go
type ImageManager struct {
    Base   *BaseServer       // in-memory store + base implementations
    Auth   AuthProvider      // cloud auth + registry sync (nil = no cloud)
    Logger zerolog.Logger
}
```

**Pull flow:**
1. Check if image reference matches a cloud registry (`Auth.IsCloudRegistry`)
2. If yes, get cloud auth token (`Auth.GetToken`)
3. Fetch real image metadata from registry (`FetchImageMetadata`) — config, layers, sizes
4. Store image with real metadata in `BaseServer.Store`
5. Return progress stream to client

**Push flow:**
1. Resolve image in local store
2. If cloud registry, call `Auth.OnPush(imageID, registry, repo, tag)`
3. Return progress stream (real or error)

**Tag flow:**
1. Delegate to `BaseServer.ImageTag`
2. If cloud registry, call `Auth.OnTag(imageID, registry, repo, newTag)`

**Remove flow:**
1. Collect cloud references before removal
2. Delegate to `BaseServer.ImageRemove`
3. Call `Auth.OnRemove(registry, repo, tags)` for each cloud reference

## AuthProvider Interface

```go
type AuthProvider interface {
    GetToken(registry string) (string, error)
    IsCloudRegistry(registry string) bool
    OnPush(imageID, registry, repo, tag string) error
    OnTag(imageID, registry, repo, newTag string) error
    OnRemove(registry, repo string, tags []string) error
}
```

All `On*` methods are **non-fatal** — they log warnings on failure and return errors that callers may ignore. This prevents cloud sync issues from breaking local operations.

## Per-Cloud Implementations

### AWS ECR (`backends/aws-common/ecr_auth.go`)

```go
type ECRAuthProvider struct {
    ecr    *ecr.Client
    logger zerolog.Logger
    ctx    func() context.Context
}
```

| Method | Implementation |
|--------|---------------|
| `GetToken` | `ecr.GetAuthorizationToken()` → `"Basic {base64}"` |
| `IsCloudRegistry` | `*.dkr.ecr.*.amazonaws.com` pattern match |
| `OnPush` | `ecr.CreateRepository()` + `ecr.PutImage()` with OCI manifest |
| `OnTag` | `ecr.CreateRepository()` + `ecr.PutImage()` with new tag |
| `OnRemove` | `ecr.BatchDeleteImage()` for all tags |

**Used by:** ECS, Lambda

### GCP Artifact Registry (`backends/gcp-common/ar_auth.go`)

```go
type ARAuthProvider struct {
    ctx    func() context.Context
    logger zerolog.Logger
}
```

| Method | Implementation |
|--------|---------------|
| `GetToken` | `google.FindDefaultCredentials()` → `"Bearer {token}"` |
| `IsCloudRegistry` | `*.gcr.io`, `*-docker.pkg.dev` |
| `OnPush` | `core.OCIPush()` — OCI registry v2 API |
| `OnTag` | `core.OCIPush()` with new tag |
| `OnRemove` | `DELETE /v2/{repo}/manifests/{tag}` (graceful 404/405) |

**Used by:** Cloud Run, Cloud Run Functions

### Azure Container Registry (`backends/azure-common/acr_auth.go`)

```go
type ACRAuthProvider struct {
    Logger zerolog.Logger
}
```

| Method | Implementation |
|--------|---------------|
| `GetToken` | `azidentity.NewDefaultAzureCredential()` → `"Bearer {token}"` |
| `IsCloudRegistry` | `*.azurecr.io` suffix |
| `OnPush` | `core.OCIPush()` — OCI registry v2 API |
| `OnTag` | GET source manifest → PUT with new tag |
| `OnRemove` | HEAD for digest → `DELETE /v2/{repo}/manifests/{digest}` |

**Used by:** ACA, Azure Functions

## OCI Push

Source: `backends/core/oci_push.go`

Implements the OCI Distribution Spec v2 push protocol:

1. **Initiate upload** — `POST /v2/{repo}/blobs/uploads/`
2. **Upload blob** — `PUT /v2/{repo}/blobs/uploads/{uuid}?digest={digest}`
3. **Put manifest** — `PUT /v2/{repo}/manifests/{tag}`

Used by GCP and Azure auth providers. ECR uses the ECR SDK directly (PutImage) instead of the OCI protocol.

**Resolved by BUG-788 (round 8).** Earlier sockerless versions had several
push-path issues — ECR-direct push that called PutImage without uploading
blobs (BUG-638), `ImagePush` returning a synthetic "Pushed" stream
regardless of OCI outcome (BUG-640), and OCI push falling back to an
empty gzip layer when no real layer data was available (BUG-646). All
three are closed by BUG-788's real registry-to-registry layer mirror:
`core.FetchLayerBlob` downloads each layer's compressed bytes during
`ImagePull`, caches them in `Store.LayerContent[compressedDigest]` keyed
by the source manifest digest, and `OCIPush` requires `ManifestLayers`
and uses each entry's compressed digest verbatim. No recompute, no
empty-layer fallback, no synthetic success.

## Registry Metadata

Source: `backends/core/registry.go`

`FetchImageMetadata(ref)` fetches real image configuration from Docker v2 registries:

1. Resolve registry, authenticate (anonymous or cloud token)
2. Fetch manifest (`GET /v2/{repo}/manifests/{tag}`)
3. Fetch config blob (`GET /v2/{repo}/blobs/{config_digest}`)
4. Parse OCI image config → Cmd, Entrypoint, Env, ExposedPorts, WorkingDir, Labels
5. Extract layer digests, sizes, history
6. Cache result in-memory

When fetch fails (registry unreachable, auth error), `FetchImageMetadata`
returns the real underlying error — there is no synthetic-metadata
fallback. Earlier sockerless versions (BUG-648) returned a fabricated
metadata stub on error, which masked auth bugs as fake-success;
Phase 90's no-fakes audit (BUG-731-734) and BUG-788's metadata-must-be-real
path closed that.

## Backend Initialization

All 6 cloud backends initialize ImageManager identically:

```go
s.images = &core.ImageManager{
    Base:   s.BaseServer,
    Auth:   cloudcommon.NewAuthProvider(clients, logger, s.ctx),
    Logger: logger,
}
```

Then delegate image methods:

```go
func (s *Server) ImagePull(ref, auth string) (io.ReadCloser, error) {
    return s.images.Pull(ref, auth)
}
func (s *Server) ImagePush(name, tag, auth string) (io.ReadCloser, error) {
    return s.images.Push(name, tag, auth)
}
// ... etc for Tag, Remove, Load, Build
```
