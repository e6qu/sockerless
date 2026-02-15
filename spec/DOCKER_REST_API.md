# Docker Engine REST API: Implementation Comparison

> **Baseline:** Docker Engine API **v1.45** (Docker 25.x, released January 2024)
>
> **Last updated:** February 2026
>
> **Scope:** REST HTTP API compatibility only. CLI-only tools and CRI (Container Runtime Interface) are out of scope.

---

## Table of Contents

1. [Docker Engine API v1.45 — The Baseline](#1-docker-engine-api-v145--the-baseline)
2. [Implementation Comparison Matrix](#2-implementation-comparison-matrix)
3. [Podman](#3-podman)
4. [Rancher Desktop](#4-rancher-desktop)
5. [Colima and Docker Desktop](#5-colima-and-docker-desktop)
6. [Balena Engine](#6-balena-engine)
7. [Testcontainers Cloud](#7-testcontainers-cloud)
8. [Proprietary and Non-Docker APIs](#8-proprietary-and-non-docker-apis)
9. [Per-Endpoint Compatibility Detail](#9-per-endpoint-compatibility-detail)
10. [Projects Evaluated and Excluded](#10-projects-evaluated-and-excluded)

---

## 1. Docker Engine API v1.45 — The Baseline

The Docker Engine API v1.45 is a RESTful API served over a Unix socket (`/var/run/docker.sock`) or TCP (with optional mTLS). It comprises **15 endpoint categories** with over **100 individual endpoints**.

### 1.1 Endpoint Categories

| Category | Endpoints | Description |
|----------|-----------|-------------|
| **Containers** | ~23 | Full lifecycle: create, start, stop, kill, remove, inspect, logs, exec, attach, wait, resize, stats, top, changes, export, update, rename, pause/unpause, archive, prune |
| **Images** | ~14 | List, build, pull, inspect, history, push, tag, remove, search, prune, get/save, load, commit |
| **Networks** | 7 | List, inspect, create, connect, disconnect, remove, prune |
| **Volumes** | 6 | List, create, inspect, update (cluster), remove, prune |
| **Exec** | 4 | Create, start, inspect, resize |
| **System** | 7 | Auth, info, version, ping (GET/HEAD), events, system/df |
| **Swarm** | 7 | Inspect, init, join, leave, update, unlockkey, unlock |
| **Nodes** | 4 | List, inspect, delete, update |
| **Services** | 6 | List, create, inspect, delete, update, logs |
| **Tasks** | 3 | List, inspect, logs |
| **Secrets** | 5 | List, create, inspect, delete, update |
| **Configs** | 5 | List, create, inspect, delete, update |
| **Plugins** | 10 | List, privileges, pull, inspect, remove, enable, disable, upgrade, create, push, set |
| **Distribution** | 1 | Get image descriptor/manifest from registry |
| **Session** | 1 | Start interactive session (BuildKit, h2c upgrade) |

### 1.2 Key Protocol Features

| Feature | Detail |
|---------|--------|
| **Versioning** | URL path prefix `/v1.45/...`; server negotiates down to min v1.24 |
| **Content types** | `application/json` (most); `application/x-tar` (build context, image export/import, archive) |
| **Authentication** | Unix socket file permissions; TCP with mTLS; `X-Registry-Auth` header (base64 JSON) for registry ops |
| **Streaming** | Multiplexed stream (8-byte header: stream type + length) for non-TTY; raw stream for TTY |
| **WebSocket** | `GET /containers/{id}/attach/ws` — only WebSocket endpoint |
| **Connection hijacking** | Attach and exec endpoints hijack HTTP connection for bidirectional I/O |
| **Error format** | `{"message": "..."}` with standard HTTP status codes |

---

## 2. Implementation Comparison Matrix

### 2.1 High-Level Overview

| Implementation | Type | Runs Real `dockerd`? | Docker API Compat | Swarm Support | Plugin Support | Build Engine | Maintained? |
|---|---|---|---|---|---|---|---|
| **Docker Engine** | Reference | Yes | 100% (baseline) | Full | Full | BuildKit | Yes |
| **Docker Desktop** | Wrapper | Yes | 100% | Full | Full | BuildKit | Yes |
| **Mirantis CR (MCR)** | Enterprise distro | Yes | 100% | Full | Full | BuildKit | Yes |
| **Podman** | Reimplementation | No | High (~90%) | None | None | Buildah | Yes |
| **Rancher Desktop (dockerd)** | Wrapper | Yes | Very High (~98%) | None | Limited | BuildKit | Yes |
| **Rancher Desktop (containerd)** | Shim | No | Partial (~70%) | None | None | BuildKit | Yes |
| **Colima** | Wrapper | Yes | Very High (~99%) | Single-node | Limited | BuildKit | Yes |
| **Balena Engine** | Fork (v1.39) | Yes (forked) | High (~95% of v1.39) | Unsupported | Limited | Legacy builder | Yes |
| **Testcontainers Cloud** | Proxy/Shim | No | High (subset) | None | None | Limited | Yes |
| **Fly.io Machines** | Proprietary | No | None (0%) | N/A | N/A | Remote BuildKit | Yes |

### 2.2 Per-Category Compatibility

Legend: **Full** = identical behavior, **High** = works with minor differences, **Partial** = basic operations work, **None** = not implemented, **N/A** = not applicable

| Endpoint Category | Docker Engine | Podman | Rancher (dockerd) | Rancher (containerd) | Colima | Balena Engine | TC Cloud |
|---|---|---|---|---|---|---|---|
| Containers (CRUD) | Full | High | Full | High | Full | Full | High |
| Container Exec | Full | High | Full | High | Full | Full | High |
| Container Attach | Full | High | Full | High | Full | Full | Partial |
| Container Logs | Full | High | Full | High | Full | Full | High |
| Container Stats | Full | High | Full | High | Full | Full | Partial |
| Images (CRUD) | Full | High | Full | High | Full | Full | High |
| Image Build | Full | High | Full | Partial | Full | Full (legacy) | Limited |
| Networks | Full | High | Full | Partial | Full | Full | High |
| Volumes | Full | High | Full | Partial | Full | Full | High |
| System/Info/Ping | Full | High | Full | High | Full | Full | High |
| Events | Full | High | Full | Partial | Full | Full | Partial |
| Swarm | Full | **None** | **None** | **None** | Single-node | Unsupported | **None** |
| Nodes | Full | **None** | **None** | **None** | Single-node | Unsupported | **None** |
| Services | Full | **None** | **None** | **None** | Single-node | Unsupported | **None** |
| Tasks | Full | **None** | **None** | **None** | Single-node | Unsupported | **None** |
| Secrets | Full | **None** | **None** | **None** | Single-node | Unsupported | **None** |
| Configs | Full | **None** | **None** | **None** | Single-node | Unsupported | **None** |
| Plugins | Full | **None** | Limited | **None** | Limited | Limited | **None** |
| Distribution | Full | High | Full | Partial | Full | Full | Partial |
| Session (BuildKit) | Full | **None** | Full | **None** | Full | **None** | **None** |

---

## 3. Podman

### 3.1 Overview

| Property | Value |
|----------|-------|
| **Project** | [github.com/containers/podman](https://github.com/containers/podman) |
| **Latest version** | 5.x (2024-2025) |
| **Target Docker API** | v1.40 (Docker 19.03) baseline, incremental additions from v1.41-v1.44 |
| **Architecture** | Daemonless; on-demand REST API via `podman system service` |
| **Socket path** | Rootful: `/run/podman/podman.sock`; Rootless: `$XDG_RUNTIME_DIR/podman/podman.sock` |
| **Dual API** | Compat API (Docker-compatible) + Libpod API (Podman-native), served on same socket |
| **License** | Apache 2.0 |

### 3.2 Compatibility Details

Podman exposes two parallel API surfaces on the same HTTP server:

1. **Compat API** — Routes matching Docker API paths (`/v1.40/containers/json`, etc.)
2. **Libpod API** — Routes under `/v1.40/libpod/` with Podman-native extensions (pods, manifests, kube play, CRIU checkpoint/restore)

### 3.3 Known Incompatibilities

#### Critical (Breaking)

| Area | Incompatibility | Impact |
|------|-----------------|--------|
| **Swarm** | Entirely absent. `/swarm/*`, `/services/*`, `/tasks/*`, `/nodes/*`, `/secrets/*`, `/configs/*` return errors | Any Docker Swarm workflow fails |
| **Plugins** | Docker plugin system (`/plugins/*`) not implemented | No managed plugins; limited volume plugin support via separate mechanism |
| **BuildKit** | Not supported. Uses Buildah instead | `# syntax=` directives fail; `RUN --mount=type=cache/secret/ssh` partially supported via Buildah; BuildKit session protocol absent; `--cache-from`/`--cache-to` behavior differs |
| **Session endpoint** | `POST /session` not implemented | BuildKit gRPC session unavailable |

#### Moderate (Behavioral Differences)

| Area | Difference | Detail |
|------|------------|--------|
| **Container Inspect** | Schema differences | `NetworkSettings.Networks` may differ; `State` has extra fields (`ConmonPid`); `HostConfig` may omit Docker-specific options |
| **Image references** | Fully-qualified names required by default | `nginx` fails; must use `docker.io/library/nginx` unless `unqualified-search-registries` configured in `/etc/containers/registries.conf` |
| **Network drivers** | Only `bridge`, `macvlan`, `host`, `none` | No `overlay` driver (Swarm-dependent); uses Netavark + aardvark-dns instead of libnetwork |
| **System Info** | Different field values | `Swarm` always inactive; `SecurityOptions` shows SELinux/AppArmor; `Runtimes` lists `crun`/`runc` not containerd |
| **Build cache** | Buildah cache differs from Docker/BuildKit | Different layer caching behavior; no shared build cache |
| **Container logs** | Stream framing edge cases | Multiplexed stream 8-byte header generally works; historical edge cases with non-TTY framing |
| **host.docker.internal** | Not automatic | Must use `--add-host=host.docker.internal:host-gateway` or rely on `host.containers.internal` |
| **Rootless networking** | Uses pasta/slirp4netns | Userspace networking; ports < 1024 require `net.ipv4.ip_unprivileged_port_start` sysctl |
| **Idle timeout** | API server shuts down when idle | Default 5-second timeout; use `--time 0` for persistent service |

#### Minor

| Area | Difference |
|------|------------|
| Volume drivers | `local` works; third-party Docker volume plugins have limited support |
| Exec `ConsoleSize` | Option from Docker API v1.42 may not be supported in all Podman versions |
| Container Update | Supported but may not cover all Docker `HostConfig` resource fields |
| Docker context | `docker context` commands not supported; use `DOCKER_HOST` env var |
| Events | Event types/format compatible; some Podman-specific event types may appear |

### 3.4 Podman-Only Extensions (Libpod API)

These endpoints are available under `/libpod/` and have no Docker equivalent:

| Feature | Endpoints | Description |
|---------|-----------|-------------|
| **Pods** | `/libpod/pods/*` | Kubernetes-style pod grouping (create, start, stop, inspect, stats, prune) |
| **Manifests** | `/libpod/manifests/*` | Multi-arch manifest list management |
| **Kube Play** | `POST /libpod/play/kube` | Deploy from Kubernetes YAML |
| **Kube Generate** | `POST /libpod/generate/{name}/kube` | Generate Kubernetes YAML from containers/pods |
| **Checkpoint/Restore** | `/libpod/containers/{name}/checkpoint`, `/restore` | CRIU-based container checkpointing |
| **Secrets** | `/libpod/secrets/*` | Podman-native secrets (not Swarm secrets) |
| **Healthcheck** | `GET /libpod/containers/{name}/healthcheck` | Run healthcheck on demand |
| **Mount/Unmount** | `/libpod/containers/{name}/mount`, `/unmount` | Mount/unmount container filesystem |

### 3.5 Docker SDK Compatibility

| SDK | Status | Notes |
|-----|--------|-------|
| Python (`docker` / `docker-py`) | Works | Set `base_url` to Podman socket; may need explicit `version='1.40'` |
| Node.js (`dockerode`) | Works | Set `socketPath` to Podman socket |
| Go (`docker/client`) | Works | Set `DOCKER_HOST` to Podman socket |
| Docker Compose v2 | Works (mostly) | `depends_on` with health checks works; Swarm deploy keys ignored/fail; BuildKit build options may fail |

---

## 4. Rancher Desktop

### 4.1 Overview

| Property | Value |
|----------|-------|
| **Project** | [github.com/rancher-sandbox/rancher-desktop](https://github.com/rancher-sandbox/rancher-desktop) |
| **Latest version** | v1.14.x – v1.16.x (2024-2025) |
| **Architecture** | Electron app managing a Linux VM (Lima/QEMU on macOS, WSL2 on Windows) |
| **Runtime backends** | `dockerd` (moby) OR `containerd` — user-selectable |
| **Socket path** | `~/.rd/docker.sock` (symlinked to `/var/run/docker.sock`) |
| **Docker context** | Creates `rancher-desktop` context pointing to its socket |
| **License** | Apache 2.0 |

### 4.2 dockerd Backend — High Compatibility

When using the dockerd backend, Rancher Desktop runs the real Docker Engine inside its VM. API compatibility is the same as running Docker Engine natively, with these exceptions:

| Limitation | Detail |
|------------|--------|
| **Swarm** | Deliberately not supported (Rancher Desktop focuses on Kubernetes) |
| **Plugins** | Limited — VM environment may not support plugin installation |
| **Host networking** | `--network host` refers to VM's network, not macOS/Windows host |
| **Volume mounts** | Host dirs shared via virtiofs/9p/sshfs; performance varies; path must be in shared directory |
| **Port forwarding** | Handled by Rancher Desktop's own mechanism; edge cases with UDP or high port counts |

### 4.3 containerd Backend — Partial Compatibility

When using the containerd backend, a Docker API compatibility shim translates HTTP requests into containerd gRPC calls. Key limitations:

| Area | Status |
|------|--------|
| Container CRUD | Works |
| Exec/Attach | Works |
| Image operations | Works |
| Build (`/build`) | Partial — uses BuildKit directly; tools calling Docker build API over socket may see issues |
| Networks | Partial — CNI-based, not libnetwork; advanced features may differ |
| Volumes | Basic operations only; no Docker volume drivers |
| Events | Partial — may not produce identical events |
| Inspect output | JSON structure may differ in field names/nesting |
| Swarm/Plugins | None |

### 4.4 Switching Between Backends

Switching requires a Rancher Desktop VM restart. Images are **not shared** between backends (different storage systems). Users needing full Docker API compatibility should use the dockerd backend.

---

## 5. Colima and Docker Desktop

### 5.1 Docker Desktop

| Property | Value |
|----------|-------|
| **Product** | Docker Desktop (macOS, Windows, Linux) |
| **Architecture** | GUI application running a Linux VM (Apple Hypervisor / HyperKit on macOS, Hyper-V / WSL2 on Windows) with `dockerd` inside |
| **Docker API version** | Same as bundled Docker Engine (v1.44-v1.45 in Docker Desktop 4.x) |
| **Socket path** | `/var/run/docker.sock` (macOS/Linux); `npipe:////./pipe/docker_engine` (Windows) |
| **License** | Proprietary; free for personal/education/small business; paid for larger organizations |

Docker Desktop runs the real Docker Engine inside a managed VM. The REST API is **100% identical** to bare-metal Docker Engine. Additional proprietary features (Extensions, Docker Scout, Docker Init, Dev Environments) use separate APIs/CLIs and do not affect the core Docker Engine REST API.

### 5.2 Mirantis Container Runtime (MCR)

| Property | Value |
|----------|-------|
| **Previously** | Docker Enterprise Engine |
| **Base** | Moby/Docker Engine with enterprise patches |
| **Docker API version** | Tracks upstream — v1.43-v1.45 depending on MCR version |
| **License** | Proprietary/commercial |
| **Maintained** | Yes (commercial product with paid support) |

MCR is Docker Engine with enterprise features (FIPS 140-2 compliance, certified plugins, extended support lifecycle). The Docker REST API is **100% identical** to community Docker Engine. The Mirantis Kubernetes Engine (MKE, formerly Docker UCP) adds its own management API on a different port but does not alter the core Docker Engine API.

### 5.3 Colima Overview

| Property | Value |
|----------|-------|
| **Project** | [github.com/abiosoft/colima](https://github.com/abiosoft/colima) |
| **Latest version** | v0.8.x (2024-2025) |
| **Architecture** | Lima VM manager running a Linux VM with QEMU or macOS Virtualization.framework |
| **Runtime** | Full `dockerd` inside VM (default) or `containerd` via `--runtime containerd` |
| **Socket path** | `~/.colima/default/docker.sock` |
| **Docker API version** | v1.44 or v1.45 (matches bundled Docker Engine 24.x/25.x) |
| **License** | MIT |

### 5.4 Compatibility

Colima runs **unmodified Docker Engine** (`dockerd`). The REST API is **100% identical** to bare-metal Docker because the same `dockerd` binary handles all requests. Any HTTP client will get responses from genuine Docker Engine.

| Area | Compatibility | Notes |
|------|--------------|-------|
| All Container APIs | Full | Same dockerd |
| All Image APIs | Full | Same dockerd |
| All Network APIs | Full | Same dockerd; `--network=host` refers to VM network |
| All Volume APIs | Full | Same dockerd; mount performance depends on VM filesystem sharing |
| Swarm | Functional (single-node) | Works because dockerd supports it; multi-node requires manual networking |
| Plugins | Limited | Depends on VM kernel support |
| Build/BuildKit | Full | BuildKit included; `docker buildx` works with QEMU emulation for multi-platform |

### 5.5 Operational Differences (Not API-Level)

| Difference | Detail |
|------------|--------|
| Volume mount performance | Slower than native with QEMU/sshfs; use `--vm-type vz --mount-type virtiofs` on macOS 13+ for near-native |
| Shared directories | Only `~` shared by default; paths outside fail in bind mounts |
| GPU passthrough | Not available through VM layer |
| Docker Desktop extensions | Not supported (proprietary) |

### 5.6 containerd Runtime Mode

With `colima start --runtime containerd`, no dockerd runs and **no Docker REST API is available**. Only `nerdctl` CLI works. This mode is unsuitable for any tool requiring the Docker socket API.

### 5.7 Comparison: Docker Desktop vs Colima vs Rancher Desktop (dockerd)

| Aspect | Docker Desktop | Colima | Rancher Desktop (dockerd) |
|--------|----------------|--------|---------------------------|
| Docker API | Identical (same dockerd) | Identical (same dockerd) | Identical (same dockerd, no Swarm) |
| License | Proprietary | MIT | Apache 2.0 |
| VM technology | Apple Hypervisor / LinuxKit | QEMU or macOS VZ (via Lima) | Lima / WSL2 |
| Docker Extensions | Yes | No | No |
| Docker Scout, Init | Yes | No | No |
| Kubernetes | Built-in (optional) | Via k3s (optional) | Built-in k3s |
| Socket path | `/var/run/docker.sock` | `~/.colima/default/docker.sock` | `~/.rd/docker.sock` |

---

## 6. Balena Engine

### 6.1 Overview

| Property | Value |
|----------|-------|
| **Project** | [github.com/balena-os/balena-engine](https://github.com/balena-os/balena-engine) |
| **Fork of** | Moby (Docker Engine), forked around Docker 18.09.x |
| **Docker API version** | **v1.39** (frozen at fork point; does not track upstream) |
| **Socket path** | `/var/run/balena-engine.sock` (also `/var/run/balena.sock`) |
| **Target platform** | ARM and x86 IoT/embedded devices (Raspberry Pi, Jetson, Intel NUC, etc.) |
| **License** | Apache 2.0 |
| **Maintained** | Yes (by Balena for their IoT fleet platform) |

### 6.2 What It Is

Balena Engine is a **hard fork** of Docker/Moby, not a reimplementation. It is Docker Engine v18.09 with IoT-specific patches. Because it is a fork, it exposes the exact same REST HTTP API as Docker Engine v1.39, including identical endpoint paths, request/response schemas, and protocol behaviors.

### 6.3 Modifications from Upstream Docker

| Modification | Detail |
|---|---|
| **Delta image updates** | Proprietary delta-update mechanism to minimize bandwidth for OTA pulls. Uses same `POST /images/create` endpoint with custom headers |
| **Atomic image pulls** | Pulls are atomic — if interrupted, they roll back completely (critical for unreliable IoT networks) |
| **Smaller binary** | Stripped features to reduce binary size for constrained devices |
| **No BuildKit** | Fork predates BuildKit integration; `POST /build` uses legacy builder |
| **No Swarm** | Code from fork may exist but is not supported or tested |
| **Persistent logging** | Modified logging driver behavior for constrained storage |

### 6.4 Compatibility

| Area | Compatibility | Notes |
|------|--------------|-------|
| Containers CRUD | Full (v1.39) | Identical to Docker 18.09 |
| Images CRUD | Full + delta extensions | Delta pull is additive, not breaking |
| Networks | Full (v1.39) | |
| Volumes | Full (v1.39) | |
| Exec | Full (v1.39) | |
| System/Info/Ping | Full | Reports as `balena-engine` |
| Build (legacy) | Full | No BuildKit; legacy builder only |
| BuildKit/Session | **None** | Fork predates BuildKit |
| Swarm | Unsupported | Code present but untested |
| Plugins | Limited | |
| API version | **Frozen at v1.39** | Does not support v1.40+ features (e.g., `ConsoleSize` in exec, platform in pull) |

### 6.5 Key Limitation

The API is frozen at **v1.39** (Docker 18.09 era). Features added in Docker API v1.40+ are absent:
- No `platform` parameter on `POST /images/create` (multi-arch pull)
- No `ConsoleSize` in exec create
- No cluster volume support
- No BuildKit session endpoint
- Missing various v1.40-v1.45 schema additions

Tools and SDKs that negotiate API version and work with v1.39 will function correctly. Tools requiring v1.40+ features will fail.

---

## 7. Testcontainers Cloud

### 7.1 Overview

| Property | Value |
|----------|-------|
| **Product** | Testcontainers Cloud (by AtomicJar, acquired by Docker Inc in 2023) |
| **Architecture** | Local agent exposes Docker-compatible socket; container execution happens in the cloud |
| **Socket** | Local Unix socket or TCP endpoint mimicking Docker Engine API |
| **Docker API compat** | High (for Testcontainers library use cases) |
| **License** | Proprietary SaaS |
| **Maintained** | Yes (now part of Docker Inc) |

### 7.2 How It Works

Testcontainers Cloud runs a local agent (`testcontainers-cloud-agent`) that:

1. Listens on a Docker-compatible socket (or sets `DOCKER_HOST`)
2. Intercepts Docker API calls from the Testcontainers library
3. Forwards container operations to cloud-hosted infrastructure
4. Returns Docker Engine API-conformant responses

From the client's perspective, it behaves like a local Docker Engine. The actual containers run remotely on cloud infrastructure.

### 7.3 Compatibility

| Area | Status | Notes |
|------|--------|-------|
| Container CRUD | High | Create, start, stop, inspect, remove work |
| Container Logs | High | Streaming supported |
| Container Exec | High | |
| Image Pull | High | Images pulled in the cloud, not locally |
| Networks | High | Created in the cloud |
| Volumes | High | Cloud-side volumes |
| Build | Limited | Some build scenarios differ |
| Swarm | None | Not needed for test containers |
| Plugins | None | |
| System/Ping | High | Reports as Testcontainers Cloud |

### 7.4 Key Limitations

- Only implements the Docker API subset that Testcontainers libraries use — not a general-purpose Docker replacement
- Network behavior differs (containers run remotely; `localhost` access patterns change)
- Port mapping works differently (ports forwarded from the cloud)
- Not suitable as a general Docker daemon substitute

---

## 8. Proprietary and Non-Docker APIs

This section covers projects with their own REST APIs that are **not** Docker-compatible. They are included for reference and concept mapping.

**Key finding:** No actively maintained micro-VM project provides a Docker-compatible REST HTTP API. Micro-VM technologies operate at the VMM or OCI runtime layer, below the Docker API in the stack.

### 8.1 Fly.io Machines — Proprietary API

| Property | Value |
|----------|-------|
| **Project** | [fly.io](https://fly.io) |
| **API** | Machines API — completely proprietary REST API |
| **Base URL** | `https://api.machines.dev/v1/` |
| **Auth** | Bearer token (`Authorization: Bearer <fly-api-token>`) |
| **VMM** | Firecracker micro-VMs |
| **Docker API compat** | **None (0%)** |
| **Maintained** | Yes (primary Fly.io platform) |

#### Concept Mapping

| Docker Concept | Fly.io Equivalent | Notes |
|---|---|---|
| Container | Machine | Firecracker micro-VM with its own kernel |
| `docker run` | `POST /apps/{app}/machines` | Create and start a Machine |
| `docker start/stop` | `POST .../start`, `POST .../stop` | Similar lifecycle |
| `docker exec` | `POST .../exec` | Execute inside running Machine |
| `docker ps` | `GET /apps/{app}/machines` | Scoped to app, not global |
| Volume | Volume | NVMe-backed, region-pinned |
| Network | Private Network (6PN) | WireGuard-based; no user-defined networks |
| Image | OCI Image | Standard Docker/OCI images accepted |
| Build | Remote BuildKit or local | Via `fly deploy` |

**Key architectural difference:** Fly.io Machines are full Firecracker micro-VMs with separate kernels. There is no Docker socket, no Docker daemon, and no compatibility layer. The Machines API is the sole control plane.

### 8.2 Kata Containers — OCI Runtime (No Own API)

| Property | Value |
|----------|-------|
| **Project** | [github.com/kata-containers/kata-containers](https://github.com/kata-containers/kata-containers) |
| **Type** | OCI-compatible container runtime (same layer as `runc`) |
| **VMMs** | QEMU (default), Cloud Hypervisor (recommended), Firecracker (limited), Dragonball |
| **Docker API** | Does not implement any — runs **underneath** Docker as a runtime |
| **Maintained** | Yes (OpenInfra Foundation, 3.x series) |

Kata Containers is transparent at the API level. Docker does not know whether `runc` or `kata-runtime` is executing a container. All Docker API endpoints work normally because `dockerd` handles the API. However, some **behavioral differences** exist due to the VM boundary:

| Affected Operation | Impact |
|--------------------|--------|
| `--privileged` | Elevated privileges inside guest VM, not host |
| `--pid=host` | Meaningless — VM has its own kernel/PID namespace |
| `--device` | Requires VFIO passthrough; Firecracker does not support hotplug |
| `docker checkpoint` (CRIU) | Not supported across VM boundary |
| `docker stats` | cgroup metrics track VM process, not individual container processes |
| `docker update` (resource limits) | Requires VMM hotplug support (QEMU yes, Firecracker no) |
| Volume I/O | Additional latency from virtio-fs/9pfs |
| Container start latency | +100-500ms from VM boot overhead |
| `docker build` | Works but slow (VM start/stop per build step) |

### 8.3 Firecracker — VMM (No Docker API)

| Property | Value |
|----------|-------|
| **Project** | [github.com/firecracker-microvm/firecracker](https://github.com/firecracker-microvm/firecracker) |
| **Type** | Virtual Machine Monitor (VMM) |
| **API** | Own REST API over Unix socket for VM configuration/lifecycle |
| **Docker API compat** | None — operates at VM level, not container level |
| **Maintained** | Yes (AWS; used by Lambda and Fargate) |

Firecracker's API (`PUT /machine-config`, `PUT /boot-source`, `PUT /drives/{id}`, `PUT /actions`) manages VMs, not containers. No concept of images, Dockerfiles, or container networking.

### 8.4 Cloud Hypervisor — VMM (No Docker API)

| Property | Value |
|----------|-------|
| **Project** | [github.com/cloud-hypervisor/cloud-hypervisor](https://github.com/cloud-hypervisor/cloud-hypervisor) |
| **Type** | Virtual Machine Monitor (VMM), Rust-based |
| **API** | Own REST API over Unix socket (`/api/v1/vm.create`, `/api/v1/vm.boot`, etc.) |
| **Docker API compat** | None |
| **Maintained** | Yes (Linux Foundation) |

Similar role to Firecracker but with more features (hot-plug, VFIO, vDPA, live migration). Used as a VMM backend in Kata Containers.

### 8.5 LXD / Incus — Own REST API (Not Docker-Compatible)

| Property | LXD | Incus |
|----------|-----|-------|
| **Maintainer** | Canonical | Linux Containers community |
| **API** | Own REST API (`/1.0/instances`, `/1.0/images`, etc.) | Same (forked from LXD) |
| **Docker API compat** | None | None |
| **VM technology** | QEMU (full VMs), not micro-VMs | QEMU (full VMs), not micro-VMs |
| **Maintained** | Yes | Yes |

Both have comprehensive REST APIs over Unix socket / HTTPS but with completely different endpoints, object models, and semantics from Docker.

---

## 9. Per-Endpoint Compatibility Detail

### 9.1 Containers

| Endpoint | Docker | Podman | Notes |
|----------|--------|--------|-------|
| `GET /containers/json` | Yes | Yes | Filters work; minor differences in output fields |
| `POST /containers/create` | Yes | Yes | Most `HostConfig` fields work; some obscure Swarm-related options silently ignored |
| `GET /containers/{id}/json` | Yes | Yes | `NetworkSettings`, `State` have Podman-specific fields; `ConmonPid` present |
| `GET /containers/{id}/top` | Yes | Yes | |
| `GET /containers/{id}/logs` | Yes | Yes | Multiplexed stream format supported; historical edge cases in framing |
| `GET /containers/{id}/changes` | Yes | Yes | |
| `GET /containers/{id}/export` | Yes | Yes | |
| `GET /containers/{id}/stats` | Yes | Yes | |
| `POST /containers/{id}/resize` | Yes | Yes | |
| `POST /containers/{id}/start` | Yes | Yes | |
| `POST /containers/{id}/stop` | Yes | Yes | |
| `POST /containers/{id}/restart` | Yes | Yes | |
| `POST /containers/{id}/kill` | Yes | Yes | |
| `POST /containers/{id}/update` | Yes | Partial | May not cover all resource fields |
| `POST /containers/{id}/rename` | Yes | Yes | |
| `POST /containers/{id}/pause` | Yes | Yes | |
| `POST /containers/{id}/unpause` | Yes | Yes | |
| `POST /containers/{id}/attach` | Yes | Yes | Edge cases with TTY handling |
| `GET /containers/{id}/attach/ws` | Yes | Yes | WebSocket attach supported |
| `POST /containers/{id}/wait` | Yes | Yes | |
| `DELETE /containers/{id}` | Yes | Yes | `force` and `v` params work |
| `HEAD /containers/{id}/archive` | Yes | Yes | |
| `GET /containers/{id}/archive` | Yes | Yes | |
| `PUT /containers/{id}/archive` | Yes | Yes | |
| `POST /containers/prune` | Yes | Yes | |

### 9.2 Images

| Endpoint | Docker | Podman | Notes |
|----------|--------|--------|-------|
| `GET /images/json` | Yes | Yes | |
| `POST /build` | Yes | Yes (Buildah) | No BuildKit; `# syntax=` fails; `RUN --mount` partially supported via Buildah |
| `POST /build/prune` | Yes | Partial | Buildah cache differs |
| `POST /images/create` (pull) | Yes | Yes | `X-Registry-Auth` works; multi-platform pull works |
| `GET /images/{name}/json` | Yes | Yes | Minor `RootFS` / layer differences |
| `GET /images/{name}/history` | Yes | Yes | |
| `POST /images/{name}/push` | Yes | Yes | |
| `POST /images/{name}/tag` | Yes | Yes | |
| `DELETE /images/{name}` | Yes | Yes | |
| `POST /images/search` | Yes | Yes | |
| `POST /images/prune` | Yes | Yes | |
| `GET /images/get` (export) | Yes | Yes | |
| `POST /images/load` | Yes | Yes | |
| `POST /commit` | Yes | Yes | |

### 9.3 Networks

| Endpoint | Docker | Podman | Notes |
|----------|--------|--------|-------|
| `GET /networks` | Yes | Yes | |
| `GET /networks/{id}` | Yes | Yes | IPAM fields may differ; no `Scope: "swarm"` |
| `POST /networks/create` | Yes | Yes | `bridge`, `macvlan`, `host`, `none` only; no `overlay` |
| `POST /networks/{id}/connect` | Yes | Yes | |
| `POST /networks/{id}/disconnect` | Yes | Yes | |
| `DELETE /networks/{id}` | Yes | Yes | |
| `POST /networks/prune` | Yes | Yes | |

### 9.4 Volumes

| Endpoint | Docker | Podman | Notes |
|----------|--------|--------|-------|
| `GET /volumes` | Yes | Yes | |
| `POST /volumes/create` | Yes | Yes | `local` driver; limited third-party plugin support |
| `GET /volumes/{name}` | Yes | Yes | `Options`/`Status` may differ slightly |
| `PUT /volumes/{name}` | Yes | No | Cluster volumes (Swarm) not supported |
| `DELETE /volumes/{name}` | Yes | Yes | |
| `POST /volumes/prune` | Yes | Yes | |

### 9.5 Exec

| Endpoint | Docker | Podman | Notes |
|----------|--------|--------|-------|
| `POST /containers/{id}/exec` | Yes | Yes | All common fields supported |
| `POST /exec/{id}/start` | Yes | Yes | |
| `POST /exec/{id}/resize` | Yes | Yes | |
| `GET /exec/{id}/json` | Yes | Yes | |

### 9.6 System

| Endpoint | Docker | Podman | Notes |
|----------|--------|--------|-------|
| `POST /auth` | Yes | Yes | |
| `GET /info` | Yes | Yes | Swarm=inactive; different `SecurityOptions`, `Runtimes` |
| `GET /version` | Yes | Yes | Reports Podman version; `ApiVersion` reflects compat target |
| `GET /_ping` | Yes | Yes | Returns `OK`, `API-Version` header |
| `HEAD /_ping` | Yes | Yes | |
| `GET /events` | Yes | Yes | Streaming works; Podman-specific event types may appear |
| `GET /system/df` | Yes | Yes | |

### 9.7 Swarm-Dependent Endpoints (Not in Podman)

All of the following return errors or are entirely absent in Podman:

- `GET/POST /swarm`, `/swarm/init`, `/swarm/join`, `/swarm/leave`, `/swarm/update`, `/swarm/unlockkey`, `/swarm/unlock`
- `GET/POST/DELETE /services/*`
- `GET /tasks/*`
- `GET/POST/DELETE /nodes/*`
- `GET/POST/DELETE /secrets/*`
- `GET/POST/DELETE /configs/*`

### 9.8 Plugins (Not in Podman)

All `/plugins/*` endpoints are unimplemented in Podman.

---

## 10. Projects Evaluated and Excluded

The following projects were evaluated but do not implement the Docker REST HTTP API:

| Project | Type | Why Excluded |
|---------|------|-------------|
| **nerdctl** | CLI for containerd | CLI-only compatibility; no REST API server; talks to containerd via gRPC |
| **Sysbox (Nestybox)** | OCI runtime | Plugs in under Docker as OCI runtime (`--runtime=sysbox-runc`); no own API; now owned by Docker Inc |
| **gVisor (runsc)** | OCI runtime | Plugs in under Docker as OCI runtime; no own API |
| **crun / youki** | OCI runtimes | Same layer as runc; no own API |
| **libkrun** | Micro-VM OCI runtime | OCI runtime with KVM isolation; no own API |
| **Weaveworks Ignite** | Firecracker container manager | Docker-like CLI only (no REST API); project dead (Weaveworks liquidated 2024) |
| **Flintlock** | Micro-VM manager | gRPC API only; project dead (Weaveworks) |
| **firecracker-containerd** | containerd runtime shim | Containerd shim for Firecracker; no Docker API |
| **HyperContainer (hyperd)** | Micro-VM container runtime | Had partial Docker API compat; dead since ~2019 |
| **Vorteil** | Docker-to-micro-VM converter | Own API; project dormant |
| **Unikraft / NanoVMs** | Unikernel platforms | Own tooling; no Docker API compat |
| **Singularity / Apptainer** | HPC container runtime | Own CLI/API; pulls Docker images but no Docker REST API |
| **Buildah** | OCI image builder | CLI and Go library only; no REST API server (exposed via Podman's API) |
| **cri-dockerd (Mirantis)** | CRI-to-Docker adapter | Client of Docker API, not a server; translates CRI gRPC to Docker REST |
| **Docker Socket Proxy** | Security proxy | Filters/restricts Docker API access; proxies to real dockerd, not a reimplementation |
| **AWS ECS / Fargate** | Cloud service | Proprietary AWS API; no Docker REST API endpoint |
| **Azure Container Instances** | Cloud service | Proprietary Azure API; Docker CLI integration removed in 2023 |
| **Google Cloud Run** | Cloud service | Proprietary GCP API; accepts OCI images but no Docker REST API |

---

## References

- Docker Engine API v1.45: [docs.docker.com/engine/api/v1.45](https://docs.docker.com/engine/api/v1.45/)
- Docker Desktop: [docs.docker.com/desktop](https://docs.docker.com/desktop/)
- Mirantis Container Runtime: [docs.mirantis.com/mcr](https://docs.mirantis.com/mcr/)
- Podman API (Swagger): [docs.podman.io/en/latest/_static/api.html](https://docs.podman.io/en/latest/_static/api.html)
- Podman system service: [docs.podman.io/en/latest/markdown/podman-system-service.1.html](https://docs.podman.io/en/latest/markdown/podman-system-service.1.html)
- Rancher Desktop: [docs.rancherdesktop.io](https://docs.rancherdesktop.io/)
- Colima: [github.com/abiosoft/colima](https://github.com/abiosoft/colima)
- Balena Engine: [github.com/balena-os/balena-engine](https://github.com/balena-os/balena-engine)
- Testcontainers Cloud: [testcontainers.com/cloud](https://testcontainers.com/cloud/)
- Fly.io Machines API: [fly.io/docs/machines/api](https://fly.io/docs/machines/api/)
- Kata Containers: [katacontainers.io](https://katacontainers.io/)
- Firecracker: [github.com/firecracker-microvm/firecracker](https://github.com/firecracker-microvm/firecracker)
- Cloud Hypervisor: [github.com/cloud-hypervisor/cloud-hypervisor](https://github.com/cloud-hypervisor/cloud-hypervisor)
- nerdctl: [github.com/containerd/nerdctl](https://github.com/containerd/nerdctl)
- LXD: [documentation.ubuntu.com/lxd](https://documentation.ubuntu.com/lxd/)
- Incus: [github.com/lxc/incus](https://github.com/lxc/incus)
