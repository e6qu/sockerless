# TFINT-003: GCP Terraform Integration (CloudRun + GCF)

**Status:** DONE
**Phase:** 11 — Full Terraform Integration Testing

## Description

Run the full CloudRun and GCF terraform modules against the GCP simulator. Fix simulator API gaps.

## Results

- **CloudRun:** 13 resources apply + destroy cleanly
- **GCF:** 7 resources apply + destroy cleanly

## Simulator Fixes

- Compute: Subnetwork CRUD + secondary IP ranges
- VPC Access: Connector `connectedProjects` field
- DNS: Managed zone `privateVisibilityConfig`, SOA/NS record sets
- Storage: Bucket `iamConfiguration`, `softDeletePolicy`, `hierarchicalNamespace` fields
- Artifact Registry: `cleanupPolicies`, `cleanupPolicyDryRun` fields
- IAM: Resource-level IAM policy (bucket/AR repo `getIamPolicy`/`setIamPolicy`)
- Service Usage: Batch enable endpoint
