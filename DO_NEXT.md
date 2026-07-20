# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current Branch

`fix/shauth-sso-rp-matrix` completed the first-party Shauth contract for Sockerless Admin plus all three simulator dashboards. A real Shauth, Ory Hydra, PostgreSQL, and Chromium matrix proved shared sign-on and global logout from every relying party. The direct architecture image publication fix had already merged through pull request 808.

## Continue Here

1. Merge this branch through its single pull request, verify the published direct architecture manifests and generic index, and deploy them as real Amazon Elastic Container Service services. Register Admin's callback `/auth/shauth/callback`, post-logout `/auth/signed-out`, front-channel `/auth/shauth/frontchannel-logout`, and back-channel `/auth/shauth/backchannel-logout` coordinates. For each `aws`, `gcp`, and `azure` origin, register callback `/auth/oidc/callback`, post-logout `/auth/signed-out`, front-channel `/auth/oidc/frontchannel-logout`, and back-channel `/auth/oidc/backchannel-logout`; supply its `SIM_UI_OIDC_*`, `SIM_UI_PUBLIC_URL`, and `SIM_UI_SESSION_SECRET` values; then run the proven direct/catalog/global-logout matrix against the live origins.
2. Resolve BUG-2569: make the local Amazon Elastic Container Service Terraform simulator apply/destroy harness terminate deterministically without weakening the real provider path.
3. Resolve BUG-2589: configure the local Azure Container Registry Tasks SDK harness with a Docker-trusted simulator registry transport, matching the working CI coordinate.
4. Continue complete simulator and backend fidelity work, including the open live-cloud cells documented in BUGS.md.

## Recent Validation

- The compiled Admin and all three compiled simulator dashboards passed a clean real PostgreSQL, Ory Hydra, Shauth, and Chromium matrix after the harness built every production frontend instead of inheriting local `dist` directories. It covered direct entry, catalog entry, shared sign-on, identity, logout initiated by every relying party, global cross-application revocation, exact app-local signed-out destinations, signed-out reload, and fail-closed re-entry against the exact CI-green Shauth verified-email revision.
- Admin and simulator authentication accepted explicit HTTP development coordinates only on loopback hosts, required `client_secret_post`, exposed front-channel logout outside the local-session boundary, revoked locally before provider failure, and required the OIDC Back-Channel Logout event claim to be exactly the empty JSON object.
- The merged `9fe520a3ef14` release demonstrated that provenance-enabled native tags were OCI indexes with attestation children; the corrected workflow disabled provenance and added registry-shape assertions that reject that contract violation before a publication job succeeds.
- The complete AWS, Google Cloud, and Microsoft Azure simulator suites and vet passed after the shared operator authorization boundary covered each `/sim/v1/*` dashboard handler; direct tests proved UI and dashboard data redirected unauthenticated browsers while health remained public.
- The exact Amazon Elastic Container Service continuous-integration smoke image built and passed all 15 real simulator/backend Docker lifecycle assertions. The Google Cloud Run and Azure Container Apps smoke images built successfully, and all four GitLab smoke images resolved their complete local module graphs and compiled successfully.
- Sockerless Admin's complete Go suite and vet passed with real server-tracked sessions, signed OIDC Back-Channel Logout validation, `sid`/`sub` revocation, `jti` replay rejection, and RP-Initiated Logout.
- Admin and the simulator relying parties preserved the configured issuer exactly, accepted only same-origin browser logout requests, constrained the discovered logout endpoint to the issuer origin, required form-body back-channel tokens, validated the required logout event as a JSON object, and combined replay consumption with session revocation atomically.
- The shared simulator UI-auth module passed signed-session, direct-entry redirect, identity, cross-origin logout, signed back-channel revocation, and replay-rejection tests; every provider shared-server suite proved that only UI routes were protected while native cloud routes remained unaffected.
- The compiled Admin, AWS, Google Cloud, and Microsoft Azure servers passed 42 Playwright scenarios across responsive shell behavior, both themes, self-contained browser assets, every navigation surface, and real management/simulator HTTP data.
- The seven backend interfaces passed 77 Playwright scenarios against freshly compiled binaries and matching real simulators; their harnesses provisioned prerequisite resources through public cloud API operations, rejected stale/dead coordinates, and served no remote browser dependency. Continuous integration ran every Admin, simulator, and backend browser suite.
- The shared UI unit suite passed 50 tests; every affected UI package passed TypeScript checking and its production build.
- Amazon ECS service-discovery SDK coverage proved that A-record registries rejected explicit ports and accepted task-ENI-only registration, matching the real Amazon ECS control plane.
- The AWS, Google Cloud, and Microsoft Azure ARM64 release images built with their real embedded dashboards and served `/ui/` plus validated operator identity/logout coordinates from running containers.
- The complete shared UI typecheck and test suites passed, including signed-in identity, accessible user details, local logout, and visible identity failures.
- The corrected fuzz harness passed every simulator, nested shared-module, core, Docker backend, and agent target in the same module layout used by GitHub Actions; the formerly flaky router target also passed ten consecutive one-second runs with bounded workers.
- The Bleeplab `runner-sockerless` GitHub Actions job passed against real Sockerless simulator and backend binaries.
- Bleephub's complete server, browser, GitHub Command Line Interface, and web application jobs passed. Its runner consumer job exercised the same real Sockerless build context on a Linux runner.
- Sockerless PR #800 completed the full required continuous-integration matrix successfully before the final orphan-test and documentation cleanup.
