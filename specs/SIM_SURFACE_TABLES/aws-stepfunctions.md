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
| `Action AWSStepFunctions.CreateStateMachine` | ✓ `simulators/aws/stepfunctions.go:98::handleSFNCreateStateMachine` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeStateMachine` | ✓ `simulators/aws/stepfunctions.go:99::handleSFNDescribeStateMachine` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListStateMachines` | ✓ `simulators/aws/stepfunctions.go:100::handleSFNListStateMachines` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DeleteStateMachine` | ✓ `simulators/aws/stepfunctions.go:101::handleSFNDeleteStateMachine` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.UpdateStateMachine` | ✓ `simulators/aws/stepfunctions.go:102::handleSFNUpdateStateMachine` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.TagResource` | ✓ `simulators/aws/stepfunctions.go:103::handleSFNTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.UntagResource` | ✓ `simulators/aws/stepfunctions.go:104::handleSFNUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListTagsForResource` | ✓ `simulators/aws/stepfunctions.go:105::handleSFNListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ValidateStateMachineDefinition` | ✓ `simulators/aws/stepfunctions.go:106::handleSFNValidateStateMachineDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListStateMachineVersions` | ✓ `simulators/aws/stepfunctions.go:107::handleSFNListStateMachineVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.StartExecution` | ✓ `simulators/aws/stepfunctions.go:108::handleSFNStartExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeExecution` | ✓ `simulators/aws/stepfunctions.go:109::handleSFNDescribeExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.GetExecutionHistory` | ✓ `simulators/aws/stepfunctions.go:110::handleSFNGetExecutionHistory` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListExecutions` | ✓ `simulators/aws/stepfunctions.go:111::handleSFNListExecutions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.StopExecution` | ✓ `simulators/aws/stepfunctions.go:112::handleSFNStopExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.CreateActivity` | ✓ `simulators/aws/stepfunctions.go:118::handleSFNCreateActivity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DeleteActivity` | ✓ `simulators/aws/stepfunctions.go:119::handleSFNDeleteActivity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeActivity` | ✓ `simulators/aws/stepfunctions.go:120::handleSFNDescribeActivity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListActivities` | ✓ `simulators/aws/stepfunctions.go:121::handleSFNListActivities` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.GetActivityTask` | ✓ `simulators/aws/stepfunctions.go:122::handleSFNGetActivityTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.SendTaskSuccess` | ✓ `simulators/aws/stepfunctions.go:123::handleSFNSendTaskSuccess` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.SendTaskFailure` | ✓ `simulators/aws/stepfunctions.go:124::handleSFNSendTaskFailure` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.SendTaskHeartbeat` | ✓ `simulators/aws/stepfunctions.go:125::handleSFNSendTaskHeartbeat` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.PublishStateMachineVersion` | ✓ `simulators/aws/stepfunctions.go:131::handleSFNPublishStateMachineVersion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DeleteStateMachineVersion` | ✓ `simulators/aws/stepfunctions.go:132::handleSFNDeleteStateMachineVersion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.CreateStateMachineAlias` | ✓ `simulators/aws/stepfunctions.go:133::handleSFNCreateStateMachineAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DeleteStateMachineAlias` | ✓ `simulators/aws/stepfunctions.go:134::handleSFNDeleteStateMachineAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeStateMachineAlias` | ✓ `simulators/aws/stepfunctions.go:135::handleSFNDescribeStateMachineAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListStateMachineAliases` | ✓ `simulators/aws/stepfunctions.go:136::handleSFNListStateMachineAliases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.UpdateStateMachineAlias` | ✓ `simulators/aws/stepfunctions.go:137::handleSFNUpdateStateMachineAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeStateMachineForExecution` | ✓ `simulators/aws/stepfunctions.go:140::handleSFNDescribeStateMachineForExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.RedriveExecution` | ✓ `simulators/aws/stepfunctions.go:141::handleSFNRedriveExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.DescribeMapRun` | ✓ `simulators/aws/stepfunctions.go:143::handleSFNDescribeMapRun` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.ListMapRuns` | ✓ `simulators/aws/stepfunctions.go:144::handleSFNListMapRuns` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.UpdateMapRun` | ✓ `simulators/aws/stepfunctions.go:145::handleSFNUpdateMapRun` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.TestState` | ✓ `simulators/aws/stepfunctions.go:148::handleSFNTestState` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSStepFunctions.StartSyncExecution` | ✓ `simulators/aws/stepfunctions.go:149::handleSFNStartSyncExecution` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
