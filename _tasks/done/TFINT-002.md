# TFINT-002: AWS Terraform Integration (ECS + Lambda)

**Status:** DONE
**Phase:** 11 — Full Terraform Integration Testing

## Description

Run the full ECS and Lambda terraform modules against the AWS simulator. Fix simulator API gaps.

## Results

- **ECS:** 21 resources apply + destroy cleanly
- **Lambda:** 5 resources apply + destroy cleanly

## Simulator Fixes

- EC2: VPC `enableDnsHostnames` attribute response format, default CIDR block
- ECS: Container Insights setting, execute-command configuration
- CloudWatch: `DescribeLogGroups` handler, log group `creationTime`/`storedBytes`
- IAM: `ListPolicyVersions` endpoint, role trust policy serialization
- EFS: Mount target description format, `OwnerId` field
- CloudMap: `GetNamespace` 404 handling
