# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/shauth-sso-rp-matrix` |
| Branch purpose | Native release jobs published direct architecture manifests, the operator applications enforced one real Shauth single-sign-on and global-logout contract across Admin plus all three simulator dashboards, Admin kept its browser-session signing key independent from its OpenID Connect client credential, and reused Amazon ECS attached containers retained correct task-generation lifecycle and log streams. |
| Product ownership | The standalone repositories own product source, web user interfaces, Terraform, official-client tests, and runner consumer harnesses. Sockerless remains a standalone Docker-compatible cloud backend and simulator project. |
| Integration contract | Both product harnesses build and exercise real Sockerless simulator and backend binaries through a named build context; they contain no Sockerless source dependency. |
| Operator identity | `sockerless-admin` supports optional Shauth OpenID Connect browser sign-in, `client_secret_post`, exact issuer validation, server-tracked relying-party sessions signed by an independent 32-byte-or-longer secret, front- and back-channel logout, and RP-initiated global logout to its public no-cache signed-out page. Explicit HTTP development coordinates are limited to loopback hosts. Its unauthenticated `GET /healthz` liveness route supports infrastructure probes without exposing protected console state. |
| Operator interface | Admin and the AWS, Google Cloud, and Microsoft Azure dashboards share a saturated, responsive, accessible light/dark shell with explicit service naming, keyboard focus, self-contained browser assets, and real browser coverage against cleanly built production bundles embedded in each compiled server. |
| Backend interface | Every backend web interface is self-contained and its Playwright harness compiles the current backend, starts the matching real simulator where required, provisions prerequisite cloud resources through public cloud API operations, and verifies status, navigation, resources, metrics, and browser assets. Continuous integration runs the complete Admin, simulator, and backend browser matrix. |
| Simulator identity | The three dashboards share first-party OpenID Connect authorization-code + PKCE, exact issuer validation, signed server-tracked sessions, identity, RP-Initiated Logout, strict same-origin controls, front-channel revocation, and form-only signed back-channel revocation with atomic replay rejection. Their HTML and dashboard-data routes share the same authorization boundary while health checks and native cloud API slices remain unchanged. |
| Logout-token contract | Admin and all three simulator relying parties require issued-at and future-expiry claims, reject replay atomically, and passed a real matrix that proved signed back-channel delivery at every application endpoint. |
| Amazon ECS attached restarts | Every reused attached-container start owns a fresh wait channel and task/log coordinate. Delayed old task pollers cannot close a newer cycle, and a real two-cycle simulator/backend test returned each task's distinct script output without stale CloudWatch events. |
| API-only contract | Every cloud simulator reports its configured runtime and workload-execution capability at `/health`. `SIM_RUNTIME=process` remains a cloud-independent API-only mode: durable control-plane and data-plane APIs remain available, while workload execution is rejected instead of being reported as running. |
| Release images | The operator console and the Amazon Web Services, Google Cloud, and Microsoft Azure simulators publish direct native ARM64 and AMD64 OCI image manifests plus an immutable short-SHA multi-architecture OCI index only after a push to `main`. The publication job rejects architecture tags that resolve to indexes or generic indexes containing any platform other than Linux ARM64 and AMD64. |
| Smoke images | Amazon Elastic Container Service, Google Cloud Run, Azure Container Apps, and GitLab smoke images include the shared real-execution, OpenID Connect, and agent modules their standalone Go graphs require. The Amazon Elastic Container Service image passed the complete 15-assertion real simulator/backend lifecycle smoke suite. |
| Fuzz CI | The nightly harness selected headless simulator builds, respected nested Go modules, bounded worker concurrency, distinguished target failures from crashers, and collected only newly minimized inputs. |
| Amazon ECS service discovery | The AWS simulator validated service-registry ports against AWS Cloud Map DNS record types, rejecting port coordinates for A-record registrations while preserving SRV registrations. |
| Latest merged pull request | [#807 Protect simulator dashboard data with OIDC](https://github.com/e6qu/sockerless/pull/807) applied the first-party simulator identity boundary to both rendered operator interfaces and their dashboard-data handlers. |
| Infrastructure | The private `e6qu/infra` Terragrunt repository owns the shared development environment and pins immutable standalone application and Sockerless image revisions. |
| Open bugs | See [BUGS.md](BUGS.md). The Amazon Elastic Container Service Terraform simulator lifecycle and Azure Container Registry Tasks registry trust gaps remained open. |

## What's Next

- Merge this branch, publish its corrected short-SHA images, and deploy the updated Admin and simulator images in the shared development environment. Register each callback, post-logout, front-channel logout, and back-channel logout coordinate in Shauth while preserving the simulators' native cloud protocol contracts.
- Re-run the standalone Bleeplab GitLab Runner consumer against the merged Sockerless revision so its slower two-cycle source-fetch path exercises the corrected wait-channel and CloudWatch stream generation.
- Continue fidelity work on the tracked Amazon Elastic Container Service Terraform simulator lifecycle and Azure Container Registry Tasks registry-trust issues.
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
