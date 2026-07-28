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
history, input/output inspection, publishing, aliases, tags, and redrive. AWS
Private Certificate Authority and Amazon Data Firehose added complete
authority lifecycle, encrypted delivery-stream, and Amazon S3 delivery
workflows. The production UI passed 239 Chromium package tests, and the
authenticated Shauth/Ory Hydra/PostgreSQL matrix exercised all four services
through federated AWS credentials.

AWS Step Functions executed optimized and SDK Amazon ECS and AWS CodeBuild
tasks with request/response, synchronous, callback, failure, and cancellation
semantics. AWS Amplify encrypted connected-repository credentials, cloned
private repositories, and executed Python and Node.js build specifications.
Amazon Relational Database Service ran real PostgreSQL and MySQL data planes
with native TLS and IAM database authentication. Explicit AWS Lambda
deployments and AWS CodeBuild workloads reached downstream AWS APIs through
the standard global or per-service endpoint coordinates. The production AWS
console passed 239 Chromium tests and its authenticated browser matrix covered
the Amplify and RDS workflows.

The AWS CLI harness provisioned and validated the official Session Manager
plugin when the host lacked it, so Amazon ECS ExecuteCommand coverage no longer
depended on undeclared host tooling. Route-conformance builds registered the
full AWS surface without starting runtime evaluator goroutines, removing the
store-rebinding race while production builds retained their Amazon CloudWatch
and Application Auto Scaling evaluators.

The CI closure kept the external-client contracts real. CloudWatch
metric-stream CLI coverage provisioned Amazon S3, IAM, and Amazon Data Firehose
resources instead of using placeholder ARNs. Azure Container Apps and Azure
Functions Terraform modules and examples used HashiCorp AzureRM 5.0.0, and the
production-shaped Azure simulator stack migrated every resource whose provider
schema became ID-based. The official provider completed a
Microsoft.Subscription apply, zero-drift plan, and destroy. Google Discovery
drift failures retained the exact newest response as a short-lived artifact;
the transient Cloud Resource Manager 20260715 rollout disappeared from every
sampled edge, so the pinned 20260709 documents remained the truthful source.
The Azure Terraform job installed Ubuntu's signed Caddy package through its
retry- and timeout-bounded APT path, so a third-party repository bootstrap
could no longer consume the provider test's execution budget.

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

No locally actionable bug remained in this workspace. AWS Private Certificate
Authority implemented all 23 vendored operations and supplied real authority,
key, certificate, revocation-list, permission, policy, and audit-report state
to AWS Certificate Manager. Amazon Data Firehose implemented all 12 vendored
operations with durable encrypted buffering and IAM-authorized Amazon S3
delivery for direct writes, Amazon SNS subscriptions, and Amazon CloudWatch
metric streams. Both services shipped with official AWS SDK, AWS CLI,
Terraform, and authenticated browser coverage.

The external review's locally actionable gaps were closed: AWS Step Functions
ran Amazon ECS and AWS CodeBuild workloads, AWS Amplify authenticated private
repositories and ran multi-language builds, Amazon RDS exposed real native data
planes with IAM database authentication, and deployed workloads used the
standard SDK endpoint environment variables. Explicit Lambda deployment
remained intentional because AWS Lambda itself runs only functions a caller
creates; the repository retained its truthful unaudited/non-production
warning because functional validation did not constitute an independent
security audit.

The next pass should recheck the six external blockers below and resume only
when their missing credentials, upstream API coordinates, published schemas,
provider transports, or external repository become available. Mobile push and
SMS remained under BUG-2712 because no available public AWS configuration
exposed the carrier/provider primitives needed for faithful delivery.

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
