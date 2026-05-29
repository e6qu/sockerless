# Sim surface — aws-elbv2

Surface registered in `simulators/aws/elbv2.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateLoadBalancer` | ✓ `simulators/aws/elbv2.go:86::handleELBv2CreateLoadBalancer` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeLoadBalancers` | ✓ `simulators/aws/elbv2.go:87::handleELBv2DescribeLoadBalancers` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteLoadBalancer` | ✓ `simulators/aws/elbv2.go:88::handleELBv2DeleteLoadBalancer` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ModifyLoadBalancerAttributes` | ✓ `simulators/aws/elbv2.go:89::handleELBv2ModifyLoadBalancerAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeLoadBalancerAttributes` | ✓ `simulators/aws/elbv2.go:90::handleELBv2DescribeLoadBalancerAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeCapacityReservation` | ✓ `simulators/aws/elbv2.go:91::handleELBv2DescribeCapacityReservation` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action SetSecurityGroups` | ✓ `simulators/aws/elbv2.go:92::handleELBv2SetSecurityGroups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action SetSubnets` | ✓ `simulators/aws/elbv2.go:93::handleELBv2SetSubnets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateTargetGroup` | ✓ `simulators/aws/elbv2.go:95::handleELBv2CreateTargetGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeTargetGroups` | ✓ `simulators/aws/elbv2.go:96::handleELBv2DescribeTargetGroups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteTargetGroup` | ✓ `simulators/aws/elbv2.go:97::handleELBv2DeleteTargetGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ModifyTargetGroup` | ✓ `simulators/aws/elbv2.go:98::handleELBv2ModifyTargetGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ModifyTargetGroupAttributes` | ✓ `simulators/aws/elbv2.go:99::handleELBv2ModifyTargetGroupAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeTargetGroupAttributes` | ✓ `simulators/aws/elbv2.go:100::handleELBv2DescribeTargetGroupAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action RegisterTargets` | ✓ `simulators/aws/elbv2.go:101::handleELBv2RegisterTargets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeregisterTargets` | ✓ `simulators/aws/elbv2.go:102::handleELBv2DeregisterTargets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeTargetHealth` | ✓ `simulators/aws/elbv2.go:103::handleELBv2DescribeTargetHealth` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateListener` | ✓ `simulators/aws/elbv2.go:105::handleELBv2CreateListener` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeListeners` | ✓ `simulators/aws/elbv2.go:106::handleELBv2DescribeListeners` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeListenerAttributes` | ✓ `simulators/aws/elbv2.go:107::handleELBv2DescribeListenerAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ModifyListenerAttributes` | ✓ `simulators/aws/elbv2.go:108::handleELBv2ModifyListenerAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteListener` | ✓ `simulators/aws/elbv2.go:109::handleELBv2DeleteListener` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AddTags` | ✓ `simulators/aws/elbv2.go:111::handleELBv2AddTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action RemoveTags` | ✓ `simulators/aws/elbv2.go:112::handleELBv2RemoveTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeTags` | ✓ `simulators/aws/elbv2.go:113::handleELBv2DescribeTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeAccountLimits` | ✓ `simulators/aws/elbv2.go:114::handleELBv2DescribeAccountLimits` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
Issue #263 closed the AWS managed load-balancer gap for the ELBv2 public Query API. The implemented slice covers application/network load balancer lifecycle, target groups, listeners, target registration/health, mutable load-balancer/target-group/listener attributes, tagging, account limits, and the provider-read `DescribeCapacityReservation` operation. Coverage uses the official `elasticloadbalancingv2` Go SDK in `simulators/aws/sdk-tests/elbv2_test.go`, AWS CLI `elbv2` lifecycle coverage in `simulators/aws/cli-tests/elbv2_test.go`, and Terraform `aws_lb`, `aws_lb_target_group`, and `aws_lb_listener` resources in `simulators/aws/terraform-tests/main.tf`.
<!-- HAND-WRITTEN END -->
