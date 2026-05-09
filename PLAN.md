# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **Real execution** — sims and backends actually run commands; no stubs, fakes, or mocks.
3. **External validation** — proven by unmodified external test suites.
4. **No new frontend abstractions** — Docker REST API is the only interface.
5. **Driver-first handlers** — handler code routes through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **State persistence** — every task ends with a state save (PLAN / STATUS / WHAT_WE_DID / DO_NEXT / BUGS / memory).
8. **No fallbacks, no skips, no defers, no fakes** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find.
9. **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
10. **Single work-branch rule** — all in-flight work lands on one branch. User handles every merge.
11. **Cross-cloud is permanently off the table** — cloud-specific drivers extend the generic shape; cross-cloud duplication is fine, in-cloud duplication consolidates into `*-common`.

## Closed phases (PR index)

Headline-only. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

| PR | Phases | Headline |
|---|---|---|
| #112–123 | 86–123 | Sim parity; stateless backends; FaaS pod overlays; storage-backing driver pilot; **8/8 runner cells GREEN.** |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #127 | 129#4 + 130–132 | Orphan pod-Service GC; sim parity prep (`generateIdToken` + Compute Disks); bleephub workflows + oauth REST + UI. |
| #128 | 134 | Makefile standardization + per-app leaf Makefiles + stack orchestration. |
| #129 | 135 | Sim host model + 3-tier coverage + native arm64 CI runners. |
| #130 | 128 | Runner job timeout (bootstrap timer + cloud-native cap; SIGTERM → 30s → SIGKILL → exit 124). |
| #131 | 124 | Network discovery driver (host-aliases / cloud-dns / service-mesh / nat-gateway-only). |
| #132 | 125 | DNS driver (cloud-map / cloud-dns-zone / private-dns-zone / service-discovery / none). |
| #133 | 126 | Access driver (iam-role / id-token / mTLS / none-internal). |
| #134 | 127 | Storage driver expansion (pd-ephemeral / efs-ephemeral / azure-files-ephemeral). |

## Roadmap (ordered)

### 1. Phase 121b — Azure sim hardening + driver consolidation (in flight, PR #135)

See [DO_NEXT.md](DO_NEXT.md) for the full sub-task list. Headline: Azure sim cloud-faithful (Files data plane + AAD JWT), all-6-backends test harness restructured to `SOCKERLESS_TEST_TARGET=sim|cloud` (no skips, no fallbacks, no build tags), in-memory storage backing driver, driver consolidation into `*-common` (pattern B), `host-aliases` registered everywhere, AZF + Lambda DNS / network-discovery / access driver gaps closed.

### 2. Phase 78 — UI polish

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

## Driver phase template

Storage backing (Phase 123) is the pilot. Each driver phase follows:

1. `api/<dim>_driver.go` — enum + struct fields on the relevant config.
2. `backends/core/<dim>_driver.go` — driver interface + registry + no-op default.
3. `backends/<cloud>-common/<dim>_<impl>.go` — per-cloud impl (pattern B: shared by both backends in that cloud).
4. `backends/<cloud-product>/server.go` — wires the per-cloud driver into the backend's registry at startup.
5. Operator config: env var selects the driver per backend.
6. **No-fallbacks at resolve** — unset / unknown driver name returns an error.
7. Migration of existing inline calls to the registry.

Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Sockerless GCE-style backend (would unlock Phase 127 GCP `pd-ephemeral` for real workloads).
