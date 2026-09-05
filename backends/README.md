# Backends

Backends implement the Sockerless internal API by translating Docker-compatible container operations into cloud-specific resources. Each backend is a separate Go module that serves the Docker REST API (`:3375`) and its internal management endpoints on the same HTTP mux — there is no separate frontend process (see [ARCHITECTURE.md](../ARCHITECTURE.md)).

## Architecture

All backends (except `docker`) are built on top of `core`, a shared library that provides:

- In-memory state management for containers, images, networks, volumes
- HTTP route registration for 50+ Docker API endpoints
- Agent drivers for exec, filesystem, and streaming operations
- Reverse agent registry for FaaS callback connections
- Dockerfile parsing and image build support
- Container health checking
- The cloud-neutral pieces every cloud backend assembles Docker semantics from: shared-volume configuration (`SharedVolumes`, `ParseSharedVolumes`, `TranslateSharedVolumeBinds`), buffered stdin and attach for one-invocation workloads (`StdinPipe`, `BufferedAttachStream`), the bootstrap overlay image (`OverlayImageSpec`, `OverlayContentTag`, `TarOverlayContext`), pod manifests and network-pod materialisation, the pod lifecycle loops (`CloudPodStart` and siblings), managed-volume listing/inspection/pruning, the DNS-zone discovery skeleton (`DNSZoneDiscovery`), and cloud-error mapping (`MapCloudError`)

Per-cloud code that is shared within one cloud family lives in `aws-common`, `gcp-common`, and `azure-common` (SDK client configuration, the cloud's volume storage manager, image resolution against the cloud's registry, the shared-volume tuple format, the DNS record operations behind the shared skeleton, log readers). These three modules never import each other; see [AGENTS.md](../AGENTS.md) § "Shared code has three homes". The exec-envelope wire contract the backends and the bootstrap binaries both speak is `agent/envelope`.

Cloud backends override specific `api.Backend` methods (create, start, stop, kill, remove, logs) via self-dispatch to map container operations to cloud resources, while inheriting everything else from core. For the full system architecture, see [ARCHITECTURE.md](../ARCHITECTURE.md).

## Backends

| Backend | Module | Cloud Resource | Agent Mode |
|---------|--------|----------------|------------|
| [core](core/) | `backend-core` | _(shared library)_ | _(n/a)_ |
| [ecs](ecs/) | `backend-ecs` | ECS Fargate Tasks | Forward |
| [lambda](lambda/) | `backend-lambda` | Lambda Functions | Reverse |
| [cloudrun](cloudrun/) | `backend-cloudrun` | Cloud Run Jobs/Services | Reverse for Service-backed exec |
| [cloudrun-functions](cloudrun-functions/) | `backend-gcf` | Cloud Run Functions | Reverse |
| [aca](aca/) | `backend-aca` | Container Apps Jobs/Apps | Reverse for App-backed exec |
| [azure-functions](azure-functions/) | `backend-azf` | Function Apps | Reverse |
| [docker](docker/) | `backend-docker` | Docker Containers | _(native)_ |

**Exec transport:**
- **ECS** — Uses the configured ECS cloud access path, primarily ECS ExecuteCommand / SSM.
- **Reverse agent** — Workload bootstrap dials back to the backend via `SOCKERLESS_CALLBACK_URL`; required for FaaS/Service/App exec paths.

## Building

Each backend is a separate Go module with its own `go.mod`. Build from the backend directory:

```sh
make backends/ecs/build
# binary lands at backends/ecs/sockerless-backend-ecs
```

Or use the top-level Makefile targets.

## Common flags

All backends accept:

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:3375` | Listen address |
| `-log-level` | `info` | Log level: debug, info, warn, error |

## Configuration

Backends support two configuration methods:

1. **`config.yaml`** (preferred) — A unified YAML file at `~/.sockerless/config.yaml` with named environments and simulator definitions. See the [CLI documentation](../cmd/sockerless/README.md) for the format.
2. **Environment variables** — Traditional per-backend env vars (documented in each backend's README).

**Load order:** config.yaml → context env vars (legacy) → process environment variables → defaults. The YAML config is loaded by the CLI and exported as environment variables before starting the backend binary, so backends themselves only read env vars.

## Common environment variables

| Variable | Description |
|----------|-------------|
| `SOCKERLESS_CALLBACK_URL` | Backend URL for reverse agent connections |
| `SOCKERLESS_ENDPOINT_URL` | Custom cloud API endpoint, commonly a local simulator/cloud-slice endpoint. This changes routing only; API semantics remain cloud-shaped. |
| `SOCKERLESS_FETCH_IMAGE_CONFIG` | Set to `true` to fetch real image configs from Docker registries |

## Testing

Backend integration tests run against the [simulators from the sockerless-cloud repository](https://github.com/e6qu/sockerless-cloud), consumed as pinned Go modules (`make install-simulators` builds them into `tests/.build/`):

```sh
# Run all simulator-backend integration tests (and every other integration suite)
make test-integration

# Per-backend integration tests via path delegation
make backends/ecs/test-integration                  # AWS: ECS
make backends/lambda/test-integration               # AWS: Lambda
make backends/cloudrun/test-integration             # GCP: Cloud Run
make backends/cloudrun-functions/test-integration   # GCP: Cloud Run Functions
make backends/aca/test-integration                  # Azure: Container Apps
make backends/azure-functions/test-integration      # Azure: Azure Functions
```

Full Terraform integration tests deploy real cloud resources:

```sh
make tf-int-test-aws      # ECS + Lambda
make tf-int-test-gcp      # CloudRun + GCF
make tf-int-test-azure    # ACA + AZF
```
