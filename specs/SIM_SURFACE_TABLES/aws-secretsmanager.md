# Sim surface — aws-secretsmanager

Surface registered in `simulators/aws/secretsmanager.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action secretsmanager.CreateSecret` | ✓ `simulators/aws/secretsmanager.go:196::handleSMCreateSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.GetSecretValue` | ✓ `simulators/aws/secretsmanager.go:197::handleSMGetSecretValue` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.DescribeSecret` | ✓ `simulators/aws/secretsmanager.go:198::handleSMDescribeSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.UpdateSecret` | ✓ `simulators/aws/secretsmanager.go:199::handleSMUpdateSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.PutSecretValue` | ✓ `simulators/aws/secretsmanager.go:200::handleSMPutSecretValue` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.DeleteSecret` | ✓ `simulators/aws/secretsmanager.go:201::handleSMDeleteSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.ListSecrets` | ✓ `simulators/aws/secretsmanager.go:202::handleSMListSecrets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.ListSecretVersionIds` | ✓ `simulators/aws/secretsmanager.go:203::handleSMListSecretVersionIds` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.TagResource` | ✓ `simulators/aws/secretsmanager.go:204::handleSMTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.UntagResource` | ✓ `simulators/aws/secretsmanager.go:205::handleSMUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.GetResourcePolicy` | ✓ `simulators/aws/secretsmanager.go:206::handleSMGetResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.GetRandomPassword` | ✓ `simulators/aws/secretsmanager.go:207::handleSMGetRandomPassword` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.PutResourcePolicy` | ✓ `simulators/aws/secretsmanager.go:208::handleSMPutResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.DeleteResourcePolicy` | ✓ `simulators/aws/secretsmanager.go:209::handleSMDeleteResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.ValidateResourcePolicy` | ✓ `simulators/aws/secretsmanager.go:210::handleSMValidateResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.RestoreSecret` | ✓ `simulators/aws/secretsmanager.go:211::handleSMRestoreSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.RotateSecret` | ✓ `simulators/aws/secretsmanager.go:212::handleSMRotateSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.CancelRotateSecret` | ✓ `simulators/aws/secretsmanager.go:213::handleSMCancelRotateSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.BatchGetSecretValue` | ✓ `simulators/aws/secretsmanager.go:214::handleSMBatchGetSecretValue` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.UpdateSecretVersionStage` | ✓ `simulators/aws/secretsmanager.go:215::handleSMUpdateSecretVersionStage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.ReplicateSecretToRegions` | ✓ `simulators/aws/secretsmanager.go:216::handleSMReplicateSecretToRegions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.RemoveRegionsFromReplication` | ✓ `simulators/aws/secretsmanager.go:217::handleSMRemoveRegionsFromReplication` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.StopReplicationToReplica` | ✓ `simulators/aws/secretsmanager.go:218::handleSMStopReplicationToReplica` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
