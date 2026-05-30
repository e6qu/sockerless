# Sim surface — aws-kms

Surface registered in `simulators/aws/kms.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action TrentService.CreateKey` | ✓ `simulators/aws/kms.go:58::handleKMSCreateKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DescribeKey` | ✓ `simulators/aws/kms.go:59::handleKMSDescribeKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListKeys` | ✓ `simulators/aws/kms.go:60::handleKMSListKeys` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ScheduleKeyDeletion` | ✓ `simulators/aws/kms.go:61::handleKMSScheduleKeyDeletion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.Encrypt` | ✓ `simulators/aws/kms.go:62::handleKMSEncrypt` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.Decrypt` | ✓ `simulators/aws/kms.go:63::handleKMSDecrypt` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GenerateDataKey` | ✓ `simulators/aws/kms.go:64::handleKMSGenerateDataKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.CreateAlias` | ✓ `simulators/aws/kms.go:65::handleKMSCreateAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DeleteAlias` | ✓ `simulators/aws/kms.go:66::handleKMSDeleteAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListAliases` | ✓ `simulators/aws/kms.go:67::handleKMSListAliases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GetKeyPolicy` | ✓ `simulators/aws/kms.go:68::handleKMSGetKeyPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.PutKeyPolicy` | ✓ `simulators/aws/kms.go:69::handleKMSPutKeyPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListResourceTags` | ✓ `simulators/aws/kms.go:70::handleKMSListResourceTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GetKeyRotationStatus` | ✓ `simulators/aws/kms.go:71::handleKMSGetKeyRotationStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
