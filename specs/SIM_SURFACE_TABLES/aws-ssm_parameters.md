# Sim surface — aws-ssm_parameters

Surface registered in `simulators/aws/ssm_parameters.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AmazonSSM.PutParameter` | ✓ `simulators/aws/ssm_parameters.go:74::handleSSMPutParameter` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonSSM.GetParameter` | ✓ `simulators/aws/ssm_parameters.go:75::handleSSMGetParameter` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonSSM.GetParameters` | ✓ `simulators/aws/ssm_parameters.go:76::handleSSMGetParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonSSM.GetParametersByPath` | ✓ `simulators/aws/ssm_parameters.go:77::handleSSMGetParametersByPath` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonSSM.DescribeParameters` | ✓ `simulators/aws/ssm_parameters.go:78::handleSSMDescribeParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonSSM.DeleteParameter` | ✓ `simulators/aws/ssm_parameters.go:79::handleSSMDeleteParameter` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonSSM.DeleteParameters` | ✓ `simulators/aws/ssm_parameters.go:80::handleSSMDeleteParameters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonSSM.AddTagsToResource` | ✓ `simulators/aws/ssm_parameters.go:81::handleSSMAddTagsToResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonSSM.RemoveTagsFromResource` | ✓ `simulators/aws/ssm_parameters.go:82::handleSSMRemoveTagsFromResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonSSM.ListTagsForResource` | ✓ `simulators/aws/ssm_parameters.go:83::handleSSMListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
