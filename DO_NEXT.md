# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current Branch

`main` contains the completed Bleephub and Bleeplab extraction. Their source, user interfaces, Terraform modules, official-client tests, and runner consumer harnesses now live in the standalone `e6qu/bleephub` and `e6qu/bleeplab` repositories. Sockerless remains the real simulator/backend dependency exercised by both consumer harnesses.

## Continue Here

1. Deploy the published Sockerless operator console and Amazon Web Services, Google Cloud, and Microsoft Azure simulator images as real Amazon Elastic Container Service services. Keep the operator's unauthenticated `/healthz` liveness coordinate separate from the Shauth-protected browser/operator boundary, consume each simulator's generic `/health` runtime-capability contract, and preserve the simulators' native cloud protocol endpoints for official software development kit, command-line interface, and Terraform clients.
2. Resolve BUG-2569: make the local Amazon Elastic Container Service Terraform simulator apply/destroy harness terminate deterministically without weakening the real provider path.
3. Resolve BUG-2589: configure the local Azure Container Registry Tasks SDK harness with a Docker-trusted simulator registry transport, matching the working CI coordinate.
4. Continue complete simulator and backend fidelity work, including the open live-cloud cells documented in BUGS.md.

## Recent Validation

- The Bleeplab `runner-sockerless` GitHub Actions job passed against real Sockerless simulator and backend binaries.
- Bleephub's complete server, browser, GitHub Command Line Interface, and web application jobs passed. Its runner consumer job exercised the same real Sockerless build context on a Linux runner.
- Sockerless PR #800 completed the full required continuous-integration matrix successfully before the final orphan-test and documentation cleanup.
