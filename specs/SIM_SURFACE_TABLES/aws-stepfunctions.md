# AWS Step Functions

Surface: `simulators/aws/stepfunctions.go`.

Canonical reference: <https://docs.aws.amazon.com/step-functions/latest/apireference/>

Protocol: AWS JSON 1.0 (`X-Amz-Target: AWSStepFunctions.<Op>`).

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## State machines

| Operation | X-Amz-Target | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| CreateStateMachine | `AWSStepFunctions.CreateStateMachine` | ✓ `TestSFN_StateMachineCRUD_SDK` | ✓ `TestSFN_StateMachineCRUD_CLI` | ✓ `aws_sfn_state_machine` | Returns `stateMachineArn` + `creationDate`. |
| DescribeStateMachine | `AWSStepFunctions.DescribeStateMachine` | ✓ | ✓ | ✓ | |
| ListStateMachines | `AWSStepFunctions.ListStateMachines` | ✓ | ✓ | — | Paginated via `nextToken`. |
| UpdateStateMachine | `AWSStepFunctions.UpdateStateMachine` | ✗ | ✗ | ✓ | Updates definition and/or roleArn. |
| DeleteStateMachine | `AWSStepFunctions.DeleteStateMachine` | ✓ | ✓ | ✓ | |
| TagResource | `AWSStepFunctions.TagResource` | ✓ | ✓ | ✓ | |
| UntagResource | `AWSStepFunctions.UntagResource` | ✓ | ✓ | ✓ | |
| ListTagsForResource | `AWSStepFunctions.ListTagsForResource` | ✓ | ✓ | ✓ | |

## Executions

| Operation | X-Amz-Target | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| StartExecution | `AWSStepFunctions.StartExecution` | ✓ `TestSFN_ExecutionLifecycle_SDK`; ✓ `TestSFN_FailState_SDK` | ✓ `TestSFN_ExecutionLifecycle_CLI` | — | Executes supported ASL states (`Pass`, `Succeed`, `Fail`, `Wait`) and records terminal status. |
| DescribeExecution | `AWSStepFunctions.DescribeExecution` | ✓ | ✓ | — | |
| ListExecutions | `AWSStepFunctions.ListExecutions` | ✓ | ✓ | — | Filterable by `statusFilter`. |
| StopExecution | `AWSStepFunctions.StopExecution` | ✓ `TestSFN_StopExecution_SDK` | ✗ | — | Aborts running executions. |
