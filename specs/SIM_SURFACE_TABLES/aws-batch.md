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
| CreateComputeEnvironment | `POST /v1/computeenvironments` | ✓ `TestBatch_ComputeEnvironment_SDK` | ✓ `TestBatch_ComputeEnvironment_CLI` | ✓ `aws_batch_compute_environment` | |
| DescribeComputeEnvironments | `GET /v1/computeenvironments` | ✓ | ✓ | ✓ | Filter by `?computeEnvironments=`. |
| UpdateComputeEnvironment | `PATCH /v1/computeenvironments/{name}` | ✓ | ✓ | ✓ | |
| DeleteComputeEnvironment | `DELETE /v1/computeenvironments/{name}` | ✓ | ✓ | ✓ | |

## Job Queues

| Operation | Verb + path | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| CreateJobQueue | `POST /v1/jobqueues` | ✓ `TestBatch_JobQueue_SDK` | ✓ `TestBatch_JobQueue_CLI` | ✓ `aws_batch_job_queue` | |
| DescribeJobQueues | `GET /v1/jobqueues` | ✓ | ✓ | ✓ | Filter by `?jobQueues=`. |
| UpdateJobQueue | `PATCH /v1/jobqueues/{name}` | ✓ | ✗ | ✓ | |
| DeleteJobQueue | `DELETE /v1/jobqueues/{name}` | ✓ | ✓ | ✓ | |

## Job Definitions

| Operation | Verb + path | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| RegisterJobDefinition | `POST /v1/jobdefinitions` | ✓ `TestBatch_JobDefinition_SDK` | ✓ `TestBatch_JobDefinition_CLI` | ✓ `aws_batch_job_definition` | Auto-increments `revision`. |
| DescribeJobDefinitions | `GET /v1/jobdefinitions` | ✓ | ✓ | ✓ | Filter by `?jobDefinitionName=` and `?status=`. |
| DeregisterJobDefinition | `DELETE /v1/jobdefinitions/{name}` | ✓ | ✓ | ✓ | Marks revision `INACTIVE`. |

## Jobs

| Operation | Verb + path | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| SubmitJob | `POST /v1/jobs` | ✓ `TestBatch_JobSubmitDescribe_SDK` | ✓ `TestBatch_SubmitJob_CLI` | — | Completes immediately with `SUCCEEDED`. |
| DescribeJobs | `POST /v1/jobs/describe` | ✓ | ✓ | — | Body: `{"jobs": [...ids]}`. |
| ListJobs | `GET /v1/jobs` | ✓ | ✓ | — | Filter by `?jobQueue=` and `?jobStatus=`. |
| CancelJob | `POST /v1/jobs/{id}/cancel` | ✗ | ✗ | — | No-op on terminal jobs. |
