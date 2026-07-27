# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Completed Baseline

AWS Lambda image invocations honoured `VpcConfig` at launch. The control plane
validated that configured subnets and security groups belonged to one Amazon
Virtual Private Cloud (VPC), and the runtime leased its address from those
subnets instead of joining Docker's default bridge. Linux execution used a
pause-container network namespace, a real VPC veth, security-group filters,
route-driven egress, and a dedicated link-local DNAT endpoint for the AWS Lambda
Runtime API. Portable execution used the same VPC identifiers and addresses
through the container engine's VPC network.

The official AWS SDK test created an Amazon ECS Fargate task on `awsvpc`,
invoked a VPC-configured Lambda image, and proved the function reached the task
at its private address. The AWS CLI and Terraform suites launched
VPC-configured functions through their normal public surfaces. The complete
Lambda SDK and CLI suites passed, as did the production-shaped Terraform
apply/destroy.

Amazon Cloud Map custom health checks reported AWS's fixed failure threshold of
`1`; the SDK Create/Get/List paths agreed. The Terraform fixture used supported
Cloud Map and DynamoDB schema, configured local state through the local backend,
and validated without deprecation warnings. Normal AWS SDK, CLI, and Terraform
harness completion terminated the simulator through its cleanup path and left
no workload containers or simulator VPC networks behind.

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

## Durable Validation Contract

- Simulator endpoints were exercised through official SDK, vendor CLI, and
  Terraform surfaces in the same change.
- Tests differed between simulator and cloud only in endpoint and credential
  coordinates.
- Production builds created every frontend before any UI-bearing Go binary.
- Workflow changes kept every ordinary job at or below 15 minutes and
  preserved exact AWS CLI and SDK shard coverage.
- Every observed failure or warning was fixed or recorded with evidence in
  [BUGS.md](BUGS.md).
