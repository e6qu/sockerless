# Sockerless Specification

> **Version:** 0.2.0
>
> **Date:** February 2026
>
> **Status:** Updated specification — reflects actual implementation as of Phase 35
>
> **Mission:** A Docker-compatible REST API daemon that executes containers on cloud serverless backends (AWS ECS, Google Cloud Run, Azure Container Apps, and others) instead of a local Docker Engine.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Design Goals and Non-Goals](#2-design-goals-and-non-goals)
3. [Target Docker API Surface](#3-target-docker-api-surface)
4. [Endpoint Specifications](#4-endpoint-specifications)
5. [Protocol Requirements](#5-protocol-requirements)
6. [System Architecture](#6-system-architecture)
7. [Internal API — Frontend ↔ Backend](#7-internal-api--frontend--backend)
8. [Agent Protocol](#8-agent-protocol)
9. [Cloud Backend Mapping](#9-cloud-backend-mapping)
10. [Volume Emulation](#10-volume-emulation)
11. [Network Emulation](#11-network-emulation)
12. [Docker Compose Support](#12-docker-compose-support)
13. [CI Runner Compatibility](#13-ci-runner-compatibility)
    - [13.1 GitLab Runner Docker-Executor](#131-gitlab-runner-docker-executor)
    - [13.2 GitHub Actions Runner](#132-github-actions-runner)
    - [13.3 Gap Analysis and Solutions](#133-gap-analysis-and-solutions)
    - [13.4 FaaS Backend Compatibility](#134-faas-backend-compatibility)
    - [13.5 Backend Compatibility Matrix for CI Runners](#135-backend-compatibility-matrix-for-ci-runners)
14. [Limitations and Exclusions](#14-limitations-and-exclusions)
15. [Configuration](#15-configuration)
16. [Implementation Phases](#16-implementation-phases)

---

## 1. Project Overview

### 1.1 What Sockerless Is

Sockerless is a daemon that:

1. Listens on a Unix socket (or TCP) and speaks the Docker Engine REST API
2. Translates Docker API calls into cloud provider operations (AWS ECS tasks, Google Cloud Run jobs, Azure Container Apps jobs, etc.)
3. Maintains an in-memory (or lightweight persistent) state of containers, networks, and volumes it manages
4. Implements the multiplexed stream protocol for attach/exec I/O

A standard Docker CLI (`docker run`, `docker ps`, `docker logs`, etc.) or Docker SDK connects to sockerless exactly as it would to a real Docker daemon. The difference is that containers run on cloud serverless infrastructure rather than on the local machine.

### 1.2 Primary Use Cases

| Priority | Use Case | What It Requires |
|----------|----------|------------------|
| **P0** | GitLab Runner with docker-executor | Containers, exec, attach (hijacked + multiplexed), volumes, networks, image pull, logs, wait |
| **P0** | GitHub Actions Runner with container jobs | Containers, exec (primary mechanism), image pull, logs, networks, health checks |
| **P1** | `docker compose up/down/ps/logs` | Multi-container orchestration, networks, volumes, label-based grouping |
| **P2** | General `docker run` / `docker exec` usage | Interactive developer use |

### 1.3 Why This Matters

No existing project fills this niche. As documented in `DOCKER_REST_API.md`:

- Docker Engine, Docker Desktop, Colima, Rancher Desktop all run containers locally
- Podman reimplements the API but still runs containers locally
- Testcontainers Cloud proxies the API to remote Docker Engines (still Docker under the hood)
- Fly.io Machines has a proprietary API (0% Docker compat)
- No cloud service (AWS ECS, GCR, Azure ACA) exposes a Docker-compatible REST API

Sockerless bridges this gap: **Docker API on top, cloud serverless capacity on the bottom.**

---

## 2. Design Goals and Non-Goals

### 2.1 Goals

| # | Goal |
|---|------|
| G1 | GitLab Runner docker-executor works without modification |
| G2 | GitHub Actions Runner container jobs work without modification |
| G3 | `docker compose up/down/ps/logs` works for basic service stacks |
| G4 | Backend-agnostic API layer with pluggable cloud adapters |
| G5 | Standard Docker CLI works as the client (no custom CLI needed) |
| G6 | Multiplexed stream protocol (8-byte header framing) for attach/exec I/O |
| G7 | Emulate volumes via cloud storage and networks via cloud VPC/service mesh |

### 2.2 Non-Goals

| # | Non-Goal | Reason | Status |
|---|----------|--------|--------|
| ~~N1~~ | ~~`docker build` / `POST /build`~~ | ~~Out of scope~~ | **Implemented** (Phase 34). Dockerfile parser supports FROM, COPY, ADD, ENV, CMD, ENTRYPOINT, WORKDIR, ARG, LABEL, EXPOSE, USER. RUN instructions are no-op (sufficient for CI Dockerfile patterns). Multi-stage builds supported. |
| N2 | Swarm endpoints (`/swarm/*`, `/services/*`, `/tasks/*`, `/nodes/*`, `/secrets/*`, `/configs/*`) | Neither CI runner uses Swarm. No cloud backend maps to it. | Not implemented |
| N3 | Plugin endpoints (`/plugins/*`) | Docker plugin system is not relevant to cloud backends. | Not implemented |
| N4 | Session/BuildKit endpoint (`POST /session`) | BuildKit-specific. | Not implemented |
| N5 | Distribution endpoint (`GET /distribution/{name}/json`) | Not used by target CI runners. | Not implemented |
| ~~N6~~ | ~~Container filesystem archive (`GET/PUT/HEAD /containers/{id}/archive`)~~ | ~~Not used by target CI runners~~ | **Implemented** (Phase 25). Required for `docker cp` before `docker start` pattern used by gitlab-ci-local. Pre-start staging directories support archive extraction before container start. |
| N7 | Image push (`POST /images/{name}/push`) | Users push images to registries externally. | Not implemented |
| N8 | Sub-second container startup | Cloud backends have inherent startup latency (5-60s). Accepted tradeoff. | Accepted |
| ~~N9~~ | ~~FaaS backends as primary targets~~ | ~~Deferred to future phases~~ | **Implemented** (Phase 19). Lambda, Cloud Functions, and Azure Functions backends are fully operational with reverse agent support for exec/attach. FaaS backends use reverse agent exclusively — the agent inside the function dials back to the backend via `SOCKERLESS_CALLBACK_URL`. Helper and cache containers auto-stop after 500ms. Not CI-runner compatible but useful for short-lived workloads. |

---

## 3. Target Docker API Surface

### 3.1 Supported Endpoints — Summary

**50+ endpoints** across 8 categories. This started as the union of endpoints required by GitLab Runner docker-executor and GitHub Actions Runner, and has been extended to cover additional Docker API operations including build, archive, and lifecycle management.

| Category | Count | Endpoints |
|----------|-------|-----------|
| System | 6 | `_ping` (GET, HEAD), `version`, `info`, `events`, `system/df` |
| Images | 9 | pull, inspect, load, tag, auth, build, list, remove, history, prune |
| Containers | 17 | create, start, inspect, list, logs, attach, wait, stop, kill, remove, restart, rename, pause, unpause, top, stats, prune |
| Exec | 3 | create, start, inspect |
| Networks | 7 | create, list, inspect, connect, disconnect, remove, prune |
| Volumes | 5 | create, list, inspect, remove, prune |
| Archive | 3 | put, head, get |

### 3.2 Endpoint Priority

Each endpoint is classified by which CI runner requires it. Endpoints marked "Impl" were not originally in scope but have been implemented.

| Endpoint | GitLab Runner | GitHub Runner | Docker Compose | Priority | Status |
|----------|:---:|:---:|:---:|:---:|:---:|
| **System** | | | | | |
| `GET /_ping` | Yes | — | Yes | P0 | Done |
| `HEAD /_ping` | Yes | — | Yes | P0 | Done |
| `GET /version` | Yes | Yes | Yes | P0 | Done |
| `GET /info` | Yes | — | Yes | P0 | Done |
| `GET /events` | — | — | Yes | P2 | Done |
| `GET /system/df` | — | — | — | P2 | Done |
| **Images** | | | | | |
| `POST /images/create` (pull) | Yes | Yes | Yes | P0 | Done |
| `GET /images/{name}/json` | Yes | — | Yes | P0 | Done |
| `POST /images/load` | Yes | — | — | P1 | Done |
| `POST /images/{name}/tag` | Yes | — | — | P1 | Done |
| `POST /auth` | — | Yes | Yes | P1 | Done |
| `POST /build` | — | — | Yes | Impl | Done (Phase 34) |
| `GET /images/json` | — | — | Yes | Impl | Done |
| `DELETE /images/{name}` | — | — | — | Impl | Done |
| `GET /images/{name}/history` | — | — | — | Impl | Done |
| `POST /images/prune` | — | — | — | Impl | Done |
| **Containers** | | | | | |
| `POST /containers/create` | Yes | Yes | Yes | P0 | Done |
| `POST /containers/{id}/start` | Yes | Yes | Yes | P0 | Done |
| `GET /containers/{id}/json` | Yes | Yes | Yes | P0 | Done |
| `GET /containers/json` | Yes | Yes | Yes | P0 | Done |
| `GET /containers/{id}/logs` | Yes | Yes | Yes | P0 | Done |
| `POST /containers/{id}/attach` | Yes | — | — | P0 | Done |
| `POST /containers/{id}/wait` | Yes | Yes | — | P0 | Done |
| `POST /containers/{id}/stop` | Yes | — | Yes | P0 | Done |
| `POST /containers/{id}/kill` | Yes | — | Yes | P0 | Done |
| `DELETE /containers/{id}` | Yes | Yes | Yes | P0 | Done |
| `POST /containers/{id}/restart` | — | — | Yes | Impl | Done |
| `POST /containers/{id}/rename` | — | — | — | Impl | Done |
| `POST /containers/{id}/pause` | — | — | Yes | Impl | Done |
| `POST /containers/{id}/unpause` | — | — | Yes | Impl | Done |
| `GET /containers/{id}/top` | — | — | — | Impl | Done |
| `GET /containers/{id}/stats` | — | — | — | Impl | Done |
| `POST /containers/prune` | — | — | Yes | Impl | Done |
| **Archive** | | | | | |
| `PUT /containers/{id}/archive` | — | — | — | Impl | Done (Phase 25) |
| `HEAD /containers/{id}/archive` | — | — | — | Impl | Done (Phase 25) |
| `GET /containers/{id}/archive` | — | — | — | Impl | Done (Phase 25) |
| **Exec** | | | | | |
| `POST /containers/{id}/exec` | Yes | Yes | — | P0 | Done |
| `POST /exec/{id}/start` | Yes | Yes | — | P0 | Done |
| `GET /exec/{id}/json` | — | — | — | P2 | Done |
| **Networks** | | | | | |
| `POST /networks/create` | Yes | Yes | Yes | P0 | Done |
| `GET /networks` | Yes | — | Yes | P0 | Done |
| `GET /networks/{id}` | Yes | — | Yes | P0 | Done |
| `POST /networks/{id}/connect` | — | — | Yes | Impl | Done |
| `POST /networks/{id}/disconnect` | Yes | — | — | P1 | Done |
| `DELETE /networks/{id}` | Yes | Yes | Yes | P0 | Done |
| `POST /networks/prune` | — | Yes | — | P1 | Done |
| **Volumes** | | | | | |
| `POST /volumes/create` | Yes | — | Yes | P0 | Done |
| `GET /volumes` | Yes | — | Yes | P0 | Done |
| `GET /volumes/{name}` | Yes | — | Yes | P0 | Done |
| `DELETE /volumes/{name}` | Yes | — | Yes | P0 | Done |
| `POST /volumes/prune` | — | — | Yes | Impl | Done |

### 3.3 API Version

Sockerless targets **Docker API v1.44** (Docker 25.0). This is chosen because:

- GitLab Runner uses API version negotiation and has specific code paths for v1.44 (MAC address field moved from `Config` to `NetworkingConfig.EndpointsConfig`)
- GitHub Actions Runner requires minimum v1.35
- v1.44 is modern enough to satisfy all current clients but avoids v1.45-specific features we don't need

The `GET /_ping` response will include `API-Version: 1.44` and `Docker-Experimental: false`.

---

## 4. Endpoint Specifications

### 4.1 System Endpoints

#### `GET /_ping` / `HEAD /_ping`

Returns `OK` (plain text). Headers:
```
API-Version: 1.44
Docker-Experimental: false
Ostype: linux
Builder-Version: 0
```

Used by Docker SDK for API version negotiation. Must respond fast (no backend calls).

#### `GET /version`

```json
{
  "Platform": { "Name": "Sockerless" },
  "Version": "0.1.0",
  "ApiVersion": "1.44",
  "MinAPIVersion": "1.44",
  "Os": "linux",
  "Arch": "amd64",
  "KernelVersion": "",
  "BuildTime": "2026-02-15T00:00:00.000000000+00:00",
  "Components": [
    {
      "Name": "Sockerless",
      "Version": "0.1.0",
      "Details": {
        "Backend": "<configured-backend-name>"
      }
    }
  ]
}
```

#### `GET /info`

Returns system information. Key fields that CI runners read:

| Field | Sockerless Value | Why |
|-------|-----------------|-----|
| `OSType` | `"linux"` | GitLab Runner uses this to detect container OS type |
| `Architecture` | `"x86_64"` | Informational |
| `NCPU` | Backend-dependent | |
| `MemTotal` | Backend-dependent | |
| `ServerVersion` | `"0.1.0"` | |
| `Swarm.LocalNodeState` | `"inactive"` | No Swarm support |
| `Runtimes` | `{"sockerless": {"path": "sockerless"}}` | |
| `DefaultRuntime` | `"sockerless"` | |
| `SecurityOptions` | `[]` | |

### 4.2 Image Endpoints

#### `POST /images/create` — Pull Image

Query parameters:
- `fromImage` (required): Image reference (e.g., `docker.io/library/nginx:latest`)
- `tag`: Tag (often included in `fromImage`)
- `platform`: OS/arch (e.g., `linux/amd64`)

Headers:
- `X-Registry-Auth`: Base64-encoded JSON credentials (`{"username":"...","password":"...","serveraddress":"..."}`)

**Response:** Streaming JSON (one JSON object per line), Docker pull progress format:
```json
{"status":"Pulling from library/nginx","id":"latest"}
{"status":"Pulling fs layer","progressDetail":{},"id":"abc123"}
{"status":"Download complete","progressDetail":{},"id":"abc123"}
{"status":"Pull complete","progressDetail":{},"id":"abc123"}
{"status":"Digest: sha256:..."}
{"status":"Status: Downloaded newer image for nginx:latest"}
```

**Sockerless behavior:**
- Records the image reference in the internal state store
- Does NOT actually pull the image locally — the cloud backend will pull it when a container is created
- Validates that the image exists in the registry (HEAD request to registry API) if credentials are provided
- The progress output can be synthetic (immediate "Pull complete") since no actual download occurs locally

#### `GET /images/{name}/json` — Inspect Image

Returns image metadata. For sockerless, this will be a **minimal response** constructed from the registry manifest (or cached from a previous pull):

```json
{
  "Id": "sha256:<digest>",
  "RepoTags": ["nginx:latest"],
  "RepoDigests": ["nginx@sha256:..."],
  "Created": "2026-01-01T00:00:00Z",
  "Size": 0,
  "VirtualSize": 0,
  "Config": {
    "Env": ["PATH=/usr/local/sbin:..."],
    "Cmd": ["nginx", "-g", "daemon off;"],
    "Entrypoint": null,
    "ExposedPorts": {"80/tcp": {}},
    "Labels": {},
    "WorkingDir": ""
  },
  "Os": "linux",
  "Architecture": "amd64"
}
```

GitLab Runner reads `Config.Env` and `Config.Cmd` from this. If registry metadata retrieval is not available, return a minimal valid response. Image config can be fetched from the registry's manifest API without pulling layers.

#### `POST /images/load` — Load Image from Tar

Accepts a tar stream containing a Docker image. For sockerless:
- Extract image metadata from the tar
- Push the image to a configured registry (cloud backends pull from registries, not local stores)
- Or: record the image in internal state for later use
- GitLab Runner uses this for its helper image

#### `POST /images/{name}/tag` — Tag Image

Records a new tag for an existing image in the internal state. No cloud backend interaction needed.

#### `POST /auth` — Registry Authentication

Validates credentials against a registry. Used by GitHub Actions Runner's `docker login`.

Request body: `{"username":"...","password":"...","serveraddress":"..."}`
Response: `{"Status": "Login Succeeded"}`

Store credentials for later use with `POST /images/create`.

### 4.3 Container Endpoints

#### `POST /containers/create` — Create Container

This is the most complex endpoint. Query parameter: `name` (optional container name).

**Request body structure** (fields actually used by CI runners):

```json
{
  "Image": "nginx:latest",
  "Hostname": "runner-abc-project-1-concurrent-0",
  "Cmd": ["sh", "-c", "echo hello"],
  "Entrypoint": ["/bin/sh"],
  "Env": ["CI=true", "GITLAB_CI=true"],
  "Labels": {
    "com.gitlab.gitlab-runner.managed": "true",
    "com.docker.compose.project": "myapp"
  },
  "User": "",
  "Tty": false,
  "AttachStdin": true,
  "AttachStdout": true,
  "AttachStderr": true,
  "OpenStdin": true,
  "StdinOnce": true,
  "ExposedPorts": {"80/tcp": {}},
  "WorkingDir": "/app",
  "HostConfig": {
    "Binds": ["/builds:/builds", "/cache:/cache"],
    "Memory": 536870912,
    "NanoCPUs": 1000000000,
    "NetworkMode": "<network-id>",
    "PortBindings": {"80/tcp": [{"HostPort": "8080"}]},
    "RestartPolicy": {"Name": "no"},
    "ExtraHosts": ["service:10.0.0.2"],
    "Dns": ["8.8.8.8"],
    "Privileged": false,
    "VolumesFrom": [],
    "Tmpfs": {}
  },
  "NetworkingConfig": {
    "EndpointsConfig": {
      "<network-name>": {
        "Aliases": ["postgres", "db"],
        "MacAddress": "02:42:ac:11:00:02"
      }
    }
  }
}
```

**Response:**
```json
{
  "Id": "<64-char-hex-id>",
  "Warnings": []
}
```

**Sockerless behavior:**
1. Generate a unique 64-character hex container ID
2. Store the full container configuration in internal state
3. Resolve `Binds` to cloud volume mappings (see [Volume Emulation](#10-volume-emulation))
4. Resolve `NetworkMode` and `NetworkingConfig` to cloud network mappings (see [Network Emulation](#11-network-emulation))
5. Do NOT start the cloud backend task yet (that happens on `POST /containers/{id}/start`)

**Fields that must be preserved in state** (read back by inspect):
- `Image`, `Hostname`, `Cmd`, `Entrypoint`, `Env`, `Labels`, `User`, `Tty`
- `ExposedPorts`, `WorkingDir`
- `HostConfig.*` (all fields listed above)
- `NetworkingConfig.*`
- `AttachStdin`, `AttachStdout`, `AttachStderr`, `OpenStdin`, `StdinOnce`

**Fields that can be silently ignored** (not relevant to cloud backends):
- `MacAddress` (v1.44 location: inside `EndpointsConfig`)
- `CgroupParent`, `DeviceCgroupRules`, `PidMode`, `UTSMode`
- `SecurityOpt`, `CapAdd`, `CapDrop`, `UsernsMode`
- `ShmSize`, `Sysctls`, `OomKillDisable`, `OomScoreAdj`
- `Runtime`, `Isolation`, `Init`

Silently-ignored fields should still be stored and returned in inspect responses for client compatibility, but they do not need to affect cloud backend behavior.

#### `POST /containers/{id}/start` — Start Container

**Sockerless behavior:**
1. Translate the stored container configuration into a cloud backend task/job:
   - Image → cloud task definition / job spec
   - Env → cloud task environment variables
   - Cmd/Entrypoint → cloud task command
   - Resource limits → cloud task resource allocation
   - Network → cloud VPC / network configuration
   - Volume binds → cloud storage mounts
2. Launch the cloud backend task
3. Start the exec/attach agent inside the container (see [Agent Protocol](#8-agent-protocol))
4. Update container state to `Running`

Response: `204 No Content` (same as Docker)

#### `GET /containers/{id}/json` — Inspect Container

Returns full container metadata. **CI runners rely on specific fields:**

| Field | GitLab Runner Reads | GitHub Runner Reads |
|-------|:---:|:---:|
| `State.Status` ("created", "running", "exited") | Yes | — |
| `State.Running` (bool) | Yes | — |
| `State.ExitCode` (int) | Yes | — |
| `State.Health.Status` ("healthy", "unhealthy", "starting") | — | Yes |
| `Config.Env` (array) | — | Yes (PATH extraction) |
| `Config.Healthcheck` (object) | — | Yes (presence check) |
| `NetworkSettings.IPAddress` | Yes | — |
| `NetworkSettings.Networks.<name>.IPAddress` | Yes | — |
| `NetworkSettings.Ports` | — | Yes (port mapping) |

**Response structure (key fields):**
```json
{
  "Id": "<64-char-hex>",
  "Created": "2026-02-15T12:00:00Z",
  "Name": "/container-name",
  "State": {
    "Status": "running",
    "Running": true,
    "Paused": false,
    "Restarting": false,
    "OOMKilled": false,
    "Dead": false,
    "Pid": 1,
    "ExitCode": 0,
    "Error": "",
    "StartedAt": "2026-02-15T12:00:01Z",
    "FinishedAt": "0001-01-01T00:00:00Z",
    "Health": {
      "Status": "healthy",
      "FailingStreak": 0,
      "Log": []
    }
  },
  "Config": {
    "Image": "nginx:latest",
    "Env": ["PATH=/usr/local/sbin:..."],
    "Cmd": ["nginx"],
    "Entrypoint": null,
    "Hostname": "...",
    "Labels": {},
    "ExposedPorts": {},
    "Tty": false,
    "OpenStdin": false,
    "Healthcheck": null,
    "WorkingDir": ""
  },
  "HostConfig": { "..." },
  "NetworkSettings": {
    "IPAddress": "10.0.0.5",
    "Ports": {
      "80/tcp": [{"HostIp": "0.0.0.0", "HostPort": "8080"}]
    },
    "Networks": {
      "bridge": {
        "IPAddress": "10.0.0.5",
        "Aliases": ["db"],
        "NetworkID": "..."
      }
    }
  }
}
```

#### `GET /containers/json` — List Containers

Query parameters:
- `all` (bool): Include stopped containers (default: only running)
- `filters` (JSON): Label, status, id, name filters

**Filter formats used by CI runners:**
```
# GitLab Runner - find containers by label
filters={"label":["com.gitlab.gitlab-runner.managed=true"]}

# GitHub Actions Runner - find by label
filters={"label":["<instance-label>"]}

# GitHub Actions Runner - find by id + status
filters={"id":["<container-id>"],"status":["running"]}
```

**Response:** Array of container summary objects:
```json
[
  {
    "Id": "<64-char-hex>",
    "Names": ["/container-name"],
    "Image": "nginx:latest",
    "ImageID": "sha256:...",
    "Command": "nginx -g 'daemon off;'",
    "Created": 1708000000,
    "State": "running",
    "Status": "Up 5 minutes",
    "Ports": [{"PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"}],
    "Labels": {"com.gitlab.gitlab-runner.managed": "true"},
    "NetworkSettings": {
      "Networks": {
        "bridge": {"IPAddress": "10.0.0.5"}
      }
    }
  }
]
```

Sockerless must support filtering by: `label`, `id`, `name`, `status`.

#### `GET /containers/{id}/logs` — Container Logs

Query parameters:
- `stdout` (bool): Include stdout
- `stderr` (bool): Include stderr
- `follow` (bool): Stream logs (long-poll)
- `timestamps` (bool): Add RFC3339Nano timestamps
- `tail` (string): Number of lines from end ("all" or number)
- `details` (bool): Include extra attributes

**Response format:** Multiplexed stream (when `Tty: false`, which is the CI runner case):
```
[stream_type: 1 byte][0x00 0x00 0x00][size: 4 bytes big-endian][payload: size bytes]
```
Where `stream_type` = `1` (stdout) or `2` (stderr).

**Sockerless behavior:**
- Fetch logs from the cloud backend's logging service (CloudWatch, Cloud Logging, Azure Monitor)
- Format into Docker's multiplexed stream protocol
- For `follow=true`, keep the connection open and stream new log entries

GitLab Runner reads logs with: `stdout=true, stderr=true, timestamps=true, follow=false` (one-shot, 64KB limit).
GitHub Runner reads logs with: `details=true, stdout=true, stderr=true`.

#### `POST /containers/{id}/attach` — Attach to Container

This is a **connection-hijacking** endpoint. After the HTTP response headers, the connection is "hijacked" — it becomes a raw bidirectional byte stream.

Query parameters:
- `stream` (bool): Stream attached
- `stdin` (bool): Attach stdin
- `stdout` (bool): Attach stdout
- `stderr` (bool): Attach stderr

**Response:** HTTP 101 Switching Protocols (or 200 with hijacked connection), then multiplexed stream.

**How GitLab Runner uses attach:**
1. Creates container with `Cmd` set to the script, `OpenStdin: true`
2. Calls `POST /containers/{id}/attach` with `stream=true, stdin=true, stdout=true, stderr=true` BEFORE starting the container
3. Calls `POST /containers/{id}/start`
4. Streams stdin (the build script) over the hijacked connection
5. Reads stdout/stderr from the hijacked connection using the 8-byte multiplexed frame protocol
6. Connection closes when container exits

This is the **most critical endpoint** for GitLab Runner compatibility. See [Section 8](#8-agent-protocol) for implementation strategy.

#### `POST /containers/{id}/wait` — Wait for Container to Exit

Query parameter:
- `condition` (string): `not-running` (used by GitLab Runner), `next-exit`, `removed`

**Response** (when container exits):
```json
{
  "StatusCode": 0,
  "Error": null
}
```

Long-polling endpoint. Blocks until the container reaches the specified condition. Sockerless polls the cloud backend task status and returns when the task completes.

#### `POST /containers/{id}/stop` — Stop Container

Query parameter:
- `t` (int): Seconds to wait before killing (default: 10)

Sends SIGTERM, waits `t` seconds, then SIGKILL. For sockerless, translates to the cloud backend's stop/terminate mechanism.

Response: `204 No Content`

#### `POST /containers/{id}/kill` — Kill Container

Query parameter:
- `signal` (string): Signal to send (default: `SIGKILL`)

Immediate termination. For sockerless, translates to force-stopping the cloud backend task.

Response: `204 No Content`

#### `DELETE /containers/{id}` — Remove Container

Query parameters:
- `v` (bool): Remove associated anonymous volumes
- `force` (bool): Kill and remove even if running

Response: `204 No Content`

Sockerless: Remove the container from internal state. If the cloud backend task is still running and `force=true`, stop it first. Clean up any associated cloud resources (volumes, network attachments).

### 4.4 Exec Endpoints

#### `POST /containers/{id}/exec` — Create Exec Instance

**Request body:**
```json
{
  "AttachStdin": true,
  "AttachStdout": true,
  "AttachStderr": true,
  "Tty": false,
  "Cmd": ["sh", "-c", "echo hello"],
  "Env": ["FOO=bar"],
  "WorkingDir": "/app"
}
```

**Response:**
```json
{
  "Id": "<exec-id>"
}
```

Records the exec configuration. Does not execute anything yet.

#### `POST /exec/{id}/start` — Start Exec Instance

**Request body:**
```json
{
  "Detach": false,
  "Tty": false
}
```

**Response:** Connection hijack with multiplexed stream (same as attach). This is another **connection-hijacking** endpoint.

When `Detach: false`, the response hijacks the HTTP connection and streams stdin/stdout/stderr bidirectionally. The exec command runs inside the already-running container.

**This is the primary mechanism for GitHub Actions Runner** — every workflow step is `docker exec` into the job container.

See [Section 8](#8-agent-protocol) for implementation strategy.

#### `GET /exec/{id}/json` — Inspect Exec Instance

```json
{
  "ID": "<exec-id>",
  "Running": false,
  "ExitCode": 0,
  "Pid": 0
}
```

### 4.5 Network Endpoints

#### `POST /networks/create`

**Request body:**
```json
{
  "Name": "github_network_abc123",
  "Labels": {"com.gitlab.gitlab-runner.managed": "true"},
  "EnableIPv6": false,
  "Driver": "bridge",
  "Options": {
    "com.docker.network.driver.mtu": "1500"
  }
}
```

**Response:**
```json
{
  "Id": "<64-char-hex>",
  "Warning": ""
}
```

Sockerless: Creates a logical network in internal state. The actual cloud networking is configured when containers join this network. See [Network Emulation](#11-network-emulation).

#### `GET /networks` — List Networks

Query parameter: `filters` (JSON)

Filter formats used: `{"label":["key=value"]}`, `{"name":["name"]}`

#### `GET /networks/{id}` — Inspect Network

Returns network details including connected containers:
```json
{
  "Name": "build-network",
  "Id": "<hex>",
  "Created": "2026-02-15T12:00:00Z",
  "Scope": "local",
  "Driver": "bridge",
  "EnableIPv6": false,
  "IPAM": {
    "Driver": "default",
    "Config": [{"Subnet": "172.18.0.0/16", "Gateway": "172.18.0.1"}]
  },
  "Containers": {
    "<container-id>": {
      "Name": "postgres",
      "IPv4Address": "172.18.0.2/16"
    }
  },
  "Labels": {}
}
```

#### `POST /networks/{id}/disconnect`

```json
{"Container": "<container-id>", "Force": true}
```

#### `DELETE /networks/{id}`

Response: `204 No Content`

#### `POST /networks/prune`

Query parameter: `filters` (JSON, e.g., `{"label":["key"]}`)

Response: `{"NetworksDeleted": ["net1", "net2"]}`

### 4.6 Volume Endpoints

#### `POST /volumes/create`

```json
{
  "Name": "runner-cache-sha256abc",
  "Driver": "local",
  "Labels": {"com.gitlab.gitlab-runner.managed": "true"}
}
```

**Response:**
```json
{
  "Name": "runner-cache-sha256abc",
  "Driver": "local",
  "Mountpoint": "/var/lib/docker/volumes/runner-cache-sha256abc/_data",
  "Labels": {"com.gitlab.gitlab-runner.managed": "true"},
  "Scope": "local",
  "CreatedAt": "2026-02-15T12:00:00Z"
}
```

Sockerless: Creates cloud storage (see [Volume Emulation](#10-volume-emulation)) and records volume metadata.

#### `GET /volumes` — List Volumes

Response:
```json
{
  "Volumes": [ ... ],
  "Warnings": []
}
```

#### `GET /volumes/{name}` — Inspect Volume

Returns volume metadata (same shape as create response).

#### `DELETE /volumes/{name}`

Query parameter: `force` (bool)

Response: `204 No Content`

Sockerless: Remove cloud storage. If force=true, remove even if in use.

---

## 5. Protocol Requirements

### 5.1 API Versioning

All requests may include a version prefix: `/v1.44/containers/json`. Sockerless accepts `v1.44` and treats unversioned requests as v1.44. Requests for versions > 1.44 return `400 Bad Request` with `{"message":"client version 1.XX is too new. Maximum supported API version is 1.44"}`.

### 5.2 Multiplexed Stream Protocol

When `Tty: false` (which is always the case for CI runners), attach and exec use Docker's multiplexed stream format:

```
+--------+--------+--------+--------+--------+--------+--------+--------+
| STREAM | 0x00   | 0x00   | 0x00   | SIZE1  | SIZE2  | SIZE3  | SIZE4  |
+--------+--------+--------+--------+--------+--------+--------+--------+
```

- **Byte 0 (STREAM):** `0` = stdin, `1` = stdout, `2` = stderr
- **Bytes 1-3:** Padding (zeros)
- **Bytes 4-7:** Payload size as big-endian uint32
- **Following bytes:** Payload (exactly `SIZE` bytes)

GitLab Runner uses `stdcopy.StdCopy` from the Docker Go SDK to demultiplex this. Any incorrect framing breaks CI job output.

### 5.3 Connection Hijacking

The `attach` and `exec/start` endpoints use HTTP connection hijacking:

1. Client sends HTTP POST request
2. Server responds with `101 Switching Protocols` (or `200 OK` with `Connection: Upgrade`)
3. The TCP connection is now a raw bidirectional byte stream
4. Server writes multiplexed frames for stdout/stderr
5. Client writes raw bytes for stdin
6. Connection closes when the command/container exits

This is implemented differently from WebSocket — it uses Docker's custom protocol. The `Content-Type` response header is `application/vnd.docker.raw-stream` (or `application/vnd.docker.multiplexed-stream`).

### 5.4 Error Responses

All errors follow Docker's format:
```json
{"message": "No such container: abc123"}
```

Standard HTTP status codes:
- `200` / `201` / `204`: Success
- `304`: Not modified (container already started/stopped)
- `400`: Bad request (invalid parameters)
- `404`: Not found (no such container/network/volume/image)
- `409`: Conflict (container already running, name in use)
- `500`: Server error

---

## 6. System Architecture

### 6.1 Component Overview

Sockerless is composed of **three independent component types** — frontend, backend, and agent — each compiled as a separate binary with its own Go module and dependencies.

```
Any Docker-compatible client                              Cloud Provider API
(Docker CLI, Podman, SDKs,                                      │
 Compose, Testcontainers, etc.)                                 │
    │                                                           │
    │ Docker REST API v1.44 (Unix socket / TCP)                 │
    ▼                                                           │
┌────────────────────────────┐  Internal HTTP/JSON  ┌──────────────────────┐
│  Frontend                  │ ◄──────────────────► │  Backend             │
│  (separate binary)         │  + WebSocket (stream) │  (separate binary)   │
│                            │  Unix socket / TCP   │                      │
│  • Stateless               │                      │  • Stateful          │
│  • OpenAPI-generated types │                      │  • Cloud SDK types   │
│  • Mux stream protocol     │                      │  • Persistent state  │
│  • Connection hijacking    │                      │  • Capability report │
│  • Bridges client ↔ agent  │                      │  • Launches tasks    │
└──────────┬─────────────────┘                      └──────────────────────┘
           │
           │ WebSocket (frontend connects TO agent, on demand)
           │ Triggered by exec / attach / logs operations
           │ Frontend must have network access to cloud containers
           ▼
┌──────────────────────────┐
│  Agent                   │  (inside cloud container, PID 1 or sidecar)
│  (separate binary)       │
│                          │
│  • WebSocket SERVER      │
│  • Listens on port 9111  │
│  • Exec: fork+exec       │
│  • Attach: pipe stdio    │
│  • Health check runner   │
│  • Standalone, no shared │
│    imports                │
└──────────────────────────┘
```

### 6.2 Project Structure — Go Modules and Binaries

Each component is a separate Go module with its own `go.mod`, enforcing dependency isolation at the build level. No component imports another component's code.

```
sockerless/
├── spec/                              # Specifications and research docs
│   ├── SOCKERLESS_SPEC.md
│   └── DOCKER_REST_API.md
│
├── api/                               # Module: shared internal API types
│   └── go.mod                         #   Zero external dependencies
│
├── frontends/
│   └── docker/                        # Module: Docker REST API frontend
│       └── go.mod                     #   Deps: api/, OpenAPI-generated types
│
├── backends/
│   ├── core/                          # Module: shared backend library
│   │   └── go.mod                     #   Deps: api/, sandbox/, gorilla/websocket
│   ├── memory/                        # Module: in-memory backend
│   │   └── go.mod                     #   Deps: api/, core/, sandbox/
│   ├── docker/                        # Module: real Docker daemon backend
│   │   └── go.mod                     #   Deps: api/, Docker client SDK
│   ├── ecs/                           # Module: AWS ECS Fargate backend
│   │   └── go.mod                     #   Deps: api/, AWS SDK v2
│   ├── lambda/                        # Module: AWS Lambda backend
│   │   └── go.mod                     #   Deps: api/, AWS SDK v2
│   ├── cloudrun/                      # Module: Google Cloud Run backend
│   │   └── go.mod                     #   Deps: api/, GCP SDK
│   ├── cloudrun-functions/            # Module: Google Cloud Run Functions
│   │   └── go.mod                     #   Deps: api/, GCP SDK
│   ├── aca/                           # Module: Azure Container Apps backend
│   │   └── go.mod                     #   Deps: api/, Azure SDK
│   └── azure-functions/               # Module: Azure Functions backend
│       └── go.mod                     #   Deps: api/, Azure SDK
│
├── agent/                             # Module: sockerless-agent binary
│   └── go.mod                         #   Deps: gorilla/websocket only
│                                      #   NO api/ import — fully standalone
│
├── sandbox/                           # Module: WASM sandbox (busybox + shell)
│   └── go.mod                         #   Deps: wazero, mvdan.cc/sh
│
├── simulators/
│   ├── aws/                           # Module: AWS API simulator (Lambda, ECS, ECR, CloudWatch)
│   ├── gcp/                           # Module: GCP API simulator (Cloud Run, GCF, Artifact Registry, Logging)
│   └── azure/                         # Module: Azure API simulator (ACA, AZF, ACR, Monitor)
│
├── bleephub/                          # Module: GitHub Actions server (Azure DevOps-derived API)
│   └── go.mod                         #   Implements the internal API that actions/runner expects
│
├── tests/                             # Module: black-box API tests
│   └── go.mod                         #   Deps: Docker client SDK (as test client)
│
├── terraform/
│   └── modules/                       # Terraform modules for cloud infrastructure
│       ├── ecs/
│       ├── lambda/
│       ├── cloudrun/
│       ├── gcf/
│       ├── aca/
│       └── azf/
│
└── Makefile
```

**18+ Go modules, 13+ binaries:**

| # | Module | Binary Name | External Dependencies |
|---|--------|------------|----------------------|
| 1 | `api/` | *(library — no binary)* | None (stdlib only) |
| 2 | `frontends/docker/` | `sockerless-docker-frontend` | `api/`, OpenAPI-generated types, `gorilla/websocket` |
| 3 | `backends/core/` | *(library — no binary)* | `api/`, `sandbox/`, `agent/`, `gorilla/websocket` |
| 4 | `backends/memory/` | `sockerless-backend-memory` | `api/`, `core/`, `sandbox/` |
| 5 | `backends/docker/` | `sockerless-backend-docker` | `api/`, `github.com/docker/docker` client SDK |
| 6 | `backends/ecs/` | `sockerless-backend-ecs` | `api/`, `core/`, AWS SDK v2 |
| 7 | `backends/lambda/` | `sockerless-backend-lambda` | `api/`, `core/`, AWS SDK v2 |
| 8 | `backends/cloudrun/` | `sockerless-backend-cloudrun` | `api/`, `core/`, GCP SDK |
| 9 | `backends/cloudrun-functions/` | `sockerless-backend-gcf` | `api/`, `core/`, GCP SDK |
| 10 | `backends/aca/` | `sockerless-backend-aca` | `api/`, `core/`, Azure SDK |
| 11 | `backends/azure-functions/` | `sockerless-backend-azf` | `api/`, `core/`, Azure SDK |
| 12 | `agent/` | `sockerless-agent` | `gorilla/websocket` only |
| 13 | `sandbox/` | *(library — no binary)* | `wazero`, `mvdan.cc/sh`, `go-busybox` |
| 14 | `simulators/aws/` | `sim-aws` | AWS SDK types (for request/response formats) |
| 15 | `simulators/gcp/` | `sim-gcp` | GCP protobuf types |
| 16 | `simulators/azure/` | `sim-azure` | Azure SDK types |
| 17 | `bleephub/` | `bleephub` | JWT, HTTP server |
| 18 | `tests/` | *(test binary — `go test`)* | Docker client SDK |

### 6.3 Frontend

The frontend is a **stateless** HTTP server that:

1. Listens on a Unix socket (or TCP) and speaks Docker Engine REST API v1.44
2. Accepts connections from **any** Docker-compatible client (Docker CLI, Podman remote, Docker SDKs in Python/Go/Node/Rust/.NET, Docker Compose, Testcontainers, etc.)
3. Translates Docker REST API requests into internal API calls to the backend
4. Handles the Docker-specific protocol features: multiplexed streams, connection hijacking, `X-Registry-Auth` headers
5. On exec/attach requests, opens a WebSocket connection **to the agent** inside the cloud container and bridges the Docker client's hijacked connection to the agent's WebSocket

**Types:** Auto-generated from Docker's published OpenAPI/Swagger specification. No dependency on Docker's Go SDK. This ensures spec-compliance without coupling to any particular client implementation.

**State:** None persistent. Only ephemeral in-memory data: active WebSocket connections to agents, in-flight exec sessions.

**Client compatibility:** Because the frontend implements the Docker REST API from the OpenAPI spec (not from Docker's Go SDK), it is a standards-compliant server that works with any client speaking the Docker HTTP API — Docker CLI, Podman, any language SDK, or custom HTTP clients.

### 6.4 Backend

Each backend is a **stateful** daemon that:

1. Listens on a Unix socket (or TCP) for internal API calls from the frontend
2. Owns all state (containers, networks, volumes, images) in thread-safe in-memory maps (`sync.Map` and `StateStore[T]`). State is ephemeral — lost on restart. No persistent database layer (SQLite was considered but not implemented).
3. Translates internal API calls into cloud provider operations using the provider's native SDK and types
4. Reports its capabilities so the frontend can return `501 Not Implemented` for unsupported operations

**State ownership:** The backend is the single source of truth for all resource state:

| Object | State Fields |
|--------|-------------|
| Container | ID, name, config, status, cloud task ID, agent address, IP, ports, timestamps, exit code, labels |
| Network | ID, name, labels, IPAM subnet, connected containers |
| Volume | ID, name, labels, cloud storage ID, mount references |
| Image | Reference, digest, config (env/cmd/entrypoint), pulled timestamp |
| Exec | ID, container ID, cmd, env, workdir, running, exit code |

**Capability reporting:** Each backend declares what it supports:

```json
{
  "exec": true,
  "attach": true,
  "logs_follow": true,
  "volumes": true,
  "networks": true,
  "max_timeout_seconds": 0,
  "health_checks": true
}
```

FaaS backends (Lambda, Cloud Functions, Azure Functions) report `exec: false`, `attach: false`, `max_timeout_seconds: 900` (or similar). The frontend uses this to return `501` for unsupported operations.

**Docker backend special case:** The Docker backend (`backends/docker/`) talks to a real Docker daemon. It does NOT need the agent — it uses Docker's native exec and attach APIs directly. This makes it the ideal reference backend for testing: run the same test suite against the memory backend, the Docker backend, and cloud backends to verify behavior consistency.

### 6.5 Agent

The agent is a **standalone** binary that runs inside cloud containers. It is completely independent — no shared imports with the frontend, backend, or `api/` module.

| Property | Value |
|----------|-------|
| Role | WebSocket server/client inside the cloud container |
| Port | Listens on `:9111` (forward mode) or dials back to backend (reverse mode) |
| Auth | Token-based (`SOCKERLESS_AGENT_TOKEN`) |
| Lifetime | Runs for the container's lifetime as PID 1 (wrapping the user process) or as a sidecar |
| Binary size | Static binary, ~5-10 MB |

**Two agent modes:**

| Mode | Direction | Used By |
|---|---|---|
| **Forward agent** | Backend connects TO agent at `{container_IP}:9111` | ECS, Cloud Run, ACA |
| **Reverse agent** | Agent dials back to backend via `SOCKERLESS_CALLBACK_URL` | Lambda, GCF, Azure Functions |

Forward agent is used when the backend can reach the container's network (VPC, VNet). Reverse agent is used for FaaS backends where the function cannot accept inbound connections.

**When the agent is NOT needed:**
- Docker backend: uses Docker's native exec/attach
- Memory backend: direct WASM sandbox execution
- Synthetic mode: no real exec (returns mock output)

**When the agent IS needed:**
- All cloud backends (ECS, Cloud Run, ACA, Lambda, GCF, Azure Functions)

### 6.6 Driver Architecture (Phase 30)

The backend uses a **driver chain** pattern (chain of responsibility) for dispatching exec, filesystem, streaming, and process lifecycle operations. Each driver interface has a `Fallback` field that forms a chain:

```
Agent Driver → WASM Process Driver → Synthetic Driver
```

**4 Driver Interfaces:**

| Interface | Methods | Purpose |
|---|---|---|
| `ExecDriver` | `Exec(containerID, execConfig) → (exitCode, error)` | Execute commands inside containers |
| `FilesystemDriver` | `PutArchive`, `HeadArchive`, `GetArchive` | Container filesystem access (`docker cp`) |
| `StreamDriver` | `Logs`, `Attach` | Log streaming and container attach |
| `ProcessLifecycleDriver` | `Start`, `Stop`, `WaitCh` | Container process lifecycle |

**`DriverSet`** on `BaseServer` holds the active driver chain. `InitDrivers()` constructs the chain based on available capabilities:

- **Cloud backends**: Agent → Synthetic (no WASM)
- **Memory backend**: Agent → WASM Process → Synthetic (calls `InitDrivers()` again after setting `ProcessFactory`)
- **Docker backend**: Does not use drivers (direct Docker SDK passthrough)

The chain pattern allows graceful fallback — if a container doesn't have an agent connected, the WASM driver handles exec; if no WASM process exists, synthetic mode returns mock output.

### 6.7 Dependency Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        Import Rules                             │
│                                                                 │
│  frontend ──imports──► api/                                     │
│  backend  ──imports──► api/                                     │
│  agent    ──imports──► nothing shared (fully standalone)         │
│  tests    ──imports──► Docker SDK (as test client, not api/)    │
│                                                                 │
│  frontend NEVER imports backend or agent                        │
│  backend  NEVER imports frontend or agent                       │
│  agent    NEVER imports frontend, backend, or api/              │
│  No circular dependencies possible                              │
└─────────────────────────────────────────────────────────────────┘
```

---

## 7. Internal API — Frontend ↔ Backend

### 7.1 Overview

The internal API mirrors the Docker REST API structure but without Docker-specific protocol quirks (no multiplexed streams, no connection hijacking, no `X-Registry-Auth` headers). It is defined by the `api/` module using its own types (zero external dependencies).

The frontend translates: `Docker types (OpenAPI-generated) ↔ api/ types ↔ internal HTTP/JSON`

The backend translates: `api/ types ↔ cloud SDK types`

### 7.2 Transport

| Operation Type | Transport | Examples |
|---|---|---|
| CRUD (request-response) | HTTP/JSON | create/start/stop/inspect/list/remove containers, networks, volumes, images |
| Streaming (long-lived) | WebSocket | logs with `follow=true`, container wait |

Both share the same Unix socket / TCP address. WebSocket upgrades happen on specific endpoints.

### 7.3 Internal API Endpoints

All routes are prefixed with `/internal/v1/`.

#### System

| Method | Path | Maps to Docker API |
|--------|------|--------------------|
| `GET` | `/internal/v1/capabilities` | *(no Docker equivalent)* |
| `GET` | `/internal/v1/info` | `GET /info` |
| `GET` | `/internal/v1/version` | `GET /version` |

#### Containers

| Method | Path | Maps to Docker API |
|--------|------|--------------------|
| `POST` | `/internal/v1/containers` | `POST /containers/create` |
| `GET` | `/internal/v1/containers` | `GET /containers/json` |
| `GET` | `/internal/v1/containers/{id}` | `GET /containers/{id}/json` |
| `POST` | `/internal/v1/containers/{id}/start` | `POST /containers/{id}/start` |
| `POST` | `/internal/v1/containers/{id}/stop` | `POST /containers/{id}/stop` |
| `POST` | `/internal/v1/containers/{id}/kill` | `POST /containers/{id}/kill` |
| `DELETE` | `/internal/v1/containers/{id}` | `DELETE /containers/{id}` |
| `GET` | `/internal/v1/containers/{id}/logs` | `GET /containers/{id}/logs` |
| `WS` | `/internal/v1/containers/{id}/logs/stream` | `GET /containers/{id}/logs?follow=true` |
| `WS` | `/internal/v1/containers/{id}/wait` | `POST /containers/{id}/wait` |

#### Exec

| Method | Path | Maps to Docker API |
|--------|------|--------------------|
| `POST` | `/internal/v1/containers/{id}/exec` | `POST /containers/{id}/exec` |
| `GET` | `/internal/v1/exec/{id}` | `GET /exec/{id}/json` |

Note: `POST /exec/{id}/start` is NOT proxied through the backend. The frontend handles exec streaming directly by connecting to the agent. The backend only stores exec metadata.

#### Images

| Method | Path | Maps to Docker API |
|--------|------|--------------------|
| `POST` | `/internal/v1/images/pull` | `POST /images/create` |
| `GET` | `/internal/v1/images/{name}` | `GET /images/{name}/json` |
| `POST` | `/internal/v1/images/load` | `POST /images/load` |
| `POST` | `/internal/v1/images/{name}/tag` | `POST /images/{name}/tag` |
| `POST` | `/internal/v1/auth` | `POST /auth` |

#### Networks

| Method | Path | Maps to Docker API |
|--------|------|--------------------|
| `POST` | `/internal/v1/networks` | `POST /networks/create` |
| `GET` | `/internal/v1/networks` | `GET /networks` |
| `GET` | `/internal/v1/networks/{id}` | `GET /networks/{id}` |
| `POST` | `/internal/v1/networks/{id}/disconnect` | `POST /networks/{id}/disconnect` |
| `DELETE` | `/internal/v1/networks/{id}` | `DELETE /networks/{id}` |
| `POST` | `/internal/v1/networks/prune` | `POST /networks/prune` |

#### Volumes

| Method | Path | Maps to Docker API |
|--------|------|--------------------|
| `POST` | `/internal/v1/volumes` | `POST /volumes/create` |
| `GET` | `/internal/v1/volumes` | `GET /volumes` |
| `GET` | `/internal/v1/volumes/{name}` | `GET /volumes/{name}` |
| `DELETE` | `/internal/v1/volumes/{name}` | `DELETE /volumes/{name}` |

### 7.4 Capability Reporting

`GET /internal/v1/capabilities` returns the backend's supported features:

```json
{
  "backend": "ecs",
  "version": "0.1.0",
  "capabilities": {
    "exec": true,
    "attach": true,
    "logs": true,
    "logs_follow": true,
    "volumes": true,
    "networks": true,
    "health_checks": true,
    "image_pull": true,
    "image_load": false,
    "max_timeout_seconds": 0,
    "agent_required": true
  }
}
```

The frontend calls this once at startup (and caches it). When a Docker API request maps to an unsupported capability, the frontend returns `501 Not Implemented` with `{"message":"This backend (lambda) does not support exec"}`.

### 7.5 Agent Address

When the backend starts a container, it provisions the agent and records the agent's address. The container inspect response includes an `agent_address` field:

```json
{
  "id": "abc123...",
  "agent_address": "10.0.1.47:9111",
  "status": "running",
  "...": "..."
}
```

The frontend reads this address when it needs to connect to the agent for exec/attach operations.

---

## 8. Agent Protocol

### 8.1 Agent as WebSocket Server

The agent runs inside the cloud container and listens for WebSocket connections from the frontend. The frontend connects **on demand** — only when a user triggers an exec, attach, or streaming logs operation.

```
User: docker exec container-1 sh
    │
    ▼
Frontend                                        Agent (in cloud container)
    │                                                │
    ├── GET /internal/v1/containers/{id} ──→ Backend │
    │   (gets agent_address: 10.0.1.47:9111)         │
    │                                                │
    ├── WebSocket CONNECT ──────────────────────────→ │ ws://10.0.1.47:9111/
    │   Authorization: Bearer <token>                │
    │                                                │
    ├── {"type":"exec","cmd":["sh"],...} ───────────→ │
    │                                                │ (agent fork+exec "sh")
    │                                                │
    │ ←── {"type":"stdout","data":"$ "} ──────────── │
    │ ──── {"type":"stdin","data":"ls\n"} ──────────→ │
    │ ←── {"type":"stdout","data":"file1 file2\n"} ─ │
    │                                                │
    │ ←── {"type":"exit","code":0} ────────────────── │
    │                                                │
    ├── WebSocket CLOSE ────────────────────────────→ │
```

### 8.2 Authentication

The backend generates a one-time token when launching the cloud task. This token is:
1. Injected into the container as env var `SOCKERLESS_AGENT_TOKEN`
2. Returned to the frontend as part of the container's inspect data

When the frontend connects to the agent, it presents the token in the `Authorization: Bearer <token>` header. The agent validates the token and rejects unauthorized connections.

### 8.3 WebSocket Message Protocol

All messages are JSON. Each message has a `type` field.

**Frontend → Agent:**

| Type | Fields | Purpose |
|------|--------|---------|
| `exec` | `id`, `cmd`, `env`, `workdir`, `tty` | Fork a new process |
| `attach` | `id` | Attach to the main process (PID 1's child) |
| `stdin` | `id`, `data` (base64) | Pipe bytes to process stdin |
| `signal` | `id`, `signal` | Send signal (SIGTERM, SIGKILL) to process |
| `close_stdin` | `id` | Close process stdin (EOF) |
| `resize` | `id`, `width`, `height` | Resize TTY (future) |

**Agent → Frontend:**

| Type | Fields | Purpose |
|------|--------|---------|
| `stdout` | `id`, `data` (base64) | Process stdout bytes |
| `stderr` | `id`, `data` (base64) | Process stderr bytes |
| `exit` | `id`, `code` | Process exited with code |
| `error` | `id`, `message` | Error (process not found, etc.) |
| `health` | `status`, `log` | Health check result |

The `id` field identifies the exec/attach session (supports multiple concurrent sessions over the same WebSocket).

### 8.4 Attach Flow (GitLab Runner Pattern)

GitLab Runner attaches BEFORE starting the container. The frontend buffers the attach until the container starts and the agent becomes reachable:

```
GitLab Runner                Frontend                    Backend              Agent
    │                            │                          │                   │
    ├── POST /containers/create → │ ── POST /internal/... → │                   │
    │ ←── {Id: "abc"} ────────── │ ←── {id: "abc"} ──────── │                   │
    │                            │                          │                   │
    ├── POST /containers/abc/    │                          │                   │
    │   attach ─────────────────→ │ (buffer hijacked conn)  │                   │
    │   (hijacked connection)    │                          │                   │
    │                            │                          │                   │
    ├── POST /containers/abc/    │                          │                   │
    │   start ──────────────────→ │ ── POST .../start ────→ │ ── launch task ─→ │
    │                            │                          │                   │
    │                            │ ←── {agent_address} ──── │                   │
    │                            │                          │                   │
    │                            │ ── ws://agent:9111 ────────────────────────→ │
    │                            │ ── {"type":"attach"} ──────────────────────→ │
    │                            │                          │                   │
    │ ←── mux stdout ─────────── │ ←── {"type":"stdout"} ────────────────────── │
    │ ──── stdin ───────────────→ │ ──── {"type":"stdin"} ────────────────────→ │
    │                            │                          │                   │
    │ (connection closes on exit)│                          │                   │
```

The frontend is responsible for:
- Buffering the attach request until the agent is reachable
- Translating between Docker's multiplexed stream protocol (8-byte header framing) and the agent's WebSocket JSON messages
- Bridging the hijacked HTTP connection to the WebSocket connection

### 8.5 Exec Flow (GitHub Actions Runner Pattern)

```
GitHub Runner               Frontend                    Backend              Agent
    │                            │                          │                   │
    │ (container already running with "tail -f /dev/null")  │                   │
    │                            │                          │                   │
    ├── POST /containers/abc/    │                          │                   │
    │   exec ──────────────────→ │ ── POST .../exec ──────→ │ (store exec meta) │
    │ ←── {Id: "exec-1"} ────── │ ←── {id: "exec-1"} ───── │                   │
    │                            │                          │                   │
    ├── POST /exec/exec-1/start → │                          │                   │
    │   (hijacked connection)    │                          │                   │
    │                            │ ── ws://agent:9111 ────────────────────────→ │
    │                            │ ── {"type":"exec",       │                   │
    │                            │     "cmd":["sh","-c",    │                   │
    │                            │     "script"]} ────────────────────────────→ │
    │                            │                          │                   │
    │ ←── mux stdout ─────────── │ ←── {"type":"stdout"} ────────────────────── │
    │ ──── stdin ───────────────→ │ ──── {"type":"stdin"} ────────────────────→ │
    │ ←── mux stderr ─────────── │ ←── {"type":"stderr"} ────────────────────── │
    │                            │                          │                   │
    │ ←── exit code ────────────  │ ←── {"type":"exit","code":0} ────────────── │
```

### 8.6 Agent Injection

The backend is responsible for injecting the agent into cloud containers:

| Method | Description | Best For |
|---|---|---|
| **Volume mount** | Mount agent binary from cloud storage (EFS, GCS, Azure Files) and prepend to entrypoint | ECS, ACA |
| **Init container** | Run init container that copies agent binary to shared volume | ECS, ACA |
| **Image layer** | Backend builds a wrapper image with agent included | Cloud Run (no volume mount at startup) |
| **Not needed** | Backend has native exec (Docker backend) | Docker, Memory |

The backend modifies the container's entrypoint to: `["/sockerless-agent", "--", <original-entrypoint>]`

The agent starts the original command as a child process and serves WebSocket connections for exec/attach.

---

## 9. Cloud Backend Mapping

### 9.1 Container-Based Backends

These support long-running containers with logging. Best suited for CI runner workloads.

#### AWS ECS (Fargate)

| Docker Concept | ECS Mapping |
|---|---|
| Container create | `RegisterTaskDefinition` (stores config + registers task def) |
| Container start | `RunTask` (Fargate launch type); polls until RUNNING; extracts agent address from ENI IP |
| Container stop | `StopTask` |
| Container kill | `StopTask` (ECS has no SIGKILL; tasks get 30s SIGTERM then forced stop) |
| Container remove | `StopTask` (if running) + `DeregisterTaskDefinition` |
| Container inspect | Local state (not `DescribeTasks` — state stored at create/start time) |
| Container list | Local state with label/status filtering |
| Container logs | CloudWatch Logs (`GetLogEvents` / `FilterLogEvents`) |
| Exec / Attach | Via forward agent (agent address from task's ENI private IP on port 9111) |
| Image pull | Records ref in state; ECS pulls from ECR / Docker Hub at `RunTask` time |
| Network | In-memory virtual IPs; tasks share VPC subnets from infrastructure |
| Volume | In-memory metadata; EFS mounts configured at infrastructure level |
| Health check | Agent-executed health checks reported to backend |

**Agent networking:** Fargate tasks get private IPs in the configured VPC. The frontend must be able to reach these IPs (same VPC, VPC peering, or transit gateway). Agent listens on port 9111 — security group must allow inbound from frontend.

**Startup latency:** 10-45 seconds (image pull + Fargate capacity allocation).

#### Google Cloud Run (Jobs)

| Docker Concept | Cloud Run Mapping |
|---|---|
| Container create | Registers in local store (deferred job creation) |
| Container start | `CreateJob` + `RunJob` — job is created at start time to support clean restarts |
| Container stop/kill | `CancelExecution` |
| Container remove | `DeleteJob` |
| Container inspect/list | Local state |
| Container logs | Cloud Logging via Log Admin API |
| Exec / Attach | Via forward agent (polls for RUNNING, extracts agent address) |
| Image pull | Records ref; Cloud Run pulls from Artifact Registry/GCR/Docker Hub at job creation |
| Network | In-memory virtual IPs; jobs share VPC from infrastructure |
| Volume | In-memory metadata |

Also supports **reverse agent** via `SOCKERLESS_CALLBACK_URL`.

**Startup latency:** 5-30 seconds.

#### Azure Container Apps (Jobs)

| Docker Concept | ACA Mapping |
|---|---|
| Container create | Registers in local store |
| Container start | `BeginCreateOrUpdate` (Job) + `BeginStart` (Execution) |
| Container stop/kill | `BeginStopExecution` |
| Container remove | `BeginDelete` (Job) |
| Container inspect/list | Local state |
| Container logs | Azure Monitor Log Analytics (`QueryWorkspace`) |
| Exec / Attach | Via forward agent (polls for RUNNING, extracts agent address from VNet IP) |
| Image pull | Records ref; ACA pulls from ACR/Docker Hub at job creation |
| Network | In-memory virtual IPs; jobs share ACA Environment VNet from infrastructure |
| Volume | In-memory metadata |

Also supports **reverse agent** via `SOCKERLESS_CALLBACK_URL`.

**Startup latency:** 10-60 seconds.

### 9.2 FaaS Backends

FaaS backends use **reverse agent** exclusively: the agent inside the function dials back to the backend via `SOCKERLESS_CALLBACK_URL` (WebSocket). This enables exec and attach for the duration of the function invocation. Helper and cache containers auto-stop after 500ms.

FaaS backends route container variants differently:
- `services` → `services-wasm` (WASM sandbox)
- `custom-image` → `custom-image-wasm` (WASM sandbox)
- `container-action` → `container-action-faas` (reverse agent lifecycle)

#### AWS Lambda

| Docker Concept | Lambda Mapping |
|---|---|
| Container create | `CreateFunction` (container image from ECR) |
| Container start | `Invoke` (async); agent inside function calls back |
| Container stop | Disconnects reverse agent (function runs to completion) |
| Container kill | Disconnects reverse agent |
| Container remove | `DeleteFunction` |
| Container logs | CloudWatch Logs `GetLogEvents` |
| Exec / Attach | Via reverse agent (agent dials back to backend) |
| Image pull | Records ref; Lambda pulls from ECR at function create |
| Max timeout | 15 minutes |

**Capabilities:** `exec: true` (via reverse agent), `attach: true` (via reverse agent), `logs: true, logs_follow: false, max_timeout_seconds: 900`

#### Google Cloud Run Functions

| Docker Concept | Cloud Run Functions Mapping |
|---|---|
| Container create | `CreateFunction` (2nd gen, Docker runtime) — synchronous, 1-3 min |
| Container start | HTTP POST invoke (async); agent inside function calls back |
| Container stop | No-op (runs to completion) |
| Container kill | Disconnects reverse agent |
| Container remove | `DeleteFunction` |
| Container logs | Cloud Logging via Log Admin API |
| Exec / Attach | Via reverse agent |
| Image pull | Records ref; Cloud Functions pulls from Artifact Registry |
| Max timeout | 60 minutes (2nd gen) |

**Capabilities:** `exec: true` (via reverse agent), `attach: true` (via reverse agent), `logs: true, logs_follow: false, max_timeout_seconds: 3600`

#### Azure Functions

| Docker Concept | Azure Functions Mapping |
|---|---|
| Container create | Create App Service Plan + Function App (custom container) |
| Container start | Start Function App; agent inside function calls back |
| Container stop | Stop Function App |
| Container kill | Disconnects reverse agent |
| Container remove | Delete Function App + App Service Plan |
| Container logs | Azure Monitor Log Analytics query |
| Exec / Attach | Via reverse agent |
| Image pull | Records ref; Functions pull from ACR at deploy |
| Max timeout | Function timeout configurable (default 600s) |

**Capabilities:** `exec: true` (via reverse agent), `attach: true` (via reverse agent), `logs: true, logs_follow: false, max_timeout_seconds: 600`

> **Note:** While FaaS backends now support exec/attach via reverse agent, they are still **not compatible with CI runners** due to timeout limits, lack of persistent shared volumes, and the reverse agent lifecycle model. They are useful for short-lived, fire-and-forget container workloads.

### 9.3 Docker Backend (Reference / Testing)

| Docker Concept | Docker Mapping |
|---|---|
| Container create + start | `POST /containers/create` + `POST /containers/{id}/start` on real Docker daemon |
| Exec / Attach | Docker's native exec and attach (no agent needed) |
| Everything else | 1:1 passthrough to Docker daemon |

**Capabilities:** `exec: true, attach: true, logs: true, logs_follow: true, volumes: true, networks: true, health_checks: true, agent_required: false`

The Docker backend is the **reference implementation** for testing. Running the test suite against the Docker backend validates that sockerless produces responses identical to a real Docker daemon.

### 9.4 Memory Backend

In-memory backend with WASM sandbox for real command execution. Uses wazero
v1.11.0 (pure Go WASM runtime) + go-busybox (41 BusyBox applets compiled to
WASM via TinyGo) + mvdan.cc/sh v3.12.0 (shell interpreter). Per-container
temp directories provide isolated virtual filesystems with Alpine-like rootfs.

**Execution model:** mvdan.cc/sh parses shell syntax natively in Go and dispatches
individual commands to WASM busybox applets via wazero. WASI Preview 1 has no
fork/exec, so the Go host orchestrates all process spawning. The `sandbox/`
module is standalone — the memory backend enables it via `ProcessFactory`.

**Supported features:**
- Real shell execution: pipes, `&&`/`||`, redirects, variable expansion, subshells
- 21+ Go-implemented builtins (pwd, cd, mkdir, cp, mv, rm, cat, echo, env, etc.)
- Volumes via symlinks in rootDir + DirMounts for WASM
- `docker cp` (archive PUT/HEAD/GET) with pre-start staging directories
- Interactive shell via attach
- PATH-aware command resolution
- Exec with working directory and environment variables
- `tail -f /dev/null` keepalive (blocks on context, no inotify in WASI)

**Limitations:** No real network access from WASM sandbox. No fork/exec (WASI P1).
GitLab Runner E2E requires `SOCKERLESS_SYNTHETIC=1` because gitlab-runner uses
helper binaries (`gitlab-runner-helper`, `gitlab-runner-build`) that can't run in
WASM. GitHub `act` runner works without synthetic mode.

**Capabilities:** All true (real WASM execution). `agent_required: false`

### 9.5 Backend Comparison Matrix

| Capability | Docker | Memory | ECS | Cloud Run | ACA | Lambda | CR Func | Az Func |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| Long-running | Yes | Yes | Yes | Yes (24h) | Yes | No (15m) | No (60m) | No (10m) |
| Exec | Yes | Yes (WASM) | Yes* | Yes* | Yes* | Yes** | Yes** | Yes** |
| Attach | Yes | Yes (WASM) | Yes* | Yes* | Yes* | Yes** | Yes** | Yes** |
| Log stream | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| Log follow | Yes | Yes | Yes | Yes | Yes | No | No | No |
| Volumes | Yes | Yes (symlinks) | Yes | Partial | Yes | No | No | No |
| Networks | Yes | Yes (in-memory) | Yes | Yes | Yes | No | No | No |
| Health checks | Yes | Yes (exec) | Yes | Yes | Yes | No | No | No |
| Docker build | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| Archive (docker cp) | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| Agent mode | N/A | N/A | Forward | Forward | Forward | Reverse | Reverse | Reverse |
| Agent needed | No | No | Yes | Yes | Yes | Yes | Yes | Yes |
| Startup latency | <1s | <1s | 10-45s | 5-30s | 10-60s | 1-5s | 1-5s | 1-5s |

\* Via forward agent (backend connects to agent inside container)
\*\* Via reverse agent (agent inside function dials back to backend)

---

## 10. Volume Emulation

### 10.1 Overview

GitLab Runner creates volumes for:
- `/builds/<namespace>/<project>` — shared between helper and build containers (git clone, build artifacts)
- `/cache` — persistent cache across jobs
- Additional user-configured volumes

Both containers mount the same volume for data sharing. This is critical for the helper→build container handoff pattern.

### 10.2 Strategy

| Volume Type | Cloud Emulation | Used For |
|---|---|---|
| Named volumes (Docker `VolumeCreate`) | Cloud shared filesystem (EFS, GCS, Azure Files) | Caches |
| Bind mounts (from `HostConfig.Binds`) | Cloud shared filesystem mounted at specified path | Build data sharing |
| `VolumesFrom` (inherit from another container) | Same shared filesystem mount applied to multiple tasks | Helper↔Build sharing |
| Anonymous volumes | Ephemeral task storage | Temp data |

### 10.3 Implementation per Backend

> **Note:** In the current implementation, volume operations are handled **in-memory** by `backends/core/`. Cloud backends store volume metadata in local state and apply volume mounts when launching cloud tasks. The cloud-native storage provisioning described below is done via Terraform infrastructure modules (`terraform/modules/`), not dynamically by the backend at runtime.

#### AWS ECS + EFS
- Pre-provisioned EFS filesystem referenced in task definitions
- EFS access points configured at infrastructure level
- Multiple Fargate tasks mount the same access point → shared data

#### Google Cloud Run + GCS
- Pre-provisioned GCS bucket or Filestore instance
- Mounted via Cloud Storage FUSE in the container at infrastructure level

#### Azure Container Apps + Azure Files
- Pre-provisioned Azure Files share
- Mounted in container app job at infrastructure level

### 10.4 Volume Lifecycle

| Docker Operation | Sockerless Action |
|---|---|
| `POST /volumes/create` | Create cloud storage resource (or logical directory in shared filesystem) |
| Bind mount in `POST /containers/create` | Record mount mapping; apply when launching cloud task |
| `VolumesFrom` in `POST /containers/create` | Copy mount list from referenced container's config |
| `DELETE /volumes/{name}` | Delete cloud storage resource |

---

## 11. Network Emulation

### 11.1 Overview

CI runners create per-job networks so that:
- Service containers (postgres, redis, etc.) and the build container can communicate
- DNS resolution works for service aliases (e.g., `postgres` resolves to the postgres container's IP)
- Job isolation — containers from different jobs cannot communicate

### 11.2 Strategy

> **Note:** In the current implementation, network operations are handled **in-memory** by `backends/core/`. Networks are stored as local state with virtual IP assignments. Cloud backends do not dynamically create Security Groups, Cloud Map namespaces, or Cloud DNS zones at runtime. Cloud networking is configured at the infrastructure level via Terraform modules.

| Cloud Backend | Network Emulation |
|---|---|
| All backends | In-memory virtual IP assignment from 172.18.0.0/16 subnet |
| AWS ECS | Tasks share VPC subnets configured at infrastructure level |
| Google Cloud Run | Jobs share VPC configured at infrastructure level |
| Azure Container Apps | Jobs share ACA Environment VNet configured at infrastructure level |

### 11.3 IP Address Assignment

Sockerless assigns virtual IPs from a private subnet (e.g., `172.18.0.0/16`) to each container on a network. These IPs are returned in inspect responses. Actual cloud networking may use different IPs, but the sockerless-level IPs provide the correct inspect output for CI runners.

For DNS-based service discovery (which is what CI runners actually rely on), the container aliases (e.g., `postgres`, `redis`) are registered in the cloud's DNS service so they resolve to the actual cloud IPs.

### 11.4 Network Lifecycle

| Docker Operation | Sockerless Action |
|---|---|
| `POST /networks/create` | Create network in local state; allocate IPAM subnet |
| Container create with `NetworkMode`/`EndpointsConfig` | Assign virtual IP from subnet, store aliases |
| `POST /networks/{id}/disconnect` | Remove container from network state |
| `DELETE /networks/{id}` | Remove from local state |
| `POST /networks/prune` | Remove networks with no connected containers |

### 11.5 ExtraHosts Support

GitLab Runner uses `ExtraHosts` when `FF_NETWORK_PER_BUILD` is disabled (legacy mode). Sockerless passes these as environment entries or injects them into `/etc/hosts` via the exec agent.

---

## 12. Docker Compose Support

### 12.1 Scope

Docker Compose v2 communicates with the Docker daemon via the same REST API. It does NOT use any special endpoints. The compose operations map to our supported endpoints:

| Compose Command | Docker API Calls |
|---|---|
| `docker compose up` | Pull images → Create networks → Create volumes → Create containers → Start containers |
| `docker compose down` | Stop containers → Remove containers → Remove networks → (optionally remove volumes) |
| `docker compose ps` | List containers filtered by `com.docker.compose.project` label |
| `docker compose logs` | Get container logs for each service |

### 12.2 Required Label Support

Docker Compose sets these labels on all objects. Sockerless must store and filter by them:

| Label | Purpose |
|---|---|
| `com.docker.compose.project` | Project name (directory name or `-p` flag) |
| `com.docker.compose.service` | Service name from compose file |
| `com.docker.compose.container-number` | Instance number (for `scale`) |
| `com.docker.compose.oneoff` | "True" for `docker compose run` containers |
| `com.docker.compose.project.config_files` | Compose file paths |
| `com.docker.compose.project.working_dir` | Project working directory |

### 12.3 Service Dependencies and Health Checks

`docker compose up` with `depends_on` and health checks uses:
1. `POST /containers/create` with `Healthcheck` config from compose file
2. `GET /containers/{id}/json` polling `.State.Health.Status` until `"healthy"`
3. Sequential container startup based on dependency graph

Sockerless must support the health check polling pattern. The sockerless agent can run health check commands inside the container and report status.

---

## 13. CI Runner Compatibility

This section provides a deep analysis of how GitLab Runner docker-executor and GitHub Actions Runner interact with the Docker API, identifies every gap that arises when translating these interactions to cloud backends, and proposes concrete solutions. The analysis is based on source code examination of both runners.

### 13.1 GitLab Runner Docker-Executor

#### 13.1.1 Docker API Surface Used

GitLab Runner's Docker client interface (`helpers/docker/client.go`) defines exactly **32 methods**. The complete list of Docker API calls the runner can make:

| Category | API Calls |
|---|---|
| Version | `ClientVersion()`, `ServerVersion()` |
| Images | `ImageInspectWithRaw`, `ImagePullBlocking`, `ImageImportBlocking`, `ImageLoad`, `ImageTag` |
| Containers | `ContainerList`, `ContainerCreate`, `ContainerStart`, `ContainerKill`, `ContainerStop`, `ContainerInspect`, `ContainerAttach`, `ContainerRemove`, `ContainerWait`, `ContainerLogs`, `ContainerExecCreate`, `ContainerExecAttach` |
| Networks | `NetworkCreate`, `NetworkRemove`, `NetworkDisconnect`, `NetworkList`, `NetworkInspect` |
| Volumes | `VolumeCreate`, `VolumeRemove`, `VolumeInspect`, `VolumeList` |
| System | `Info()` |

**Not used:** `ContainerRestart`, `ContainerPause/Unpause`, `ContainerRename`, `ContainerUpdate`, `ContainerTop`, `ContainerStats`, `ContainerArchive/CopyTo/CopyFrom`, `ContainerExecStart` (uses `ContainerExecAttach` instead), `ContainerExecInspect`, `ImageList`, `ImageRemove`, `ImagePush`, `ImageBuild`, any Swarm/Plugin/Secret/Config endpoints.

#### 13.1.2 Job Lifecycle (Exact Sequence)

```
1. API Version Negotiation
   GET /_ping → reads API-Version header
   WithAPIVersionNegotiation() adjusts all subsequent calls

2. Image Pull Phase
   ImageInspectWithRaw(helperImage)           # check if helper exists locally
   ImageLoad(helperTar) → ImageTag(id, ref)   # or: load from embedded tar
   ImagePullBlocking(buildImage)              # pull build image
   ImagePullBlocking(serviceImage)            # pull each service image

3. Network Setup
   NetworkCreate("runner-net-<guid>")         # per-build network (FF_NETWORK_PER_BUILD)

4. Volume Setup
   VolumeCreate("runner-<hash>-cache-<hash>") # cache volume
   VolumeCreate("runner-<hash>-build-<hash>") # build dir volume (if cache_dir unset)

5. Service Containers
   For each service:
     ContainerCreate(serviceImage, network, volumes, aliases)
     ContainerStart(serviceID)
     ContainerInspect(serviceID)              # poll State.Status != "created"
     # Health check: create SEPARATE helper container running
     #   ["gitlab-runner-helper", "health-check"] to TCP-probe service ports

6. Build Container
   ContainerCreate(buildImage, cmd=[shell], entrypoint, network, volumes, stdin=true)
   ContainerAttach(buildID, stream=true, stdin=true, stdout=true, stderr=true)  # BEFORE START
   ContainerStart(buildID)
   # Stdin: pipe build script
   # Stdout/Stderr: read via multiplexed stream (stdcopy.StdCopy)
   ContainerWait(buildID, condition="not-running")

7. Helper Container (same as build, used for git clone, artifacts, cache)
   ContainerCreate(helperImage, cmd=["gitlab-runner-build"], network, SAME volumes)
   ContainerAttach(helperID, ...)             # BEFORE START
   ContainerStart(helperID)
   ContainerWait(helperID, ...)

8. Cleanup (5-minute timeout, parallel)
   For each container:
     ContainerKill(id, "SIGTERM")             # or: execScriptOnContainer to send SIGTERM
     ContainerStop(id, timeout=0)
     NetworkDisconnect(networkID, id, force=true)
     ContainerRemove(id, force=true, removeVolumes=true)
   NetworkRemove(networkID)
   VolumeRemove(volumeID)
```

#### 13.1.3 Critical Behaviors

**Attach-before-start pattern:**
The `Exec` method in `executors/docker/internal/exec/exec.go` calls `ContainerAttach()` BEFORE `ContainerStart()`. Docker's API returns the hijacked connection immediately — it registers interest in the stream before the container is running. The runner then calls `ContainerStart()`, and the attach stream begins receiving output once the process starts. There is **no explicit timeout** on the attach call beyond the job-level timeout (from `.gitlab-ci.yml`). The connection can sit idle for minutes waiting for container output.

**Build script execution via attach (NOT exec):**
In the normal flow, the runner pipes the build script through the attach stdin connection. The container's `Cmd` is set to the shell command, and the script is streamed as stdin. This means **attach is the primary execution mechanism**, not exec.

**Exec usage is limited:**
Exec is used only for: (1) sending SIGTERM to container processes during cleanup (`execScriptOnContainer` runs `sh -c <sigterm-script>`), and (2) the newer CI Steps/Functions mode (`ContainerExecCreate` + `ContainerExecAttach`). Note: the runner uses `ContainerExecAttach` (which combines start + attach into one hijacked connection), NOT `ContainerExecStart` as a separate call.

**Helper image loading:**
The runner tries three strategies in order: (1) `ImageInspectWithRaw` to check if image exists, (2) `ImageLoad` from embedded `.docker.tar.zst` file + `ImageTag`, (3) `ImagePullBlocking` from `registry.gitlab.com`. After loading from tar, it reads the JSON response stream for `"Loaded image:"` to get the image ID, then tags it. The `helper_image` config setting bypasses tar loading entirely and pulls from a registry.

**Volume sharing:**
ALL containers in the job (helper, build, services) get the same `Binds` list from `e.volumesManager.Binds()`. This includes named volumes for `/builds/<namespace>/<project>` and `/cache`. The helper container clones the repo into the build volume; the build container reads it from the same volume.

**Service readiness:**
The runner does NOT use Docker's built-in `HEALTHCHECK` / `State.Health`. Instead, it creates a SEPARATE health-check container using the helper image, running `["gitlab-runner-helper", "health-check"]`. This container TCP-probes the service's exposed ports. Before probing, it polls `ContainerInspect` until `State.Status` is no longer `"created"`. The `wait_for_services_timeout` setting (default 30s) controls the overall timeout.

**Network aliases:**
With `FF_NETWORK_PER_BUILD` enabled (recommended), the runner creates a per-build user-defined bridge network. Service aliases (e.g., `["postgres", "db"]`) are set via `NetworkingConfig.EndpointsConfig[networkName].Aliases`. Docker's embedded DNS resolves these aliases within the network. Without this flag (legacy), aliases use `ExtraHosts` entries injected into `/etc/hosts`.

**API version negotiation:**
Uses `WithAPIVersionNegotiation()` which sends `GET /_ping`, reads the `API-Version` header, and uses the min of client/server versions. Has version-aware code paths: v1.44+ puts MAC address in `EndpointsConfig`, pre-v1.44 puts it in `Config.MacAddress`. HTTP timeouts: TLS handshake 60s, response headers 120s, dialer 300s.

**Cleanup:**
Uses a dedicated context with 5-minute timeout. Removes all tracked containers in parallel via goroutines + `sync.WaitGroup`. Sequence: kill → stop → network disconnect → remove → remove volumes → remove network.

#### 13.1.4 Requirements for Sockerless

| Requirement | Priority | Sockerless Solution |
|---|---|---|
| `GET /_ping` with `API-Version: 1.44` header | P0 | Frontend responds immediately, no backend call |
| `WithAPIVersionNegotiation()` | P0 | Return correct header; frontend adapts if client requests older version |
| `ImagePullBlocking` with `X-Registry-Auth` | P0 | Record image ref + creds; cloud backend pulls from registry at start |
| `ImageLoad` (tar) + `ImageTag` | P1 | Accept tar, push to configured registry (or recommend `helper_image` config) |
| `ImageImportBlocking` (.tar.xz) | P1 | Same as ImageLoad |
| `ImageInspectWithRaw` with `Config.Env`, `Config.Cmd`, `Config.Entrypoint`, `Config.ExposedPorts` | P0 | Fetch image config from registry manifest API (no layer download needed) |
| `ContainerCreate` with full `HostConfig`, `NetworkingConfig`, stdin options | P0 | Store all fields; translate to cloud task config on start |
| `ContainerAttach` before `ContainerStart` | P0 | Frontend returns hijacked connection immediately; buffers until agent reachable |
| `ContainerStart` | P0 | Launch cloud task with agent; block until container is running |
| `ContainerWait` with `condition=not-running` | P0 | Frontend long-polls backend until cloud task exits |
| `ContainerInspect` with `State.Status`, `State.Running`, `State.ExitCode`, `NetworkSettings.IPAddress`, `NetworkSettings.Networks` | P0 | Backend returns current state from cloud task status |
| `ContainerLogs` with `stdout=true, stderr=true, timestamps=true` | P0 | Fetch from cloud logging (CloudWatch, Cloud Logging, Azure Monitor) |
| `ContainerExecCreate` + `ContainerExecAttach` | P1 | Agent handles exec with hijacked bidirectional stream |
| `ContainerKill` / `ContainerStop` / `ContainerRemove` (force) | P0 | Translate to cloud task stop/delete |
| `NetworkCreate` with labels, `NetworkRemove`, `NetworkDisconnect` | P0 | Cloud network emulation (VPC, service discovery) |
| `NetworkList`, `NetworkInspect` (with `Containers` map) | P0 | Backend tracks container-to-network membership |
| `VolumeCreate` (named), `VolumeRemove`, `VolumeInspect`, `VolumeList` | P0 | Cloud shared filesystem (EFS, GCS, Azure Files) |
| `EndpointsConfig.Aliases` for service DNS resolution | P0 | Cloud DNS service discovery (Cloud Map, Cloud DNS, ACA DNS) |
| `Info()` with `OSType: "linux"` | P0 | Frontend returns static info |
| Multiplexed stream protocol (8-byte header framing) | P0 | Frontend handles framing; agent sends raw stdout/stderr |

**Recommended GitLab Runner configuration for sockerless:**
```toml
[runners.docker]
  host = "unix:///var/run/sockerless.sock"   # Point to sockerless frontend
  helper_image = "registry.example.com/sockerless/gitlab-runner-helper:latest"  # Avoid tar loading
  pull_policy = "always"                      # Cloud backends always pull
  network_mtu = 1500                          # Explicit MTU
  wait_for_services_timeout = 120             # Allow for cloud startup latency

[runners.feature_flags]
  FF_NETWORK_PER_BUILD = true                 # Use user-defined networks (required)
```

### 13.2 GitHub Actions Runner

#### 13.2.1 Docker API Surface Used

GitHub Actions Runner shells out to the `docker` CLI (not the REST API directly). The runner's `DockerCommandManager.cs` wraps these CLI commands, which map to REST API calls:

| CLI Command | REST API Endpoint |
|---|---|
| `docker version` | `GET /version` |
| `docker pull <image>` | `POST /images/create` |
| `docker login` | `POST /auth` |
| `docker create --entrypoint ... --network ... -v ... -e ... --label ... <image> <args>` | `POST /containers/create` |
| `docker start <id>` | `POST /containers/{id}/start` |
| `docker ps --filter ...` | `GET /containers/json?filters=...` |
| `docker inspect <id>` (full JSON + `--format` templates) | `GET /containers/{id}/json` |
| `docker exec -i --workdir ... -e ... <id> <cmd>` | `POST /containers/{id}/exec` + `POST /exec/{id}/start` |
| `docker logs --details --stdout --stderr <id>` | `GET /containers/{id}/logs` |
| `docker port <id>` | Derived from `GET /containers/{id}/json` → `NetworkSettings.Ports` |
| `docker rm --force <id>` | `DELETE /containers/{id}?force=true` |
| `docker network create --label ... <name>` | `POST /networks/create` |
| `docker network rm <name>` | `DELETE /networks/{id}` |
| `docker network prune --filter label=...` | `POST /networks/prune` |
| `docker wait <id>` *(container actions only)* | `POST /containers/{id}/wait` |
| `docker logs --follow <id>` *(container actions only)* | `GET /containers/{id}/logs?follow=true` |

**Not used:** `docker stop`, `docker kill`, `docker attach`, `docker volume *`, `docker build`, `docker push`, any Swarm/Plugin commands.

#### 13.2.2 Job Lifecycle (Container Jobs)

```
1. Version Check
   docker version → GET /version (requires Server.APIVersion ≥ 1.35)

2. Network Setup
   docker network create --label <hash> github_network_<GUID>
   # Failure is FATAL — no fallback to default bridge

3. Image Pull (with retries, 3 attempts)
   docker pull <job-image>
   docker pull <service-image>  # for each service

4. Container Creation
   # Job container (long-lived):
   docker create --entrypoint "tail" --name <name> --network <net> \
     -v "/var/run/docker.sock:/var/run/docker.sock" \
     -v <workspace-mounts> -e <env-vars> --label <labels> \
     <image> "-f" "/dev/null"

   # Service containers:
   docker create --name <name> --network <net> \
     -e <env-vars> --label <labels> --health-cmd ... \
     <service-image>

5. Container Start
   docker start <id>
   docker ps --filter id=<id> --filter status=running  # verify running

6. Inspect & Setup
   docker inspect <id>    # full JSON
   # Parse Config.Env → extract PATH value
   # Parse Config.Healthcheck → determine if health polling needed

7. Health Check Polling (for services with HEALTHCHECK)
   docker inspect --format '{{if .Config.Healthcheck}}{{print .State.Health.Status}}{{end}}'
   # Exponential backoff: 2s, 3s, 7s, 13s... (~5-6 retries)
   # If no HEALTHCHECK defined → container considered ready immediately

8. Port Discovery (for services)
   docker port <id>
   # Parses: "5432/tcp -> 0.0.0.0:32768"
   # Populates job.services.<name>.ports[<containerPort>] context

9. Step Execution (for each workflow step)
   docker exec -i --workdir <path> -e VAR1=val1 -e VAR2=val2 \
     <container-id> bash -e /path/to/step-script.sh
   # Step script is volume-mounted, not piped via stdin
   # Exit code from `docker exec` process = step result
   # NO retry on exec failure — immediate step failure

10. Cleanup
    docker rm --force <job-container>       # kill + remove
    docker rm --force <service-container>   # for each
    docker network rm <network-name>
    docker network prune --filter label=<hash>
```

#### 13.2.3 Container Actions (`uses: docker://image`)

Container actions use a fundamentally different pattern from container jobs:

```
1. docker create with ORIGINAL entrypoint (NOT overridden to tail)
2. docker start
3. docker logs --follow (stream output)
4. docker wait (get exit code from container's own process)
5. docker rm
```

The container runs its own entrypoint to completion. Exit code comes from `docker wait` (the container's exit code, not exec). Volume mounts are limited to workspace and temp directories. `--workdir` is set to `/github/workspace`.

#### 13.2.4 Critical Behaviors

**`tail -f /dev/null` entrypoint override:**
The runner ALWAYS overrides the entrypoint for container jobs and service containers: `--entrypoint "tail" <image> "-f" "/dev/null"`. The original entrypoint is discarded. If the image lacks `tail` in `$PATH` (e.g., distroless, `FROM scratch`), the container fails with `exec: "tail": executable file not found`. There is NO fallback.

**PATH extraction from `Config.Env`:**
After starting a container, the runner calls `docker inspect` and reads `Config.Env` — an array of `KEY=VALUE` strings. It iterates through the array looking for entries starting with `PATH=`. This PATH is used when constructing `docker exec` commands to find binaries inside the container. If `Config.Env` is empty or missing `PATH`, the runner may fail to locate executables.

**Exec with stdin (`-i`):**
The runner uses `docker exec -i` for ALL step executions. The `-i` flag keeps stdin open — the runner pipes the step script into the container. The exec creates a temporary script file, volume-mounts the temp directory into the container, and executes via `docker exec -i --workdir <path> -e <env>... <container> bash -e /path/to/script.sh`. Exit codes are read from the `docker exec` process return code (not from `docker inspect`).

**No exec retry:**
If `docker exec` fails (container not running, connection lost, etc.), the step fails immediately. There is NO exponential backoff or retry logic for exec. This means the exec facility must be reliable from the first attempt.

**Docker socket mount:**
The runner ALWAYS adds `-v "/var/run/docker.sock:/var/run/docker.sock"` to the job container. This is hardcoded. The runner does NOT fail if the socket doesn't work inside the container — its own Docker commands execute from the host, not from inside the container. The socket is a best-effort convenience for actions needing Docker-in-Docker.

**Health check polling:**
Uses `docker inspect --format '{{if .Config.Healthcheck}}{{print .State.Health.Status}}{{end}}'`. If `.Config.Healthcheck` is absent (no HEALTHCHECK instruction), the template outputs empty string and the runner skips health polling — container is considered ready immediately. Polling uses exponential backoff: 2s, 3s, 7s, 13s... with ~5-6 retries (total ~30-60s).

**Network creation is mandatory:**
The runner ALWAYS creates a network (`github_network_<GUID>`) when container jobs or services are present. Network creation failure is FATAL — no fallback. Cleanup includes `docker network rm` + `docker network prune --filter label=<hash>`.

**No startup grace period:**
After `docker start`, the runner immediately checks `docker ps --filter id=<id> --filter status=running`. Docker containers start in <1s, but cloud containers take 5-60s. If the container is not "running" when checked, the job may fail.

#### 13.2.5 Requirements for Sockerless

| Requirement | Priority | Sockerless Solution |
|---|---|---|
| `GET /version` with `ApiVersion ≥ 1.35` | P0 | Return `1.44` |
| `POST /images/create` (pull) with retries | P0 | Record image ref; cloud pulls at container start |
| `POST /auth` (docker login) | P1 | Store credentials for later pull operations |
| `POST /containers/create` with `Entrypoint: ["tail"]`, `Cmd: ["-f", "/dev/null"]` | P0 | Accept; agent substitutes as keep-alive mechanism |
| `POST /containers/create` with `Binds` including `/var/run/docker.sock` | P0 | Accept silently; optionally mount sockerless socket |
| `POST /containers/{id}/start` → container is "running" immediately | P0 | Block `start` until cloud task is actually running |
| `GET /containers/json` with filters `id`, `status`, `label` | P0 | Backend supports all filter types |
| `GET /containers/{id}/json` with `Config.Env` (merged image + user env, including PATH) | P0 | Fetch image config from registry; merge with user env |
| `GET /containers/{id}/json` with `Config.Healthcheck` (presence) and `State.Health.Status` | P0 | Agent runs health check commands; reports status |
| `GET /containers/{id}/json` with `NetworkSettings.Ports` | P0 | Backend tracks port mappings for service containers |
| `POST /containers/{id}/exec` with `AttachStdin: true` | P0 | Agent handles exec with stdin pipe |
| `POST /exec/{id}/start` with hijacked bidirectional stream | P0 | Frontend bridges hijacked connection to agent WebSocket |
| Exit code from exec (not from container inspect) | P0 | Agent reports exec exit code; frontend returns to client |
| `DELETE /containers/{id}?force=true` | P0 | Stop cloud task + remove from state |
| `POST /networks/create` (failure is fatal) | P0 | Must succeed reliably |
| `DELETE /networks/{id}` | P0 | Tear down cloud networking |
| `POST /networks/prune` with label filter | P1 | Clean up orphaned cloud networks |
| `GET /containers/{id}/logs?details=true&stdout=true&stderr=true` | P0 | Fetch from cloud logging service |
| Multiplexed stream framing for exec I/O | P0 | Frontend handles framing |

### 13.3 Gap Analysis and Solutions

This section identifies every gap between what CI runners expect and what cloud backends provide, with concrete solutions.

#### Gap 1: Attach-Before-Start Timing (GitLab Runner)

**Problem:** GitLab Runner calls `ContainerAttach` before `ContainerStart`. Docker returns the hijacked connection instantly because the daemon holds it. Cloud backends don't have a container to attach to yet.

**Solution:** The frontend returns `101 Switching Protocols` immediately and holds the hijacked connection. When `ContainerStart` is called, the backend launches the cloud task. Once the agent is reachable (agent reports ready via backend polling), the frontend opens a WebSocket to the agent and begins bridging the buffered hijacked connection to the agent's stream.

**Risk:** If the job timeout is very short and cloud startup takes too long, the attach will sit idle until the job times out. No mitigation needed — this is the correct behavior (the runner's context controls the timeout).

#### Gap 2: Near-Instant Container Start (GitHub Actions Runner)

**Problem:** After `docker start`, the runner immediately runs `docker ps --filter status=running` to verify the container started. Docker containers start in <1s. Cloud backends take 5-60s.

**Solution:** The `POST /containers/{id}/start` endpoint MUST block until the cloud task is actually running (agent is reachable). Only then return `204 No Content`. This way, the subsequent `docker ps` check sees "running" status. The trade-off is that `docker start` takes 5-60s instead of <1s, but the runner doesn't have a timeout on the start call itself — only the overall job timeout applies.

**Implementation:** Backend launches cloud task → polls cloud API for task status → returns 204 only when task is in "RUNNING" state. Frontend forwards the 204 to the Docker client.

#### Gap 3: Image Config for PATH Extraction (GitHub Actions Runner)

**Problem:** The runner reads `Config.Env` from `docker inspect` to extract the `PATH` environment variable. Sockerless doesn't pull images locally, so it doesn't have the image config.

**Solution:** When `POST /images/create` (pull) is called, the backend fetches the image's manifest and config blob from the registry API WITHOUT downloading any layers. The image config contains `Env`, `Cmd`, `Entrypoint`, `ExposedPorts`, `WorkingDir`, `Labels`, and `Healthcheck`. These are stored in the backend's image table and returned in both `GET /images/{name}/json` and merged into `GET /containers/{id}/json` → `Config.Env`.

**Registry API calls needed:** `GET /v2/<name>/manifests/<tag>` → `GET /v2/<name>/blobs/<config-digest>`. These are lightweight (config blob is typically <10KB). Authentication uses the credentials from `X-Registry-Auth` or `POST /auth`.

#### Gap 4: `tail -f /dev/null` Entrypoint Handling (GitHub Actions Runner)

**Problem:** The runner overrides the entrypoint to `tail -f /dev/null`. On cloud backends, the sockerless agent needs to be PID 1 (or sidecar) to serve exec/attach.

**Solution:** The frontend detects the `tail -f /dev/null` entrypoint pattern in `POST /containers/create` and flags it as a "keep-alive container". When the backend launches the cloud task, it injects the agent as the entrypoint: `["/sockerless-agent", "--keep-alive", "--"]`. The agent keeps the container alive (replaces `tail -f /dev/null`) and serves exec/attach requests. The `--keep-alive` flag tells the agent NOT to run a child process — it just stays alive and waits for exec commands.

For containers with real entrypoints (non-tail), the agent wraps the original command: `["/sockerless-agent", "--", <original-entrypoint>, <args>...]`.

#### Gap 5: Helper Image Loading (GitLab Runner)

**Problem:** GitLab Runner tries to load the helper image from an embedded `.docker.tar.zst` via `ImageLoad` before falling back to registry pull. Sockerless has no local image store.

**Solution (recommended):** Configure `helper_image` in GitLab Runner's `config.toml` to point to a registry-hosted copy of the helper image. This bypasses tar loading entirely. The runner will use `ImagePullBlocking` instead.

**Solution (fallback):** Implement `POST /images/load` to accept the tar stream, extract the image manifest and layers, and push them to a configured staging registry (e.g., ECR, Artifact Registry, ACR). Then store the image reference for later use. `POST /images/{name}/tag` records the new tag in the backend's image table.

#### Gap 6: Volume Sharing Between Containers (GitLab Runner)

**Problem:** Helper and build containers share the same Docker volumes (for git clone → build handoff). Cloud containers are isolated tasks — they don't share local filesystems.

**Solution:** Cloud shared filesystems:
- **ECS:** EFS filesystem with access points. Multiple Fargate tasks mount the same EFS access point → shared data.
- **Cloud Run:** GCS bucket mounted via Cloud Storage FUSE. Multiple jobs access the same bucket path.
- **ACA:** Azure Files share. Multiple container app jobs mount the same file share.

Volume creation (`POST /volumes/create`) provisions a directory/access-point in the shared filesystem. Container creation records the volume mount mapping. Container start applies the mount to the cloud task definition.

**Performance consideration:** Cloud shared filesystems add latency vs. local Docker volumes. EFS latency is 0.5-5ms per operation. For CI workloads (git clone, build), this is acceptable.

#### Gap 7: Docker Socket Bind Mount (GitHub Actions Runner)

**Problem:** The runner always mounts `/var/run/docker.sock:/var/run/docker.sock`. This path doesn't exist on cloud backends.

**Solution:** Accept the bind mount in `POST /containers/create` without error. Two strategies:
1. **Silently ignore** (default): The mount is recorded in state and returned in inspect, but not applied to the cloud task. Actions that need Docker-in-Docker will fail with a connection error inside the container. This is acceptable — the runner itself doesn't use the socket from inside the container.
2. **Mount sockerless socket** (optional, configurable): Inject the sockerless frontend's socket endpoint into the container, enabling Docker-in-Docker via sockerless. Requires the agent to expose the endpoint or a tunnel.

#### Gap 8: Service Readiness TCP Probing (GitLab Runner)

**Problem:** GitLab Runner creates a SEPARATE health-check container (using the helper image) that TCP-probes the service container's ports. This health-check container also needs to run on the cloud backend and be able to reach the service container's network.

**Solution:** The health-check container runs as another cloud task on the same network (same VPC, same security group/Cloud Map namespace). It receives the same `Binds` as other containers. The cloud DNS service discovery ensures it can resolve the service container's alias. The health-check container runs `gitlab-runner-helper health-check`, which probes TCP ports. When the service is ready, the health-check container exits with code 0.

**Latency consideration:** Creating a health-check cloud task adds 5-60s of startup latency. To mitigate: start the health-check container in parallel with the service container (both at `ContainerStart` time). The health-check container's own startup latency overlaps with the service container's startup.

#### Gap 9: Health Check Status Reporting (GitHub Actions Runner)

**Problem:** The runner polls `docker inspect` for `.Config.Healthcheck` (presence) and `.State.Health.Status` (one of `"starting"`, `"healthy"`, `"unhealthy"`). This requires health check execution inside the container.

**Solution:** If the image defines a `HEALTHCHECK` instruction (detected during image config fetch from registry), the agent runs the health check command inside the container at the specified interval. The agent reports health status to the backend. The backend stores it and returns it in `GET /containers/{id}/json` → `State.Health.Status`.

If no `HEALTHCHECK` is defined, `Config.Healthcheck` is absent/null and `State.Health` is omitted entirely (matching Docker's behavior). The runner skips health polling in this case.

#### Gap 10: Network Aliases and DNS Resolution (Both Runners)

**Problem:** Both runners rely on DNS resolution for service aliases. GitLab uses `EndpointsConfig.Aliases`; GitHub uses network-scoped container names. Docker provides this via its embedded DNS server on user-defined networks.

**Solution per backend:**
- **ECS:** AWS Cloud Map service discovery. Create a private DNS namespace. Register each container as a service instance with its aliases. Cloud Map DNS resolves aliases to task ENI IPs.
- **Cloud Run:** Cloud DNS private zone. Create A records for each container's aliases pointing to the container's internal IP.
- **ACA:** Container Apps environment provides internal DNS. Container names and configured aliases resolve within the environment's VNet.

#### Gap 11: Exec Reliability (GitHub Actions Runner)

**Problem:** The runner does NOT retry failed exec calls. If exec fails once, the step fails. The agent must be ready to accept exec on the first attempt.

**Solution:** The agent starts its WebSocket server as the first thing on startup (before running the user process). The backend waits for agent readiness (health probe to agent's WebSocket endpoint) before returning from `POST /containers/{id}/start`. By the time the runner calls `docker exec`, the agent is guaranteed to be listening.

**Agent readiness check:** Backend polls `ws://agent:9111/health` (a simple HTTP GET on the same port). Agent responds with `200 OK` once its WebSocket server is accepting connections. Backend only returns `204` from start once this check passes.

#### Gap 12: Cloud Startup Latency

**Problem:** Cloud backends have 5-60s startup latency. Both runners assume near-instant starts. This affects:
- Time-to-first-exec (GitHub)
- Attach stream responsiveness (GitLab)
- Service readiness timeout (both)
- Overall job duration

**Solutions:**
1. **Absorb latency in `docker start`:** Block the start API call until the container is fully running and the agent is accepting connections. The runner doesn't time the start call separately.
2. **Parallel container startup:** When multiple containers are created (services + job container), start them in parallel on the cloud backend rather than sequentially. The Docker API is sequential (create, start, create, start...), but the backend can begin provisioning eagerly on create.
3. **Pre-warming (future):** Keep warm capacity in the cloud backend (e.g., pre-provisioned Fargate tasks, Cloud Run minimum instances) to reduce cold start time.
4. **Increased service timeouts:** Recommend `wait_for_services_timeout = 120` (GitLab) to account for cloud startup.

#### Gap 13: Multiplexed Stream Protocol Fidelity

**Problem:** Both runners use Docker's 8-byte header multiplexed stream protocol for attach and exec I/O. GitLab Runner uses `stdcopy.StdCopy` to demultiplex. Any incorrect framing breaks CI job output.

**Solution:** The frontend is responsible for correct framing. The agent sends raw stdout/stderr bytes via WebSocket JSON messages. The frontend wraps each chunk in the 8-byte header format:
- Byte 0: stream type (1=stdout, 2=stderr)
- Bytes 1-3: padding (0x00)
- Bytes 4-7: payload length (big-endian uint32)
- Followed by payload bytes

For stdin (client → agent), the frontend reads raw bytes from the hijacked connection and forwards them as `{"type":"stdin","data":"<base64>"}` WebSocket messages.

#### Gap 14: `ImageTag` After `ImageLoad` (GitLab Runner)

**Problem:** After `ImageLoad`, the runner parses the response for `"Loaded image: <id>"` or `"Loaded image ID: <id>"`, then calls `ImageTag(id, name+":"+tag)`. Sockerless must return a compatible response format.

**Solution:** `POST /images/load` response must stream JSON objects including a line matching `{"stream":"Loaded image: sha256:<digest>\n"}` or `{"stream":"Loaded image ID: sha256:<digest>\n"}`. The subsequent `POST /images/{name}/tag` records the new tag in the backend's image table.

### 13.4 FaaS Backend Compatibility

**FaaS backends (Lambda, Cloud Run Functions, Azure Functions) now support exec and attach via reverse agent**, but are still **NOT compatible with CI runners** due to other limitations:

| CI Runner Requirement | FaaS Capability | Gap |
|---|---|---|
| Long-running container (minutes-hours) | Max 15min (Lambda), 60min (CR Functions), 10min (Az Functions) | Timeout too short for most CI jobs |
| Exec into running container | **Supported** (via reverse agent) | No gap (but timeout-limited) |
| Attach to container I/O | **Supported** (via reverse agent) | No gap (but timeout-limited) |
| Volume sharing between containers | No persistent shared storage | Can't share build artifacts |
| Network aliases for service discovery | No user-defined networking | Can't reach service containers |
| Helper binary execution | Binaries can't run in FaaS context | gitlab-runner-helper won't work |

**Reverse agent mode:** The agent inside the function dials back to the backend via `SOCKERLESS_CALLBACK_URL` (WebSocket). This enables exec and attach for the duration of the function invocation. The FaaS invoke goroutine waits for agent disconnect before stopping the container.

**FaaS backends ARE useful for:** Short-lived container workloads (batch processing, webhooks, container actions, scheduled tasks) that can complete within the function timeout. They support `docker run` (create + start + exec + attach + logs + wait + remove) via reverse agent.

### 13.5 Backend Compatibility Matrix for CI Runners

| Feature | Docker | Memory | ECS | Cloud Run | ACA | Lambda | CR Func | Az Func |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| **GitLab Runner** | Yes | Yes† | Yes | Yes | Yes | **No** | **No** | **No** |
| **GitHub Actions Runner** | Yes | Yes | Yes | Yes | Yes | **No** | **No** | **No** |
| **GitHub `act` Runner** | Yes | Yes | Yes | Yes | Yes | Yes‡ | Yes‡ | Yes‡ |
| **gitlab-ci-local** | Yes | Yes | Yes | Yes | Yes | Yes‡ | Yes‡ | Yes‡ |
| Attach-before-start | Yes | Yes | Yes* | Yes* | Yes* | Yes** | Yes** | Yes** |
| Exec with stdin | Yes | Yes (WASM) | Yes* | Yes* | Yes* | Yes** | Yes** | Yes** |
| Volume sharing | Yes | Yes (symlinks) | Yes | Yes | Yes | No | No | No |
| Health check polling | Yes | Yes (exec) | Yes* | Yes* | Yes* | No | No | No |
| Container actions | Yes | Yes (WASM) | Yes* | Yes* | Yes* | Yes** | Yes** | Yes** |
| Docker build | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| Docker cp (archive) | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| Startup latency | <1s | <1s | 10-45s | 5-30s | 10-60s | 1-5s | 1-5s | 1-5s |

\* Via forward agent
\*\* Via reverse agent
† GitLab Runner E2E requires `SOCKERLESS_SYNTHETIC=1` (helper binaries can't run in WASM)
‡ FaaS backends limited by function timeout

**Recommendation:** For CI runner workloads, use container-based backends (ECS, Cloud Run, ACA). FaaS backends work with lightweight CI tools (`act`, `gitlab-ci-local`) but not with full CI runners.

---

## 14. Limitations and Exclusions

### 14.1 Inherent Cloud Backend Limitations

| Limitation | Impact | Mitigation |
|---|---|---|
| **Startup latency** (5-60s per container) | CI jobs take longer to start | Pre-warm containers; parallel startup; accept latency for serverless benefits |
| **No local filesystem** | `bind` mounts from host paths don't exist | Map to cloud storage; agent-based file injection |
| **No `--privileged`** | Can't run Docker-in-Docker in privileged mode | Use alternative DinD approaches (sysbox, rootless Docker) or kaniko for builds |
| **No device access** (`--device`) | Can't pass GPU or other devices | Some backends support GPU (ECS with GPU instances); not serverless in that case |
| **No process namespace sharing** (`--pid=host`) | Can't see host processes | Not relevant in cloud context |
| **Cold start variability** | Container startup time is not deterministic | Implement readiness tracking; retry logic in the daemon |

### 14.2 API Endpoints Not Supported

These endpoints return `501 Not Implemented` with a descriptive message:

| Category | Endpoints | Reason |
|---|---|---|
| Swarm | All `/swarm/*`, `/services/*`, `/tasks/*`, `/nodes/*`, `/secrets/*`, `/configs/*` | Not applicable to cloud backends |
| Plugins | All `/plugins/*` | Not applicable |
| Session | `POST /session` | BuildKit-specific |
| Distribution | `GET /distribution/{name}/json` | Not needed |
| Build prune | `POST /build/prune` | Not used |
| Container export | `GET /containers/{id}/export` | Not used |
| Container commit | `POST /commit` | Not used |
| Container changes | `GET /containers/{id}/changes` | Not used |
| Container update | `POST /containers/{id}/update` | Not used |
| Container resize | `POST /containers/{id}/resize` | Future (TTY support) |
| Container attach/ws | `GET /containers/{id}/attach/ws` | Future (WebSocket support) |
| Image search | `POST /images/search` | Not used |
| Image save/get | `GET /images/get` | Not used |
| Image push | `POST /images/{name}/push` | Out of scope |

### 14.3 API Endpoints Now Implemented (Originally Out of Scope)

These were initially excluded from the spec but have been implemented:

| Category | Endpoints | Added In |
|---|---|---|
| Build | `POST /build` | Phase 34 — Dockerfile parser (FROM, COPY, ADD, ENV, CMD, ENTRYPOINT, WORKDIR, ARG, LABEL, EXPOSE, USER). RUN is no-op. |
| Container archive | `PUT/HEAD/GET /containers/{id}/archive` | Phase 25 — Pre-start staging for `docker cp` before `docker start` |
| Container lifecycle | `POST .../restart`, `POST .../rename`, `POST .../pause`, `POST .../unpause` | Extended endpoints |
| Container info | `GET .../top`, `GET .../stats` | Extended endpoints |
| Container prune | `POST /containers/prune` | Extended endpoint |
| Image lifecycle | `GET /images/json`, `DELETE /images/{name}`, `GET .../history`, `POST /images/prune` | Extended endpoints |
| Volume prune | `POST /volumes/prune` | Extended endpoint |
| Network connect | `POST /networks/{id}/connect` | Extended endpoint |
| System | `GET /events`, `GET /system/df` | Extended endpoints |

### 14.3 Silently Ignored Container Config Fields

These fields are accepted in `POST /containers/create` but do not affect cloud backend behavior:

```
MacAddress, CgroupParent, DeviceCgroupRules, PidMode, UTSMode,
SecurityOpt, CapAdd, CapDrop, UsernsMode, ShmSize, Sysctls,
OomKillDisable, OomScoreAdj, Runtime, Isolation, Init,
VolumesFrom (partially supported), Devices, DeviceRequests,
LogConfig, CpusetCpus, CpusetMems, CPUShares, BlkioWeight,
BlkioDeviceReadBps, BlkioDeviceWriteBps, KernelMemory,
IpcMode, GroupAdd, Ulimits, ReadonlyRootfs, StorageOpt
```

These are stored in state and returned in inspect responses for client compatibility, but the cloud backend ignores them.

---

## 15. Configuration

### 15.1 Format

All components use **command-line flags** with **environment variable** overrides. No YAML configuration files. Each backend binary has its own set of environment variables prefixed with its cloud provider abbreviation.

Priority order: CLI flags > Environment variables > Defaults

### 15.2 Frontend Configuration

The frontend is configured via command-line flags:

```sh
sockerless-docker-frontend \
  -addr :2375 \             # Listen address (TCP)
  -backend http://localhost:9100 \  # Backend address
  -log-level info            # Log level
```

### 15.3 Backend Configuration

Each backend binary reads its own environment variables. All backends share common variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SOCKERLESS_CALLBACK_URL` | | Backend URL for reverse agent connections |
| `SOCKERLESS_ENDPOINT_URL` | | Custom cloud endpoint (simulator mode) |
| `SOCKERLESS_FETCH_IMAGE_CONFIG` | `false` | Fetch image config from registry on pull |
| `SOCKERLESS_SYNTHETIC` | `false` | Use synthetic mode (no real exec) |

Backend-specific variables use prefixes: `SOCKERLESS_ECS_*`, `SOCKERLESS_LAMBDA_*`, `SOCKERLESS_GCR_*`, `SOCKERLESS_GCF_*`, `SOCKERLESS_ACA_*`, `SOCKERLESS_AZF_*`, `AWS_REGION`.

See each backend's `README.md` for the full list of configuration variables.

**State:** All backends use in-memory state (`sync.Map` + `StateStore[T]`). No persistent database. State is lost on restart.

### 15.4 Agent Configuration

The agent is configured entirely via environment variables (injected by the backend):

| Env Var | Description |
|---------|-------------|
| `SOCKERLESS_AGENT_PORT` | Port to listen on (default: `9111`) |
| `SOCKERLESS_AGENT_TOKEN` | Auth token for validating frontend connections |
| `SOCKERLESS_CALLBACK_URL` | Backend URL for reverse agent mode |

---

## 16. Implementation Phases

### Phase 1: Foundation (MVP)

**Goal:** Frontend + memory backend + Docker backend pass the test suite. `docker run`, `docker ps`, `docker logs`, `docker exec` work end-to-end with the Docker backend.

| Component | Deliverable |
|---|---|
| `api/` module | Internal API types, capability model |
| Frontend | Docker REST API server (OpenAPI-generated types), system + container + exec + image endpoints |
| Memory backend | In-memory state, simulated containers |
| Docker backend | Passthrough to real Docker daemon (reference implementation) |
| Tests | Black-box REST test suite (system, containers, exec, images) |
| Protocol | Multiplexed stream framing, connection hijacking |

### Phase 2: Agent + Cloud Backend

**Goal:** Agent works. ECS backend runs containers on Fargate. GitLab Runner docker-executor completes a CI job.

| Component | Deliverable |
|---|---|
| Agent | WebSocket server, exec, attach, process management, health checks |
| ECS backend | Fargate tasks, CloudWatch logs, EFS volumes, VPC networking, agent injection |
| Network endpoints | create, inspect, list, disconnect, remove, prune |
| Volume endpoints | create, inspect, list, remove |
| Frontend streaming | WebSocket bridge: Docker client ↔ agent |
| GitLab Runner validation | End-to-end CI job with docker-executor |

### Phase 3: GitHub Runner + Compose + More Backends

**Goal:** GitHub Actions Runner works. Docker Compose core lifecycle works. Cloud Run and ACA backends.

| Component | Deliverable |
|---|---|
| GitHub Runner validation | Container jobs with exec-based step execution |
| Compose support | up/down/ps/logs with label-based filtering |
| Cloud Run backend | Cloud Run Jobs, Cloud Logging, GCS FUSE, agent via ingress |
| ACA backend | Container Apps Jobs, Azure Monitor, Azure Files, agent via VNet |
| Health check support | Image-defined health checks, polling, status reporting |
| Image load/tag | For GitLab Runner helper images |

### Phase 4: FaaS Backends + Robustness

**Goal:** Lambda, Cloud Functions, Azure Functions backends (capability-limited). Production hardening.

| Component | Deliverable |
|---|---|
| Lambda backend | Container image functions, CloudWatch logs, capability reporting |
| Cloud Run Functions backend | 2nd gen functions, Cloud Logging |
| Azure Functions backend | Custom container functions, Azure Monitor |
| Capability enforcement | Frontend returns 501 based on backend capabilities |
| Error handling | Retries, timeouts, graceful degradation |
| Monitoring | Metrics, health endpoint |
| Events | `GET /events` streaming endpoint |
| Documentation | User guide, backend setup guides, configuration reference |

### Phases 5–14: Core Extraction, Agent Bridge, Integration Testing

Phases 5–14 completed without architectural changes to the spec. See `STATUS.md`
for detailed phase history. Key milestones:
- Phase 5: Extracted shared `backends/core/` library (~70% code reduction per backend)
- Phase 7: FaaS agent injection via reverse WebSocket connections
- Phase 8–9: All 6 cloud backends tested against local simulators (98 PASS)
- Phase 10–11: Real CI runner smoke tests + full terraform integration tests
- Phase 13–14: E2E tests (12 workflows × 7 backends for both GitHub/GitLab runners)

### Phase 15: Memory Backend WASM Sandbox

**Goal:** Replace synthetic exec with real WASM command execution in the memory backend.

| Component | Library | Purpose |
|---|---|---|
| WASM runtime | wazero v1.11.0 | Pure Go, WASI Preview 1, no CGo |
| Commands | go-busybox | 41 BusyBox applets compiled to WASM |
| Shell | mvdan.cc/sh v3.12.0 | Pipes, &&/||, redirects, variable expansion, REPL |
| Filesystem | Host temp dirs | Per-container isolated rootfs via WithDirMount |

Architecture: mvdan.cc/sh parses shell syntax natively in Go and dispatches
individual commands to WASM busybox applets via wazero. WASI Preview 1 has no
fork/exec, so the Go host orchestrates all process spawning. The `sandbox/`
module is standalone — only the memory backend enables it via `ProcessFactory`.

### Phases 16–24: Extended Endpoints, CI Runner Improvements

- Phase 19: FaaS reverse agent — Lambda, GCF, AZF backends with reverse WebSocket agent
- Phase 22: GitHub `act` upstream compatibility (91/24 pass/fail on memory)
- Phase 25: Pre-start archive staging (`docker cp` before `docker start` for gitlab-ci-local)
- Phase 26–27: Attach-before-start, stdin forwarding, PATH-aware command resolution

### Phase 30: Driver Architecture

Introduced 4 driver interfaces with chain-of-responsibility pattern:

| Driver | Purpose | Chain |
|---|---|---|
| `ExecDriver` | Execute commands in containers | Agent → Process (WASM) → Synthetic |
| `FilesystemDriver` | Archive PUT/HEAD/GET | Agent → Process → Synthetic |
| `StreamDriver` | Logs, attach streaming | Agent → Process → Synthetic |
| `ProcessLifecycleDriver` | Start/stop/wait | WASM Process → Synthetic |

Each driver has a `Fallback` field forming a chain. `DriverSet` on `BaseServer`
is auto-constructed by `InitDrivers()`. This allows backends to mix execution
strategies — e.g., a cloud backend uses Agent for running containers but falls
back to Synthetic for containers without processes.

### Phases 31–33: Shell Builtins, Service Containers, Health Checks

- Phase 31: 21+ Go-implemented builtins, pwd fix, PATH resolution — upstream act: 91/24
- Phase 33: Service container support with health checks (`backends/core/health.go`)

### Phase 34: Docker Build Endpoint

`POST /build` with Dockerfile parser supporting FROM, COPY, ADD, ENV, CMD,
ENTRYPOINT, WORKDIR, ARG, LABEL, EXPOSE, USER. RUN instructions are no-op
(echoed in build output, not executed). Multi-stage builds supported. Build
context files staged via `BuildContexts` map.

### Phase 35: bleephub (GitHub Actions Server)

`bleephub/` Go module implements the Azure DevOps-derived internal API that
the official `actions/runner` binary expects. This enables end-to-end testing
of GitHub Actions workflows against Sockerless without a real GitHub instance.

Components: auth/tokens, agent registration, broker (sessions + long-poll),
run service, timeline + logs. Job messages use PipelineContextData + TemplateToken
format. Runner runs on port 80 (strips non-standard ports from URLs).

### Current Test Coverage

| Test Type | Count | Status |
|---|---|---|
| Simulator-backend integration | 129 | All pass |
| Sandbox unit tests | 46 | All pass |
| E2E GitHub (act) | 154 (22 workflows × 7 backends) | All pass |
| E2E GitLab (gitlab-ci-local) | 175 (25 pipelines × 7 backends) | All pass |
| Upstream act (memory) | 91 PASS / 24 FAIL | Expected |
| Upstream gitlab-ci-local | 175 | All pass |
| bleephub integration | 1 | Pass |

---

## Appendices

### A. Docker CLI Command to API Mapping (Sockerless Scope)

| Docker CLI Command | REST API Endpoint | Supported |
|---|---|---|
| `docker run` | `POST /containers/create` + `POST /containers/{id}/start` + `POST /containers/{id}/attach` + `POST /containers/{id}/wait` | Yes |
| `docker create` | `POST /containers/create` | Yes |
| `docker start` | `POST /containers/{id}/start` | Yes |
| `docker stop` | `POST /containers/{id}/stop` | Yes |
| `docker kill` | `POST /containers/{id}/kill` | Yes |
| `docker rm` | `DELETE /containers/{id}` | Yes |
| `docker ps` | `GET /containers/json` | Yes |
| `docker inspect` | `GET /containers/{id}/json` | Yes |
| `docker logs` | `GET /containers/{id}/logs` | Yes |
| `docker exec` | `POST /containers/{id}/exec` + `POST /exec/{id}/start` | Yes |
| `docker attach` | `POST /containers/{id}/attach` | Yes |
| `docker wait` | `POST /containers/{id}/wait` | Yes |
| `docker pull` | `POST /images/create` | Yes |
| `docker login` | `POST /auth` | Yes |
| `docker network create` | `POST /networks/create` | Yes |
| `docker network rm` | `DELETE /networks/{id}` | Yes |
| `docker network ls` | `GET /networks` | Yes |
| `docker network inspect` | `GET /networks/{id}` | Yes |
| `docker volume create` | `POST /volumes/create` | Yes |
| `docker volume rm` | `DELETE /volumes/{name}` | Yes |
| `docker volume ls` | `GET /volumes` | Yes |
| `docker volume inspect` | `GET /volumes/{name}` | Yes |
| `docker compose up` | (pull + create networks + create volumes + create containers + start) | Yes |
| `docker compose down` | (stop + remove containers + remove networks) | Yes |
| `docker compose ps` | `GET /containers/json` with compose label filter | Yes |
| `docker compose logs` | `GET /containers/{id}/logs` for each service | Yes |
| `docker build` | `POST /build` | Yes (Phase 34) |
| `docker push` | `POST /images/{name}/push` | **No** |
| `docker images` | `GET /images/json` | Yes |
| `docker rmi` | `DELETE /images/{name}` | Yes |
| `docker cp` | `PUT/GET /containers/{id}/archive` | Yes (Phase 25) |
| `docker stats` | `GET /containers/{id}/stats` | Yes |
| `docker top` | `GET /containers/{id}/top` | Yes |
| `docker restart` | `POST /containers/{id}/restart` | Yes |
| `docker rename` | `POST /containers/{id}/rename` | Yes |
| `docker pause` | `POST /containers/{id}/pause` | Yes |
| `docker unpause` | `POST /containers/{id}/unpause` | Yes |
| `docker system df` | `GET /system/df` | Yes |
| `docker system events` | `GET /events` | Yes |
| `docker system prune` | (various prune endpoints) | Yes |
| `docker image prune` | `POST /images/prune` | Yes |
| `docker container prune` | `POST /containers/prune` | Yes |
| `docker volume prune` | `POST /volumes/prune` | Yes |
| `docker network prune` | `POST /networks/prune` | Yes |

### B. References

- Docker Engine API v1.44: https://docs.docker.com/engine/api/v1.44/
- GitLab Runner source: https://gitlab.com/gitlab-org/gitlab-runner
- GitHub Actions Runner source: https://github.com/actions/runner
- AWS ECS Exec: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-exec.html
- Google Cloud Run Jobs: https://cloud.google.com/run/docs/create-jobs
- Azure Container Apps Jobs: https://learn.microsoft.com/en-us/azure/container-apps/jobs
