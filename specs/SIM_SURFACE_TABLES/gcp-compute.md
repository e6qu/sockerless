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
| `POST /compute/v1/projects/{project}/global/networks` | ✓ `simulators/gcp/compute.go:402::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/networks/{name}` | ✓ `simulators/gcp/compute.go:437::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/networks` | ✓ `simulators/gcp/compute.go:451::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/networks/{name}` | ✓ `simulators/gcp/compute.go:469::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/global/networks/{name}` | ✓ `simulators/gcp/compute.go:480::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/regions/{region}/subnetworks` | ✓ `simulators/gcp/compute.go:506::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/subnetworks/{name}` | ✓ `simulators/gcp/compute.go:541::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/regions/{region}/subnetworks/{name}` | ✓ `simulators/gcp/compute.go:556::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/global/firewalls` | ✓ `simulators/gcp/compute.go:575::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/firewalls/{name}` | ✓ `simulators/gcp/compute.go:601::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/firewalls` | ✓ `simulators/gcp/compute.go:613::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/firewalls/{name}` | ✓ `simulators/gcp/compute.go:628::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/global/firewalls/{name}` | ✓ `simulators/gcp/compute.go:637::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/regions/{region}/routers` | ✓ `simulators/gcp/compute.go:689::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/routers/{name}` | ✓ `simulators/gcp/compute.go:711::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/routers` | ✓ `simulators/gcp/compute.go:724::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/regions/{region}/routers/{name}` | ✓ `simulators/gcp/compute.go:740::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/regions/{region}/routers/{name}` | ✓ `simulators/gcp/compute.go:750::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/operations/{name}` | ✓ `simulators/gcp/compute.go:783::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/operations/{name}` | ✓ `simulators/gcp/compute.go:797::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}` | ✓ `simulators/gcp/compute.go:835::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones` | ✓ `simulators/gcp/compute.go:838::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/machineTypes/{machineType}` | ✓ `simulators/gcp/compute.go:860::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/machineTypes` | ✓ `simulators/gcp/compute.go:875::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/diskTypes/{diskType}` | ✓ `simulators/gcp/compute.go:892::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/images/{image}` | ✓ `simulators/gcp/compute.go:918::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/images/family/{family}` | ✓ `simulators/gcp/compute.go:921::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances` | ✓ `simulators/gcp/compute.go:1072::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/instances/{name}` | ✓ `simulators/gcp/compute.go:1089::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/instances` | ✓ `simulators/gcp/compute.go:1102::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/aggregated/instances` | ✓ `simulators/gcp/compute.go:1115::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/zones/{zone}/instances/{name}` | ✓ `simulators/gcp/compute.go:1139::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances/{name}/stop` | ✓ `simulators/gcp/compute.go:1152::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances/{name}/start` | ✓ `simulators/gcp/compute.go:1164::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances/{name}/setLabels` | ✓ `simulators/gcp/compute.go:1176::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances/{name}/setTags` | ✓ `simulators/gcp/compute.go:1198::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/disks` | ✓ `simulators/gcp/compute.go:1230::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/disks/{name}` | ✓ `simulators/gcp/compute.go:1260::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/disks` | ✓ `simulators/gcp/compute.go:1274::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/zones/{zone}/disks/{name}` | ✓ `simulators/gcp/compute.go:1291::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/disks/{name}/resize` | ✓ `simulators/gcp/compute.go:1305::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/disks/{name}/setLabels` | ✓ `simulators/gcp/compute.go:1330::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/aggregated/disks` | ✓ `simulators/gcp/compute.go:1356::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/operations/{name}` | ✓ `simulators/gcp/compute.go:1394::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/operations/{name}/wait` | ✓ `simulators/gcp/compute.go:1400::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
Issue #266 closed the Compute Engine VM lifecycle gap. Zonal instance insert/get/list/delete/start/stop, aggregated instances, labels/tags, machine types, disk types, images, attached disks, and NIC metadata are covered by `simulators/gcp/sdk-tests/compute_test.go`, `simulators/gcp/cli-tests/compute_instances_test.go`, and `simulators/gcp/terraform-tests/main.tf` through `google_compute_instance`.
<!-- HAND-WRITTEN END -->
