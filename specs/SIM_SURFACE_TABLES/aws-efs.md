# Sim surface — aws-efs

Surface registered in `simulators/aws/efs.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /2015-02-01/file-systems` | ✓ `simulators/aws/efs.go:141::handleEFSCreateFileSystem` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws `aws_efs_file_system`. |
| `GET /2015-02-01/file-systems` | ✓ `simulators/aws/efs.go:142::handleEFSDescribeFileSystems` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws EFS resource refresh. |
| `DELETE /2015-02-01/file-systems/{id}` | ✓ `simulators/aws/efs.go:143::handleEFSDeleteFileSystem` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws `aws_efs_file_system` destroy. |
| `PUT /2015-02-01/file-systems/{id}/lifecycle-configuration` | ✓ `simulators/aws/efs.go:144::handleEFSPutLifecycleConfiguration` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /2015-02-01/file-systems/{id}/lifecycle-configuration` | ✓ `simulators/aws/efs.go:145::handleEFSDescribeLifecycleConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws EFS file-system refresh. |
| `POST /2015-02-01/mount-targets` | ✓ `simulators/aws/efs.go:147::handleEFSCreateMountTarget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws `aws_efs_mount_target`. |
| `GET /2015-02-01/mount-targets` | ✓ `simulators/aws/efs.go:148::handleEFSDescribeMountTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws mount-target refresh. |
| `GET /2015-02-01/mount-targets/{id}/security-groups` | ✓ `simulators/aws/efs.go:149::handleEFSDescribeMountTargetSecurityGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws mount-target refresh. |
| `PUT /2015-02-01/mount-targets/{id}/security-groups` | ✓ `simulators/aws/efs.go:150::handleEFSModifyMountTargetSecurityGroups` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /2015-02-01/mount-targets/{id}` | ✓ `simulators/aws/efs.go:151::handleEFSDeleteMountTarget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws `aws_efs_mount_target` destroy. |
| `POST /2015-02-01/access-points` | ✓ `simulators/aws/efs.go:153::handleEFSCreateAccessPoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws `aws_efs_access_point`. |
| `GET /2015-02-01/access-points` | ✓ `simulators/aws/efs.go:154::handleEFSDescribeAccessPoints` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws access-point refresh. |
| `DELETE /2015-02-01/access-points/{id}` | ✓ `simulators/aws/efs.go:155::handleEFSDeleteAccessPoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Covered by terraform-provider-aws `aws_efs_access_point` destroy. |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
