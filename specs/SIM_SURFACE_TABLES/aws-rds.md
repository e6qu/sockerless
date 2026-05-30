# Sim surface — aws-rds

Surface registered in `simulators/aws/rds.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateDBInstance` | ✓ `simulators/aws/rds.go:75::handleRDSCreate` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action DescribeDBInstances` | ✓ `simulators/aws/rds.go:76::handleRDSDescribe` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action ModifyDBInstance` | ✓ `simulators/aws/rds.go:77::handleRDSModify` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action DeleteDBInstance` | ✓ `simulators/aws/rds.go:78::handleRDSDelete` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action AddTagsToResource` | ✓ `simulators/aws/rds.go:79::handleRDSAddTags` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action ListTagsForResource` | ✓ `simulators/aws/rds.go:80::handleRDSListTags` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action RemoveTagsFromResource` | ✓ `simulators/aws/rds.go:81::handleRDSRemoveTags` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CreateDBSnapshot` | ✓ `simulators/aws/rds.go:82::handleRDSCreateSnapshot` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action DescribeDBSnapshots` | ✓ `simulators/aws/rds.go:83::handleRDSDescribeSnapshots` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action DeleteDBSnapshot` | ✓ `simulators/aws/rds.go:84::handleRDSDeleteSnapshot` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action RestoreDBInstanceFromDBSnapshot` | ✓ `simulators/aws/rds.go:85::handleRDSRestoreFromSnapshot` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
