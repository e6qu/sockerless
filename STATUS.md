# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/aws-console-real-federation-visual` |
| Branch purpose | The Amazon Web Services console was completed on both fidelity axes in one branch. Data: the console read the real AWS APIs — Amazon ECS, AWS Lambda, Amazon ECR, Amazon S3, and Amazon CloudWatch Logs — over the console's own server-side Shauth federation. The operator's Shauth id_token was exchanged through `AssumeRoleWithWebIdentity` against a registered IAM OpenID Connect provider (the simulator now verifies the web identity token against that provider's issuer, audience, and signature and reports the real token subject), and every read was signed in the browser with Signature Version 4 over the resulting session credentials — the same code and identifiers a real client uses, differing only in the endpoint and federation coordinates. The invented `/sim/v1/*` dashboard endpoint was deleted. Visual: the console was rebuilt to the Cloudscape Design System — Open Sans (Cloudscape's own console font, since Amazon Ember is proprietary) vendored as a woff2 subset, Cloudscape's action blue and rounded containers, a dark navigation header carrying a global search field and a tool cluster (notifications, settings, support, theme), and a Cloudscape-style inline-SVG icon set on the status pills and the table's search-prefixed filter and refresh control. The browser suite pins the font, action blue, container radius, and header/table controls structurally so the AWS look cannot regress unseen. |
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
