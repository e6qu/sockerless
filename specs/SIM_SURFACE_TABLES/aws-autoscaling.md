# Sim surface — aws-autoscaling

Surface registered in `simulators/aws/autoscaling.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateLaunchConfiguration` | ✓ `simulators/aws/autoscaling.go:114::handleASCreateLaunchConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeLaunchConfigurations` | ✓ `simulators/aws/autoscaling.go:115::handleASDescribeLaunchConfigurations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteLaunchConfiguration` | ✓ `simulators/aws/autoscaling.go:116::handleASDeleteLaunchConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:117::handleASCreateAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAutoScalingGroups` | ✓ `simulators/aws/autoscaling.go:118::handleASDescribeAutoScalingGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UpdateAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:119::handleASUpdateAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetDesiredCapacity` | ✓ `simulators/aws/autoscaling.go:120::handleASSetDesiredCapacity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeScalingActivities` | ✓ `simulators/aws/autoscaling.go:121::handleASDescribeScalingActivities` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateOrUpdateTags` | ✓ `simulators/aws/autoscaling.go:122::handleASCreateOrUpdateTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteTags` | ✓ `simulators/aws/autoscaling.go:123::handleASDeleteTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeTags` | ✓ `simulators/aws/autoscaling.go:124::handleASDescribeTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:125::handleASDeleteAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutScalingPolicy` | ✓ `simulators/aws/autoscaling.go:126::handleASPutScalingPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribePolicies` | ✓ `simulators/aws/autoscaling.go:127::handleASDescribePolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeletePolicy` | ✓ `simulators/aws/autoscaling.go:128::handleASDeletePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ExecutePolicy` | ✓ `simulators/aws/autoscaling.go:129::handleASExecutePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutScheduledUpdateGroupAction` | ✓ `simulators/aws/autoscaling.go:130::handleASPutScheduledUpdateGroupAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeScheduledActions` | ✓ `simulators/aws/autoscaling.go:131::handleASDescribeScheduledActions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteScheduledAction` | ✓ `simulators/aws/autoscaling.go:132::handleASDeleteScheduledAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutLifecycleHook` | ✓ `simulators/aws/autoscaling.go:133::handleASPutLifecycleHook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeLifecycleHooks` | ✓ `simulators/aws/autoscaling.go:134::handleASDescribeLifecycleHooks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteLifecycleHook` | ✓ `simulators/aws/autoscaling.go:135::handleASDeleteLifecycleHook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAutoScalingInstances` | ✓ `simulators/aws/autoscaling.go:136::handleASDescribeAutoScalingInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetInstanceHealth` | ✓ `simulators/aws/autoscaling.go:137::handleASSetInstanceHealth` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TerminateInstanceInAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:138::handleASTerminateInstanceInAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
