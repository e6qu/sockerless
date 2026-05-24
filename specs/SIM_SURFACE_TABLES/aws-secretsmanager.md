# Sim surface — aws-secretsmanager

Surface registered in `simulators/aws/secretsmanager.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action secretsmanager.CreateSecret` | ✓ `simulators/aws/secretsmanager.go:157::handleSMCreateSecret` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.GetSecretValue` | ✓ `simulators/aws/secretsmanager.go:158::handleSMGetSecretValue` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.DescribeSecret` | ✓ `simulators/aws/secretsmanager.go:159::handleSMDescribeSecret` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.UpdateSecret` | ✓ `simulators/aws/secretsmanager.go:160::handleSMUpdateSecret` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.PutSecretValue` | ✓ `simulators/aws/secretsmanager.go:161::handleSMPutSecretValue` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.DeleteSecret` | ✓ `simulators/aws/secretsmanager.go:162::handleSMDeleteSecret` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.ListSecrets` | ✓ `simulators/aws/secretsmanager.go:163::handleSMListSecrets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.ListSecretVersionIds` | ✓ `simulators/aws/secretsmanager.go:164::handleSMListSecretVersionIds` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.TagResource` | ✓ `simulators/aws/secretsmanager.go:165::handleSMTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.UntagResource` | ✓ `simulators/aws/secretsmanager.go:166::handleSMUntagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.GetResourcePolicy` | ✓ `simulators/aws/secretsmanager.go:167::handleSMGetResourcePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action secretsmanager.GetRandomPassword` | ✓ `simulators/aws/secretsmanager.go:168::handleSMGetRandomPassword` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
