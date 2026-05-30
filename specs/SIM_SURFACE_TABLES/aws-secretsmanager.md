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
| `Action secretsmanager.CreateSecret` | ✓ `simulators/aws/secretsmanager.go:157::handleSMCreateSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.GetSecretValue` | ✓ `simulators/aws/secretsmanager.go:158::handleSMGetSecretValue` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.DescribeSecret` | ✓ `simulators/aws/secretsmanager.go:159::handleSMDescribeSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.UpdateSecret` | ✓ `simulators/aws/secretsmanager.go:160::handleSMUpdateSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.PutSecretValue` | ✓ `simulators/aws/secretsmanager.go:161::handleSMPutSecretValue` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.DeleteSecret` | ✓ `simulators/aws/secretsmanager.go:162::handleSMDeleteSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.ListSecrets` | ✓ `simulators/aws/secretsmanager.go:163::handleSMListSecrets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.ListSecretVersionIds` | ✓ `simulators/aws/secretsmanager.go:164::handleSMListSecretVersionIds` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.TagResource` | ✓ `simulators/aws/secretsmanager.go:165::handleSMTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.UntagResource` | ✓ `simulators/aws/secretsmanager.go:166::handleSMUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.GetResourcePolicy` | ✓ `simulators/aws/secretsmanager.go:167::handleSMGetResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action secretsmanager.GetRandomPassword` | ✓ `simulators/aws/secretsmanager.go:168::handleSMGetRandomPassword` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
