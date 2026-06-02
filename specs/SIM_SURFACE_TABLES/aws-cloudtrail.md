# Sim surface — aws-cloudtrail

Surface registered in `simulators/aws/cloudtrail.go`. Rows below are the public CloudTrail operations registered by the simulator's CloudTrail operation loop. CloudTrail uses AWS JSON protocol targets, so the router accepts both SDK target prefixes used by current AWS clients.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CloudTrail_20131101.CreateTrail` | ✓ `simulators/aws/cloudtrail.go:78::handleCloudTrailCreateTrail` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.DescribeTrails` | ✓ `simulators/aws/cloudtrail.go:78::handleCloudTrailDescribeTrails` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.GetTrail` | ✓ `simulators/aws/cloudtrail.go:78::handleCloudTrailGetTrail` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.UpdateTrail` | ✓ `simulators/aws/cloudtrail.go:78::handleCloudTrailUpdateTrail` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.GetTrailStatus` | ✓ `simulators/aws/cloudtrail.go:78::handleCloudTrailGetTrailStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.StartLogging` | ✓ `simulators/aws/cloudtrail.go:79::handleCloudTrailStartLogging` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.StopLogging` | ✓ `simulators/aws/cloudtrail.go:79::handleCloudTrailStopLogging` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.LookupEvents` | ✓ `simulators/aws/cloudtrail.go:79::handleCloudTrailLookupEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.DeleteTrail` | ✓ `simulators/aws/cloudtrail.go:79::handleCloudTrailDeleteTrail` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.AddTags` | ✓ `simulators/aws/cloudtrail.go:80::handleCloudTrailAddTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.RemoveTags` | ✓ `simulators/aws/cloudtrail.go:80::handleCloudTrailRemoveTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.ListTags` | ✓ `simulators/aws/cloudtrail.go:80::handleCloudTrailListTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.PutEventSelectors` | ✓ `simulators/aws/cloudtrail.go:80::handleCloudTrailPutEventSelectors` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CloudTrail_20131101.GetEventSelectors` | ✓ `simulators/aws/cloudtrail.go:80::handleCloudTrailGetEventSelectors` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- SDK coverage lives in `simulators/aws/sdk-tests/autoscaling_cloudtrail_test.go` and verifies trail create/status/start/lookup/S3 delivery/stop/delete through `github.com/aws/aws-sdk-go-v2/service/cloudtrail`.
- CLI coverage lives in `simulators/aws/cli-tests/autoscaling_cloudtrail_test.go` and verifies the same logging and lookup flow through `aws cloudtrail`.
- Terraform coverage lives in `simulators/aws/terraform-tests/main.tf` and `simulators/aws/terraform-tests/apply_test.go` through `aws_cloudtrail`, including the provider's CRUD/read/tag/event-selector call sequence.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
