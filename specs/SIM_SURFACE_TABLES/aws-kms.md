# Sim surface — aws-kms

Surface registered in `simulators/aws/kms.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action TrentService.CreateKey` | ✓ `simulators/aws/kms.go:58::handleKMSCreateKey` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.DescribeKey` | ✓ `simulators/aws/kms.go:59::handleKMSDescribeKey` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.ListKeys` | ✓ `simulators/aws/kms.go:60::handleKMSListKeys` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.ScheduleKeyDeletion` | ✓ `simulators/aws/kms.go:61::handleKMSScheduleKeyDeletion` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.Encrypt` | ✓ `simulators/aws/kms.go:62::handleKMSEncrypt` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.Decrypt` | ✓ `simulators/aws/kms.go:63::handleKMSDecrypt` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.GenerateDataKey` | ✓ `simulators/aws/kms.go:64::handleKMSGenerateDataKey` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.CreateAlias` | ✓ `simulators/aws/kms.go:65::handleKMSCreateAlias` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.DeleteAlias` | ✓ `simulators/aws/kms.go:66::handleKMSDeleteAlias` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.ListAliases` | ✓ `simulators/aws/kms.go:67::handleKMSListAliases` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.GetKeyPolicy` | ✓ `simulators/aws/kms.go:68::handleKMSGetKeyPolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.PutKeyPolicy` | ✓ `simulators/aws/kms.go:69::handleKMSPutKeyPolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.ListResourceTags` | ✓ `simulators/aws/kms.go:70::handleKMSListResourceTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TrentService.GetKeyRotationStatus` | ✓ `simulators/aws/kms.go:71::handleKMSGetKeyRotationStatus` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
