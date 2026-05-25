# Sim surface — aws-ssm_parameters

Surface registered in `simulators/aws/ssm_parameters.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AmazonSSM.PutParameter` | ✓ `simulators/aws/ssm_parameters.go:74::handleSSMPutParameter` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSSM.GetParameter` | ✓ `simulators/aws/ssm_parameters.go:75::handleSSMGetParameter` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSSM.GetParameters` | ✓ `simulators/aws/ssm_parameters.go:76::handleSSMGetParameters` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSSM.GetParametersByPath` | ✓ `simulators/aws/ssm_parameters.go:77::handleSSMGetParametersByPath` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSSM.DescribeParameters` | ✓ `simulators/aws/ssm_parameters.go:78::handleSSMDescribeParameters` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSSM.DeleteParameter` | ✓ `simulators/aws/ssm_parameters.go:79::handleSSMDeleteParameter` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSSM.DeleteParameters` | ✓ `simulators/aws/ssm_parameters.go:80::handleSSMDeleteParameters` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSSM.AddTagsToResource` | ✓ `simulators/aws/ssm_parameters.go:81::handleSSMAddTagsToResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSSM.RemoveTagsFromResource` | ✓ `simulators/aws/ssm_parameters.go:82::handleSSMRemoveTagsFromResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSSM.ListTagsForResource` | ✓ `simulators/aws/ssm_parameters.go:83::handleSSMListTagsForResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
