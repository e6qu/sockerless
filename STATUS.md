# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/simulator-console-ui` |
| Branch purpose | The shared operator and simulator interface was polished across responsive light/dark layouts, Admin became a standards-compliant Shauth logout participant, all three simulator UIs gained shared first-party OpenID Connect sessions, and every smoke image carried the complete shared-module build context. |
| Product ownership | The standalone repositories own product source, web user interfaces, Terraform, official-client tests, and runner consumer harnesses. Sockerless remains a standalone Docker-compatible cloud backend and simulator project. |
| Integration contract | Both product harnesses build and exercise real Sockerless simulator and backend binaries through a named build context; they contain no Sockerless source dependency. |
| Operator identity | `sockerless-admin` supports optional Shauth OpenID Connect browser sign-in, server-tracked relying-party sessions, signed back-channel logout with replay rejection, and RP-initiated global logout while simulator cloud APIs remain protocol-faithful. Its unauthenticated `GET /healthz` liveness route supports infrastructure probes without exposing protected console state. |
| Operator interface | Admin and the AWS, Google Cloud, and Microsoft Azure dashboards share a saturated, responsive, accessible light/dark shell with explicit service naming, keyboard focus, self-contained browser assets, and real browser coverage against each compiled server. |
| Simulator identity | The three dashboards share first-party OpenID Connect authorization-code + PKCE, signed server-tracked sessions, identity, RP-Initiated Logout, and signed back-channel revocation while keeping every cloud API slice unchanged. |
| API-only contract | Every cloud simulator reports its configured runtime and workload-execution capability at `/health`. `SIM_RUNTIME=process` remains a cloud-independent API-only mode: durable control-plane and data-plane APIs remain available, while workload execution is rejected instead of being reported as running. |
| Release images | The operator console and the Amazon Web Services, Google Cloud, and Microsoft Azure simulators publish native ARM64 and AMD64 images plus an immutable short-SHA multi-architecture manifest only after a push to `main`. Every image contains its production web interface; simulator APIs and UI assets are served by the same real binary. |
| Smoke images | Amazon Elastic Container Service, Google Cloud Run, Azure Container Apps, and GitLab smoke images include the shared real-execution, OpenID Connect, and agent modules their standalone Go graphs require. The Amazon Elastic Container Service image passed the complete 15-assertion real simulator/backend lifecycle smoke suite. |
| Fuzz CI | The nightly harness selected headless simulator builds, respected nested Go modules, bounded worker concurrency, distinguished target failures from crashers, and collected only newly minimized inputs. |
| Amazon ECS service discovery | The AWS simulator validated service-registry ports against AWS Cloud Map DNS record types, rejecting port coordinates for A-record registrations while preserving SRV registrations. |
| Latest merged pull request | [#804 Expose a real Sockerless Admin liveness endpoint](https://github.com/e6qu/sockerless/pull/804) added the unauthenticated operator-console liveness coordinate used by infrastructure and Shauth monitoring. |
| Infrastructure | The private `e6qu/infra` Terragrunt repository owns the shared development environment and pins immutable standalone application and Sockerless image revisions. |
| Open bugs | See [BUGS.md](BUGS.md). The Amazon Elastic Container Service Terraform simulator lifecycle and Azure Container Registry Tasks registry trust gaps remained open. |

## What's Next

- Publish and deploy the updated operator-console and simulator images in the shared development environment. Register each Admin and simulator callback, post-logout, and back-channel logout coordinate in Shauth, and preserve the simulators' native cloud protocol contracts.
- Continue fidelity work on the tracked Amazon Elastic Container Service Terraform simulator lifecycle and Azure Container Registry Tasks registry-trust issues.
- Configure the simulator first-party OpenID Connect environment/secrets, then run the complete direct-entry, portal-entry, identity, SSO-reentry, and global-logout browser matrix without an authentication proxy.
- Keep cross-repository runner validation real: Bleephub and Bleeplab consume built Sockerless simulators/backends rather than using local stand-ins.

## Invariants

- Never auto-merge pull requests; the user handles merges.
- At most one pull request is open in Sockerless. Put all work in that pull request.
- Rebase pull-request branches on `origin/main` before pushing; then sync local `main`.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Do not bypass, remove, ignore, stash around, or unstage around pre-commit/pre-push hooks.
- Simulators remain real cloud application programming interface slices, with official software development kit, command-line interface, and Terraform coverage where those surfaces exist.
