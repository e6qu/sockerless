# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Completed Baseline

The same-day AWS SDK release wave advanced Lambda to `v1.101.0` in the Lambda
backend and official SDK suite and IAM to `v1.57.0` in the SDK suite. The
Lambda backend built and passed its tests, the complete official AWS SDK suite
passed in 546.212 seconds, and the repository-wide dependency, Terraform
provider, and GitHub Actions audit was current.

The exhaustive local AWS SDK target no longer inherited Go's ten-minute
package timeout. The shared Go library test recipe accepted module-specific
flags, the AWS SDK suite declared a 30-minute budget, and hosted CI retained
its separate four-shard limits.

Microsoft Azure resource-deletion dialogs retained Azure Resource Manager's
actionable failure after a rejected request even when a concurrent Fluent UI
backdrop event arrived. Backdrop dismissal was suppressed only while the error
was displayed; explicit Cancel and Escape remained functional. The Azure
console typecheck, all 131 package tests, and production build passed.

Secondary process-mode AWS simulators no longer inherited the default Route 53
DNS listener port. The AWS CLI harness assigned each nested process an
operating-system-selected UDP/TCP coordinate, so a simulator already using
port 5353 could not prevent the child from starting. The focused process-mode
case and the complete compute shard passed.

Amazon ECS task definitions now turn `taskRoleArn` into usable workload
credentials. The task-metadata service mints expiring `ASIA` sessions bound to
the configured IAM role, registers them with the simulator's Signature Version
4 verifier, and injects the standard ECS relative credential URI in the
real-VPC network tier. The Docker-network compatibility tier exposes the same
task-scoped endpoint through its reachable host coordinate. An official AWS CLI
container consumed the credentials and returned its exact assumed-role ARN
from `sts:GetCallerIdentity`.

Amazon ECS services now complete the discovery and failed-deployment
contracts left after the core scheduler landed. Running service tasks register
their real network address and port in AWS Cloud Map and reconcile those
instances through replacement, scale-in, deletion, and hard restart. Durable
launch failures apply bounded backoff and drive configured deployment circuit
breakers or CloudWatch alarms to failure and rollback. Restart regressions
retain failure timing and release adopted Fargate network state on deletion.
Load-balanced deployments retain one bounded reconciliation timer while in
progress, so real target health that changes after the first steady-state probe
completes the rollout without another API request or task transition.

Amazon DynamoDB enforces its 400 KiB item limit from stored attribute bytes
rather than JSON encoding size. AWS Secrets Manager owns independent
Region-scoped primary and replica records with synchronization, removal,
promotion, and persistence. AWS Step Functions generic AWS SDK integrations
now dispatch generated Smithy bindings for AWS JSON, AWS Query, REST JSON, and
REST XML protocols. The AWS console exposes ECS service deployment operations
and Secrets Manager replica management through those public service APIs.

The Terraform-in-ECS workload image carries an ahead-of-time filesystem mirror
of the exact AWS provider, so its private-subnet task performs one offline
initialization and one apply without undeclared internet egress. It publishes
exact task output through Amazon CloudWatch Logs, and terminal Step Functions
failures report that output immediately. The focused real-container execution
and exact AWS SDK N-Z shard passed.

Core filesystem-driver staging validation no longer assumed `/usr/local` was
unwritable. Both tests force the direct path to fail portably by creating the
requested destination beneath a regular file, independent of runner privilege.

Google Cloud and Microsoft Azure SDK/CLI jobs pre-fetched their separate
official-client modules through the bounded dependency-download helper before
the suites started.

Simulator SDK, CLI, and Terraform matrix jobs restored the immutable
Firecracker seed cache without trying to save their root-mutated guest
filesystems. The dedicated Firecracker job remained the cache publisher.

The Microsoft Azure workload-dispatch invariant keeps its two justified
`os/exec` exceptions as source comments without logging file-and-line-shaped
messages that GitHub's Go problem matcher turns into failure annotations.

The pre-push dependency audit's coordinated AWS SDK patch wave and Google Cloud
Spanner client release were applied across every affected Go module with the
repository-owned upgrade target. Direct pins and their resolved transitive
graphs were current again. Go 1.26 module reconciliation also advanced the
Microsoft Azure and Google Cloud common backends to the selected
`go-isatty` 0.0.24 transitive release; both focused lint and unit suites passed.

AWS Key Management Service custom key policies persisted in the simulator's
SQLite key record instead of disappearing during JSON serialization. A
store-close-and-reopen regression proved durable read-back, and the
production-shaped HashiCorp AWS provider graph supplied a custom policy so its
post-create policy waiter exercised the same contract.

AWS DynamoDB auxiliary table state no longer depended on fields excluded from
JSON serialization. TTL, point-in-time recovery, and tags lived in one durable
out-of-band settings record, deletion removed that record, IAM resource-tag
conditions read it, and a SQLite close-and-reopen regression plus the
production-shaped provider graph exercised all three convergence paths.

The hosted Google Cloud specification gate's exact Cloud SQL Admin v1 and
v1beta4 revision 20260722 artifacts were retained. Their 75 methods and routes
were unchanged, while authenticated public-route coverage implemented and
round-tripped the three newly published schema members.

The hosted Dataflow v1b3 Discovery edge advanced to revision 20260719 after
local validation. The exact preserved artifact replaced the older pin after a
structural comparison proved all 42 methods and paths and all 1,174 schema
field/type entries unchanged. A subsequent local multi-probe fetch observed
API Gateway v1 revision 20260724; all 30 methods and paths and all 143 schema
field/type entries were likewise unchanged.

AWS simulator state survived hard process replacement as a coherent cloud
slice. The SQLite envelope retained exported runtime configuration hidden from
public JSON, startup rebuilt monotonic counters and derived revisions, real
Network Load Balancing and Amazon RDS listeners rebound, and state-scoped
Amazon ECS, AWS Batch, CodeBuild, Amplify, Lambda, scheduler, and autoscaling
work was adopted or resumed. Asynchronous Lambda work became durable before
acceptance, while Step Functions checkpointed and reattached to the original
Amazon ECS or CodeBuild task. Official AWS SDK and AWS CLI restart suites
passed, and the production-shaped HashiCorp AWS provider completed apply,
hard restart, zero-change refresh, and destroy.

Legacy Amazon ECS state from releases before state-scoped workload adoption no
longer prevented that durable simulator from starting. A persisted task that
claimed `RUNNING` but had zero matching runtime containers became truthfully
`STOPPED` with the restart cause and unknown exit code; recovery continued
through the remaining tasks, while Docker or Podman discovery failures still
failed startup loudly.

The production Compose recipes enabled the existing durable stores for the
AWS, Google Cloud, and Microsoft Azure simulators on named volumes. The AWS
Batch console listed real jobs and definitions, polled status, surfaced
terminal details, and terminated live work through standard AWS APIs.
Associated AWS WAF web ACLs evaluated the complete supported statement graph,
and Elastic Load Balancing listener creates and modifications failed
transactionally when their real TCP or TLS binding could not be provisioned.
The Elastic Load Balancing official-client fixtures imported issued,
exportable AWS Certificate Manager certificates and selected isolated real
listener ports, while nested simulator processes received their own Route 53
DNS coordinate. The focused listener cases and complete AWS SDK compute shard
passed together.

AWS Glue database tags remained durable internal state and were projected only
through `GetTags`, so `GetDatabase` matched its Smithy model without losing
tags across hard process replacement. An unconfigured AWS Cloud Map HTTP
service no longer gained an invented custom-health configuration. The durable
Lambda callback restart proof read its runtime checkpoint from Amazon
CloudWatch Logs through the official AWS SDK rather than a host callback. The
complete AWS SDK services A–M shard passed with zero Smithy violations, and the
complete AWS CLI edge-delivery shard passed with real imported certificates
and isolated listener ports.

The macOS AWS Terraform container wrapper mounted the runtime Smithy report
back to the host, surfaced Podman attachment failures, and removed the exact
container plus anonymous volumes. A non-destructive Podman virtual-machine
restart cleared the volatile overlay fault, after which the complete local
provider apply, hard restart, refresh, assertions, and destroy passed.

The pre-push freshness gate advanced gRPC to 1.83.0 in both Cloud Run
backends, the shared Google Cloud backend, the simulator, and its official SDK
module. All five affected modules and the complete official Google Cloud SDK
suite passed. The Cloud Build build-and-push scenario separated real registry
container creation from startup and removed its anonymous volume, so Podman
start errors stayed visible and successful runs leaked no fixture storage.

Google Cloud Spanner executed SQL, DML, batch DML, reads, mutations,
transactions, partitions, and batch writes through real SQLite transactions
over official REST and gRPC clients. Strict DDL and composite-key behavior
passed official SDK and gcloud coverage; the HashiCorp Google provider passed
instance/database/DDL apply, a zero-change plan, and destroy.

AWS Step Functions launched the official HashiCorp Terraform image as a
synchronous Amazon ECS task, and Terraform applied Amazon SQS through the
standard global AWS endpoint. AWS Amplify retained build and test artifacts,
retry lifecycles, WAF association, hosted request enforcement, and sampled
requests. An unmodified ecs-dev-desktop graph applied 178 resources, converged
to a zero-change plan, passed external assertions with no Smithy violations,
and destroyed every resource.

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
workflows. The production UI passed 241 Chromium package tests, and the
authenticated Shauth/Ory Hydra/PostgreSQL matrix exercised all four services
through federated AWS credentials.

AWS Step Functions executed optimized and SDK Amazon ECS and AWS CodeBuild
tasks with request/response, synchronous, callback, failure, and cancellation
semantics. CodeBuild cloned authenticated Git revisions and ran checked-in or
explicit build specifications inside each project's exact configured image;
stop and workflow abort cancelled the real container. AWS Amplify encrypted
connected-repository credentials and executed backend, frontend, and test
pre/build/post phases with monorepo roots, environment precedence, declared
caches, and artifacts in a managed Python and Node.js image. Amazon Relational
Database Service ran real PostgreSQL, MySQL, and MariaDB data planes with
native TLS, IAM database authentication, live password rotation, and
stop/start volume persistence. Explicit AWS Lambda deployments and CodeBuild
workloads reached downstream AWS APIs through the standard global or
per-service endpoint coordinates. The production AWS console passed 241
Chromium tests and its authenticated browser matrix operated CodeBuild,
Amplify, and RDS through federated AWS credentials.

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
could no longer consume the provider test's execution budget. Microsoft.Network
subnets accepted AzureRM 5's plural `addressPrefixes` request and used it for
the real network fabric. Azure Container Apps environments that linked a Log
Analytics workspace explicitly selected the provider-required `log-analytics`
destination. Failed portal deletions kept their provider error inside the open
Fluent confirmation surface instead of racing with dismissal. The AWS Lambda
module's Step Functions differential role built its policy ARNs from the
module's declared `region` input, and all six production modules validated.
The complete Terraform tree also retained canonical HCL formatting.

Every AWS Terraform graph declared HashiCorp AWS provider 6.50.0, so the root
production graph and its sibling packages executed one reproducible provider
implementation. The root graph passed its complete concurrent apply, workload
assertions, refresh, and destroy through Caddy HTTPS with runtime Smithy
validation armed. The Microsoft Azure console's failed-delete assertion awaited
React Query's settled mutation before checking the retained accessible Fluent
dialog; all 131 package tests and the complete UI fan-out passed.

The final hosted freshness pass advanced the exact Google Discovery documents
for Bigtable Admin v2, Cloud Logging v2, Pub/Sub v1, and Cloud Resource Manager
v1/v3. The two methods newly present in those specifications were real
implementations: Bigtable memory layers retained enable/disable state and
etags and returned durable operations, while Cloud Resource Manager returned
resource-semantics metadata through its published v3 route. Authenticated
official-SDK transports exercised both methods, and generated surface coverage
measured Bigtable at 164 of 164 and Cloud Resource Manager v3 at 126 of 126.

AzureRM 5's complete external stack also converged after refresh.
Microsoft.OperationalInsights workspaces returned Azure's default public
network access values, and Microsoft.Storage File-share policies stayed
consistent between the ARM resource and Azure Files data plane. The official
Azure SDK round-tripped the share policy and Azure CLI round-tripped the
workspace defaults. The external stack's post-plan assertions used AzureRM
5's canonical Microsoft.Storage ARM IDs for Blob containers, Tables, and File
shares instead of the removed legacy data-plane IDs.

The pull-request CI freshness pass supplied the exact newer official Cloud
Logging v2 revision 20260724 and IAM Service Account Credentials v1 revision
20260723 Discovery documents. Their method, route, and schema-field sets were
unchanged; the newer descriptions and provenance pins were retained, and the
Google simulator route, specification, and measured-coverage suite passed.

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

BUG-2798 and BUG-2799 closed. ECS services now drive durable AWS Cloud Map
registration from real task transitions and implement persisted launch
throttling, deployment circuit-breaker rollback, and CloudWatch-alarm rollback.
Official AWS SDK and AWS CLI scenarios, hard-restart regressions, and the
production-shaped HashiCorp AWS provider graph exercised the completed data
plane.

BUG-2766 remained the next independent AWS fidelity slice: implement the
published AWS Amplify Hosting `ImageOptimization` fetch, source-policy,
transformation, validation, format-negotiation, and cache contract, then prove
it through hosted requests and external image decoders. BUG-2764 remained a
host boundary: the shared Linux test image contained the real Firecracker and
squashfs tools, while the macOS Podman virtual machine exposed no nested KVM;
the capable-Linux Terraform CI cell remained mandatory.

The completed baseline retained real AWS Private Certificate Authority and
Amazon Data Firehose implementations with official SDK, AWS CLI, Terraform,
and authenticated browser coverage.

The external review's locally actionable gaps and the follow-up implementation
audit were closed. AWS Step Functions ran and cancelled real Amazon ECS and
AWS CodeBuild workloads; CodeBuild used the requested source revision,
credential, build specification, and image; AWS Amplify ran authenticated
multi-language monorepo builds with complete phase, cache, and artifact
lifecycle; Amazon RDS exposed persistent PostgreSQL, MySQL, and MariaDB native
data planes with TLS-only IAM authentication and real password rotation; and
deployed workloads used the standard SDK endpoint environment variables.
Hosted concurrency validation preserved sub-second AWS Amplify release order,
accepted Microsoft Azure's valid subnet-before-public-prefix NAT-gateway
state, and gave the real Step Functions container integrations a
cloud-shaped cold-provisioning window with useful terminal diagnostics. The
AWS SDK shard provisioned the exact configured Alpine and official AWS CLI
images before `m.Run`, so registry transfer no longer consumed that
integration's lifecycle deadline while both real containers still executed.
Explicit Amazon ECR Public coordinates reached the container runtime unchanged,
and cancellation killed the CodeBuild workload whether Docker completed its
wait through the context or error channel, so a stopped build produced no
delayed Amazon SQS side effect. The macOS/Linux Docker validation harness loaded
Buildx output and shared the container host's PID namespace; the full
production-shaped HashiCorp AWS provider graph completed apply, a real
VPC-attached Lambda invocation, refresh, and destroy through HTTPS.
The Amazon ECS integration harness loaded its real arithmetic workload through
the backend's Docker Image Load API instead of building it outside the backend
catalog; live-cloud runs required the corresponding pre-provisioned Amazon ECR
coordinate, and all six simulator-backed real-container cases passed.
The AWS external Terraform harness preserved the original request host through
Caddy for AWS Signature Version 4, serialized heavyweight packages locally,
and assigned the root, Amazon ElastiCache, and three Amazon RDS graphs to
separate hosted runners. All five HTTPS packages completed apply, real
workload or data-plane assertions, and destroy without cross-package resource
contention.
The mandatory publication audit upgraded the AWS simulator to `go-git` 5.19.2
and its current transitive graph. The complete module suite passed, and the
authenticated dependency audit reported no drift.
The shared e2e harness loaded its compiled arithmetic fixture through every
active cloud backend's Docker Image Load API, keeping the backend catalog
authoritative. The exact e2e suite and its optional second Amazon ECS
simulator-backend path passed.
The hosted publication edge then advanced `docker/login-action` to 4.6.0.
Both immutable multi-architecture publication jobs upgraded, and action
syntax, the publication contract, and the authenticated freshness audit
passed.
Native Linux workload coordinates retained Docker's
`host.docker.internal:host-gateway` alias instead of rewriting it to the
virtual machine's default gateway; rewriting remained correct for a simulator
that itself ran in a container. The official AWS SDK Step Functions
integration passed its real Amazon ECS task, AWS CodeBuild container, and
vendor AWS CLI flow.
Publication also upgraded every newly drifted SQLite and Google Cloud client
module, moved Firestore and Spanner protobuf imports to their current
canonical modules, and passed the complete official Google Cloud SDK suite.
The exact hosted Cloud Run v1 and v2 Discovery revision 20260727 documents were
also retained; their public methods, paths, and schema fields were unchanged,
and the Google simulator route, specification, and measured-coverage suite
passed against their newer descriptions.
The three console accessibility checks anchored keyboard traversal at the
loaded document before pressing Tab, so real Chromium consistently proved each
skip link was the first in-document focus target.
Explicit Lambda deployment remained intentional because AWS Lambda itself runs
only functions a caller creates. The repository retained its truthful
unaudited/non-production warning because functional validation did not
constitute an independent security audit.

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
