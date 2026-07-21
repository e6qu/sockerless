# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Completed Baseline

`fix/shauth-product-ui-markers` qualified Sockerless through its real product interface. Admin and all three simulator dashboards exposed the authenticated operator and the real sign-out control to the browser matrix, which asserted the rendered username and clicked the product control instead of relying on a protocol-only validation page.

`fix/shauth-final-pin-ci-timeouts` had completed the exact Shauth relying-party contract for the same four applications, enforced UI-first release builds and bounded nightly fuzz concurrency, and held every ordinary GitHub Actions job to 15 minutes without dropping any of the 630 AWS CLI tests.

The real pinned Shauth, PostgreSQL, Ory Hydra, freshly compiled relying parties, and Chromium matrix passed direct and catalog entry, relying-party and provider logout, application-local signed-out return, reload, reauthentication, global revocation, release identity, anonymous fail-closed behavior, active event-stream readiness, and credential isolation.

## Remaining Work

1. The remaining Shauth catalog applications still needed the same product-interface contract before Shauth's launch-interface assertion could be enabled: SameOldChat, Intraktible, Bleephub, Bleeplab, Sharecrop, ECS Dev Desktop, and the simulator console. E6IRC already carried it.
2. Shauth still needed its strengthened validator merged last, so that qualification exercised each application's real launch interface and its registration-contract revalidation rather than the technical validation page alone.
3. The merged revision needed immutable Admin and simulator images published and deployed in the shared development environment, followed by the same exact browser matrix against the live origins.
4. The standalone Bleeplab GitLab Runner consumer needed another real run against the merged Sockerless revision so its slower two-cycle source-fetch path exercised the corrected wait-channel and CloudWatch stream generation.
5. BUG-2569 still required deterministic termination of the local Amazon Elastic Container Service Terraform simulator apply/destroy harness without changing the real provider path.
6. BUG-2625 still required provider-faithful simulator credential issuance and verification for AWS, Google Cloud, and Microsoft Azure, with exact errors and official SDK, command-line interface, and Terraform coverage.
7. The remaining live-cloud cells in BUG-1075 still required authenticated validation before being marked green.

## Durable Validation Contract

- Production builds created every frontend before any UI-bearing Go binary.
- Workflow changes kept every ordinary job at or below 15 minutes and preserved exact AWS CLI shard coverage.
- Fuzz changes exercised every discovered target and treated a missing module, build failure, target failure, or crasher as a real failure.
- Shauth changes ran the real PostgreSQL, Ory Hydra, relying-party, and Chromium matrix from the exact pinned provider revision.
