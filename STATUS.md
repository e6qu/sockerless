# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/sim-credential-enforcement-and-harness` |
| Credential enforcement | All three simulators verified caller credentials on their data planes as the real clouds do (BUG-2625): AWS recomputed and verified the SigV4 signature at the awsjson/query control plane and S3 (persisting `ASIA` temporary-credential secrets, seeding a bootstrap account credential); Google Cloud and Azure verified the bearer's signature, issuer, audience, and expiry against the simulator-minted token (GCP consolidated to one RS256 key + published JWKS; Azure reused its RS256 verifier and added the audience check). Token minters, discovery/JWKS, metadata/IMDS, health, OCI registries, and the anonymous flows (`AssumeRoleWithWebIdentity`/SAML, presigned URLs, S3 public reads) stayed exempt. Every SDK/CLI/Terraform suite, the sockerless backends (their `WithoutAuthentication`/`fakeCredential` fakes replaced with a real GCP metadata token source and Azure `DefaultAzureCredential`), and the relying-party provisioning authenticated for real; the console browser e2e reached the enforcing simulator unauthenticated, so its data-render assertions moved to the authenticated relying-party path. The AWS ECS Terraform harness (BUG-2569) made services synchronous control-plane objects and reaped subprocesses deterministically, so apply/destroy terminated with no leaks. |
| Branch purpose | The Microsoft Azure portal was completed on both fidelity axes in one branch. Data: the portal read the real Azure Resource Manager APIs — Azure Container Apps jobs, Azure Functions sites, Azure Container Registry, Azure Storage accounts — and Azure Monitor's Log Analytics query API, enumerating subscriptions and listing each provider across them, over the portal's own server-side Shauth federation. The operator's Shauth assertion was exchanged through Microsoft Entra Workload Identity Federation (`client_credentials` with a JWT-bearer `client_assertion`) against a registered federated identity credential — the simulator now verifies the assertion against the identity's credential issuer, subject, audience, and RS256 signature and issues an Azure token, distinguishing the Azure Resource Manager and Log Analytics token audiences. The invented `/sim/v1/*` dashboard endpoint was deleted. Visual: the portal (already built to the Azure portal's layout — blue header, "Microsoft Azure" wordmark, command bar, Essentials, grouped service menu) got a Fluent-style inline-SVG icon set (Fluent UI System Icons, MIT) on the command bar, status pills, chevrons, header search, and theme control, replacing the placeholder glyphs. The browser suite pins the header blue and the command/status/search icons structurally so the Azure look cannot regress unseen. |
| Product ownership | Standalone repositories owned product source, web interfaces, Terraform, official-client tests, and runner consumer harnesses. Sockerless remained a standalone Docker-compatible cloud backend and simulator project. |
| Operator identity | Admin required a Shauth administrator role, exact issuer validation, server-tracked sessions, front- and back-channel logout, and a public no-cache signed-out page. Its logout completion bridge remained reachable after local revocation and redirected only to Shauth's correlated completion endpoint. |
| Simulator identity | All three dashboards used first-party OpenID Connect authorization code with PKCE, exact issuer validation, signed server-tracked sessions, identity, RP-Initiated Logout, signed back-channel revocation, atomic replay rejection, and application-local signed-out return. Native cloud API slices remained independent of browser authentication. |
| Product-interface contract | Admin and all three simulator dashboards marked the visible account control with the exact Shauth username and the real sign-out control with a stable qualification hook. The browser matrix asserted the rendered username against each application's identity endpoint and signed out by clicking the product control, so a broken product shell failed qualification even when its protocol endpoints answered. |
| Exact browser contract | Shauth `0fda680cba964e5768ed75a9c3e5b7230c418ca6`, PostgreSQL, Ory Hydra, four freshly compiled relying parties, and Chromium passed direct and catalog entry, relying-party and provider logout, exact completion bridging, local signed-out reload and reauthentication, global revocation, anonymous fail-closed behavior, release identity, active event-stream readiness, and validator-credential isolation in eight serialized flows. |
| Production builds | The root build created every frontend bundle before compiling all 11 UI-bearing Go binaries. A repository gate rejected build-order regressions that could silently produce `noui` release binaries. |
| Continuous integration budgets | Every ordinary workflow job declared a timeout of at most 15 minutes, enforced by a tested repository gate. The historically over-budget AWS edge and Amazon EC2 command-line interface groups were split into four non-overlapping shards while all 630 AWS CLI tests remained covered exactly once. |
| Fuzzing | The nightly harness ran targets in bounded parallel batches with one Go fuzz worker per target, preserved truthful logs and crasher handling, failed on missing modules, and passed a real one-second run across every target in all nightly modules. |
| Dependency freshness | Every tracked Google Cloud Storage consumer used v1.64.0, and the complete Google Cloud Run, Google Cloud Run Functions, shared backend, and simulator SDK suites passed after the refresh. |
| Release images | Admin and the AWS, Google Cloud, and Microsoft Azure simulators published direct native ARM64 and AMD64 OCI manifests plus an immutable short-SHA multi-architecture OCI index, retaining the newest 20 complete releases. |
| Open bugs | [BUGS.md](BUGS.md) retained six evidence-backed open defects, including cloud-simulator credential authentication, the Amazon ECS Terraform simulator lifecycle, and real-cloud validation gaps. |

## Verified Gates

- The complete repository test and lint fan-outs passed.
- A clean full production build passed and embedded every current web bundle.
- The full pre-commit suite passed before final branch publication.
- The complete nightly fuzz target matrix passed with a one-second budget per target.
- The exact real PostgreSQL, Ory Hydra, Shauth, compiled relying-party, and Chromium matrix passed all eight application-and-direction validation runs.

## Invariants

- The user merged pull requests; agents never merged them.
- Sockerless kept at most one open pull request and carried all related work in that branch.
- Pull-request branches were rebased on the freshest `origin/main` before publication.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, degraded modes, or skipped required tools were accepted.
- Every observed failure was fixed or recorded with evidence in [BUGS.md](BUGS.md).
