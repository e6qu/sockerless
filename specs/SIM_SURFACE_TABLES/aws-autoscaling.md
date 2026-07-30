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
| `Action CreateLaunchConfiguration` | ✓ `simulators/aws/autoscaling.go:111::handleASCreateLaunchConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeLaunchConfigurations` | ✓ `simulators/aws/autoscaling.go:112::handleASDescribeLaunchConfigurations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteLaunchConfiguration` | ✓ `simulators/aws/autoscaling.go:113::handleASDeleteLaunchConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:114::handleASCreateAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAutoScalingGroups` | ✓ `simulators/aws/autoscaling.go:115::handleASDescribeAutoScalingGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UpdateAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:116::handleASUpdateAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetDesiredCapacity` | ✓ `simulators/aws/autoscaling.go:117::handleASSetDesiredCapacity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeScalingActivities` | ✓ `simulators/aws/autoscaling.go:118::handleASDescribeScalingActivities` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateOrUpdateTags` | ✓ `simulators/aws/autoscaling.go:119::handleASCreateOrUpdateTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteTags` | ✓ `simulators/aws/autoscaling.go:120::handleASDeleteTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeTags` | ✓ `simulators/aws/autoscaling.go:121::handleASDescribeTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:122::handleASDeleteAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutScalingPolicy` | ✓ `simulators/aws/autoscaling.go:123::handleASPutScalingPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribePolicies` | ✓ `simulators/aws/autoscaling.go:124::handleASDescribePolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeletePolicy` | ✓ `simulators/aws/autoscaling.go:125::handleASDeletePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ExecutePolicy` | ✓ `simulators/aws/autoscaling.go:126::handleASExecutePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutScheduledUpdateGroupAction` | ✓ `simulators/aws/autoscaling.go:127::handleASPutScheduledUpdateGroupAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeScheduledActions` | ✓ `simulators/aws/autoscaling.go:128::handleASDescribeScheduledActions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteScheduledAction` | ✓ `simulators/aws/autoscaling.go:129::handleASDeleteScheduledAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutLifecycleHook` | ✓ `simulators/aws/autoscaling.go:130::handleASPutLifecycleHook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeLifecycleHooks` | ✓ `simulators/aws/autoscaling.go:131::handleASDescribeLifecycleHooks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteLifecycleHook` | ✓ `simulators/aws/autoscaling.go:132::handleASDeleteLifecycleHook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAutoScalingInstances` | ✓ `simulators/aws/autoscaling.go:133::handleASDescribeAutoScalingInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetInstanceHealth` | ✓ `simulators/aws/autoscaling.go:134::handleASSetInstanceHealth` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TerminateInstanceInAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:135::handleASTerminateInstanceInAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
