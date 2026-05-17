# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **GitHub API fidelity (bleephub)** — match GitHub's REST + GraphQL paths and shapes exactly, modulo base domain. Real `gh` CLI must work directly against bleephub.
3. **Real execution** — sims and backends actually run commands; no stubs, fakes, or mocks.
4. **External validation** — proven by unmodified external test suites (`gh` binary, `actions/runner`, real Docker SDKs, Terraform providers).
5. **Driver-first handlers** — handler code routes through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **State persistence** — every task ends with a state save (STATUS / DO_NEXT / WHAT_WE_DID / MEMORY / `_tasks/done/`).
8. **No fallbacks, no skips, no defers, no fakes** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces. We are not in legacy maintenance — no shims for old behaviour. If real GitHub does X, bleephub does X.
9. **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
10. **Single work-branch rule** — all in-flight work lands on one branch. User handles every merge.
11. **Cross-cloud is permanently off the table** — cloud-specific drivers extend the generic shape; cross-cloud duplication is fine, in-cloud duplication consolidates into `*-common`.
12. **Components stay decoupled from admin / UI.** Sims, backends, bleephub remain independently configurable, buildable, runnable. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars).
13. **Persistence is opt-in + fail-loud.** Operator-requested persistence (`BLEEPHUB_PERSIST=true`, `SIM_PERSIST=true`) that fails to open or write must surface the error, never silently degrade (BUG-985/986).
14. **No phase or bug IDs in code comments.** Keep that metadata in commits / PRs / BUGS.md only; code comments document the *why*, not the lineage.

## Closed phases (PR index)

Headline-only. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

| PR | Phases | Headline |
|---|---|---|
| #112–123 | 86–123 | Sim parity; stateless backends; FaaS pod overlays; storage-backing driver pilot; **8/8 runner cells GREEN.** |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #128–134 | 124–134 | Driver framework + makefile std + sim host model + arm64 CI runners + job timeout + network/dns/access/storage drivers. |
| #135–136 | 121b | Azure sim hardening, driver consolidation pattern B, network-discovery adapter consolidation, AZF/Lambda DNS, Azure AD access. |
| #137–142 | 78–84 | UI polish + admin orchestration (`sockerless.yaml` topology, `TopologyManager`, lifecycle endpoints, UI Topology page, per-instance logs + console, cloud-resources rollup, sim UI parity, per-instance state isolation + BUG-985/986). |
| #143–144 | 85–86 | Config edit + hot reload; health + supervision surface (exit-code capture, `/diagnostics`, `<UnhealthyDiagnosticPanel>`). |
| #145–146 | 87 + 87b | Observability stack (otel-collector + VictoriaLogs + Jaeger) + component-side OTel SDK wiring. |
| #147–149 | 91 + 91b + 91c | `BackingMemory` translator across 5 backends; Lambda volume_translator framework migration; cloudrun + gcf `BackingPDEphemeral` rejection. |
| #150 | 87c | zerolog → OTel logs bridge across all 12 components. |
| #151 | 87d + 92 | Trace propagation + MeterProvider + runtime metrics; `Backing: gcs-fuse` deregistered on cloudrun + gcf. |
| #152 | docs | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |
| #153 | 153 | bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compat. |
| #154 | 154 | Broad GitHub API sweep — reactions, releases, deployments + environments, PR review comments + threads, Checks, Actions OIDC + JWKS, Pages, branch protection. |
| #155–156 | 155–156 | bleephub-specific + project-wide docs refresh; GCP dep bump. |
| #157 | 157 | Component ⇄ reference-adaptor docs sweep started (`backends/docker` only). |
| #158 | 158 | BUG-991 + BUG-992 fixes; `docs/VIBE_CODING.md` 23-pattern catalogue; `docs/GOLANG_STRONG_TYPING.md`; 3 project-local Claude skills. |
| #159 | 159 | AWS sim — CloudFront + ACM + Route 53 + WAFv2 + Amplify + IAM SLR/OIDC (11 sub-tasks, `TestStackProductionShape` cross-resource invariants). Merged 2026-05-15 at `236a387f`. |
| #160 | 160 | Two new project-local skills (`sim-handler-checklist`, `cross-resource-stack-test`) + `adaptor-fidelity-check` refinement; component-README adaptor-led sweep completed across 6 backends + 2 simulators + bleephub + `cmd/sockerless` + new `cmd/sockerless-admin/README.md` + rewritten `simulators/README.md`. Phase 157 Track A closed. Merged 2026-05-16 at `aeb0ac6e`. |
| #161 | 161 | Comprehensive vibe-slop sweep — 18 BUGs closed + bleephub GraphQL completion. Merged 2026-05-16 at `841f2456`. |
| #162 | 162 | Vibe-coding catalogue refresh — 12 new patterns (24–35) + `avoid-vibe-slop` skill expanded 17 → 26 checklist items. Doc-only. Merged 2026-05-16 at `4f602988`. |
| #163 | 163 | Makefile legacy alias rip-out + docs sweep. Merged 2026-05-16 at `d5b9d22a`. |
| #164 | 164 | Second vibe-slop sweep + terraform-provider test expansion (19 BUGs: 1014–1032). GCP terraform-tests 4 → 11 resources; Azure terraform-tests 1 → 5. Merged 2026-05-17 at `616dcd98`. |
| #165 | 165 | Third vibe-slop sweep (4 BUGs: 1033–1036) + sim test-pyramid expansion (2 BUGs: 1038/1039 + a GCS object selfLink sub-defect) + codex CLI review (2 BUGs: 1043/1044) + continuity-doc compression (~1700 → ~870 lines). 3 Open BUGs staged forward to Phase 166. Merged 2026-05-17 at `288b76d3`. |
| #167 | 166 | Real fixes for the 3 Phase-165 follow-up Open BUGs (1040 Azure azurerm + 1041 GCP IAM SA via correct `iam_beta_custom_endpoint` setting + 1042 AWS 5 sim handler gaps closed real) + codex review finding state-persistence gaps (BUG-1045). 1044 fixed / 0 open at merge. Merged 2026-05-17 at `49050c2d`. |

## Active phase

### Phase 167 + 168 — Pod-model unification + no-fallback hardening (in flight on `phase-167-pod-model-analysis`)

User directive (2026-05-17): unify FaaS-style exec on Model A (mandatory reverse-agent WebSocket), no fallbacks anywhere, FaaS max duration is a hard limit, default in-memory tmpfs where the cloud supports it, preserve driver pluggability.

**Landed so far on this branch:**
- `5f745039` — P168.1 + .2 + .4: ripped Path B (`execStartViaInvoke`) from lambda + GCF + cloudrun; deleted `core.CloudExecDriver` parallel interface; ACA `cloudExecStart` management-API fallback also ripped (was tracked as BUG-1056 once spotted).
- *(this commit)* — P168.3 + BUG-1055 + 1056: `core.ReverseAgentRegistry.WaitForAgent(ctx, id)` + per-backend `BootstrapTimeoutFromEnv` knob; ContainerStart on all 5 FaaS-style backends now blocks until the in-container bootstrap dials back (90s default, `SOCKERLESS_<BACKEND>_BOOTSTRAP_TIMEOUT_SEC` overrides); `CallbackURL` required at NewServer for lambda/gcf/cloudrun/aca/azf (fail-loud); GCF was never injecting `SOCKERLESS_CALLBACK_URL` into the function env — fixed.

### Phase 168 — Remaining sub-tasks

Builds directly on Phase 167's analysis. User direction:
- **Model A for exec wherever the platform supports per-pod long-lived invocation** — lambda + cloudrun-functions + cloudrun + aca + azf converge on reverse-agent WebSocket as the sole exec dispatch. (ecs keeps SSM; docker keeps native exec — each is the right primitive for its platform.)
- **In-memory tmpfs default** for backends whose cloud platform exposes a real EmptyDir / memory primitive (cloudrun + cloudrun-functions + ACA). Lambda + AZF keep their existing defaults — their volume translators explicitly reject `BackingMemory` because the platforms lack the primitive (codex review confirmed).
- **Hard no-fallbacks rule** — no silent substitution anywhere. `execStartViaInvoke` (Path B) deleted entirely from lambda + cloudrun-functions + **cloudrun** (the last one was missed in initial Phase 167 analysis), not preserved as opt-in. The parallel `core.CloudExecDriver` interface (`backends/core/exec_driver.go`) ripped — it existed specifically to enable the "agent missing fallback" path which no longer exists. Cleanup failures propagate (docker rm fails if cloud cleanup fails). WS drop = container dead (no reconnect). Tmpfs size > function memory = startup error (no clamping). Lambda backend startup fails loud if `SOCKERLESS_CALLBACK_URL` is empty (currently silently registers Path B driver).
- **FaaS max invocation duration is a hard limit** — no transparent re-invoke / warm-pool / checkpoint-restart hack. When the bootstrap approaches the deadline, the next `docker exec` returns `&api.ServerError{Message: "container N exceeded FaaS pod lifetime (N min); use ECS / ACA / Cloud Run Services for longer pods"}`.
- **Driver pluggability preserved** — the typed `core.ExecDriver` abstraction (in `drivers_typed.go`) STAYS. Each backend registers ONE driver of the right kind for its platform (DockerExec / SSMExec / ReverseAgentExec). The handler call site stays generic. Future operators / maintainers can swap drivers at the registration site. The "no fallback" rule means: only one driver is registered per backend, never a primary-with-backup pair.

User-confirmed decisions (2026-05-17):
- Q4 — `execStartViaInvoke` ripped entirely (no opt-in driver). Includes `core.CloudExecDriver` parallel interface.
- Q6 — cleanup failures propagate (no best-effort logging fallback).
- Q7 — FaaS pod lifetime hard limit (no extension hacks; FaaS jobs run up to platform max only).

User-approved defaults (from 2026-05-17 "begin work" directive):
- Q1 tmpfs default size = 2 GiB
- Q2 tmpfs exhaustion = fail loud (no clamping)
- Q3 reverse-agent registration timeout = 90s, `SOCKERLESS_<BACKEND>_BOOTSTRAP_TIMEOUT_SEC` overrides (landed in P168.3)
- Q5 pd-ephemeral disposition = stays registered as opt-in
- Q8 tmpfs default scope = cloudrun + cloudrun-functions + ACA only
- Q9 tmpfs size validation = startup fail-loud

BUGs in scope: 1046–1056 (11 BUGs; 1055 + 1056 surfaced during P168.3 ContainerStart survey). Status — 7 closed (1046, 1047, 1048, 1050, 1054, 1055, 1056); 4 remaining (1049, 1051, 1052, 1053).

Headline result target: 12-step CI job <60s wall-clock on Lambda / GCF / AZF / cloudrun (down from ~12 min today).

Driver model (KEPT, expanded in DO_NEXT.md):
- `core.ExecDriver` typed interface — the load-bearing abstraction. Each backend registers ONE driver. Handler is generic.
- Per-backend exec driver post-Phase-168:
  - docker → DockerExec (native)
  - ecs → SSMExec (Session Manager)
  - lambda / cloudrun / cloudrun-functions / aca / azf → ReverseAgentExec (WS to bootstrap)
- The other 13 typed drivers (Stream, Attach, FSRead/Write/Diff/Export, Commit, Build, Stats, ProcList, Logs, Signal, Registry) all unchanged.

Files DELETED:
- `backends/lambda/exec_invoke.go` (Path B + `lambdaInvokeExecDriver`)
- `backends/cloudrun-functions/exec_invoke.go`
- `backends/cloudrun/exec_invoke.go` (missed in Phase 167 analysis — added)
- `backends/core/exec_driver.go` (`CloudExecDriver` parallel interface; no callers post-deletion)
- `backends/core/exec_driver_test.go`

Files MODIFIED (~28 files): lambda + cloudrun + cloudrun-functions + ACA + AZF backends; agent bootstraps (`agent/cmd/sockerless-{lambda,gcf,azf}-bootstrap/main.go`); `backends/core/storage_driver.go`; per-backend README files; `docs/POD_MATERIALIZATION.md`; `specs/CLOUD_RESOURCE_MAPPING.md`; new e2e tests under `tests/runners/`.

Acceptance bar:
- All 11 BUGs (1046–1056) closed in this PR.
- `go test ./...` green in every touched module.
- E2E: 12-step CI job <60s wall-clock on Lambda/GCF/AZF/cloudrun (headline result).
- Code search: no `execStartViaInvoke` / `lambdaInvokeExecDriver` / `CloudExecDriver` / `cloudExecStart` / `_ = .*Disconnect` / `_, _ = .*Delete` left in lambda + cloudrun-functions + cloudrun + aca backends. `exec_invoke.go` + `core/exec_driver.go` + `aca/exec_cloud.go` files removed.
- Operator env-vars documented in per-backend READMEs.
- 11 standard CI checks green per push.

Out of scope:
- Long-lived non-FaaS backends (docker, ecs) — pod model already correct.
- Storage *implementations* — all existing drivers (efs-ephemeral / gcs-sync / azure-files-ephemeral / pd-ephemeral) stay. Only defaults change.
- AZF supervisor-in-overlay pattern (unrelated to exec dispatch).
- Pod materialization (deferred-network-pod) unchanged.
- The 13 other typed-driver dimensions (FSRead/Write/Diff/etc.) — they're already on the ReverseAgent shape for FaaS and unchanged.

### Phase 166 — Real fixes for the 3 Phase-165 follow-up Open BUGs (merged at `49050c2d`)

5 commits closed 4 BUGs (1040 Azure azurerm wiring + 12 new tf resources, 1041 GCP `google_service_account` via `iam_beta_custom_endpoint` setting, 1042 AWS 5 sim handler gaps closed real per "no stubs" directive, 1045 codex-found state-persistence gaps in DDB PITR/TTL/tags + KMS custom key policy). Narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

### Phase 165 — Third vibe-slop sweep + sim test-pyramid expansion + continuity-doc compression + codex review (merged at `288b76d3`)

10 granular commits closed 9 BUGs across four tracks (vibe-slop / test-pyramid expansion / codex review findings / continuity-doc compression). 3 Open BUGs staged forward. Narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

### Phase 164 — Second vibe-slop sweep + terraform-provider test expansion (merged at `616dcd98`)

13 granular commits closed 19 BUGs across five layered passes. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Future phases

### Track A — Live-cloud validation (one branch per cell)

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. Teardown self-sufficient per `feedback_teardown_aggressive.md`.

### Track B — Skill maturation (post-Phase 158)

Candidate additional skills as new patterns surface: `state-save`, `spec-first-implementation`, `cross-cloud-sweep`.

### Track C — Phase 91d (bookmarked indefinitely)

Real `pd-ephemeral` on cloudrun + gcf. Cloud Run's `runpb.Volume` lacks a PD field. Don't reopen until cloud capability changes.

## Driver phase template

Storage backing (Phase 127) is the pilot. Each driver phase follows:

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
- Sockerless GCE-style backend (would unlock Phase 91d real `pd-ephemeral` for real workloads).
- Marketplace / billing on bleephub — out of scope until a real consumer asks.
