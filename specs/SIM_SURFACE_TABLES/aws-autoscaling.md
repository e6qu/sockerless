# Sim surface — aws-autoscaling

Surface registered in `simulators/aws/autoscaling.go`. Rows below are the ops the sim currently registers. Auto Scaling uses the AWS Query Protocol, so each row names the public `Action` value.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateLaunchConfiguration` | ✓ `simulators/aws/autoscaling.go:54::handleASCreateLaunchConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeLaunchConfigurations` | ✓ `simulators/aws/autoscaling.go:55::handleASDescribeLaunchConfigurations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteLaunchConfiguration` | ✓ `simulators/aws/autoscaling.go:56::handleASDeleteLaunchConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:57::handleASCreateAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAutoScalingGroups` | ✓ `simulators/aws/autoscaling.go:58::handleASDescribeAutoScalingGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UpdateAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:59::handleASUpdateAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetDesiredCapacity` | ✓ `simulators/aws/autoscaling.go:60::handleASSetDesiredCapacity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeScalingActivities` | ✓ `simulators/aws/autoscaling.go:61::handleASDescribeScalingActivities` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateOrUpdateTags` | ✓ `simulators/aws/autoscaling.go:62::handleASCreateOrUpdateTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteTags` | ✓ `simulators/aws/autoscaling.go:63::handleASDeleteTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeTags` | ✓ `simulators/aws/autoscaling.go:64::handleASDescribeTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteAutoScalingGroup` | ✓ `simulators/aws/autoscaling.go:65::handleASDeleteAutoScalingGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- SDK coverage lives in `simulators/aws/sdk-tests/autoscaling_cloudtrail_test.go` and verifies launch configurations, ASG lifecycle, desired-capacity materialization into EC2 instances, and scaling activities through `github.com/aws/aws-sdk-go-v2/service/autoscaling`.
- CLI coverage lives in `simulators/aws/cli-tests/autoscaling_cloudtrail_test.go` and verifies the same lifecycle through `aws autoscaling`.
- Terraform coverage lives in `simulators/aws/terraform-tests/main.tf` and `simulators/aws/terraform-tests/apply_test.go` through `aws_launch_configuration` and `aws_autoscaling_group`.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
