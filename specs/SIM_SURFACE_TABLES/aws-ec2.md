# Sim surface — aws-ec2

Surface registered in `simulators/aws/ec2.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateVpc` | ✓ `simulators/aws/ec2.go:256::handleCreateVpc` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeVpcs` | ✓ `simulators/aws/ec2.go:257::handleDescribeVpcs` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteVpc` | ✓ `simulators/aws/ec2.go:258::handleDeleteVpc` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeVpcAttribute` | ✓ `simulators/aws/ec2.go:259::handleDescribeVpcAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ModifyVpcAttribute` | ✓ `simulators/aws/ec2.go:260::handleModifyVpcAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateSubnet` | ✓ `simulators/aws/ec2.go:263::handleCreateSubnet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeSubnets` | ✓ `simulators/aws/ec2.go:264::handleDescribeSubnets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteSubnet` | ✓ `simulators/aws/ec2.go:265::handleDeleteSubnet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ModifySubnetAttribute` | ✓ `simulators/aws/ec2.go:266::handleModifySubnetAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateInternetGateway` | ✓ `simulators/aws/ec2.go:269::handleCreateInternetGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AttachInternetGateway` | ✓ `simulators/aws/ec2.go:270::handleAttachInternetGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DetachInternetGateway` | ✓ `simulators/aws/ec2.go:271::handleDetachInternetGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeInternetGateways` | ✓ `simulators/aws/ec2.go:272::handleDescribeInternetGateways` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteInternetGateway` | ✓ `simulators/aws/ec2.go:273::handleDeleteInternetGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AllocateAddress` | ✓ `simulators/aws/ec2.go:276::handleAllocateAddress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeAddresses` | ✓ `simulators/aws/ec2.go:277::handleDescribeAddresses` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeAddressesAttribute` | ✓ `simulators/aws/ec2.go:278::handleDescribeAddressesAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ReleaseAddress` | ✓ `simulators/aws/ec2.go:279::handleReleaseAddress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateNatGateway` | ✓ `simulators/aws/ec2.go:282::handleCreateNatGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeNatGateways` | ✓ `simulators/aws/ec2.go:283::handleDescribeNatGateways` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteNatGateway` | ✓ `simulators/aws/ec2.go:284::handleDeleteNatGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateRouteTable` | ✓ `simulators/aws/ec2.go:287::handleCreateRouteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeRouteTables` | ✓ `simulators/aws/ec2.go:288::handleDescribeRouteTables` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteRouteTable` | ✓ `simulators/aws/ec2.go:289::handleDeleteRouteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateRoute` | ✓ `simulators/aws/ec2.go:290::handleCreateRoute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteRoute` | ✓ `simulators/aws/ec2.go:291::handleDeleteRoute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AssociateRouteTable` | ✓ `simulators/aws/ec2.go:292::handleAssociateRouteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DisassociateRouteTable` | ✓ `simulators/aws/ec2.go:293::handleDisassociateRouteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateSecurityGroup` | ✓ `simulators/aws/ec2.go:296::handleCreateSecurityGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeSecurityGroups` | ✓ `simulators/aws/ec2.go:297::handleDescribeSecurityGroups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeSecurityGroupRules` | ✓ `simulators/aws/ec2.go:298::handleDescribeSecurityGroupRules` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteSecurityGroup` | ✓ `simulators/aws/ec2.go:299::handleDeleteSecurityGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AuthorizeSecurityGroupIngress` | ✓ `simulators/aws/ec2.go:300::handleAuthorizeSecurityGroupIngress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AuthorizeSecurityGroupEgress` | ✓ `simulators/aws/ec2.go:301::handleAuthorizeSecurityGroupEgress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action RevokeSecurityGroupIngress` | ✓ `simulators/aws/ec2.go:302::handleRevokeSecurityGroupIngress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action RevokeSecurityGroupEgress` | ✓ `simulators/aws/ec2.go:303::handleRevokeSecurityGroupEgress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeNetworkInterfaces` | ✓ `simulators/aws/ec2.go:315::handleDescribeNetworkInterfaces` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
Issue #266 closed the EC2 VM lifecycle gap. `RunInstances`, `DescribeInstances`, `StopInstances`, `StartInstances`, `TerminateInstances`, `DescribeInstanceStatus`, `DescribeInstanceAttribute`, `ModifyInstanceAttribute`, `DescribeImages`, `DescribeInstanceTypes`, `DescribeKeyPairs`, `DescribeVolumes`, `DescribeTags`, `CreateTags`, `DeleteTags`, account/region/AZ discovery, and instance-created `DescribeNetworkInterfaces` are covered by `simulators/aws/sdk-tests/ec2_test.go`, `simulators/aws/cli-tests/ec2_test.go`, and `simulators/aws/terraform-tests/main.tf` through `aws_instance`.
<!-- HAND-WRITTEN END -->
