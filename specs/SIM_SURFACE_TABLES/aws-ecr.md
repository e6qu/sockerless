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
| `Action AmazonEC2ContainerRegistry_V20150921.CreateRepository` | ✓ `simulators/aws/ecr.go:127::handleECRCreateRepository` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribeRepositories` | ✓ `simulators/aws/ecr.go:128::handleECRDescribeRepositories` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeleteRepository` | ✓ `simulators/aws/ecr.go:129::handleECRDeleteRepository` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken` | ✓ `simulators/aws/ecr.go:130::handleECRGetAuthorizationToken` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.BatchGetImage` | ✓ `simulators/aws/ecr.go:131::handleECRBatchGetImage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.ListImages` | ✓ `simulators/aws/ecr.go:132::handleECRListImages` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribeImages` | ✓ `simulators/aws/ecr.go:133::handleECRDescribeImages` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutImage` | ✓ `simulators/aws/ecr.go:134::handleECRPutImage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.BatchDeleteImage` | ✓ `simulators/aws/ecr.go:135::handleECRBatchDeleteImage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.BatchCheckLayerAvailability` | ✓ `simulators/aws/ecr.go:136::handleECRBatchCheckLayerAvailability` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutLifecyclePolicy` | ✓ `simulators/aws/ecr.go:137::handleECRPutLifecyclePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetLifecyclePolicy` | ✓ `simulators/aws/ecr.go:138::handleECRGetLifecyclePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeleteLifecyclePolicy` | ✓ `simulators/aws/ecr.go:139::handleECRDeleteLifecyclePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.ListTagsForResource` | ✓ `simulators/aws/ecr.go:140::handleECRListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.TagResource` | ✓ `simulators/aws/ecr.go:141::handleECRTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.UntagResource` | ✓ `simulators/aws/ecr.go:142::handleECRUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.CreatePullThroughCacheRule` | ✓ `simulators/aws/ecr.go:150::handleECRCreatePullThroughCacheRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribePullThroughCacheRules` | ✓ `simulators/aws/ecr.go:151::handleECRDescribePullThroughCacheRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeletePullThroughCacheRule` | ✓ `simulators/aws/ecr.go:152::handleECRDeletePullThroughCacheRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.SetRepositoryPolicy` | ✓ `simulators/aws/ecr_layers.go:43::handleECRSetRepositoryPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetRepositoryPolicy` | ✓ `simulators/aws/ecr_layers.go:44::handleECRGetRepositoryPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeleteRepositoryPolicy` | ✓ `simulators/aws/ecr_layers.go:45::handleECRDeleteRepositoryPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.InitiateLayerUpload` | ✓ `simulators/aws/ecr_layers.go:46::handleECRInitiateLayerUpload` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.UploadLayerPart` | ✓ `simulators/aws/ecr_layers.go:47::handleECRUploadLayerPart` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.CompleteLayerUpload` | ✓ `simulators/aws/ecr_layers.go:48::handleECRCompleteLayerUpload` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetDownloadUrlForLayer` | ✓ `simulators/aws/ecr_layers.go:49::handleECRGetDownloadUrlForLayer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutImageTagMutability` | ✓ `simulators/aws/ecr_registry.go:71::handleECRPutImageTagMutability` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutImageScanningConfiguration` | ✓ `simulators/aws/ecr_registry.go:72::handleECRPutImageScanningConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.StartImageScan` | ✓ `simulators/aws/ecr_registry.go:73::handleECRStartImageScan` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribeImageScanFindings` | ✓ `simulators/aws/ecr_registry.go:74::handleECRDescribeImageScanFindings` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.StartLifecyclePolicyPreview` | ✓ `simulators/aws/ecr_registry.go:75::handleECRStartLifecyclePolicyPreview` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetLifecyclePolicyPreview` | ✓ `simulators/aws/ecr_registry.go:76::handleECRGetLifecyclePolicyPreview` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribeRegistry` | ✓ `simulators/aws/ecr_registry.go:77::handleECRDescribeRegistry` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutRegistryPolicy` | ✓ `simulators/aws/ecr_registry.go:78::handleECRPutRegistryPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetRegistryPolicy` | ✓ `simulators/aws/ecr_registry.go:79::handleECRGetRegistryPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeleteRegistryPolicy` | ✓ `simulators/aws/ecr_registry.go:80::handleECRDeleteRegistryPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutReplicationConfiguration` | ✓ `simulators/aws/ecr_registry.go:81::handleECRPutReplicationConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribeImageReplicationStatus` | ✓ `simulators/aws/ecr_registry.go:82::handleECRDescribeImageReplicationStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
