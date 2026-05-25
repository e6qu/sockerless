# Sim surface — gcp-compute

Surface registered in `simulators/gcp/compute.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /compute/v1/projects/{project}/global/networks` | ✓ `simulators/gcp/compute.go:221::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/networks/{name}` | ✓ `simulators/gcp/compute.go:256::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/networks` | ✓ `simulators/gcp/compute.go:270::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/networks/{name}` | ✓ `simulators/gcp/compute.go:288::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/global/networks/{name}` | ✓ `simulators/gcp/compute.go:299::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/regions/{region}/subnetworks` | ✓ `simulators/gcp/compute.go:325::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/subnetworks/{name}` | ✓ `simulators/gcp/compute.go:360::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/regions/{region}/subnetworks/{name}` | ✓ `simulators/gcp/compute.go:375::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/global/firewalls` | ✓ `simulators/gcp/compute.go:394::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/firewalls/{name}` | ✓ `simulators/gcp/compute.go:420::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/firewalls` | ✓ `simulators/gcp/compute.go:432::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/firewalls/{name}` | ✓ `simulators/gcp/compute.go:447::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/global/firewalls/{name}` | ✓ `simulators/gcp/compute.go:456::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/regions/{region}/routers` | ✓ `simulators/gcp/compute.go:508::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/routers/{name}` | ✓ `simulators/gcp/compute.go:530::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/routers` | ✓ `simulators/gcp/compute.go:543::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/regions/{region}/routers/{name}` | ✓ `simulators/gcp/compute.go:559::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/regions/{region}/routers/{name}` | ✓ `simulators/gcp/compute.go:569::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/operations/{name}` | ✓ `simulators/gcp/compute.go:602::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/operations/{name}` | ✓ `simulators/gcp/compute.go:616::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}` | ✓ `simulators/gcp/compute.go:651::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones` | ✓ `simulators/gcp/compute.go:654::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/disks` | ✓ `simulators/gcp/compute.go:706::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/disks/{name}` | ✓ `simulators/gcp/compute.go:736::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/disks` | ✓ `simulators/gcp/compute.go:750::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/zones/{zone}/disks/{name}` | ✓ `simulators/gcp/compute.go:767::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/disks/{name}/resize` | ✓ `simulators/gcp/compute.go:781::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/disks/{name}/setLabels` | ✓ `simulators/gcp/compute.go:806::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/aggregated/disks` | ✓ `simulators/gcp/compute.go:832::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/operations/{name}` | ✓ `simulators/gcp/compute.go:860::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
