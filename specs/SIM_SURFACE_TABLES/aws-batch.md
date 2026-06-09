# AWS Batch

Surface: `simulators/aws/batch.go`.

Canonical reference: <https://docs.aws.amazon.com/batch/latest/APIReference/>

Protocol: REST/JSON with operation-specific POST paths (`/v1/<lowercaseopname>`); no `X-Amz-Target`.

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Compute Environments

| Operation | Verb + path | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| CreateComputeEnvironment | `POST /v1/createcomputeenvironment` | ✓ `TestBatch_ComputeEnvironment_SDK` | ✓ `TestBatch_ComputeEnvironment_CLI` | ✓ `aws_batch_compute_environment` | |
| DescribeComputeEnvironments | `POST /v1/describecomputeenvironments` | ✓ | ✓ | ✓ | Body carries `computeEnvironments`. |
| UpdateComputeEnvironment | `POST /v1/updatecomputeenvironment` | ✓ | ✓ | ✓ | |
| DeleteComputeEnvironment | `POST /v1/deletecomputeenvironment` | ✓ | ✓ | ✓ | |

## Job Queues

| Operation | Verb + path | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| CreateJobQueue | `POST /v1/createjobqueue` | ✓ `TestBatch_JobQueue_SDK` | ✓ `TestBatch_JobQueue_CLI` | ✓ `aws_batch_job_queue` | |
| DescribeJobQueues | `POST /v1/describejobqueues` | ✓ | ✓ | ✓ | Body carries `jobQueues`. |
| UpdateJobQueue | `POST /v1/updatejobqueue` | ✓ | ✗ | ✓ | |
| DeleteJobQueue | `POST /v1/deletejobqueue` | ✓ | ✓ | ✓ | |

## Job Definitions

| Operation | Verb + path | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| RegisterJobDefinition | `POST /v1/registerjobdefinition` | ✓ `TestBatch_JobDefinition_SDK` | ✓ `TestBatch_JobDefinition_CLI` | ✓ `aws_batch_job_definition` | Auto-increments `revision`. |
| DescribeJobDefinitions | `POST /v1/describejobdefinitions` | ✓ | ✓ | ✓ | Body carries `jobDefinitionName` and `status`. |
| DeregisterJobDefinition | `POST /v1/deregisterjobdefinition` | ✓ | ✓ | ✓ | Marks revision `INACTIVE`. |

## Jobs

| Operation | Verb + path | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| SubmitJob | `POST /v1/submitjob` | ✓ `TestBatch_JobSubmitDescribe_SDK` | ✓ `TestBatch_SubmitJob_CLI` | — | Runs the registered container image and records status from the real exit code. |
| DescribeJobs | `POST /v1/describejobs` | ✓ | ✓ | — | Body carries `jobs`. |
| ListJobs | `POST /v1/listjobs` | ✓ | ✓ | — | Body carries `jobQueue` and `jobStatus`. |
| CancelJob | `POST /v1/canceljob` | ✗ | ✗ | — | Cancels the running workload when a handle exists. |
