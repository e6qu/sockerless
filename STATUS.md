# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/signed-out-shauth-controls` |
| Branch purpose | Sockerless Admin and all three simulator signed-out pages exposed explicit, accessible `Sign in with Shauth` controls at their exact first-party login coordinates, and the real PostgreSQL/Ory Hydra/Shauth/Chromium matrix proved each control after global logout and reload persistence. |
| Product ownership | The standalone repositories own product source, web user interfaces, Terraform, official-client tests, and runner consumer harnesses. Sockerless remains a standalone Docker-compatible cloud backend and simulator project. |
| Integration contract | Both product harnesses build and exercise real Sockerless simulator and backend binaries through a named build context; they contain no Sockerless source dependency. |
| Operator identity | `sockerless-admin` supports optional Shauth OpenID Connect browser sign-in, `client_secret_post`, exact issuer validation, server-tracked relying-party sessions signed by an independent 32-byte-or-longer secret, front- and back-channel logout, and RP-initiated global logout to its public no-cache signed-out page. Only a session carrying the Shauth `admin` role enters the operator UI or APIs; authenticated developers receive an explicit no-cache `403` and can sign out. Explicit HTTP development coordinates are limited to loopback hosts. Its unauthenticated `GET /healthz` liveness route supports infrastructure probes without exposing protected console state. |
| Operator interface | Admin and the AWS, Google Cloud, and Microsoft Azure dashboards share a saturated, responsive, accessible light/dark shell with explicit service naming, keyboard focus, self-contained browser assets, and real browser coverage against cleanly built production bundles embedded in each compiled server. Their terminal signed-out pages identify Shauth explicitly and expose the exact first-party login coordinate. |
| Backend interface | Every backend web interface is self-contained and its Playwright harness compiles the current backend, starts the matching real simulator where required, provisions prerequisite cloud resources through public cloud API operations, and verifies status, navigation, resources, metrics, and browser assets. Continuous integration runs the complete Admin, simulator, and backend browser matrix. |
| Simulator identity | The three dashboards share first-party OpenID Connect authorization-code + PKCE, exact issuer validation, signed server-tracked sessions, identity, RP-Initiated Logout, strict same-origin controls, front-channel revocation, and form-only signed back-channel revocation with atomic replay rejection. Their HTML and dashboard-data routes share the same authorization boundary while health checks and native cloud API slices remain unchanged. |
| Logout-token contract | Admin and all three simulator relying parties require issued-at and future-expiry claims, reject replay atomically, and passed a real matrix that proved signed back-channel delivery at every application endpoint. |
| Amazon ECS attached restarts | Every reused attached-container start owns a fresh wait channel and task/log coordinate. Delayed old task pollers cannot close a newer cycle, and a real two-cycle simulator/backend test returned each task's distinct script output without stale CloudWatch events. |
| API-only contract | Every cloud simulator reports its configured runtime and workload-execution capability at `/health`. `SIM_RUNTIME=process` remains a cloud-independent API-only mode: durable control-plane and data-plane APIs remain available, while workload execution is rejected instead of being reported as running. |
| Release images | The operator console and the Amazon Web Services, Google Cloud, and Microsoft Azure simulators publish direct native ARM64 and AMD64 OCI image manifests plus an immutable short-SHA multi-architecture OCI index only after a push to `main`. The publication job rejects architecture tags that resolve to indexes or generic indexes containing any platform other than Linux ARM64 and AMD64, removes unrecognized package versions, and retains the newest 20 complete immutable releases of every image. |
| Dependency freshness | The required freshness gate inspects only Git-tracked Go modules, Terraform providers, and GitHub Actions; the current Google Cloud Secret Manager and `actions/checkout` releases were pinned across every consumer. |
| Simulator registry transport | The Google Cloud Build and Azure Container Registry Tasks official SDK harnesses used the same real HTTP loopback registry coordinate and ordinary Docker-compatible push on Docker Engine and Podman. A shared test utility applied an exact, scoped Podman registry policy and reloaded the engine around each test without weakening global registry trust. |
| Smoke images | Amazon Elastic Container Service, Google Cloud Run, Azure Container Apps, and GitLab smoke images include the shared real-execution, OpenID Connect, and agent modules their standalone Go graphs require. The Amazon Elastic Container Service image passed the complete 15-assertion real simulator/backend lifecycle smoke suite. |
| Fuzz CI | The nightly harness selected headless simulator builds, respected nested Go modules, bounded worker concurrency, distinguished target failures from crashers, and collected only newly minimized inputs. |
| Amazon ECS service discovery | The AWS simulator validated service-registry ports against AWS Cloud Map DNS record types, rejecting port coordinates for A-record registrations while preserving SRV registrations. |
| Latest merged pull request | [#810 Require administrator role for Sockerless Admin](https://github.com/e6qu/sockerless/pull/810) restricted the operator UI and APIs to Shauth administrators while retaining an explicit denial and logout path for developers. |
| Infrastructure | The private `e6qu/infra` Terragrunt repository owns the shared development environment and pins immutable standalone application and Sockerless image revisions. |
| Open bugs | See [BUGS.md](BUGS.md). The Amazon Elastic Container Service Terraform simulator lifecycle gap remained open. |

## What's Next

- Merge this branch, publish its Admin and simulator images, deploy them in the shared development environment, and run the same signed-out control matrix against the live origins.
- Re-run the standalone Bleeplab GitLab Runner consumer against the merged Sockerless revision so its slower two-cycle source-fetch path exercises the corrected wait-channel and CloudWatch stream generation.
- Continue fidelity work on the tracked Amazon Elastic Container Service Terraform simulator lifecycle issue.
- Run the same direct-entry, catalog-entry, identity, shared-sign-on, app-local landing, and global-logout matrix against the deployed development origins without an authentication proxy.
- Provision `SOCKERLESS_ADMIN_SESSION_SECRET` independently from the Shauth confidential-client credential in every deployment; rotating either secret no longer rotates the other security boundary.
- Keep cross-repository runner validation real: Bleephub and Bleeplab consume built Sockerless simulators/backends rather than using local stand-ins.

## Invariants

- Never auto-merge pull requests; the user handles merges.
- At most one pull request is open in Sockerless. Put all work in that pull request.
- Rebase pull-request branches on `origin/main` before pushing; then sync local `main`.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Do not bypass, remove, ignore, stash around, or unstage around pre-commit/pre-push hooks.
- Simulators remain real cloud application programming interface slices, with official software development kit, command-line interface, and Terraform coverage where those surfaces exist.
