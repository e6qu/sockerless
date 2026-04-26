# Backend State Model Specification

How backends track containers, pods, networks, and volumes using cloud-native tags/labels as the single source of truth.

> **See also:** [CLOUD_RESOURCE_MAPPING.md](CLOUD_RESOURCE_MAPPING.md) — per-cloud authoritative mapping table (ECS task → docker container/pod, sockerless-tagged SG + Cloud Map namespace → docker network, etc.), state-derivation rules per backend, stateless recovery contract.

## Principle: Stateless Backends

Backends maintain **no local container state**. Every Docker API call (`docker ps`, `docker inspect`, etc.) queries the cloud/simulator for resources tagged with Sockerless metadata. The cloud is the source of truth.

This means:
- No `Store.Containers` map
- No `sockerless-registry.json`
- No in-memory container cache
- Backend can restart at any time without losing track of containers
- Multiple backend instances can point at the same cluster and see the same containers

## Identity Model

Each backend identifies its resources by **cluster + backend type**, not by hostname:

```
Tags:
  sockerless-managed:      true
  sockerless-cluster:      sockerless-live
  sockerless-backend:      ecs
```

Two backends pointing at the same ECS cluster with the same config see the same containers. This is intentional — it enables horizontal scaling and seamless restarts.

## Resource Tagging

### Containers (ECS tasks, Lambda functions, Cloud Run executions, ACA jobs)

```
Tags (minimal — only what has no cloud-native equivalent):
  sockerless-managed:      true                    # discovery marker
  sockerless-cluster:      sockerless-live          # backend identity
  sockerless-backend:      ecs                      # backend type
  sockerless-container-id: abc123def456789...       # Docker container ID (no cloud equivalent)
  sockerless-name:         /my-nginx                # Docker name (cloud names differ)
  sockerless-network:      testnet                  # Docker network (no cloud equivalent)
  sockerless-pod:          mypod                    # Pod membership (no cloud equivalent)
  sockerless-labels:       {"app":"web"}            # Docker labels (no cloud equivalent)
```

**Derived from cloud resource** (never stored in tags):
- `image` → task definition / function config / job spec
- `cmd`, `entrypoint`, `workdir` → task definition / function config
- `env` → task definition / function config (secret; never in tags)
- `memory`, `cpu` → task definition / function config
- `status`, `exit code` → task/execution status API
- `started_at`, `stopped_at` → task/execution timestamps
- `ip_address` → ENI attachment / execution metadata
- `created_at` → task/execution creation time

Full container config is derivable from the cloud resource:
- **ECS**: Task definition → image, cmd, entrypoint, env, workdir, memory, cpu
- **Lambda**: Function config → image, handler, memory, timeout, env
- **Cloud Run**: Job spec → image, cmd, env, resources
- **ACA**: Job spec → image, cmd, env, resources

### Data Source Priority

1. **Cloud resource config** (primary): Task definition, function config, job spec → image, cmd, entrypoint, env, workdir, memory, cpu, exposed ports
2. **Cloud resource state** (primary): Task status, function invocation, execution status → running, stopped, exit code, started/stopped timestamps, IP address
3. **Cloud resource naming** (primary): Task definition family, function name → derive container name when possible
4. **Tags** (last resort): Only for data that has no native cloud equivalent — Docker container ID, Docker-specific name format, Docker network membership, pod association, Docker labels

Tags are a supplement, not the primary data source. If the cloud API provides the information natively (which it does for image, command, environment, resources, status), use the cloud API.

### Pods

Pods are tracked as a group of containers sharing the same `sockerless-pod` tag value. No separate cloud resource is needed for the pod itself — it's a logical grouping.

```
# Container 1 in pod:
  sockerless-pod:          mypod
  sockerless-pod-role:     main        # or: sidecar

# Container 2 in pod:
  sockerless-pod:          mypod
  sockerless-pod-role:     sidecar
```

Pod listing (`podman pod ls`) queries for distinct `sockerless-pod` values across all managed resources.

For ECS: pod containers share a single ECS task (multi-container task definition). The pod name maps to the task definition family.

### Networks

Docker networks map to cloud networking resources:

| Cloud | Docker Network → | Tag |
|-------|-----------------|-----|
| ECS | VPC Security Group | `sockerless-network: {name}` on SG |
| Cloud Run | Cloud DNS Zone | `sockerless_network: {name}` on zone |
| ACA | NSG | `sockerless-network: {name}` on NSG |

```
# Security Group tags:
  sockerless-managed:      true
  sockerless-cluster:      sockerless-live
  sockerless-resource:     network
  sockerless-network-name: testnet
  sockerless-network-id:   abc123def456...
  sockerless-network-driver: bridge
  sockerless-created:      2026-04-05T12:00:00Z
```

`docker network ls` queries for SGs/zones/NSGs tagged with `sockerless-resource=network`.

### Volumes

Docker volumes map to cloud storage:

| Cloud | Docker Volume → | Tag |
|-------|----------------|-----|
| ECS | EFS access point | `sockerless-volume: {name}` |
| Cloud Run | — (not supported) | — |
| ACA | Azure Files share | `sockerless-volume: {name}` |

```
# EFS Access Point tags:
  sockerless-managed:      true
  sockerless-cluster:      sockerless-live
  sockerless-resource:     volume
  sockerless-volume-name:  mydata
  sockerless-volume-driver: local
  sockerless-created:      2026-04-05T12:00:00Z
```

`docker volume ls` queries for tagged EFS access points / Azure Files shares.

## API Call Mapping

### `docker ps` (ContainerList)

```
ECS backend:
  1. ecs:ListTasks(cluster=sockerless-live)
  2. ecs:DescribeTasks(tasks) → get lastStatus, containers, startedAt
  3. ecs:ListTagsForResource(task) → get sockerless-* tags
  4. Map to []ContainerSummary using tags + task info

Lambda backend:
  1. lambda:ListFunctions() with tag filter sockerless-managed=true
  2. Map to []ContainerSummary using function config + tags
```

### `docker inspect` (ContainerInspect)

```
ECS backend:
  1. Find task by sockerless-container-id tag (or by name)
  2. ecs:DescribeTasks → full task status
  3. ecs:DescribeTaskDefinition → image, cmd, env, resources
  4. ecs:ListTagsForResource → Docker labels, network, pod
  5. Build full Container struct
```

### `docker run` (ContainerCreate + ContainerStart)

```
ECS backend:
  1. ecs:RegisterTaskDefinition with image, cmd, env, resources
  2. ecs:RunTask with tags:
     - All sockerless-* metadata tags
     - sockerless-container-id: generated 64-char hex
     - sockerless-name: /container-name
  3. Return container ID (from tag, not task ARN)
```

### `docker network create`

```
ECS backend:
  1. ec2:CreateSecurityGroup with tags:
     - sockerless-managed=true
     - sockerless-resource=network
     - sockerless-network-name=testnet
  2. ec2:AuthorizeSecurityGroupIngress (self-referencing rule)
  3. Return network ID
```

### `docker network inspect`

```
ECS backend:
  1. ec2:DescribeSecurityGroups(filters: tag:sockerless-network-name=testnet)
  2. ecs:ListTasks → DescribeTasks → filter by SG membership
  3. Build Network struct with Containers map
```

## Tag Limits

| Cloud | Max tags | Max key length | Max value length |
|-------|----------|---------------|-----------------|
| AWS | 50 per resource | 128 chars | 256 chars |
| GCP | 64 labels | 63 chars | 63 chars |
| Azure | 50 tags | 512 chars | 256 chars |

**Implications:**
- GCP label values max 63 chars — use job spec annotations for long values
- AWS tag values max 256 chars — sufficient for most metadata as JSON
- Env vars are never stored in tags (security) — only a hash for change detection

### GCP Label Overflow Strategy

For values exceeding GCP's 63-char label limit (Docker labels JSON, long commands):

1. Store a **hash** in the GCP label: `sockerless_labels_hash: a1b2c3`
2. Store **full data** in the Cloud Run job spec's `annotations` field (no size limit):
   ```
   annotations:
     sockerless.labels: '{"app":"web","env":"prod","version":"1.2.3"}'
     sockerless.cmd: 'nginx -g daemon off;'
     sockerless.entrypoint: '/docker-entrypoint.sh'
   ```
3. On `docker inspect`, read from job spec annotations (not labels)
4. Labels are used only for **filtering** (`docker ps --filter label=app=web`)

## Migration Path

Current backends use `Store.Containers` (in-memory) + `ResourceRegistry` (JSON file). Migration:

1. **Phase 1**: Add tag writing to all create/start operations (backward compatible)
2. **Phase 2**: Implement cloud-query versions of ContainerList, ContainerInspect
3. **Phase 3**: Remove local Store.Containers, remove ResourceRegistry
4. **Phase 4**: Remove recovery.go (no longer needed — cloud IS the state)

During migration, both paths coexist: local store for fast reads, cloud query for correctness verification.

## Chinese Wall: Backends ↔ Simulators

**Backends NEVER depend on simulator code.** There is no import, no shared type, no direct reference between `backends/` and `simulators/`. Backends talk to the cloud API — which happens to be the simulator in development and the real cloud in production. The API contract is the cloud provider's API specification (AWS SDK, GCP SDK, Azure SDK).

This means:
- Backends use only cloud SDK clients (`aws-sdk-go-v2`, `cloud.google.com/go`, `azure-sdk-for-go`)
- Simulators must have **full parity** with the real cloud APIs that backends use
- If a backend calls `ecs:ListTasks` with tag filters, the simulator must support that exact API surface
- Tag-based resource discovery must work identically against simulators and real AWS/GCP/Azure

### Required Simulator API Parity

For the stateless backend model, simulators must support these tag-based query APIs:

**AWS Simulator:**
- `ecs:ListTasks` with tag filter support
- `ecs:DescribeTasks` → returns tags
- `ecs:ListTagsForResource` → returns all tags for a task ARN
- `ec2:DescribeSecurityGroups` with `tag:sockerless-*` filters
- `lambda:ListFunctions` with tag filter
- `lambda:ListTags` / `lambda:GetFunction` → returns tags

**GCP Simulator:**
- Cloud Run Jobs list with label selector filter
- Cloud DNS managed zones list with label filter
- Artifact Registry repos with label filter

**Azure Simulator:**
- ACA Jobs list with tag filter (via Azure Resource Graph or direct API)
- NSG list with tag filter
- Azure Files shares with tag filter

Simulators are tested against the same SDK tests and CLI tests that validate real cloud behavior. If a query works against real AWS, it must work against the simulator.
