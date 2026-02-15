# Sockerless Specification

> **Version:** 0.1.0 (Draft)
>
> **Date:** February 2026
>
> **Status:** Initial specification — API surface and architecture
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

| # | Non-Goal | Reason |
|---|----------|--------|
| N1 | `docker build` / `POST /build` | Out of scope. CI runners can use kaniko, buildx, or external build services. |
| N2 | Swarm endpoints (`/swarm/*`, `/services/*`, `/tasks/*`, `/nodes/*`, `/secrets/*`, `/configs/*`) | Neither CI runner uses Swarm. No cloud backend maps to it. |
| N3 | Plugin endpoints (`/plugins/*`) | Docker plugin system is not relevant to cloud backends. |
| N4 | Session/BuildKit endpoint (`POST /session`) | Tied to docker build, which is out of scope. |
| N5 | Distribution endpoint (`GET /distribution/{name}/json`) | Not used by target CI runners. |
| N6 | Container filesystem archive (`GET/PUT/HEAD /containers/{id}/archive`) | Not used by target CI runners. |
| N7 | Image push (`POST /images/{name}/push`) | Users push images to registries externally. |
| N8 | Sub-second container startup | Cloud backends have inherent startup latency (5-60s). Accepted tradeoff. |
| N9 | FaaS backends as primary targets | Lambda, Cloud Functions, Azure Functions have 15-min timeouts and no exec. Not viable for CI jobs. Deferred to future phases. |

---

## 3. Target Docker API Surface

### 3.1 Supported Endpoints — Summary

**31 endpoints** across 6 categories. This is the union of endpoints required by GitLab Runner docker-executor and GitHub Actions Runner.

| Category | Count | Endpoints |
|----------|-------|-----------|
| System | 4 | `_ping` (GET, HEAD), `version`, `info` |
| Images | 5 | pull, inspect, load, tag, auth |
| Containers | 10 | create, start, inspect, list, logs, attach, wait, stop, kill, remove |
| Exec | 3 | create, start, inspect |
| Networks | 6 | create, list, inspect, disconnect, remove, prune |
| Volumes | 4 | create, list, inspect, remove |

### 3.2 Endpoint Priority

Each endpoint is classified by which CI runner requires it:

| Endpoint | GitLab Runner | GitHub Runner | Docker Compose | Priority |
|----------|:---:|:---:|:---:|:---:|
| **System** | | | | |
| `GET /_ping` | Yes | — | Yes | P0 |
| `HEAD /_ping` | Yes | — | Yes | P0 |
| `GET /version` | Yes | Yes | Yes | P0 |
| `GET /info` | Yes | — | Yes | P0 |
| **Images** | | | | |
| `POST /images/create` (pull) | Yes | Yes | Yes | P0 |
| `GET /images/{name}/json` | Yes | — | Yes | P0 |
| `POST /images/load` | Yes | — | — | P1 |
| `POST /images/{name}/tag` | Yes | — | — | P1 |
| `POST /auth` | — | Yes | Yes | P1 |
| **Containers** | | | | |
| `POST /containers/create` | Yes | Yes | Yes | P0 |
| `POST /containers/{id}/start` | Yes | Yes | Yes | P0 |
| `GET /containers/{id}/json` | Yes | Yes | Yes | P0 |
| `GET /containers/json` | Yes | Yes | Yes | P0 |
| `GET /containers/{id}/logs` | Yes | Yes | Yes | P0 |
| `POST /containers/{id}/attach` | Yes | Yes | — | P0 |
| `POST /containers/{id}/wait` | Yes | Yes | — | P0 |
| `POST /containers/{id}/stop` | Yes | — | Yes | P0 |
| `POST /containers/{id}/kill` | Yes | — | Yes | P0 |
| `DELETE /containers/{id}` | Yes | Yes | Yes | P0 |
| **Exec** | | | | |
| `POST /containers/{id}/exec` | Yes | Yes | — | P0 |
| `POST /exec/{id}/start` | Yes | Yes | — | P0 |
| `GET /exec/{id}/json` | — | — | — | P2 |
| **Networks** | | | | |
| `POST /networks/create` | Yes | Yes | Yes | P0 |
| `GET /networks` | Yes | — | Yes | P0 |
| `GET /networks/{id}` | Yes | — | Yes | P0 |
| `POST /networks/{id}/disconnect` | Yes | — | — | P1 |
| `DELETE /networks/{id}` | Yes | Yes | Yes | P0 |
| `POST /networks/prune` | — | Yes | — | P1 |
| **Volumes** | | | | |
| `POST /volumes/create` | Yes | — | Yes | P0 |
| `GET /volumes` | Yes | — | Yes | P0 |
| `GET /volumes/{name}` | Yes | — | Yes | P0 |
| `DELETE /volumes/{name}` | Yes | — | Yes | P0 |

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
3. Start the exec/attach agent inside the container (see [Exec and Attach Strategy](#10-exec-and-attach-strategy))
4. Update container state to `Running`

Response: `204 No Content` (same as Docker)

#### `GET /containers/{id}/json` — Inspect Container

Returns full container metadata. **CI runners rely on specific fields:**

| Field | GitLab Runner Reads | GitHub Runner Reads |
|-------|:---:|:---:|
| `State.Status` ("created", "running", "exited") | Yes | — |
| `State.Running` (bool) | Yes | — |
| `State.ExitCode` (int) | Yes | — |
| `State.Health.Status` ("healthy", "unhealthy", "starting") | Yes | Yes |
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

Sockerless: Creates a logical network in internal state. The actual cloud networking is configured when containers join this network. See [Section 9](#9-network-emulation).

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

Sockerless: Creates cloud storage (see [Section 8](#8-volume-emulation)) and records volume metadata.

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
│   ├── memory/                        # Module: in-memory backend
│   │   └── go.mod                     #   Deps: api/ only
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
├── tests/                             # Module: black-box API tests
│   └── go.mod                         #   Deps: Docker client SDK (as test client)
│
└── Makefile
```

**12 Go modules, 10 binaries:**

| # | Module | Binary Name | External Dependencies |
|---|--------|------------|----------------------|
| 1 | `api/` | *(library — no binary)* | None (stdlib only) |
| 2 | `frontends/docker/` | `sockerless-docker-frontend` | `api/`, OpenAPI-generated types, `gorilla/websocket` |
| 3 | `backends/memory/` | `sockerless-backend-memory` | `api/` only |
| 4 | `backends/docker/` | `sockerless-backend-docker` | `api/`, `github.com/docker/docker` client SDK |
| 5 | `backends/ecs/` | `sockerless-backend-ecs` | `api/`, AWS SDK v2 |
| 6 | `backends/lambda/` | `sockerless-backend-lambda` | `api/`, AWS SDK v2 |
| 7 | `backends/cloudrun/` | `sockerless-backend-cloudrun` | `api/`, GCP SDK |
| 8 | `backends/cloudrun-functions/` | `sockerless-backend-cloudrun-functions` | `api/`, GCP SDK |
| 9 | `backends/aca/` | `sockerless-backend-aca` | `api/`, Azure SDK |
| 10 | `backends/azure-functions/` | `sockerless-backend-azure-functions` | `api/`, Azure SDK |
| 11 | `agent/` | `sockerless-agent` | `gorilla/websocket` only |
| 12 | `tests/` | *(test binary — `go test`)* | Docker client SDK |

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
2. Owns all persistent state (containers, networks, volumes, images) in a local database (SQLite)
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
| Role | WebSocket **server** inside the cloud container |
| Port | Listens on `:9111` (configurable via env var) |
| Connection direction | **Frontend connects to agent** (not the other way) |
| Auth | Frontend presents a token when connecting; agent validates it |
| Lifetime | Runs for the container's lifetime as PID 1 (wrapping the user process) or as a sidecar |
| Binary size | Static binary, ~5-10 MB |

**When the agent is NOT needed:**
- Docker backend: uses Docker's native exec/attach
- Memory backend: direct in-process execution
- Any backend where the native exec mechanism is sufficient

**When the agent IS needed:**
- Cloud backends where arbitrary command execution inside running containers is not natively supported (Cloud Run, Lambda, Azure Functions)
- Cloud backends where native exec exists but does not support the full protocol (ECS — SSM exec is limited)

### 6.6 Dependency Flow

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
| Container create + start | `RunTask` (Fargate launch type) with task definition registered from container config |
| Container stop | `StopTask` |
| Container kill | `StopTask` (ECS has no SIGKILL; tasks get 30s SIGTERM then forced stop) |
| Container remove | Deregister task definition, clean up |
| Container inspect | `DescribeTasks` |
| Container logs | CloudWatch Logs (`GetLogEvents` / `FilterLogEvents`) |
| Exec / Attach | Via sockerless-agent (agent address from task's ENI private IP) |
| Image pull | ECS pulls from ECR / Docker Hub / any registry natively |
| Network | VPC subnets, security groups, AWS Cloud Map for service discovery |
| Volume | EFS mount, ephemeral storage (20-200 GB) |
| Port mapping | Security group + task IP:port |
| Health check | ECS native health check + agent health check reporting |

**Agent networking:** Fargate tasks get private IPs in the configured VPC. The frontend must be able to reach these IPs (same VPC, VPC peering, or transit gateway). Agent listens on port 9111 — security group must allow inbound from frontend.

**Startup latency:** 10-45 seconds (image pull + Fargate capacity allocation).

#### Google Cloud Run (Jobs)

| Docker Concept | Cloud Run Mapping |
|---|---|
| Container create + start | `CreateJob` + `RunJob` (or `CreateExecution`) |
| Container stop | `CancelExecution` |
| Container logs | Cloud Logging (`entries.list`) |
| Exec / Attach | Via sockerless-agent (agent on Cloud Run's ingress port) |
| Image pull | Cloud Run pulls from Artifact Registry / GCR / Docker Hub natively |
| Network | VPC Connector or Direct VPC Egress |
| Volume | Cloud Storage FUSE, GCS, in-memory (tmpfs) |

**Agent networking:** Cloud Run containers serve traffic on the port defined by `PORT` env var. The agent can share this port (mux HTTP + WebSocket) or use Cloud Run's sidecar support. Frontend reaches agent via Cloud Run's internal URL.

**Startup latency:** 5-30 seconds.

#### Azure Container Apps (Jobs)

| Docker Concept | ACA Mapping |
|---|---|
| Container create + start | `Create Job` + `Start Job Execution` |
| Container stop | `Stop Job Execution` |
| Container logs | Azure Monitor / Log Analytics |
| Exec / Attach | Via sockerless-agent (agent on container port, VNet accessible) |
| Image pull | ACA pulls from ACR / Docker Hub natively |
| Network | VNet integration (managed environment) |
| Volume | Azure Files, ephemeral storage |

**Startup latency:** 10-60 seconds.

### 9.2 FaaS Backends

FaaS backends have hard limitations but are included for short-lived container workloads. They report reduced capabilities.

#### AWS Lambda

| Docker Concept | Lambda Mapping |
|---|---|
| Container create + start | Create/update Lambda function + invoke |
| Container stop/kill | N/A (function exits on its own) |
| Container logs | CloudWatch Logs |
| Exec / Attach | **Not supported** (reported via capabilities) |
| Image pull | Lambda pulls container images from ECR |
| Max timeout | 15 minutes |

**Capabilities:** `exec: false, attach: false, logs: true, logs_follow: false, max_timeout_seconds: 900, agent_required: false`

#### Google Cloud Run Functions

| Docker Concept | Cloud Run Functions Mapping |
|---|---|
| Container create + start | Deploy function (2nd gen, container-based) + invoke |
| Container stop/kill | N/A |
| Container logs | Cloud Logging |
| Exec / Attach | **Not supported** |
| Image pull | Functions pull from Artifact Registry |
| Max timeout | 60 minutes (2nd gen) |

**Capabilities:** `exec: false, attach: false, logs: true, logs_follow: false, max_timeout_seconds: 3600, agent_required: false`

#### Azure Functions

| Docker Concept | Azure Functions Mapping |
|---|---|
| Container create + start | Deploy function (custom container) + invoke |
| Container stop/kill | N/A |
| Container logs | Azure Monitor |
| Exec / Attach | **Not supported** |
| Image pull | Functions pull from ACR |
| Max timeout | 5-10 minutes (consumption plan) |

**Capabilities:** `exec: false, attach: false, logs: true, logs_follow: false, max_timeout_seconds: 600, agent_required: false`

### 9.3 Docker Backend (Reference / Testing)

| Docker Concept | Docker Mapping |
|---|---|
| Container create + start | `POST /containers/create` + `POST /containers/{id}/start` on real Docker daemon |
| Exec / Attach | Docker's native exec and attach (no agent needed) |
| Everything else | 1:1 passthrough to Docker daemon |

**Capabilities:** `exec: true, attach: true, logs: true, logs_follow: true, volumes: true, networks: true, health_checks: true, agent_required: false`

The Docker backend is the **reference implementation** for testing. Running the test suite against the Docker backend validates that sockerless produces responses identical to a real Docker daemon.

### 9.4 Memory Backend (Testing)

In-memory backend for fast unit/integration tests. All state is in-memory, containers are simulated (no actual processes run). Exec returns mock output.

**Capabilities:** All true (simulated). `agent_required: false`

### 9.5 Backend Comparison Matrix

| Capability | Docker | Memory | ECS | Cloud Run | ACA | Lambda | CR Func | Az Func |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| Long-running | Yes | Sim | Yes | Yes (24h) | Yes | No (15m) | No (60m) | No (10m) |
| Exec | Yes | Sim | Yes* | Yes* | Yes* | **No** | **No** | **No** |
| Attach | Yes | Sim | Yes* | Yes* | Yes* | **No** | **No** | **No** |
| Log stream | Yes | Sim | Yes | Yes | Yes | Yes | Yes | Yes |
| Log follow | Yes | Sim | Yes | Yes | Yes | No | No | No |
| Volumes | Yes | Sim | Yes | Partial | Yes | No | No | No |
| Networks | Yes | Sim | Yes | Yes | Yes | No | No | No |
| Health checks | Yes | Sim | Yes | Yes | Yes | No | No | No |
| Agent needed | No | No | Yes | Yes | Yes | No | No | No |
| Startup latency | <1s | 0 | 10-45s | 5-30s | 10-60s | 1-5s | 1-5s | 1-5s |

\* Via sockerless-agent

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

#### AWS ECS + EFS
1. Create an EFS filesystem (or use a pre-provisioned one) per sockerless instance
2. Create EFS access points for each named volume
3. Mount the EFS access point in the ECS task definition
4. Multiple tasks (containers) can mount the same access point → shared data

#### Google Cloud Run + GCS
1. Create a GCS bucket (or use a pre-provisioned one) per sockerless instance
2. Mount via Cloud Storage FUSE in the container
3. Shared access via the same bucket/path prefix

#### Azure Container Apps + Azure Files
1. Create an Azure Files share per sockerless instance
2. Mount the share in the container app job
3. Shared access via the same file share

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

Each sockerless network maps to an isolated cloud networking construct:

| Cloud Backend | Network Emulation |
|---|---|
| AWS ECS | VPC security group per network; AWS Cloud Map service discovery namespace for DNS aliases |
| Google Cloud Run | VPC network with tags; Cloud DNS private zone for aliases |
| Azure Container Apps | Environment-scoped networking; internal DNS for aliases |

### 11.3 IP Address Assignment

Sockerless assigns virtual IPs from a private subnet (e.g., `172.18.0.0/16`) to each container on a network. These IPs are returned in inspect responses. Actual cloud networking may use different IPs, but the sockerless-level IPs provide the correct inspect output for CI runners.

For DNS-based service discovery (which is what CI runners actually rely on), the container aliases (e.g., `postgres`, `redis`) are registered in the cloud's DNS service so they resolve to the actual cloud IPs.

### 11.4 Network Lifecycle

| Docker Operation | Sockerless Action |
|---|---|
| `POST /networks/create` | Create cloud networking construct (security group, Cloud Map namespace, etc.) |
| Container create with `NetworkMode`/`EndpointsConfig` | Assign virtual IP, register DNS aliases |
| `POST /networks/{id}/disconnect` | Remove DNS alias, detach from security group |
| `DELETE /networks/{id}` | Tear down cloud networking construct |
| `POST /networks/prune` | Clean up unused cloud networking constructs |

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

### 13.1 GitLab Runner Docker-Executor

GitLab Runner's docker-executor has a specific lifecycle (see `DOCKER_REST_API.md` research). Key requirements for sockerless:

| Requirement | Sockerless Support | Notes |
|---|---|---|
| API version negotiation (`GET /_ping` → `API-Version` header) | Yes | Return `1.44` |
| Pull images with `X-Registry-Auth` | Yes | Forward to cloud backend's image pull |
| Create per-build bridge network | Yes | Cloud network emulation |
| Create helper, build, and service containers | Yes | Each becomes a cloud task |
| `ContainerAttach` BEFORE `ContainerStart` (attach-then-start pattern) | Yes | Buffer attach until agent connects |
| Multiplexed stream framing (8-byte headers) | Yes | Protocol requirement |
| `ContainerWait` with `WaitConditionNotRunning` | Yes | Poll cloud task status |
| Volume sharing between helper and build containers | Yes | Cloud shared filesystem |
| Service container readiness polling via `ContainerInspect` | Yes | Report container state |
| Container cleanup (stop → disconnect → remove) | Yes | Tear down cloud tasks |
| Labels on all objects (`com.gitlab.gitlab-runner.*`) | Yes | Stored in internal state |

**GitLab Runner known compatibility notes:**
- Runner uses `WithAPIVersionNegotiation()` — will accept our v1.44
- Runner has MAC address handling code for v1.44 — we can accept and ignore MAC addresses
- Runner's `FF_NETWORK_PER_BUILD` (network-per-build feature flag) should be **enabled** — this is the modern approach and maps well to cloud networking
- Runner's Podman compatibility notes apply: idle timeout issues won't affect us (we're always-on), but the multiplexed stream protocol is critical

### 13.2 GitHub Actions Runner

GitHub Actions Runner's container support has a different pattern:

| Requirement | Sockerless Support | Notes |
|---|---|---|
| `docker version` check (min v1.35) | Yes | Our v1.44 exceeds this |
| `docker pull` with retry (3x) | Yes | |
| `docker create` with `--entrypoint tail ... -f /dev/null` | Yes | Long-running container for exec |
| `docker start` + `docker ps` (verify running) | Yes | |
| `docker exec -i --workdir ... -e ...` for each step | Yes | Primary execution mechanism |
| `docker inspect` for health check polling | Yes | |
| `docker inspect` for PATH extraction from `Config.Env` | Yes | Return image's env vars |
| `docker port` (reads from inspect `NetworkSettings.Ports`) | Yes | |
| `docker rm --force` for cleanup | Yes | |
| `docker network create` + `docker network rm` | Yes | |
| `docker network prune --filter label=...` | Yes | |
| Label-based filtering in `docker ps` | Yes | |

**GitHub Actions Runner known compatibility notes:**
- Runner does NOT use the Docker REST API directly — it shells out to `docker` CLI
- Runner overrides entrypoint to `tail -f /dev/null` and uses exec for all steps
- Runner requires multiplexed stream framing for exec I/O
- Runner does NOT use `docker stop` — it uses `docker rm --force` (kill + remove)
- Runner uses `--format` go templates on inspect output — our inspect response must include all required fields
- Runner mounts the Docker socket into the container (`/var/run/docker.sock`) — in the cloud context, this could be the sockerless socket for Docker-in-Docker scenarios, or could be omitted

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
| Build | `POST /build`, `POST /build/prune` | Out of scope (N2) |
| Swarm | All `/swarm/*`, `/services/*`, `/tasks/*`, `/nodes/*`, `/secrets/*`, `/configs/*` | Not applicable to cloud backends |
| Plugins | All `/plugins/*` | Not applicable |
| Session | `POST /session` | BuildKit-specific |
| Distribution | `GET /distribution/{name}/json` | Not needed |
| Container archive | `GET/PUT/HEAD /containers/{id}/archive` | Not used by CI runners |
| Container export | `GET /containers/{id}/export` | Not used |
| Container commit | `POST /commit` | Not used |
| Container stats | `GET /containers/{id}/stats` | Future enhancement |
| Container top | `GET /containers/{id}/top` | Future enhancement |
| Container changes | `GET /containers/{id}/changes` | Not used |
| Container update | `POST /containers/{id}/update` | Not used |
| Container rename | `POST /containers/{id}/rename` | Not used |
| Container pause/unpause | `POST /containers/{id}/pause`, `/unpause` | Not used |
| Container resize | `POST /containers/{id}/resize` | Future (TTY support) |
| Container attach/ws | `GET /containers/{id}/attach/ws` | Future (WebSocket support) |
| Image search | `POST /images/search` | Not used |
| Image save/get | `GET /images/get` | Not used |
| Image push | `POST /images/{name}/push` | Out of scope |
| Image prune | `POST /images/prune` | Future enhancement |
| Image remove | `DELETE /images/{name}` | Future enhancement |
| Image history | `GET /images/{name}/history` | Not used |
| Image list | `GET /images/json` | Future enhancement |
| Container prune | `POST /containers/prune` | Future enhancement |
| Volume prune | `POST /volumes/prune` | Future enhancement |
| System df | `GET /system/df` | Future enhancement |
| Events | `GET /events` | Future enhancement |

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

All components use YAML configuration files with environment variable overrides and CLI flag overrides (highest priority).

Priority order: CLI flags > Environment variables > YAML config file > Defaults

### 15.2 Frontend Configuration

```yaml
# sockerless-frontend.yaml
listen:
  socket: /var/run/sockerless.sock   # Unix socket path
  tcp: ""                            # Optional TCP address (e.g., "0.0.0.0:2375")

backend:
  address: /var/run/sockerless-backend.sock  # Backend Unix socket or TCP address

api:
  version: "1.44"                    # Docker API version to advertise

logging:
  level: info                        # debug, info, warn, error
  format: json                       # json, text
```

### 15.3 Backend Configuration

```yaml
# sockerless-backend.yaml
listen:
  socket: /var/run/sockerless-backend.sock
  tcp: ""

state:
  driver: sqlite                     # sqlite, memory
  path: /var/lib/sockerless/state.db

agent:
  binary_path: /usr/local/bin/sockerless-agent  # Path to agent binary (for injection)
  port: 9111                         # Port agent listens on inside containers
  token_length: 32                   # Length of generated auth tokens

# Backend-specific configuration (varies per backend binary)
# Example for ECS:
ecs:
  region: us-east-1
  cluster: sockerless
  subnets: ["subnet-abc123"]
  security_groups: ["sg-abc123"]
  task_role_arn: "arn:aws:iam::..."
  execution_role_arn: "arn:aws:iam::..."
  efs_filesystem_id: "fs-abc123"
  log_group: /sockerless/containers
```

### 15.4 Agent Configuration

The agent is configured entirely via environment variables (injected by the backend):

| Env Var | Description |
|---------|-------------|
| `SOCKERLESS_AGENT_PORT` | Port to listen on (default: `9111`) |
| `SOCKERLESS_AGENT_TOKEN` | Auth token for validating frontend connections |
| `SOCKERLESS_ORIGINAL_ENTRYPOINT` | Original container entrypoint (agent runs this as child) |
| `SOCKERLESS_ORIGINAL_CMD` | Original container CMD |

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
| `docker build` | `POST /build` | **No** |
| `docker push` | `POST /images/{name}/push` | **No** |
| `docker images` | `GET /images/json` | Future |
| `docker rmi` | `DELETE /images/{name}` | Future |
| `docker stats` | `GET /containers/{id}/stats` | Future |

### B. References

- Docker Engine API v1.44: https://docs.docker.com/engine/api/v1.44/
- GitLab Runner source: https://gitlab.com/gitlab-org/gitlab-runner
- GitHub Actions Runner source: https://github.com/actions/runner
- AWS ECS Exec: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-exec.html
- Google Cloud Run Jobs: https://cloud.google.com/run/docs/create-jobs
- Azure Container Apps Jobs: https://learn.microsoft.com/en-us/azure/container-apps/jobs
