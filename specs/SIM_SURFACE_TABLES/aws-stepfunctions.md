# Sim surface — aws-stepfunctions

Surface registered in `simulators/aws/stepfunctions.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AWSStepFunctions.CreateStateMachine` | ✓ `simulators/aws/stepfunctions.go:95::handleSFNCreateStateMachine` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeStateMachine` | ✓ `simulators/aws/stepfunctions.go:96::handleSFNDescribeStateMachine` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListStateMachines` | ✓ `simulators/aws/stepfunctions.go:97::handleSFNListStateMachines` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DeleteStateMachine` | ✓ `simulators/aws/stepfunctions.go:98::handleSFNDeleteStateMachine` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.UpdateStateMachine` | ✓ `simulators/aws/stepfunctions.go:99::handleSFNUpdateStateMachine` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.TagResource` | ✓ `simulators/aws/stepfunctions.go:100::handleSFNTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.UntagResource` | ✓ `simulators/aws/stepfunctions.go:101::handleSFNUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListTagsForResource` | ✓ `simulators/aws/stepfunctions.go:102::handleSFNListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ValidateStateMachineDefinition` | ✓ `simulators/aws/stepfunctions.go:103::handleSFNValidateStateMachineDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListStateMachineVersions` | ✓ `simulators/aws/stepfunctions.go:104::handleSFNListStateMachineVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.StartExecution` | ✓ `simulators/aws/stepfunctions.go:105::handleSFNStartExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeExecution` | ✓ `simulators/aws/stepfunctions.go:106::handleSFNDescribeExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.GetExecutionHistory` | ✓ `simulators/aws/stepfunctions.go:107::handleSFNGetExecutionHistory` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListExecutions` | ✓ `simulators/aws/stepfunctions.go:108::handleSFNListExecutions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.StopExecution` | ✓ `simulators/aws/stepfunctions.go:109::handleSFNStopExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.CreateActivity` | ✓ `simulators/aws/stepfunctions.go:115::handleSFNCreateActivity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DeleteActivity` | ✓ `simulators/aws/stepfunctions.go:116::handleSFNDeleteActivity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeActivity` | ✓ `simulators/aws/stepfunctions.go:117::handleSFNDescribeActivity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListActivities` | ✓ `simulators/aws/stepfunctions.go:118::handleSFNListActivities` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.GetActivityTask` | ✓ `simulators/aws/stepfunctions.go:119::handleSFNGetActivityTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.SendTaskSuccess` | ✓ `simulators/aws/stepfunctions.go:120::handleSFNSendTaskSuccess` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.SendTaskFailure` | ✓ `simulators/aws/stepfunctions.go:121::handleSFNSendTaskFailure` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.SendTaskHeartbeat` | ✓ `simulators/aws/stepfunctions.go:122::handleSFNSendTaskHeartbeat` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.PublishStateMachineVersion` | ✓ `simulators/aws/stepfunctions.go:128::handleSFNPublishStateMachineVersion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DeleteStateMachineVersion` | ✓ `simulators/aws/stepfunctions.go:129::handleSFNDeleteStateMachineVersion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.CreateStateMachineAlias` | ✓ `simulators/aws/stepfunctions.go:130::handleSFNCreateStateMachineAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DeleteStateMachineAlias` | ✓ `simulators/aws/stepfunctions.go:131::handleSFNDeleteStateMachineAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeStateMachineAlias` | ✓ `simulators/aws/stepfunctions.go:132::handleSFNDescribeStateMachineAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListStateMachineAliases` | ✓ `simulators/aws/stepfunctions.go:133::handleSFNListStateMachineAliases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.UpdateStateMachineAlias` | ✓ `simulators/aws/stepfunctions.go:134::handleSFNUpdateStateMachineAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeStateMachineForExecution` | ✓ `simulators/aws/stepfunctions.go:137::handleSFNDescribeStateMachineForExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.RedriveExecution` | ✓ `simulators/aws/stepfunctions.go:138::handleSFNRedriveExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeMapRun` | ✓ `simulators/aws/stepfunctions.go:140::handleSFNDescribeMapRun` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListMapRuns` | ✓ `simulators/aws/stepfunctions.go:141::handleSFNListMapRuns` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.UpdateMapRun` | ✓ `simulators/aws/stepfunctions.go:142::handleSFNUpdateMapRun` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.TestState` | ✓ `simulators/aws/stepfunctions.go:145::handleSFNTestState` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.StartSyncExecution` | ✓ `simulators/aws/stepfunctions.go:146::handleSFNStartSyncExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
