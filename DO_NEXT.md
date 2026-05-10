# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`main` (clean). PR #139 merged 2026-05-10 (Phase 80 admin UI Topology page + state save post-#138). Start a new branch for Phase 81.

## Resume here

**Phase 81 — per-instance logs + live troubleshooting console.**

Three deliverables on top of the Phase 79/80 surface that already exists:

1. **SSE log tail.** New endpoint `GET /api/v1/topology/projects/{p}/instances/{i}/logs?follow=1` reads `.stack-pids/<name>.log` and streams new lines via Server-Sent Events. Without `follow=1`, returns the last N lines (default 200) as a JSON array. Honour `?lines=<N>` for both modes.
2. **Combined-timeline view.** Multi-instance log view in admin UI: pick any subset of instances within a project; merged stream sorts by timestamp prefix when present, otherwise interleaves by arrival order. Each line tagged with the instance name. Pause / resume / clear controls.
3. **API console panel.** Free-form HTTP request against any running instance: pick instance → endpoint base resolved from `Instance.Port` → method + path + headers + body inputs → fire → render response (status + headers + body, with JSON pretty-print + copy buttons).

Wiring location:
- New Go file `cmd/sockerless-admin/api_topology_logs.go` for the SSE handler.
- New UI route `/ui/topology/:project/:instance/logs` for single-instance tail; `/ui/topology/:project/console` for the multi-instance combined view + API console panel.
- ProjectLogsPage (`/ui/projects/:name/logs`) currently uses the legacy `/api/v1/projects/{p}/logs` endpoint — leave for now; Phase 83 (sim UI parity) sweep will retire it.

Component-decoupled invariant still applies: SSE reads admin's own `.stack-pids/<name>.log` (admin's bookkeeping), not anything component-side. The API console talks to the instance's existing public surface; instances don't grow new endpoints to support admin.

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap on this branch

Phases 79.2 → 80 → 81 → 82 → 83 → 84 → 85 → 86 → 87. See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once the natural seam appears (e.g. after Phase 86 ships, Phase 87 — observability — is independent and can land as its own PR).

## After this branch

- Phases 91–94 — real per-cloud volume provisioning (lifts `emptyDir` → real `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`).
- Live-cloud validation track — Lambda live, Cloud Run Services + ACA Apps live, AZF cloud-dns + Lambda service-mesh + ACA/AZF Azure AD live.
