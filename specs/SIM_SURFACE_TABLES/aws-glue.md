# AWS Glue

Surface: `simulators/aws/glue.go`.

Canonical reference: <https://docs.aws.amazon.com/glue/latest/dg/aws-glue-api.html>

Protocol: AWS JSON 1.1 (`X-Amz-Target: AWSGlue.<Op>`).

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Data Catalog — Databases

| Operation | X-Amz-Target | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| CreateDatabase | `AWSGlue.CreateDatabase` | ✓ `TestGlue_DatabaseCRUD_SDK` | ✓ `TestGlue_DatabaseCRUD_CLI` | ✓ `aws_glue_catalog_database` | |
| GetDatabase | `AWSGlue.GetDatabase` | ✓ | ✓ | ✓ | Returns `Database` object. |
| GetDatabases | `AWSGlue.GetDatabases` | ✓ | ✓ | ✓ | Paginated via `NextToken`. |
| DeleteDatabase | `AWSGlue.DeleteDatabase` | ✓ | ✓ | ✓ | |

## Data Catalog — Tables

| Operation | X-Amz-Target | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| CreateTable | `AWSGlue.CreateTable` | ✓ `TestGlue_TableCRUD_SDK` | ✓ `TestGlue_TableCRUD_CLI` | ✓ `aws_glue_catalog_table` | Validates database exists. |
| GetTable | `AWSGlue.GetTable` | ✓ | ✓ | ✓ | Returns `Table` object. |
| GetTables | `AWSGlue.GetTables` | ✓ | ✓ | ✓ | Filtered by `DatabaseName`. |
| DeleteTable | `AWSGlue.DeleteTable` | ✓ | ✓ | ✓ | |

## Jobs

| Operation | X-Amz-Target | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| CreateJob | `AWSGlue.CreateJob` | ✓ `TestGlue_JobCRUD_SDK` | ✓ `TestGlue_JobCRUD_CLI` | ✓ `aws_glue_job` | |
| GetJob | `AWSGlue.GetJob` | ✓ | ✓ | ✓ | |
| GetJobs | `AWSGlue.GetJobs` | ✓ | ✓ | ✓ | Paginated via `NextToken`. |
| DeleteJob | `AWSGlue.DeleteJob` | ✓ | ✓ | ✓ | |
| StartJobRun | `AWSGlue.StartJobRun` | ✓ | ✓ | — | Executes Python shell scripts stored at the job's S3 script location. |
| GetJobRun | `AWSGlue.GetJobRun` | ✓ | ✓ | — | |
| GetJobRuns | `AWSGlue.GetJobRuns` | ✓ | ✗ | — | All runs for a job. |

## Tags

| Operation | X-Amz-Target | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| TagResource | `AWSGlue.TagResource` | ✓ `TestGlue_Tags_SDK` | ✗ | ✓ | |
| UntagResource | `AWSGlue.UntagResource` | ✓ | ✗ | ✓ | |
| GetTags | `AWSGlue.GetTags` | ✓ | ✗ | ✓ | |
