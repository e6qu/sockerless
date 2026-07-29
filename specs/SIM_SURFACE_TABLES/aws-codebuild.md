# Sim surface — aws-codebuild

Surface registered in `simulators/aws/codebuild.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CodeBuild_20161006.CreateProject` | ✓ `simulators/aws/codebuild.go:156::handleCBCreateProject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.BatchGetProjects` | ✓ `simulators/aws/codebuild.go:157::handleCBBatchGetProjects` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListProjects` | ✓ `simulators/aws/codebuild.go:158::handleCBListProjects` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.UpdateProject` | ✓ `simulators/aws/codebuild.go:159::handleCBUpdateProject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DeleteProject` | ✓ `simulators/aws/codebuild.go:160::handleCBDeleteProject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.StartBuild` | ✓ `simulators/aws/codebuild.go:161::handleCBStartBuild` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.StopBuild` | ✓ `simulators/aws/codebuild.go:162::handleCBStopBuild` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.RetryBuild` | ✓ `simulators/aws/codebuild.go:163::handleCBRetryBuild` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.BatchGetBuilds` | ✓ `simulators/aws/codebuild.go:164::handleCBBatchGetBuilds` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListBuildsForProject` | ✓ `simulators/aws/codebuild.go:165::handleCBListBuildsForProject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListBuilds` | ✓ `simulators/aws/codebuild.go:166::handleCBListBuilds` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.CreateReportGroup` | ✓ `simulators/aws/codebuild.go:168::handleCBCreateReportGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.UpdateReportGroup` | ✓ `simulators/aws/codebuild.go:169::handleCBUpdateReportGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DeleteReportGroup` | ✓ `simulators/aws/codebuild.go:170::handleCBDeleteReportGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListReportGroups` | ✓ `simulators/aws/codebuild.go:171::handleCBListReportGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.BatchGetReportGroups` | ✓ `simulators/aws/codebuild.go:172::handleCBBatchGetReportGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListReports` | ✓ `simulators/aws/codebuild.go:173::handleCBListReports` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListReportsForReportGroup` | ✓ `simulators/aws/codebuild.go:174::handleCBListReportsForReportGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.BatchGetReports` | ✓ `simulators/aws/codebuild.go:175::handleCBBatchGetReports` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ImportSourceCredentials` | ✓ `simulators/aws/codebuild.go:177::handleCBImportSourceCredentials` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListSourceCredentials` | ✓ `simulators/aws/codebuild.go:178::handleCBListSourceCredentials` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DeleteSourceCredentials` | ✓ `simulators/aws/codebuild.go:179::handleCBDeleteSourceCredentials` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.BatchDeleteBuilds` | ✓ `simulators/aws/codebuild_extended.go:150::handleCBBatchDeleteBuilds` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.StartBuildBatch` | ✓ `simulators/aws/codebuild_extended.go:153::handleCBStartBuildBatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.StopBuildBatch` | ✓ `simulators/aws/codebuild_extended.go:154::handleCBStopBuildBatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.RetryBuildBatch` | ✓ `simulators/aws/codebuild_extended.go:155::handleCBRetryBuildBatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DeleteBuildBatch` | ✓ `simulators/aws/codebuild_extended.go:156::handleCBDeleteBuildBatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.BatchGetBuildBatches` | ✓ `simulators/aws/codebuild_extended.go:157::handleCBBatchGetBuildBatches` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListBuildBatches` | ✓ `simulators/aws/codebuild_extended.go:158::handleCBListBuildBatches` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListBuildBatchesForProject` | ✓ `simulators/aws/codebuild_extended.go:159::handleCBListBuildBatchesForProject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.CreateFleet` | ✓ `simulators/aws/codebuild_extended.go:162::handleCBCreateFleet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.UpdateFleet` | ✓ `simulators/aws/codebuild_extended.go:163::handleCBUpdateFleet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DeleteFleet` | ✓ `simulators/aws/codebuild_extended.go:164::handleCBDeleteFleet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.BatchGetFleets` | ✓ `simulators/aws/codebuild_extended.go:165::handleCBBatchGetFleets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListFleets` | ✓ `simulators/aws/codebuild_extended.go:166::handleCBListFleets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.StartSandbox` | ✓ `simulators/aws/codebuild_extended.go:169::handleCBStartSandbox` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.StopSandbox` | ✓ `simulators/aws/codebuild_extended.go:170::handleCBStopSandbox` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.StartSandboxConnection` | ✓ `simulators/aws/codebuild_extended.go:171::handleCBStartSandboxConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.BatchGetSandboxes` | ✓ `simulators/aws/codebuild_extended.go:172::handleCBBatchGetSandboxes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListSandboxes` | ✓ `simulators/aws/codebuild_extended.go:173::handleCBListSandboxes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListSandboxesForProject` | ✓ `simulators/aws/codebuild_extended.go:174::handleCBListSandboxesForProject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.StartCommandExecution` | ✓ `simulators/aws/codebuild_extended.go:177::handleCBStartCommandExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.BatchGetCommandExecutions` | ✓ `simulators/aws/codebuild_extended.go:178::handleCBBatchGetCommandExecutions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListCommandExecutionsForSandbox` | ✓ `simulators/aws/codebuild_extended.go:179::handleCBListCommandExecutionsForSandbox` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.CreateWebhook` | ✓ `simulators/aws/codebuild_extended.go:182::handleCBCreateWebhook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.UpdateWebhook` | ✓ `simulators/aws/codebuild_extended.go:183::handleCBUpdateWebhook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DeleteWebhook` | ✓ `simulators/aws/codebuild_extended.go:184::handleCBDeleteWebhook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DeleteReport` | ✓ `simulators/aws/codebuild_extended.go:187::handleCBDeleteReport` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DescribeTestCases` | ✓ `simulators/aws/codebuild_extended.go:188::handleCBDescribeTestCases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DescribeCodeCoverages` | ✓ `simulators/aws/codebuild_extended.go:189::handleCBDescribeCodeCoverages` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.GetReportGroupTrend` | ✓ `simulators/aws/codebuild_extended.go:190::handleCBGetReportGroupTrend` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.PutResourcePolicy` | ✓ `simulators/aws/codebuild_extended.go:193::handleCBPutResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.GetResourcePolicy` | ✓ `simulators/aws/codebuild_extended.go:194::handleCBGetResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.DeleteResourcePolicy` | ✓ `simulators/aws/codebuild_extended.go:195::handleCBDeleteResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.UpdateProjectVisibility` | ✓ `simulators/aws/codebuild_extended.go:198::handleCBUpdateProjectVisibility` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.InvalidateProjectCache` | ✓ `simulators/aws/codebuild_extended.go:199::handleCBInvalidateProjectCache` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListCuratedEnvironmentImages` | ✓ `simulators/aws/codebuild_extended.go:200::handleCBListCuratedEnvironmentImages` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListSharedProjects` | ✓ `simulators/aws/codebuild_extended.go:201::handleCBListSharedProjects` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CodeBuild_20161006.ListSharedReportGroups` | ✓ `simulators/aws/codebuild_extended.go:202::handleCBListSharedReportGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
Build and build-batch jobs clone supported Git sources through either imported,
AWS Key Management Service-encrypted source credentials or an AWS Secrets
Manager source credential. They resolve the requested source revision and
checked-in build specification, then run the commands inside the project's
exact configured container image with the repository mounted at
`CODEBUILD_SRC_DIR`. The real container exit determines terminal status and
phase context. StopBuild, StopBuildBatch, and an aborted synchronous AWS Step
Functions task cancel the underlying container rather than only changing the
control-plane row.

The official AWS SDK and AWS CLI suites prove success, failure, retry, stop,
batch, private authenticated Git, Secrets Manager authentication, source
revision, and real configured-image execution. An AWS Step Functions
integration runs the AWS CLI inside that container against Amazon SQS through
the standard `AWS_ENDPOINT_URL` coordinate and proves both downstream delivery
and cancellation by the absence of a delayed write.
<!-- HAND-WRITTEN END -->
