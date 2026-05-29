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
| `Action DescribeAccountAttributes` | ✓ `simulators/aws/ec2.go:292::handleDescribeAccountAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeAvailabilityZones` | ✓ `simulators/aws/ec2.go:293::handleDescribeAvailabilityZones` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeRegions` | ✓ `simulators/aws/ec2.go:294::handleDescribeRegions` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateVpc` | ✓ `simulators/aws/ec2.go:295::handleCreateVpc` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeVpcs` | ✓ `simulators/aws/ec2.go:296::handleDescribeVpcs` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteVpc` | ✓ `simulators/aws/ec2.go:297::handleDeleteVpc` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeVpcAttribute` | ✓ `simulators/aws/ec2.go:298::handleDescribeVpcAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ModifyVpcAttribute` | ✓ `simulators/aws/ec2.go:299::handleModifyVpcAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateSubnet` | ✓ `simulators/aws/ec2.go:302::handleCreateSubnet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeSubnets` | ✓ `simulators/aws/ec2.go:303::handleDescribeSubnets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteSubnet` | ✓ `simulators/aws/ec2.go:304::handleDeleteSubnet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ModifySubnetAttribute` | ✓ `simulators/aws/ec2.go:305::handleModifySubnetAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateInternetGateway` | ✓ `simulators/aws/ec2.go:308::handleCreateInternetGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AttachInternetGateway` | ✓ `simulators/aws/ec2.go:309::handleAttachInternetGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DetachInternetGateway` | ✓ `simulators/aws/ec2.go:310::handleDetachInternetGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeInternetGateways` | ✓ `simulators/aws/ec2.go:311::handleDescribeInternetGateways` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteInternetGateway` | ✓ `simulators/aws/ec2.go:312::handleDeleteInternetGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AllocateAddress` | ✓ `simulators/aws/ec2.go:315::handleAllocateAddress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeAddresses` | ✓ `simulators/aws/ec2.go:316::handleDescribeAddresses` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeAddressesAttribute` | ✓ `simulators/aws/ec2.go:317::handleDescribeAddressesAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ReleaseAddress` | ✓ `simulators/aws/ec2.go:318::handleReleaseAddress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateNatGateway` | ✓ `simulators/aws/ec2.go:321::handleCreateNatGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeNatGateways` | ✓ `simulators/aws/ec2.go:322::handleDescribeNatGateways` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteNatGateway` | ✓ `simulators/aws/ec2.go:323::handleDeleteNatGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateRouteTable` | ✓ `simulators/aws/ec2.go:326::handleCreateRouteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeRouteTables` | ✓ `simulators/aws/ec2.go:327::handleDescribeRouteTables` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteRouteTable` | ✓ `simulators/aws/ec2.go:328::handleDeleteRouteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateRoute` | ✓ `simulators/aws/ec2.go:329::handleCreateRoute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteRoute` | ✓ `simulators/aws/ec2.go:330::handleDeleteRoute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AssociateRouteTable` | ✓ `simulators/aws/ec2.go:331::handleAssociateRouteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DisassociateRouteTable` | ✓ `simulators/aws/ec2.go:332::handleDisassociateRouteTable` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateSecurityGroup` | ✓ `simulators/aws/ec2.go:335::handleCreateSecurityGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeSecurityGroups` | ✓ `simulators/aws/ec2.go:336::handleDescribeSecurityGroups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeSecurityGroupRules` | ✓ `simulators/aws/ec2.go:337::handleDescribeSecurityGroupRules` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteSecurityGroup` | ✓ `simulators/aws/ec2.go:338::handleDeleteSecurityGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AuthorizeSecurityGroupIngress` | ✓ `simulators/aws/ec2.go:339::handleAuthorizeSecurityGroupIngress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AuthorizeSecurityGroupEgress` | ✓ `simulators/aws/ec2.go:340::handleAuthorizeSecurityGroupEgress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action RevokeSecurityGroupIngress` | ✓ `simulators/aws/ec2.go:341::handleRevokeSecurityGroupIngress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action RevokeSecurityGroupEgress` | ✓ `simulators/aws/ec2.go:342::handleRevokeSecurityGroupEgress` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action RunInstances` | ✓ `simulators/aws/ec2.go:345::handleRunInstances` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeInstances` | ✓ `simulators/aws/ec2.go:346::handleDescribeInstances` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TerminateInstances` | ✓ `simulators/aws/ec2.go:347::handleTerminateInstances` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action StopInstances` | ✓ `simulators/aws/ec2.go:348::handleStopInstances` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action StartInstances` | ✓ `simulators/aws/ec2.go:349::handleStartInstances` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeInstanceStatus` | ✓ `simulators/aws/ec2.go:350::handleDescribeInstanceStatus` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeInstanceAttribute` | ✓ `simulators/aws/ec2.go:351::handleDescribeInstanceAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ModifyInstanceAttribute` | ✓ `simulators/aws/ec2.go:352::handleModifyInstanceAttribute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateTags` | ✓ `simulators/aws/ec2.go:353::handleCreateTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteTags` | ✓ `simulators/aws/ec2.go:354::handleDeleteTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeTags` | ✓ `simulators/aws/ec2.go:355::handleDescribeTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeVolumes` | ✓ `simulators/aws/ec2.go:356::handleDescribeVolumes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeImages` | ✓ `simulators/aws/ec2.go:357::handleDescribeImages` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeInstanceTypes` | ✓ `simulators/aws/ec2.go:358::handleDescribeInstanceTypes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeKeyPairs` | ✓ `simulators/aws/ec2.go:359::handleDescribeKeyPairs` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DescribeNetworkInterfaces` | ✓ `simulators/aws/ec2.go:371::handleDescribeNetworkInterfaces` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
Issue #266 closed the EC2 VM lifecycle gap. `RunInstances`, `DescribeInstances`, `StopInstances`, `StartInstances`, `TerminateInstances`, `DescribeInstanceStatus`, `DescribeInstanceAttribute`, `ModifyInstanceAttribute`, `DescribeImages`, `DescribeInstanceTypes`, `DescribeKeyPairs`, `DescribeVolumes`, `DescribeTags`, `CreateTags`, `DeleteTags`, account/region/AZ discovery, and instance-created `DescribeNetworkInterfaces` are covered by `simulators/aws/sdk-tests/ec2_test.go`, `simulators/aws/cli-tests/ec2_test.go`, and `simulators/aws/terraform-tests/main.tf` through `aws_instance`.
<!-- HAND-WRITTEN END -->
