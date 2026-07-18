# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `main` |
| Branch purpose | Sockerless remains the standalone Docker-compatible cloud backend and simulator project after the Bleephub and Bleeplab extractions. |
| Product ownership | The standalone repositories own product source, web user interfaces, Terraform, official-client tests, and runner consumer harnesses. Sockerless remains a standalone Docker-compatible cloud backend and simulator project. |
| Integration contract | Both product harnesses build and exercise real Sockerless simulator and backend binaries through a named build context; they contain no Sockerless source dependency. |
| Operator identity | `sockerless-admin` supports optional Shauth OpenID Connect browser sign-in for its operator UI and administration API while simulator cloud APIs remain protocol-faithful. |
| API-only contract | Every cloud simulator reports its configured runtime and workload-execution capability at `/health`. `SIM_RUNTIME=process` remains a cloud-independent API-only mode: durable control-plane and data-plane APIs remain available, while workload execution is rejected instead of being reported as running. |
| Release images | The operator console and the Amazon Web Services, Google Cloud, and Microsoft Azure simulators publish native ARM64 and AMD64 images plus an immutable short-SHA multi-architecture manifest. Every image contains its production web interface; simulator APIs and UI assets are served by the same real binary. |
| Latest merged pull request | [#800 Extract Bleephub and Bleeplab](https://github.com/e6qu/sockerless/pull/800) removed all tracked product code and stale local product paths from Sockerless. |
| Infrastructure | [Infra PR #4](https://github.com/e6qu/infra/pull/4) pinned its Terragrunt Bleephub module source to the standalone repository root commit. |
| Open bugs | See [BUGS.md](BUGS.md). The Amazon Elastic Container Service Terraform simulator lifecycle and Azure Container Registry Tasks registry trust gaps remained open. |

## What's Next

- Deploy the published operator-console and simulator images in the shared development environment, keeping the console behind Shauth, reading the generic simulator capability contract, and preserving the simulators' native cloud protocol contracts.
- Continue fidelity work on the tracked Amazon Elastic Container Service Terraform simulator lifecycle and Azure Container Registry Tasks registry-trust issues.
- Keep cross-repository runner validation real: Bleephub and Bleeplab consume built Sockerless simulators/backends rather than using local stand-ins.

## Invariants

- Never auto-merge pull requests; the user handles merges.
- At most one pull request is open in Sockerless. Put all work in that pull request.
- Rebase pull-request branches on `origin/main` before pushing; then sync local `main`.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Do not bypass, remove, ignore, stash around, or unstage around pre-commit/pre-push hooks.
- Simulators remain real cloud application programming interface slices, with official software development kit, command-line interface, and Terraform coverage where those surfaces exist.
