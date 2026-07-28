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

## Next Recommended Slice

The next recommended AWS fidelity slice remained BUG-2679: Amazon EC2
`DeleteSubnet` dependency enforcement. Completion meant:

- `DeleteSubnet` returned Amazon EC2's `DependencyViolation` while any elastic
  network interface remained in the subnet, including interfaces backing
  Amazon ECS tasks and active AWS Lambda invocations.
- The dependency decision came from cloud state rather than local container
  state, and asynchronous task launch could not race subnet deletion into an
  invalid topology.
- Official AWS SDK, AWS CLI, and Terraform coverage exercised the refusal and
  the successful delete after the dependency was removed.

The next related Amazon ECS slices remained BUG-2680 (`StartTask` launched real
containers through the `RunTask` execution path) and BUG-2681 (sandbox selection
followed launch type instead of applying Fargate restrictions universally).

## Other Queued Fidelity Work

- BUG-2676 retained one Google Cloud Run service in two independent v1/v2
  stores; completion required one cloud resource with two API projections.
- BUG-2677 retained the Azure Files Share ACL gap that blocked
  `azurerm_storage_share` coverage.
- BUG-1075 retained the authenticated real-cloud backend cells that required
  operator credentials.
- BUG-2656 retained abnormal-exit cleanup; ordinary AWS harness shutdown was
  clean, but a process killed before defers ran still required an external
  run-labelled reaper.
- BUG-2690 retained Amazon Amplify's synthetic success lifecycle for
  build-shaped jobs without both a clonable HTTP(S) source and an explicit
  build specification; completion required real source/default-build
  resolution or the matching AWS service error.

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
