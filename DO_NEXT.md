# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Completed Baseline

`fix/shauth-final-pin-ci-timeouts` completed the exact Shauth relying-party contract for Sockerless Admin and all three simulator dashboards. The branch also enforced UI-first release builds, bounded nightly fuzz concurrency, and a 15-minute maximum for every ordinary GitHub Actions job without dropping any of the 630 AWS CLI tests.

The real pinned Shauth, PostgreSQL, Ory Hydra, freshly compiled relying parties, and Chromium matrix passed direct and catalog entry, relying-party and provider logout, application-local signed-out return, reload, reauthentication, global revocation, release identity, anonymous fail-closed behavior, active event-stream readiness, and credential isolation.

## Remaining Work

1. The merged revision needed immutable Admin and simulator images published and deployed in the shared development environment, followed by the same exact eight-flow browser matrix against the live origins.
2. The standalone Bleeplab GitLab Runner consumer needed another real run against the merged Sockerless revision so its slower two-cycle source-fetch path exercised the corrected wait-channel and CloudWatch stream generation.
3. BUG-2569 still required deterministic termination of the local Amazon Elastic Container Service Terraform simulator apply/destroy harness without changing the real provider path.
4. BUG-2625 still required provider-faithful simulator credential issuance and verification for AWS, Google Cloud, and Microsoft Azure, with exact errors and official SDK, command-line interface, and Terraform coverage.
5. The remaining live-cloud cells in BUG-1075 still required authenticated validation before being marked green.

## Durable Validation Contract

- Production builds created every frontend before any UI-bearing Go binary.
- Workflow changes kept every ordinary job at or below 15 minutes and preserved exact AWS CLI shard coverage.
- Fuzz changes exercised every discovered target and treated a missing module, build failure, target failure, or crasher as a real failure.
- Shauth changes ran the real PostgreSQL, Ory Hydra, relying-party, and Chromium matrix from the exact pinned provider revision.
