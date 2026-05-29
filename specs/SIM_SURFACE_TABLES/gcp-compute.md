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
| `POST /compute/v1/projects/{project}/global/networks` | ✓ `simulators/gcp/compute.go:437::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/networks/{name}` | ✓ `simulators/gcp/compute.go:472::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/networks` | ✓ `simulators/gcp/compute.go:486::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/networks/{name}` | ✓ `simulators/gcp/compute.go:504::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/global/networks/{name}` | ✓ `simulators/gcp/compute.go:515::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/regions/{region}/subnetworks` | ✓ `simulators/gcp/compute.go:541::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/subnetworks/{name}` | ✓ `simulators/gcp/compute.go:576::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/regions/{region}/subnetworks/{name}` | ✓ `simulators/gcp/compute.go:591::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/global/firewalls` | ✓ `simulators/gcp/compute.go:610::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/firewalls/{name}` | ✓ `simulators/gcp/compute.go:636::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/firewalls` | ✓ `simulators/gcp/compute.go:648::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/firewalls/{name}` | ✓ `simulators/gcp/compute.go:663::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/global/firewalls/{name}` | ✓ `simulators/gcp/compute.go:672::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/regions/{region}/addresses` | ✓ `simulators/gcp/compute.go:725::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/addresses/{name}` | ✓ `simulators/gcp/compute.go:764::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/addresses` | ✓ `simulators/gcp/compute.go:777::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/regions/{region}/addresses/{name}/setLabels` | ✓ `simulators/gcp/compute.go:793::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/regions/{region}/addresses/{name}` | ✓ `simulators/gcp/compute.go:817::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/regions/{region}/routers` | ✓ `simulators/gcp/compute.go:826::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/routers/{name}` | ✓ `simulators/gcp/compute.go:852::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/routers` | ✓ `simulators/gcp/compute.go:865::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/regions/{region}/routers/{name}` | ✓ `simulators/gcp/compute.go:881::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/regions/{region}/routers/{name}` | ✓ `simulators/gcp/compute.go:891::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/routers/{name}/getRouterStatus` | ✓ `simulators/gcp/compute.go:927::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/operations/{name}` | ✓ `simulators/gcp/compute.go:947::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/regions/{region}/operations/{name}` | ✓ `simulators/gcp/compute.go:961::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/regions/{region}/operations/{name}/wait` | ✓ `simulators/gcp/compute.go:974::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}` | ✓ `simulators/gcp/compute.go:1054::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones` | ✓ `simulators/gcp/compute.go:1057::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/machineTypes/{machineType}` | ✓ `simulators/gcp/compute.go:1079::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/machineTypes` | ✓ `simulators/gcp/compute.go:1094::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/diskTypes/{diskType}` | ✓ `simulators/gcp/compute.go:1111::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/images/{image}` | ✓ `simulators/gcp/compute.go:1137::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/images/family/{family}` | ✓ `simulators/gcp/compute.go:1140::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances` | ✓ `simulators/gcp/compute.go:1291::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/instances/{name}` | ✓ `simulators/gcp/compute.go:1308::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/instances` | ✓ `simulators/gcp/compute.go:1321::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/aggregated/instances` | ✓ `simulators/gcp/compute.go:1334::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/zones/{zone}/instances/{name}` | ✓ `simulators/gcp/compute.go:1358::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances/{name}/stop` | ✓ `simulators/gcp/compute.go:1371::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances/{name}/start` | ✓ `simulators/gcp/compute.go:1383::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances/{name}/setLabels` | ✓ `simulators/gcp/compute.go:1395::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/instances/{name}/setTags` | ✓ `simulators/gcp/compute.go:1417::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/disks` | ✓ `simulators/gcp/compute.go:1449::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/disks/{name}` | ✓ `simulators/gcp/compute.go:1479::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/disks` | ✓ `simulators/gcp/compute.go:1493::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/zones/{zone}/disks/{name}` | ✓ `simulators/gcp/compute.go:1510::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/disks/{name}/resize` | ✓ `simulators/gcp/compute.go:1524::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/disks/{name}/setLabels` | ✓ `simulators/gcp/compute.go:1549::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/aggregated/disks` | ✓ `simulators/gcp/compute.go:1575::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/zones/{zone}/operations/{name}` | ✓ `simulators/gcp/compute.go:1613::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/zones/{zone}/operations/{name}/wait` | ✓ `simulators/gcp/compute.go:1619::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
Issue #266 closed the Compute Engine VM lifecycle gap. Zonal instance insert/get/list/delete/start/stop, aggregated instances, labels/tags, machine types, disk types, images, attached disks, and NIC metadata are covered by `simulators/gcp/sdk-tests/compute_test.go`, `simulators/gcp/cli-tests/compute_instances_test.go`, and `simulators/gcp/terraform-tests/main.tf` through `google_compute_instance`.

Issue #279 closed the Compute NAT/public-IP parity pass. Regional address insert/get/list/delete, address `setLabels`, manual Cloud NAT router patch/validation, router status, and regional operation wait are covered by `simulators/gcp/sdk-tests/compute_test.go`, `simulators/gcp/cli-tests/compute_nat_test.go`, and `simulators/gcp/terraform-tests/main.tf` through `google_compute_address`, `google_compute_router`, and `google_compute_router_nat`.
<!-- HAND-WRITTEN END -->
