# Do Next

**Resume pointer for the next session.** Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-121b-azure-sim-hardening` — off `origin/main` at 3e39e3a (PR #134 merged 2026-05-09). Single work-branch rule: everything stacks here, no side branches.

## Ordered roadmap (do them in this order)

1. **Phase 78 — UI polish** (dark mode, design tokens, error UX, container detail modal, accessibility).

## Active — Phase 121b (Azure sim hardening + Azure backend test-harness restructure, ready for PR + CI)

Mirror of Phase 121 GCP sim hardening, plus a no-fallback / no-skip / explicit-config restructure of the Azure backend integration test harnesses. All sub-tasks shipped:

- **121b-A — Azure Files data plane on disk.** `simulators/azure/files.go` previously returned mock XML for every file/directory operation. New `handleAzureFilesPath` services the real Azure Files REST verbs (PUT directory, PUT file, PUT range, GET, HEAD, DELETE) and persists everything under `FileShareHostDir(account, share)`. End-to-end consistency between data-plane writers and ACA / AZF workload mounts.
- **121b-B — HS256-signed Azure AD JWT.** `simulators/azure/auth.go` previously emitted `alg:none` tokens. New `mintAzureSimJWT` produces a real-shape Azure AD access token (HS256 + `kid` header + full claim set). JWKS publishes the `kid`.
- **121b-C — All 6 backends' integration test harness restructured.** No `SOCKERLESS_INTEGRATION` gate, no skip, no fallback, no `//go:build integration` build tag. Every backend's `TestMain` (ACA, AZF, ECS, Lambda, Cloud Run, Cloud Run Functions) requires `SOCKERLESS_TEST_TARGET=sim|cloud` + `docker` + `go` on PATH; sim path builds + starts the per-cloud simulator on a free port and pre-creates fixed sim fixtures; cloud path requires explicit `SOCKERLESS_ENDPOINT_URL` + per-backend ARM/IAM env vars. Per-test `skipIfNoIntegration` helpers deleted across the board.
- **121b-D — Azure terraform-test darwin fail-loud.** No more macOS skip — `t.Fatal` with a clear explanation. Run via Linux container or in CI.
- **121b-E — Makefile + CI updates.** `make/go-app.mk` + `make/go-lib.mk` ship `test-integration` (`=sim`) + `test-integration-cloud` (`=cloud`). CI sets only `SOCKERLESS_TEST_TARGET=sim`; the legacy `SOCKERLESS_INTEGRATION` env is removed entirely from the codebase.

5 new in-binary unit tests (`files_test.go` × 3 + `auth_test.go` × 2); all azure sim + sdk + cli + terraform-tests green locally.

## Follow-up phases (queued)

1. **In-memory storage backing driver** (user request 2026-05-09) — add a `core.StorageBacking` that uses the execution environment's memory as the backing for Docker/Podman volume translation. Sibling to `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral` (Phase 127). Available across all 6 backends as the no-cost test path; persists nothing across container lifecycles.

## Standing rules

- **Never merge PRs** — user handles all merges. Push only.
- **Never push `main`.** Branch off `origin/main`, PR it.
- **Single work-branch rule** — everything stacks here; no side branches.
- **State save after every task** — STATUS / PLAN / WHAT_WE_DID / DO_NEXT / BUGS.
- **Bugs file before fix** — every CI / live failure lands in BUGS.md as a one-liner before any analysis or fix attempt. Header counts updated in the same edit.
- **No fakes / no fallbacks** — every gap is a real bug; cross-cloud sweep on every find.
- **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
- **Backend ↔ host primitive must match** — ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, etc.
- **Sim binary arch ≠ workload arch** — sim runs host-native; workloads carry arch config (default `linux/arm64`); never `os/exec` workloads from a sim handler. (`feedback_sim_workload_arch.md` + `feedback_sim_host_model.md`)
- **Driver phase entry** — start with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass before code.

## Open bugs

None. Detail in [BUGS.md](BUGS.md).
