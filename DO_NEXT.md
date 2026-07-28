# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Completed Baseline

AWS Lambda implemented all 85 operations in the vendored Smithy service model.
ZIP and image functions executed through the AWS Lambda Runtime API; layers,
versions, aliases, function URLs, concurrency, capacity providers, response
streaming, code signing, durable executions, callbacks, timeouts, pagination,
and lifecycle validation retained real service state and response shapes.
Deployment-package and layer roots were readable by Lambda's sandbox user and
mounted read-only, so managed runtimes executed the same ZIP on Linux and
Docker Desktop.

AWS Step Functions implemented all 37 operations in its vendored Smithy model.
Standard and Express Workflows executed JSONPath and JSONata definitions with
Pass, Task, Choice, Wait, Succeed, Fail, Parallel, Map, distributed Map,
activities, callbacks, retries, nested workflows, Lambda tasks, redrive,
versions, and aliases. Execution snapshots and histories retained immutable,
service-shaped events and input/output.

Official AWS SDK, AWS CLI, and Terraform suites exercised both services through
their public APIs. Selected control-plane, runtime, history, nested-workflow,
distributed-Map, ZIP/layer, and version/alias flows ran against short-lived
live AWS resources and matched the simulator differential. The live resources
and temporary IAM roles were removed after validation.

The AWS console exposed Lambda overview, code, test, logs, configuration,
layers, environment, concurrency, versions, aliases, URLs, and tags. Its Step
Functions experience exposed the graph, editable definition, execution input,
history, input/output inspection, publishing, aliases, tags, and redrive. The
production UI passed 229 Chromium package tests, and the authenticated
Shauth/Ory Hydra/PostgreSQL matrix created a state machine through federated AWS
credentials, started it, and inspected its graph and execution history.

The AWS CLI harness provisioned and validated the official Session Manager
plugin when the host lacked it, so Amazon ECS ExecuteCommand coverage no longer
depended on undeclared host tooling. Route-conformance builds registered the
full AWS surface without starting runtime evaluator goroutines, removing the
store-rebinding race while production builds retained their Amazon CloudWatch
and Application Auto Scaling evaluators.

The publication repair preserved current public contracts across the failing
client surfaces. Amazon SQS redrive used the normal enqueue path and therefore
assigned a new message ID, millisecond enqueue timestamp, FIFO sequence, and
destination delay; its validation audit used the current 1 MiB limit. An
omitted Amazon ECS launch type selected EC2 capacity rather than an AWS Fargate
sandbox. Azure Database for PostgreSQL flexible servers round-tripped their
top-level SKU through create, update, get, list, the official Azure SDK, and
the AzureRM provider. Google Cloud Run v1 collection validation located the
projected resource within the real shared collection. The Azure console's
embedded-root contract ran only in UI-bearing builds, while `noui` retained a
real 404. Google Cloud DNS and Artifact Registry specifications were refreshed
to Discovery revisions 20260723 and 20260724.

## Next Recommended Slice

The next locally actionable AWS slice became BUG-2714: AWS Private Certificate
Authority. Completion required a real authority/key/certificate lifecycle,
AWS Certificate Manager issuance from an existing authority ARN, encrypted
private-key export, revocation, and official AWS SDK, AWS CLI, and Terraform
coverage.

BUG-2712 retained the adjacent outbound-delivery work. Amazon SNS email and
email-json completed real SMTP confirmation and delivery, while an Amazon Data
Firehose service remained necessary for both SNS subscriptions and Amazon
CloudWatch metric streams. Mobile push and SMS could be connected only through
provider/carrier primitives represented in a public AWS contract; the simulator
did not invent private configuration for them.

## Externally Blocked Work

- BUG-1075 retained authenticated Google Cloud Run, Azure Container Apps,
  Azure Functions, Lambda service-mesh, and Azure identity-backed live-cloud
  cells that required operator credentials.
- BUG-2646 retained Google's publication of Cloud Run worker-pool scaling
  members in the Discovery document.
- BUG-1345 retained the upstream AzureAD Terraform provider's missing
  Microsoft Graph endpoint override.
- BUG-2523 and BUG-2441 remained owned by the external Bleephub repository,
  which was not present in this workspace.

## Durable Validation Contract

- Simulator endpoints were exercised through official SDK, vendor CLI, and
  Terraform surfaces in the same change.
- Tests differed between simulator and cloud only in endpoint and credential
  coordinates.
- Production builds created every frontend before any UI-bearing Go binary.
- Workflow changes kept every ordinary job at or below 15 minutes and
  preserved exact AWS CLI and SDK shard coverage.
- Dependency freshness retained authenticated GitHub API requests in both its
  Bash and Zsh portability passes.
- Every observed failure or warning was fixed or recorded with evidence in
  [BUGS.md](BUGS.md).
