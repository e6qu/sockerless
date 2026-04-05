# Image Registry Specification

Cloud-native container registries and the OCI Distribution v2 protocol.

## Current State

Three `AuthProvider` implementations handle per-cloud registry auth and lifecycle:

| Cloud | Provider | Registry | Auth Mechanism | File |
|-------|----------|----------|---------------|------|
| AWS | `ECRAuthProvider` | ECR (`*.dkr.ecr.*.amazonaws.com`) | `ecr.GetAuthorizationToken` → Basic | `backends/aws-common/ecr_auth.go` |
| GCP | `ARAuthProvider` | Artifact Registry (`*.gcr.io`, `*-docker.pkg.dev`) | `google.FindDefaultCredentials` → Bearer | `backends/gcp-common/ar_auth.go` |
| Azure | `ACRAuthProvider` | ACR (`*.azurecr.io`) | `azidentity.DefaultAzureCredential` → Bearer | `backends/azure-common/acr_auth.go` |

## OCI Distribution v2 Protocol

All three registries speak OCI Distribution Spec v2. Our implementations:

### Pull (FetchImageMetadata)

Source: `backends/core/registry.go`

```
GET /v2/{repo}/manifests/{tag}        → manifest (or manifest list)
GET /v2/{repo}/manifests/{digest}     → platform-specific manifest (if manifest list)
GET /v2/{repo}/blobs/{config_digest}  → OCI image config JSON
```

Auth: anonymous first, then `Www-Authenticate` → token exchange at realm URL.

Returns: Config (Cmd, Entrypoint, Env, WorkingDir, ExposedPorts, Labels, Healthcheck), layer digests, sizes, history, architecture, OS.

### Push (OCIPush)

Source: `backends/core/oci_push.go`

```
GET  /v2/                                    → ping (verify connectivity)
POST /v2/{repo}/blobs/uploads/               → initiate upload (returns Location)
PUT  {Location}?digest={sha256:...}          → upload blob content
PUT  /v2/{repo}/manifests/{tag}              → push manifest
```

Uploads: config blob + layer blobs (from `LayerContent` store or error if absent).

### Tag

- **ECR**: `OCIPush` with new tag (re-pushes manifest)
- **GCP**: `OCIPush` with new tag
- **Azure**: GET manifest by old tag → PUT manifest with new tag

### Remove

- **ECR**: `ecr.BatchDeleteImage` (SDK call, by tag)
- **GCP**: `DELETE /v2/{repo}/manifests/{tag}` (OCI v2)
- **Azure**: HEAD for digest → `DELETE /v2/{repo}/manifests/{digest}` (OCI v2, by digest)

## Missing OCI v2 Features

| Feature | OCI Spec | Status | Priority |
|---------|----------|--------|----------|
| Blob mount (cross-repo copy) | `POST /v2/{repo}/blobs/uploads/?mount={digest}&from={repo}` | Not implemented | Medium |
| Chunked upload | `PATCH {Location}` with Content-Range | Not implemented | Low |
| Manifest list (multi-arch) | `application/vnd.oci.image.index.v1+json` | Read only (FetchImageMetadata handles it) | Medium for push |
| Referrers API | `GET /v2/{repo}/referrers/{digest}` | Not implemented | Low (OCI 1.1) |
| OCI artifact types | `artifactType` field in manifest | Not implemented | Low (OCI 1.1) |

## Cloud Registry Features

### ECR

| Feature | API | Status |
|---------|-----|--------|
| Repository creation | `ecr.CreateRepository` | Implemented (OnPush) |
| Image deletion | `ecr.BatchDeleteImage` | Implemented (OnRemove) |
| Lifecycle policies | `ecr.PutLifecyclePolicy` | Not implemented |
| Image scanning | `ecr.StartImageScan` | Not implemented |
| Pull-through cache | `ecr.CreatePullThroughCacheRule` | Not implemented |
| Replication | `ecr.PutReplicationConfiguration` | Not implemented |
| Image tag immutability | `ecr.PutImageTagMutability` | Not implemented |

### Artifact Registry

| Feature | API | Status |
|---------|-----|--------|
| Repository creation | `artifactregistry.CreateRepository` | Not implemented (repo must pre-exist) |
| Image deletion | OCI v2 DELETE | Implemented (OnRemove) |
| Vulnerability scanning | `containeranalysis.ListOccurrences` | Not implemented |
| Remote repositories | Console/gcloud only | Not implemented |
| Cleanup policies | `artifactregistry.UpdateRepository` | Not implemented |

### ACR

| Feature | API | Status |
|---------|-----|--------|
| Repository listing | ACR REST API | Not implemented |
| Image deletion | OCI v2 DELETE | Implemented (OnRemove) |
| Security scanning | Microsoft Defender API | Not implemented |
| Geo-replication | `containerregistry.Replications` | Not implemented |
| Connected registries | `containerregistry.ConnectedRegistries` | Not implemented |
| Content trust (signing) | Notation integration | Not implemented |

## Auth Token Caching

Currently no auth token caching exists. Each operation calls `GetToken()` fresh. Cloud tokens typically last 1-12 hours:

| Cloud | Token Lifetime | Caching Strategy |
|-------|---------------|-----------------|
| ECR | 12 hours | Cache with TTL |
| GCP | 1 hour | Cache with refresh |
| Azure | 1 hour | Cache with refresh |

Implementing token caching would reduce latency on repeated operations.
