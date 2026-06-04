# Sim surface — aws-application-autoscaling

Surface registered in `simulators/aws/application_autoscaling.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AnyScaleFrontendService.RegisterScalableTarget` | ✓ `simulators/aws/application_autoscaling.go:58::handleAppASRegisterScalableTarget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AnyScaleFrontendService.DeregisterScalableTarget` | ✓ `simulators/aws/application_autoscaling.go:59::handleAppASDeregisterScalableTarget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AnyScaleFrontendService.DescribeScalableTargets` | ✓ `simulators/aws/application_autoscaling.go:60::handleAppASDescribeScalableTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AnyScaleFrontendService.PutScalingPolicy` | ✓ `simulators/aws/application_autoscaling.go:61::handleAppASPutScalingPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AnyScaleFrontendService.DeleteScalingPolicy` | ✓ `simulators/aws/application_autoscaling.go:62::handleAppASDeleteScalingPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AnyScaleFrontendService.DescribeScalingPolicies` | ✓ `simulators/aws/application_autoscaling.go:63::handleAppASDescribeScalingPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AnyScaleFrontendService.ListTagsForResource` | ✓ `simulators/aws/application_autoscaling.go:64::handleAppASListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AnyScaleFrontendService.TagResource` | ✓ `simulators/aws/application_autoscaling.go:65::handleAppASTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AnyScaleFrontendService.UntagResource` | ✓ `simulators/aws/application_autoscaling.go:66::handleAppASUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
