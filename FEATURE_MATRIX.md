# Docker API Compatibility Matrix

This document maps Docker Engine API endpoints to their implementation status across all Sockerless backends. The frontend (`frontends/docker`) translates standard Docker REST API calls (e.g., `POST /containers/create`) into internal backend API calls (`POST /internal/v1/containers`).

## Backends

| Short Name | Package | Description |
|------------|---------|-------------|
| **Core** | `backends/core` | In-memory simulator (shared by all BaseServer backends) |
| **Docker** | `backends/docker` | Real Docker Engine passthrough |
| **ECS** | `backends/ecs` | AWS ECS Fargate |
| **CloudRun** | `backends/cloudrun` | Google Cloud Run Jobs |
| **ACA** | `backends/aca` | Azure Container Apps Jobs |
| **Lambda** | `backends/lambda` | AWS Lambda (FaaS) |
| **GCF** | `backends/cloudrun-functions` | Google Cloud Run Functions (FaaS) |
| **AZF** | `backends/azure-functions` | Azure Functions (FaaS) |

## Status Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Fully implemented |
| ⚠️ | Partial implementation (see notes) |
| ❌ | Returns `NotImplementedError` or not registered |
| ➖ | Not applicable for this backend type |

## Containers

| Command | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|---------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker create` | `POST /containers/create` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker start` | `POST /containers/{id}/start` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker stop` | `POST /containers/{id}/stop` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker restart` | `POST /containers/{id}/restart` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker kill` | `POST /containers/{id}/kill` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker rm` | `DELETE /containers/{id}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker ps` | `GET /containers/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker inspect` | `GET /containers/{id}/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker logs` | `GET /containers/{id}/logs` | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️ | ⚠️ | ⚠️ |
| `docker wait` | `POST /containers/{id}/wait` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker attach` | `POST /containers/{id}/attach` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker top` | `GET /containers/{id}/top` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker stats` | `GET /containers/{id}/stats` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker rename` | `POST /containers/{id}/rename` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker pause` | `POST /containers/{id}/pause` | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `docker unpause` | `POST /containers/{id}/unpause` | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `docker container prune` | `POST /containers/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker update` | `POST /containers/{id}/update` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker diff` | `GET /containers/{id}/changes` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker export` | `GET /containers/{id}/export` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker resize` | `POST /containers/{id}/resize` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Container Logs Notes

- **FaaS backends (Lambda, GCF, AZF)**: Log follow mode (`docker logs -f`) is not supported. Only single-snapshot fetch is available. Logs are retrieved from the respective cloud logging service (CloudWatch, Cloud Logging, Azure Monitor).

## Container Archive (Copy Files)

| Command | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|---------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker cp` (to container) | `PUT /containers/{id}/archive` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker cp` (stat) | `HEAD /containers/{id}/archive` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker cp` (from container) | `GET /containers/{id}/archive` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

## Exec

| Command | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|---------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker exec` (create) | `POST /containers/{id}/exec` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker exec` (start) | `POST /exec/{id}/start` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker exec` (inspect) | `GET /exec/{id}/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker exec` (resize) | `POST /exec/{id}/resize` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Exec Notes

- **Cloud backends (ECS, CloudRun, ACA)**: Exec uses the agent driver chain. If an agent is connected to the container, exec runs on the real container. Otherwise, operations return errors.
- **FaaS backends (Lambda, GCF, AZF)**: Exec uses the reverse agent (agent inside the function dials back to the backend).

## Images

| Command | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|---------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker pull` | `POST /images/create` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker images` | `GET /images/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker inspect` (image) | `GET /images/{name}/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker rmi` | `DELETE /images/{name}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker tag` | `POST /images/{name}/tag` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker history` | `GET /images/{name}/history` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker push` | `POST /images/{name}/push` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker save` | `GET /images/get` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker load` | `POST /images/load` | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `docker search` | `GET /images/search` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker import` | `POST /images/create?fromSrc=` | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `docker image prune` | `POST /images/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Image Notes

- **Cloud and FaaS backends**: `docker pull` registers the image reference in the in-memory store rather than downloading layers. The cloud platform pulls the image at container start time.
- **Image load** (`docker load`): Returns `NotImplementedError` on all cloud/FaaS backends. These backends use registry-based images only.

## Build

| Command | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|---------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker build` | `POST /build` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker builder prune` | `POST /build/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker commit` | `POST /commit` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Build Notes

- **Core**: Build processes the Dockerfile and creates an image record in the store.
- **Docker**: Proxies to the real Docker Engine build API.
- **Cloud/FaaS backends**: Build is handled by the inherited core implementation (in-memory Dockerfile processing). The resulting image is stored locally; the actual cloud build would need to be pushed to a registry.

## Networks

| Command | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|---------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker network create` | `POST /networks/create` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network ls` | `GET /networks` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network inspect` | `GET /networks/{id}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network connect` | `POST /networks/{id}/connect` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network disconnect` | `POST /networks/{id}/disconnect` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network rm` | `DELETE /networks/{id}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network prune` | `POST /networks/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Network Notes

- **ECS/CloudRun/ACA**: Networks are tracked in both the core store and a cloud-specific `NetworkState` store. The actual cloud networking (VPC, subnets) is configured at backend setup time.
- **FaaS backends (Lambda, GCF, AZF)**: Networks are in-memory only (core defaults). Docker networking concepts do not map to FaaS execution.
- **Core**: Network driver with IP allocation (IPAM). On Linux, optional platform network driver provides real network namespace isolation.

## Volumes

| Command | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|---------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker volume create` | `POST /volumes/create` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker volume ls` | `GET /volumes` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker volume inspect` | `GET /volumes/{name}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker volume rm` | `DELETE /volumes/{name}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker volume prune` | `POST /volumes/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Volume Notes

- **ECS/CloudRun/ACA**: Volume remove and prune are overridden to also clean up cloud-specific `VolumeState`.
- **FaaS backends (Lambda, GCF, AZF)**: Volumes are in-memory only (core defaults). Bind mounts and persistent volumes are not supported by FaaS execution models.

## System

| Command | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|---------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker info` | `GET /info` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker version` | `GET /version` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker ping` | `GET/HEAD/POST /_ping` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker events` | `GET /events` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker system df` | `GET /system/df` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker login` | `POST /auth` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### System Notes

- `GET /version` and `GET /_ping` are handled by the frontend, not proxied to backends.
- `GET /info` is proxied to the backend's `/internal/v1/info` endpoint.
- `GET /events` uses the core `EventBus` for all backends. Cloud backends emit events for container lifecycle transitions.

## Unsupported Docker API Endpoints

The following Docker API categories return `501 Not Implemented` from the frontend:

| Category | Endpoints |
|----------|-----------|
| Swarm | `POST /swarm/*`, `GET /swarm` |
| Nodes | `GET /nodes` |
| Services | `GET /services` |
| Tasks | `GET /tasks` |
| Secrets | `GET /secrets` |
| Configs | `GET /configs` |
| Plugins | `GET /plugins` |
| Session | `POST /session` |
| Distribution | `GET /distribution/*` |

## Sockerless Extensions (Non-Docker API)

These endpoints are Sockerless-specific and not part of the standard Docker API.

### Pod API (Libpod-compatible)

| Operation | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|-----------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| Pod create | `POST /libpod/pods/create` | ✅ | ➖ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| Pod list | `GET /libpod/pods/json` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Pod inspect | `GET /libpod/pods/{name}/json` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Pod exists | `GET /libpod/pods/{name}/exists` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Pod start | `POST /libpod/pods/{name}/start` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Pod stop | `POST /libpod/pods/{name}/stop` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Pod kill | `POST /libpod/pods/{name}/kill` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Pod remove | `DELETE /libpod/pods/{name}` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Pod Notes

- **Docker backend**: Does not implement the pod API (uses real Docker which has no pod concept).
- **FaaS backends**: Pod create rejects multi-container pods with `NotImplementedError`. Single-container "pods" work normally.

### Management API

| Operation | API Route | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF|
|-----------|-----------|------|--------|-----|----------|-----|--------|-----|-----|
| Health check | `GET /internal/v1/healthz` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Backend status | `GET /internal/v1/status` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Container summary | `GET /internal/v1/containers/summary` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Metrics | `GET /internal/v1/metrics` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Cloud check | `GET /internal/v1/check` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Provider info | `GET /internal/v1/provider` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Config reload | `POST /internal/v1/reload` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Resource list | `GET /internal/v1/resources` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Orphaned resources | `GET /internal/v1/resources/orphaned` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Resource cleanup | `POST /internal/v1/resources/cleanup` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Agent connect | `GET /internal/v1/agent/connect` | ✅ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
