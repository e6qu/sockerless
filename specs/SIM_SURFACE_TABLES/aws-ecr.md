# Sim surface — aws-ecr

Surface registered in `simulators/aws/ecr.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AmazonEC2ContainerRegistry_V20150921.CreateRepository` | ✓ `simulators/aws/ecr.go:96::handleECRCreateRepository` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribeRepositories` | ✓ `simulators/aws/ecr.go:97::handleECRDescribeRepositories` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeleteRepository` | ✓ `simulators/aws/ecr.go:98::handleECRDeleteRepository` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken` | ✓ `simulators/aws/ecr.go:99::handleECRGetAuthorizationToken` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.BatchGetImage` | ✓ `simulators/aws/ecr.go:100::handleECRBatchGetImage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutImage` | ✓ `simulators/aws/ecr.go:101::handleECRPutImage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.BatchDeleteImage` | ✓ `simulators/aws/ecr.go:102::handleECRBatchDeleteImage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.BatchCheckLayerAvailability` | ✓ `simulators/aws/ecr.go:103::handleECRBatchCheckLayerAvailability` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutLifecyclePolicy` | ✓ `simulators/aws/ecr.go:104::handleECRPutLifecyclePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetLifecyclePolicy` | ✓ `simulators/aws/ecr.go:105::handleECRGetLifecyclePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeleteLifecyclePolicy` | ✓ `simulators/aws/ecr.go:106::handleECRDeleteLifecyclePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.ListTagsForResource` | ✓ `simulators/aws/ecr.go:107::handleECRListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.TagResource` | ✓ `simulators/aws/ecr.go:108::handleECRTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.CreatePullThroughCacheRule` | ✓ `simulators/aws/ecr.go:116::handleECRCreatePullThroughCacheRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribePullThroughCacheRules` | ✓ `simulators/aws/ecr.go:117::handleECRDescribePullThroughCacheRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeletePullThroughCacheRule` | ✓ `simulators/aws/ecr.go:118::handleECRDeletePullThroughCacheRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
