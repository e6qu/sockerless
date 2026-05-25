# Sim surface — aws-efs

Surface registered in `simulators/aws/efs.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /2015-02-01/file-systems` | ✓ `simulators/aws/efs.go:141::handleEFSCreateFileSystem` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-02-01/file-systems` | ✓ `simulators/aws/efs.go:142::handleEFSDescribeFileSystems` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /2015-02-01/file-systems/{id}` | ✓ `simulators/aws/efs.go:143::handleEFSDeleteFileSystem` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /2015-02-01/file-systems/{id}/lifecycle-configuration` | ✓ `simulators/aws/efs.go:144::handleEFSPutLifecycleConfiguration` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-02-01/file-systems/{id}/lifecycle-configuration` | ✓ `simulators/aws/efs.go:145::handleEFSDescribeLifecycleConfiguration` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /2015-02-01/mount-targets` | ✓ `simulators/aws/efs.go:147::handleEFSCreateMountTarget` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-02-01/mount-targets` | ✓ `simulators/aws/efs.go:148::handleEFSDescribeMountTargets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-02-01/mount-targets/{id}/security-groups` | ✓ `simulators/aws/efs.go:149::handleEFSDescribeMountTargetSecurityGroups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /2015-02-01/mount-targets/{id}/security-groups` | ✓ `simulators/aws/efs.go:150::handleEFSModifyMountTargetSecurityGroups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /2015-02-01/mount-targets/{id}` | ✓ `simulators/aws/efs.go:151::handleEFSDeleteMountTarget` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /2015-02-01/access-points` | ✓ `simulators/aws/efs.go:153::handleEFSCreateAccessPoint` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /2015-02-01/access-points` | ✓ `simulators/aws/efs.go:154::handleEFSDescribeAccessPoints` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /2015-02-01/access-points/{id}` | ✓ `simulators/aws/efs.go:155::handleEFSDeleteAccessPoint` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
