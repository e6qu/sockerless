# aws-common

Shared AWS library (`github.com/sockerless/aws-common`, package `awscommon`) consumed by the [ECS](../ecs/README.md) and [Lambda](../lambda/README.md) backends. It holds the AWS SDK implementations of the driver interfaces defined in [`backends/core`](../core/README.md) — registry auth, IAM-role access, Cloud Map DNS + service discovery, EFS-backed volumes, CodeBuild image builds — so both backends wire the same code against their own clients and state.

There is no `main` here; it is a Go library module with `replace` directives pointing at the in-repo `api`, `agent`, and `backends/core` modules.

## Capability areas

| File | Type | Implements | What it provides |
|---|---|---|---|
| `ecr_auth.go` | `ECRAuthProvider` | `core.AuthProvider` | ECR registry auth + image lifecycle. `GetToken` calls ECR `GetAuthorizationToken` and returns `Basic <b64>`; `IsCloudRegistry` matches the `*.dkr.ecr.*.amazonaws.com` pattern; `OnPush` / `OnTag` ensure the ECR repository exists via `CreateRepository` (the actual blob/manifest upload is done by `BaseServer.ImagePush` via `core.OCIPush`); `OnRemove` deletes tags via `BatchDeleteImage`. Failures are returned to the caller, never swallowed. |
| `access_iamrole.go` | `IAMRoleAccess` | `core.AccessDriver` | The `iam-role` access mechanism. The workload principal is the IAM role ARN attached to the task / function (`TaskRoleArn` for ECS, execution role for Lambda); IAM is enforced at the SDK layer via SigV4, so `AuthenticatedClient` returns the default HTTP client for non-SDK paths. |
| `dns_cloudmap.go` | `CloudMapDNS` | `core.DNSDriver` | The `cloud-map` DNS mechanism. Resolves a network's Cloud Map namespace ID (via a per-backend `LookupNamespaceID` callback) to the namespace name, returned as the workload's DNS search domain. |
| `network_discovery_cloudmap.go` | `CloudMapDiscovery` | `core.NetworkDiscoveryDriver` | Service-mesh discovery via Cloud Map (`servicediscovery`). Registers each container as an instance of a per-hostname A-record service in the network's namespace (so `postgres.skls-foo.local` resolves to the instance IP), deregisters on removal (deleting the service when its last instance leaves), and resolves names via `DiscoverInstances`. Instance IDs are the container ID truncated to 12 chars (Cloud Map instance IDs are bounded to 64). Backend-specific state arrives via callbacks captured at construction. |
| `volumes.go` | `EFSManager` | — | Owns sockerless-managed EFS resources backing Docker named volumes: one filesystem per backend instance (discovered or created, with per-subnet mount targets) and one access point per volume, tagged `sockerless-managed=true` + `sockerless-volume-name=<name>`. Operators can pre-set `AgentEFSID` to reuse an existing filesystem. Access-point root paths longer than EFS's 100-char limit are replaced by a SHA256-derived short path. |
| `storage_efsephemeral.go` | `EFSEphemeralDriver` | `core.StorageBackingDriver` | The `efs-ephemeral` storage backing. Translates a `core.SharedVolumeRef` (filesystem ID + access-point ID) into a `core.BackingSpec`; the cloud attaches the access point at task startup, so `PreExec` / `PostExec` are no-ops (no data sync). |
| `build.go` | `CodeBuildService` | `core.CloudBuildService` | Docker image builds via AWS CodeBuild: uploads the build context tar to S3, generates a buildspec (extract context → `aws ecr get-login-password` → `docker build` → `docker push`), starts the build, and polls to completion. Build secrets map to CodeBuild `SECRETS_MANAGER` / `PARAMETER_STORE` env-var types via `aws:secretsmanager:` / `aws:ssm:` value prefixes. `AssembleMultiArchManifest` delegates to the universal core helper with an ECR-minted token. |
| `errors.go` | `MapAWSError` | — | Maps common AWS SDK error strings to typed `api` errors (`NotFoundError`, `ConflictError`, `InvalidParameterError`); everything else is wrapped with resource + ID context. |

## Consumers

Both AWS backends import this module (see each backend's `server.go`, `backend_impl.go`, `backend_delegates.go`, `volumes.go`):

| Backend | Uses |
|---|---|
| [`backends/ecs`](../ecs/README.md) | `NewECRAuthProvider`, `NewIAMRoleAccess`, `CloudMapDNS`, `NewCloudMapDiscovery`, `NewEFSManager` + `NewEFSEphemeralDriver`, `NewCodeBuildService`, `MapAWSError`, `SanitiseVolumePath`, `APVolumeName` |
| [`backends/lambda`](../lambda/README.md) | Same surface, wired against the Lambda backend's clients and state (`pod_materialize.go`, `volumes.go`, `server.go`) |

The drivers follow the shared-library pattern: this module owns the AWS SDK calls; backend-specific lookups (network → namespace ID, container → service ID) arrive as callbacks captured at construction, so the library stays free of backend state.

## Build / test

The Makefile includes the shared library recipes from [`make/go-lib.mk`](../../make/README.md) (standard targets per [`docs/MAKEFILE_STANDARD.md`](../../docs/MAKEFILE_STANDARD.md)):

```sh
cd backends/aws-common
make build   # compile-check (no binary output)
make test    # go test ./...
make lint    # go vet + gofmt (+ golangci-lint when available)

# or from the repo root:
make backends/aws-common/test
```

See also: [`specs/CLOUD_RESOURCE_MAPPING.md`](../../specs/CLOUD_RESOURCE_MAPPING.md) for how Docker volumes / networks map to EFS access points and Cloud Map namespaces, [`specs/IMAGE_REGISTRY.md`](../../specs/IMAGE_REGISTRY.md) for the per-cloud `AuthProvider` table, [`specs/IMAGE_BUILD.md`](../../specs/IMAGE_BUILD.md) for the cloud-build mapping.
