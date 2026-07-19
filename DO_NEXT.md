# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current Branch

`fix/ecs-a-record-service-registry-validation` contained the complete simulator service-discovery, embedded-dashboard, deployment-neutral operator identity, immutable image-publication, and truthful fuzz-harness fixes. Its ARM64 images and shared user-interface/server tests passed locally.

## Continue Here

1. Merge this branch through its single pull request, wait for its short-SHA operator-console and simulator manifests, deploy them as real Amazon Elastic Container Service services, and run the complete live Shauth SSO acceptance matrix. Keep the operator's unauthenticated `/healthz` liveness coordinate separate from the Shauth-protected browser/operator boundary, consume each simulator's generic `/health` runtime-capability contract, and preserve the simulators' native cloud protocol endpoints for official SDK, CLI, and Terraform clients.
2. Resolve BUG-2569: make the local Amazon Elastic Container Service Terraform simulator apply/destroy harness terminate deterministically without weakening the real provider path.
3. Resolve BUG-2589: configure the local Azure Container Registry Tasks SDK harness with a Docker-trusted simulator registry transport, matching the working CI coordinate.
4. Continue complete simulator and backend fidelity work, including the open live-cloud cells documented in BUGS.md.

## Recent Validation

- Amazon ECS service-discovery SDK coverage proved that A-record registries rejected explicit ports and accepted task-ENI-only registration, matching the real Amazon ECS control plane.
- The AWS, Google Cloud, and Microsoft Azure ARM64 release images built with their real embedded dashboards and served `/ui/` plus validated operator identity/logout coordinates from running containers.
- The complete shared UI typecheck and test suites passed, including signed-in identity, accessible user details, local logout, and visible identity failures.
- The corrected fuzz harness passed every simulator, nested shared-module, core, Docker backend, and agent target in the same module layout used by GitHub Actions; the formerly flaky router target also passed ten consecutive one-second runs with bounded workers.
- The Bleeplab `runner-sockerless` GitHub Actions job passed against real Sockerless simulator and backend binaries.
- Bleephub's complete server, browser, GitHub Command Line Interface, and web application jobs passed. Its runner consumer job exercised the same real Sockerless build context on a Linux runner.
- Sockerless PR #800 completed the full required continuous-integration matrix successfully before the final orphan-test and documentation cleanup.
