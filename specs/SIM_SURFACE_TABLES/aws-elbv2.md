# Sim surface — aws-elbv2

Surface registered in `simulators/aws/elbv2.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateLoadBalancer` | ✓ `simulators/aws/elbv2.go:86::handleELBv2CreateLoadBalancer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeLoadBalancers` | ✓ `simulators/aws/elbv2.go:87::handleELBv2DescribeLoadBalancers` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteLoadBalancer` | ✓ `simulators/aws/elbv2.go:88::handleELBv2DeleteLoadBalancer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyLoadBalancerAttributes` | ✓ `simulators/aws/elbv2.go:89::handleELBv2ModifyLoadBalancerAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeLoadBalancerAttributes` | ✓ `simulators/aws/elbv2.go:90::handleELBv2DescribeLoadBalancerAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeCapacityReservation` | ✓ `simulators/aws/elbv2.go:91::handleELBv2DescribeCapacityReservation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetSecurityGroups` | ✓ `simulators/aws/elbv2.go:92::handleELBv2SetSecurityGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetSubnets` | ✓ `simulators/aws/elbv2.go:93::handleELBv2SetSubnets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateTargetGroup` | ✓ `simulators/aws/elbv2.go:95::handleELBv2CreateTargetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeTargetGroups` | ✓ `simulators/aws/elbv2.go:96::handleELBv2DescribeTargetGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteTargetGroup` | ✓ `simulators/aws/elbv2.go:97::handleELBv2DeleteTargetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyTargetGroup` | ✓ `simulators/aws/elbv2.go:98::handleELBv2ModifyTargetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyTargetGroupAttributes` | ✓ `simulators/aws/elbv2.go:99::handleELBv2ModifyTargetGroupAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeTargetGroupAttributes` | ✓ `simulators/aws/elbv2.go:100::handleELBv2DescribeTargetGroupAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RegisterTargets` | ✓ `simulators/aws/elbv2.go:101::handleELBv2RegisterTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeregisterTargets` | ✓ `simulators/aws/elbv2.go:102::handleELBv2DeregisterTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeTargetHealth` | ✓ `simulators/aws/elbv2.go:103::handleELBv2DescribeTargetHealth` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateListener` | ✓ `simulators/aws/elbv2.go:105::handleELBv2CreateListener` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeListeners` | ✓ `simulators/aws/elbv2.go:106::handleELBv2DescribeListeners` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeListenerAttributes` | ✓ `simulators/aws/elbv2.go:107::handleELBv2DescribeListenerAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyListenerAttributes` | ✓ `simulators/aws/elbv2.go:108::handleELBv2ModifyListenerAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteListener` | ✓ `simulators/aws/elbv2.go:109::handleELBv2DeleteListener` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddTags` | ✓ `simulators/aws/elbv2.go:111::handleELBv2AddTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveTags` | ✓ `simulators/aws/elbv2.go:112::handleELBv2RemoveTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeTags` | ✓ `simulators/aws/elbv2.go:113::handleELBv2DescribeTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAccountLimits` | ✓ `simulators/aws/elbv2.go:114::handleELBv2DescribeAccountLimits` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
Issue #263 closed the AWS managed load-balancer gap for the ELBv2 public Query API. The implemented slice covers application/network load balancer lifecycle, target groups, listeners, target registration/health, mutable load-balancer/target-group/listener attributes, tagging, account limits, and the provider-read `DescribeCapacityReservation` operation. Coverage uses the official `elasticloadbalancingv2` Go SDK in `simulators/aws/sdk-tests/elbv2_test.go`, AWS CLI `elbv2` lifecycle coverage in `simulators/aws/cli-tests/elbv2_test.go`, and Terraform `aws_lb`, `aws_lb_target_group`, and `aws_lb_listener` resources in `simulators/aws/terraform-tests/main.tf`.
<!-- HAND-WRITTEN END -->
