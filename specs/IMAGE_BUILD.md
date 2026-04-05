# Image Build Specification

Complete specification for `docker build`, `docker buildx build`, and `podman build` mapped to cloud-native build services.

## Docker Engine API: POST /build

The build context is sent as a `tar` request body. All configuration is via query parameters:

### Query Parameters

| Parameter | Type | Docker | Podman | Sockerless | Cloud Mapping |
|-----------|------|--------|--------|------------|---------------|
| `dockerfile` | string | yes | yes | yes | Dockerfile path in context |
| `t` | string[] | yes | yes | yes | Tag(s) for built image |
| `buildargs` | JSON | yes | yes | yes | `--build-arg` values → cloud env vars |
| `labels` | JSON | yes | yes | yes | `--label` values |
| `nocache` | bool | yes | yes | yes | `noCache` on cloud service |
| `rm` | bool | yes | yes | yes | Always true for cloud (ephemeral env) |
| `forcerm` | bool | yes | yes | yes | Always true for cloud |
| `target` | string | yes | yes | yes | Multi-stage target → `--target` |
| `platform` | string | yes | yes | yes | Build platform → cloud env arch |
| `pull` | bool | yes | yes | yes | Pull base image before build |
| `q` | bool | yes | yes | yes | Suppress build output |
| `networkmode` | string | yes | yes | yes | Build container network |
| `shmsize` | int | yes | yes | yes | /dev/shm size |
| `extrahosts` | string | yes | yes | yes | Extra /etc/hosts entries |
| `cachefrom` | JSON | yes | yes | **missing** | Cache source images → cloud cache |
| `cacheto` | string | no | yes | **missing** | Cache export destination |
| `remote` | string | yes | yes | **missing** | Remote context URL (git/tar) |
| `memory` | int | yes | yes | **missing** | Build memory limit |
| `memswap` | int | yes | yes | **missing** | Build memory+swap limit |
| `cpushares` | int | yes | yes | **missing** | CPU shares |
| `cpuperiod` | int | yes | yes | **missing** | CPU CFS period |
| `cpuquota` | int | yes | yes | **missing** | CPU CFS quota |
| `cpusetcpus` | string | yes | yes | **missing** | CPU affinity |
| `outputs` | JSON | yes | yes | **missing** | BuildKit output config |
| `version` | string | yes | no | **missing** | "1" (legacy) or "2" (BuildKit) |
| `squash` | bool | yes | no | **missing** | Squash layers (experimental) |
| `cgroupparent` | string | yes | no | **missing** | Cgroup parent |

### Request Headers

| Header | Purpose |
|--------|---------|
| `Content-Type` | `application/x-tar` (required) |
| `X-Registry-Config` | Base64 JSON of registry auth configs (for pulling private base images) |

### Response

NDJSON stream: `{"stream":"Step 1/5 : FROM alpine\n"}` and `{"error":"..."}` lines.

## Podman Libpod Build API

`POST /libpod/build` — Buildah-native parameters, superset of Docker.

Key additional parameters:
- `outputformat` — `oci` or `docker` manifest format
- `layers` — cache individual layers (Buildah behavior)
- `manifest` — add result to manifest list
- `jobs` — parallel build jobs
- `isolation` — `oci`, `chroot`, or `rootless`
- `retry`, `retrydelay` — retry failed RUN steps
- `omithistory` — exclude build history
- `sbom*` — SBOM generation parameters
- `annotations` — OCI annotations
- `volume` — bind mounts during build

Podman uses **Buildah** (not BuildKit) for builds. Buildah executes directly via OCI runtime without a daemon.

**Sockerless mapping:** Route `/libpod/build` to the same cloud build service but accept Buildah-specific parameters where they map to cloud features.

## BuildKit Features (docker buildx)

These are CLI features that use BuildKit's gRPC session protocol. They do NOT map to `POST /build` query parameters — they require a session between client and BuildKit daemon.

### Secrets

```
docker buildx build --secret id=mysecret,src=secret.txt .
```

In Dockerfile: `RUN --mount=type=secret,id=mysecret cat /run/secrets/mysecret`

BuildKit mechanism: client holds secrets in memory, daemon calls back via gRPC `GetSecret(id)`. Secrets are tmpfs-mounted, never in layers.

**Cloud mapping:**

| Cloud | Secret Injection |
|-------|-----------------|
| AWS CodeBuild | `environmentVariablesOverride` with `type: SECRETS_MANAGER` or `PARAMETER_STORE` |
| GCP Cloud Build | `secretEnv` field referencing Secret Manager |
| Azure ACR Tasks | `secretArguments` field or Key Vault integration |

### SSH Forwarding

```
docker buildx build --ssh default .
```

In Dockerfile: `RUN --mount=type=ssh git clone git@github.com:org/repo.git`

**Cloud mapping:** Not directly supported. Cloud builds use service account credentials for repository access instead of SSH forwarding.

### Cache Backends

```
--cache-from type=<backend>,key=value
--cache-to   type=<backend>,key=value,mode=max
```

Backends: `registry`, `local`, `inline`, `gha`, `s3`, `azblob`

**Cloud mapping:**

| Cache Backend | AWS CodeBuild | GCP Cloud Build | Azure ACR Tasks |
|---------------|--------------|-----------------|-----------------|
| `registry` | ECR cache image | AR cache image | ACR cache image |
| `s3` | Native S3 cache in CodeBuild | GCS (via kaniko) | Not applicable |
| `azblob` | Not applicable | Not applicable | Native blob cache |
| `local` | EFS mount in build env | Not applicable | Not applicable |
| `gha` | Not applicable (CI-specific) | Not applicable | Not applicable |
| `inline` | `--build-arg BUILDKIT_INLINE_CACHE=1` | Same | Same |

### Attestations & Provenance

```
docker buildx build --attest type=provenance,mode=max --attest type=sbom .
```

Produces SLSA provenance and SBOM attestations attached to the image manifest.

**Cloud mapping:**

| Cloud | Provenance | SBOM |
|-------|-----------|------|
| AWS | CodeBuild build info → custom attestation | Inspector SBOM (automatic) |
| GCP | Cloud Build provenance (automatic with SLSA 3) | Artifact Analysis |
| Azure | Not built-in (use cosign/notation post-build) | Defender for Containers |

### Output Types

```
docker buildx build --output type=image,push=true .
docker buildx build --output type=local,dest=./out .
docker buildx build --output type=oci,dest=image.tar .
```

**Cloud mapping:** Cloud builds always produce `type=image` pushed to the cloud registry. Other output types would require downloading the result after build.

## Cloud Build Service Mapping

### AWS CodeBuild

```go
type CodeBuildService struct {
    client     *codebuild.Client
    s3         *s3.Client
    project    string // CodeBuild project name
    bucket     string // S3 bucket for context upload
    ecrRepo    string // ECR repo for output images
}
```

**Flow:**
1. Upload context tar to S3
2. `codebuild.StartBuild` with inline buildspec
3. Poll `codebuild.BatchGetBuilds` for status
4. Stream build logs from CloudWatch Logs
5. Image lands in ECR

**Config:**
- `SOCKERLESS_AWS_CODEBUILD_PROJECT` — CodeBuild project name
- `SOCKERLESS_AWS_BUILD_BUCKET` — S3 bucket for context

**Terraform:**
```hcl
resource "aws_codebuild_project" "build" {
  name         = "${local.name_prefix}-build"
  service_role = aws_iam_role.codebuild.arn
  environment {
    compute_type    = "BUILD_GENERAL1_SMALL"
    image           = "aws/codebuild/standard:7.0"
    type            = "LINUX_CONTAINER"
    privileged_mode = true
  }
  source { type = "NO_SOURCE" }
  artifacts { type = "NO_ARTIFACTS" }
}
```

### GCP Cloud Build

```go
type GCPBuildService struct {
    client  *cloudbuild.Client
    gcs     *storage.Client
    project string
    bucket  string // GCS bucket for context
    arRepo  string // Artifact Registry repo
}
```

**Flow:**
1. Upload context tar to GCS
2. `cloudbuild.CreateBuild` with docker build step
3. Poll operation until complete
4. Stream build logs from Cloud Logging
5. Image lands in Artifact Registry

**Config:**
- `SOCKERLESS_GCP_BUILD_BUCKET` — GCS bucket for context

**Terraform:** No resource needed — Cloud Build API enabled per-project.

### Azure ACR Tasks

```go
type ACRBuildService struct {
    client         *armcontainerregistry.Client
    storageAccount string
    container      string
    acrName        string
}
```

**Flow:**
1. Upload context tar to blob storage with SAS URL
2. `ScheduleRun` with DockerBuildRequest
3. Poll run status
4. Stream build logs from ACR run logs
5. Image lands in ACR

**Config:**
- `SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT` — Storage account
- `SOCKERLESS_AZURE_BUILD_CONTAINER` — Blob container name

**Terraform:**
```hcl
resource "azurerm_storage_container" "build" {
  name                  = "build-context"
  storage_account_id    = azurerm_storage_account.main.id
  container_access_type = "private"
}
```

## Interface

```go
// CloudBuildService builds Docker images on cloud infrastructure.
type CloudBuildService interface {
    Build(ctx context.Context, opts CloudBuildOptions) (*CloudBuildResult, error)
    Available() bool
}

type CloudBuildOptions struct {
    Dockerfile   string            // Dockerfile path in context (default "Dockerfile")
    ContextTar   io.Reader         // Build context as tar
    Tags         []string          // Target image tags
    BuildArgs    map[string]string // --build-arg
    Target       string            // Multi-stage --target
    NoCache      bool              // --no-cache
    Platform     string            // --platform (e.g. "linux/amd64")
    Labels       map[string]string // --label
    CacheFrom    []string          // --cache-from refs
    CacheTo      []string          // --cache-to refs
    Secrets      map[string]string // --secret id=key mapped to cloud secrets
}

type CloudBuildResult struct {
    ImageRef  string        // registry/repo:tag
    ImageID   string        // sha256:...
    Duration  time.Duration
    LogStream string        // URL or ARN for build logs
}
```

### Integration with ImageManager

```go
func (m *ImageManager) Build(opts api.ImageBuildOptions, ctx io.Reader) (io.ReadCloser, error) {
    if m.BuildService != nil && m.BuildService.Available() {
        // Delegate to cloud build
        result, err := m.BuildService.Build(context.Background(), CloudBuildOptions{...})
        // Fetch resulting image metadata from registry
        // Store in local image store
        // Return progress stream
    }
    // Fallback: local Dockerfile parsing (no RUN execution)
    return m.Base.ImageBuild(opts, ctx)
}
```

## Earthly

Earthly is a CLI tool that runs a forked BuildKit daemon. It does not expose a Docker-compatible API. Earthly users would use the Earthly CLI locally, produce images, then `docker push` to a Sockerless-connected registry. No direct integration needed.

## Design Decisions

### Secrets: inline + cloud-native

Both mechanisms supported:
1. **Inline secrets** in build request body (secret files embedded in context tar or passed as build args) — forwarded to cloud build as environment variables or mounted files
2. **Cloud-native secrets** referenced by ID — `--secret id=aws:secretsmanager:my-secret` resolved by the cloud build service directly from Secrets Manager / Secret Manager / Key Vault

The `CloudBuildOptions.Secrets` map accepts both: `{"MY_KEY": "inline-value"}` and `{"MY_KEY": "aws:secretsmanager:arn:..."}`. The cloud build service implementation decides how to inject each.

### Libpod build: full Buildah-compatible endpoint

`POST /libpod/build` implemented with Buildah parameter support. Parameters that map to cloud features are honored; parameters that are purely local (isolation modes, cgroup options) are accepted but may be no-ops on cloud backends. Key Buildah params supported:
- `outputformat` (oci/docker), `layers`, `manifest`, `jobs`
- `retry`, `retrydelay`
- `annotations`, `labels`
- `sbom*` parameters (mapped to cloud scanning/attestation)
- `cachefrom`, `cacheto`, `cachettl`

### Cache: parse and pass through

The backend parses `--cache-from` and `--cache-to` parameters and passes them to the cloud build service. Each cloud implementation maps cache backends to its native mechanism:
- `type=registry,ref=...` → used directly by cloud build docker command
- `type=s3,bucket=...` → CodeBuild S3 cache / kaniko S3 for Cloud Build
- `type=azblob,...` → ACR Tasks blob cache
- `type=inline` → `BUILDKIT_INLINE_CACHE=1` build arg

The backend does NOT implement its own cache layer — it delegates entirely to the cloud build service.

## What We Don't Support (and Why)

| Feature | Reason |
|---------|--------|
| BuildKit gRPC session protocol | Cloud builds replace local BuildKit |
| SSH agent forwarding | Cloud builds use service account credentials |
| `--output type=local` | Cloud builds produce images, not local filesystem output |
| Earthly Earthfile format | Separate CLI tool, no API integration |
