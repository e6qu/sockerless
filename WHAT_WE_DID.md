# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs/CLIs/Terraform.

85 phases, 756 tasks, 651 bugs tracked. See [STATUS.md](STATUS.md), [BUGS.md](BUGS.md), [specs/](specs/).

## Phase 86 — Complete Runner Support (no-AWS track)

Closing the gap to running unmodified `actions/runner` / `gitlab-runner` against real github.com / gitlab.com with ECS and Lambda backends on real AWS. The no-AWS-credentials track landed in one branch (`phase86-complete-runner-support`):

- **P86-001/002** — Removed the silent `-wasm` / `-faas` variant remapping from the E2E runner harnesses. `container-action-faas.yml` renamed to `container-action.yml`; stale `services` / `custom-image` entries pruned from the pipeline lists. Added `docs/runner-capability-matrix.md` as the going-forward record.
- **P86-003** — Per-hostname Cloud Map services + `DnsSearchDomains` on ECS task defs. Services on live Fargate now get per-name DNS, not a shared `containers` record. Design in `docs/ECS_SERVICES_DESIGN.md`.
- **P86-004a** — Lambda `ContainerStop` / `ContainerKill` clamp future invocations via `UpdateFunctionConfiguration(Timeout=1)` and call a `disconnectReverseAgent` stub that P86-005 will fill in.
- **P86-004b** — Lambda `docker logs -f` lazy-resolves the CloudWatch log stream so follow-mode works when opened before the first invocation.
- **P86-005** — Lambda live-exec skeleton: design doc `docs/LAMBDA_EXEC_DESIGN.md`, `sockerless-lambda-bootstrap` binary, `backends/lambda/image_inject.go` with 3 unit tests, `LambdaConfig.CallbackURL` wiring. Runtime-API loop stays TODO until AWS access.
- **P86-007** — `docs/ECS_LIVE_SETUP.md`, `docs/GITHUB_RUNNER_SAAS.md`, `docs/GITLAB_RUNNER_SAAS.md`.

Needs-AWS track (P86-008…013) is blocked on credentials + github.com / gitlab.com test tokens.
