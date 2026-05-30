# Sim surface — aws-ec2

Surface registered in `simulators/aws/ec2.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action DescribeAccountAttributes` | ✓ `simulators/aws/ec2.go:292::handleDescribeAccountAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAvailabilityZones` | ✓ `simulators/aws/ec2.go:293::handleDescribeAvailabilityZones` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeRegions` | ✓ `simulators/aws/ec2.go:294::handleDescribeRegions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateVpc` | ✓ `simulators/aws/ec2.go:295::handleCreateVpc` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeVpcs` | ✓ `simulators/aws/ec2.go:296::handleDescribeVpcs` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteVpc` | ✓ `simulators/aws/ec2.go:297::handleDeleteVpc` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeVpcAttribute` | ✓ `simulators/aws/ec2.go:298::handleDescribeVpcAttribute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyVpcAttribute` | ✓ `simulators/aws/ec2.go:299::handleModifyVpcAttribute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateSubnet` | ✓ `simulators/aws/ec2.go:302::handleCreateSubnet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeSubnets` | ✓ `simulators/aws/ec2.go:303::handleDescribeSubnets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteSubnet` | ✓ `simulators/aws/ec2.go:304::handleDeleteSubnet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifySubnetAttribute` | ✓ `simulators/aws/ec2.go:305::handleModifySubnetAttribute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateInternetGateway` | ✓ `simulators/aws/ec2.go:308::handleCreateInternetGateway` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AttachInternetGateway` | ✓ `simulators/aws/ec2.go:309::handleAttachInternetGateway` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DetachInternetGateway` | ✓ `simulators/aws/ec2.go:310::handleDetachInternetGateway` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeInternetGateways` | ✓ `simulators/aws/ec2.go:311::handleDescribeInternetGateways` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteInternetGateway` | ✓ `simulators/aws/ec2.go:312::handleDeleteInternetGateway` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AllocateAddress` | ✓ `simulators/aws/ec2.go:315::handleAllocateAddress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAddresses` | ✓ `simulators/aws/ec2.go:316::handleDescribeAddresses` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAddressesAttribute` | ✓ `simulators/aws/ec2.go:317::handleDescribeAddressesAttribute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ReleaseAddress` | ✓ `simulators/aws/ec2.go:318::handleReleaseAddress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateNatGateway` | ✓ `simulators/aws/ec2.go:321::handleCreateNatGateway` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeNatGateways` | ✓ `simulators/aws/ec2.go:322::handleDescribeNatGateways` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteNatGateway` | ✓ `simulators/aws/ec2.go:323::handleDeleteNatGateway` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateRouteTable` | ✓ `simulators/aws/ec2.go:326::handleCreateRouteTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeRouteTables` | ✓ `simulators/aws/ec2.go:327::handleDescribeRouteTables` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteRouteTable` | ✓ `simulators/aws/ec2.go:328::handleDeleteRouteTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateRoute` | ✓ `simulators/aws/ec2.go:329::handleCreateRoute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteRoute` | ✓ `simulators/aws/ec2.go:330::handleDeleteRoute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AssociateRouteTable` | ✓ `simulators/aws/ec2.go:331::handleAssociateRouteTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DisassociateRouteTable` | ✓ `simulators/aws/ec2.go:332::handleDisassociateRouteTable` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateSecurityGroup` | ✓ `simulators/aws/ec2.go:335::handleCreateSecurityGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeSecurityGroups` | ✓ `simulators/aws/ec2.go:336::handleDescribeSecurityGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeSecurityGroupRules` | ✓ `simulators/aws/ec2.go:337::handleDescribeSecurityGroupRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteSecurityGroup` | ✓ `simulators/aws/ec2.go:338::handleDeleteSecurityGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AuthorizeSecurityGroupIngress` | ✓ `simulators/aws/ec2.go:339::handleAuthorizeSecurityGroupIngress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AuthorizeSecurityGroupEgress` | ✓ `simulators/aws/ec2.go:340::handleAuthorizeSecurityGroupEgress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RevokeSecurityGroupIngress` | ✓ `simulators/aws/ec2.go:341::handleRevokeSecurityGroupIngress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RevokeSecurityGroupEgress` | ✓ `simulators/aws/ec2.go:342::handleRevokeSecurityGroupEgress` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RunInstances` | ✓ `simulators/aws/ec2.go:345::handleRunInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeInstances` | ✓ `simulators/aws/ec2.go:346::handleDescribeInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TerminateInstances` | ✓ `simulators/aws/ec2.go:347::handleTerminateInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StopInstances` | ✓ `simulators/aws/ec2.go:348::handleStopInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StartInstances` | ✓ `simulators/aws/ec2.go:349::handleStartInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeInstanceStatus` | ✓ `simulators/aws/ec2.go:350::handleDescribeInstanceStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeInstanceAttribute` | ✓ `simulators/aws/ec2.go:351::handleDescribeInstanceAttribute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ModifyInstanceAttribute` | ✓ `simulators/aws/ec2.go:352::handleModifyInstanceAttribute` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateTags` | ✓ `simulators/aws/ec2.go:353::handleCreateTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteTags` | ✓ `simulators/aws/ec2.go:354::handleDeleteTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeTags` | ✓ `simulators/aws/ec2.go:355::handleDescribeTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeVolumes` | ✓ `simulators/aws/ec2.go:356::handleDescribeVolumes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeImages` | ✓ `simulators/aws/ec2.go:357::handleDescribeImages` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeInstanceTypes` | ✓ `simulators/aws/ec2.go:358::handleDescribeInstanceTypes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeKeyPairs` | ✓ `simulators/aws/ec2.go:359::handleDescribeKeyPairs` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeNetworkInterfaces` | ✓ `simulators/aws/ec2.go:371::handleDescribeNetworkInterfaces` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
Issue #266 closed the EC2 VM lifecycle gap. `RunInstances`, `DescribeInstances`, `StopInstances`, `StartInstances`, `TerminateInstances`, `DescribeInstanceStatus`, `DescribeInstanceAttribute`, `ModifyInstanceAttribute`, `DescribeImages`, `DescribeInstanceTypes`, `DescribeKeyPairs`, `DescribeVolumes`, `DescribeTags`, `CreateTags`, `DeleteTags`, account/region/AZ discovery, and instance-created `DescribeNetworkInterfaces` are covered by `simulators/aws/sdk-tests/ec2_test.go`, `simulators/aws/cli-tests/ec2_test.go`, and `simulators/aws/terraform-tests/main.tf` through `aws_instance`.

Issue #279 closed the EC2 NAT/public-IP parity pass. `AllocateAddress`, `DescribeAddresses`, `DescribeAddressesAttribute`, `ReleaseAddress`, `CreateNatGateway`, `DescribeNatGateways`, `DeleteNatGateway`, `CreateRouteTable`, `CreateRoute`, and route-table reads are covered by `simulators/aws/sdk-tests/ec2_test.go`, `simulators/aws/cli-tests/ec2_test.go`, and `simulators/aws/terraform-tests/main.tf` through `aws_eip`, `aws_nat_gateway`, and NAT Gateway routes in `aws_route_table`.
<!-- HAND-WRITTEN END -->
