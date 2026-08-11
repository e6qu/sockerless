# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients (`docker`, Docker Compose, Testcontainers, CI runners), backed by real cloud infrastructure or high-fidelity local cloud simulators. Bleephub is an independent GitHub Enterprise Server-compatible service and consumes Sockerless through its published simulator/backend integration contract.

## Non-Negotiable Principles

1. Match public application programming interfaces exactly: Docker, GitHub, and public cloud APIs.
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, degraded modes, or skip-if-tool-absent tests.
3. Cloud backends stay stateless; the cloud is the source of truth.
4. Simulators are real local cloud slices, one binary per cloud, validated through official SDK, command-line interface, and Terraform clients where applicable.
5. Bleephub metadata/index state belongs in SQLite; git objects, artifacts, caches, release assets, packages, job logs, and similar durable bytes belong in object storage or an explicit durable local-development filesystem.
6. Public/user-facing Bleephub behavior must use GitHub-shaped public paths and contracts, not `/internal/*` operator shortcuts.
7. The user merges PRs. Agents create branches, commits, and PRs only.

## Active Focus

**Cloud backend fidelity, production operation, and live-cloud validation.** (Simulator-side fidelity work continues in the [sockerless-cloud](https://github.com/e6qu/sockerless-cloud) repository, which owns the simulators, their test suites, consoles, and specs.)

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator dashboards completed one first-party OpenID Connect boundary for their rendered interfaces and operator data while native cloud protocol routes remained independent. The exact PostgreSQL, Ory Hydra, Shauth, compiled relying-party, and Chromium matrix proved direct and catalog entry, shared sign-on, identity, relying-party and provider logout, correlated logout completion, application-local signed-out return, reload, reauthentication, global revocation, release identity, anonymous fail-closed behavior, active event-stream readiness, and validator-credential isolation.

Production operation had enforceable resource and artifact contracts. Every ordinary GitHub Actions job was bounded to 15 minutes, the historically over-budget AWS edge and Amazon EC2 command-line interface groups were split without losing any of the 630 tests, nightly fuzz targets ran in bounded parallel batches, and clean production builds created every frontend before compiling all 11 UI-bearing Go binaries. Native release tags remained direct architecture manifests while each short-SHA tag remained an OCI index containing exactly Linux ARM64 and AMD64.

The fidelity work stayed evidence-driven. AWS Lambda and AWS Step Functions covered every operation in their vendored Smithy service models with executable implementations. The follow-on sweeps closed Amazon SQS runtime semantics, Amazon EC2 subnet dependencies and sparse snapshots, Amazon ECS `StartTask` and launch-type sandboxing, real Amazon Amplify builds, the AWS Certificate Manager ACME service, AWS Private Certificate Authority, Amazon Data Firehose, SMTP-backed Amazon SNS email delivery, Firehose-backed Amazon SNS and Amazon CloudWatch delivery, and repeated AWS/Microsoft Entra OpenID discovery. Google Cloud closed the described-but-unserved cryptographic, rotation, Autokey, Memorystore, and Cloud Run projection gaps; Azure Files gained Share ACL. Official SDK, vendor CLI, Terraform, RFC 8555, SMTP, Git, container, authenticated browser, and external reverse-proxy clients proved the public contracts externally.

## Active Branch Priorities

1. Extracted the three cloud simulators, their SDK/CLI/Terraform suites, console SPAs, vendored cloud API specifications, sim gate scripts/hooks, and the Firecracker/realexec harness into the standalone [sockerless-cloud](https://github.com/e6qu/sockerless-cloud) repository.
2. Consumed the simulators back as pinned Go modules: `tests/go.mod` pins `github.com/e6qu/sockerless-cloud/simulator-<cloud>` via `tool` directives; harnesses, backend integration tests, the ECS Terraform module test, and the stack targets build from that pin, and every harness Docker image installs the same modules at `ARG SOCKERLESS_CLOUD_VERSION`.
3. Kept the cross-relying-party Shauth browser matrix in this repository, building full-console simulator binaries from the pinned modules (the console `dist/` ships inside the modules).
4. Moved the simulator-side open bugs (2909, 2932, 2887, 2646, 2712, 2764, 2928, 2924, 1345) to sockerless-cloud's BUGS.md with their IDs intact; split BUG-2922 into per-repository copies.
5. Retained the `eval-arithmetic` and `container-command` workload fixtures under `tests/testdata/` for the backends' integration tests and runner harnesses.
