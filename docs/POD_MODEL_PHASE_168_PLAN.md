# Phase 168 — FaaS exec model unification + tmpfs default + no-fallback hardening

**Status:** DRAFT plan. Awaiting user review before any code changes.

This plan implements the user direction at the end of Phase 167:
- **Model A** for FaaS exec (single long-lived invocation per pod; per-step `docker exec` over reverse-agent WebSocket only).
- **In-memory tmpfs** as the default storage backing for FaaS backends.
- **Hard no-fallback / no-silent-failure rule**, verified against the audit in `docs/POD_MODEL_ANALYSIS.md`.
- **Pluggability preserved** — every existing driver stays registered; only the *default* changes. Operator can opt back into any old behaviour via env var.

Provisional defaults below are my recommendations from the chat exchange. User may override any of them.

## Audit summary (already in POD_MODEL_ANALYSIS.md)

| Severity | Location | Issue |
|---|---|---|
| P0 | `backends/lambda/backend_delegates.go:210-213` | Path B (`execStartViaInvoke`) silent fallback when reverse-agent absent. Smoking gun for "12-step = 12-min". |
| P1 | `backends/cloudrun-functions/backend_delegates.go:214-226` | Path B is the *default* for non-interactive exec. Loud-by-design but produces the same wall-clock symptom; needs inversion for FaaS-family consistency. |
| P2 | `backends/{lambda,cloudrun-functions,cloudrun}/backend_impl.go` (network Disconnect + DeleteFunction cleanup paths) | Cleanup errors swallowed via `_ =` and `_, _ =`. Best-effort cleanup matches Docker semantics; acceptable but should be observable. |
| clean | `NoOpCloudExecDriver` returns `ErrCloudExecNotSupported`; gcs-fuse explicitly NOT registered; persistence write fail-loud; bootstrap dial failures fail hard. | — |

## Provisional defaults (each is one of the 7 questions; user confirms or overrides)

| # | Decision | Provisional default | User override? |
|---|---|---|---|
| 1 | tmpfs default size | 2 GiB; env var `SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB` | TBD |
| 2 | tmpfs exhaustion behaviour | fail loud + operator guidance message | TBD |
| 3 | reverse-agent registration timeout in `ContainerStart` | 90 sec; env var `SOCKERLESS_<BACKEND>_BOOTSTRAP_TIMEOUT_SEC` | TBD |
| 4 | `execStartViaInvoke` (Path B) disposition | keep as opt-in driver (`SOCKERLESS_<BACKEND>_EXEC_DRIVER=invoke`), not default | TBD |
| 5 | `pd-ephemeral` registration | stays registered as opt-in (no namespace move) | TBD |
| 6 | cleanup-path silent errors | keep best-effort; add `s.Logger.Warn().Err(err)` everywhere for observability | TBD |
| 7 | FaaS pod lifetime > platform max | **CONFIRMED by user 2026-05-17**: accepted limitation. Fail loud at next exec ("container exceeded FaaS lifetime; use ECS / ACA / Cloud Run Services for longer pods"). No transparent re-invoke. | ✅ user-confirmed |

## BUGs to file up front

Per the standing rule: file before fixing. Filing 8 BUGs (1046–1053) at P168.0:

| ID | Sev | Area | One-liner |
|---|---|---|---|
| 1046 | P0 | `backends/lambda/backend_delegates.go::ExecStart` | Silent Path A → Path B fallback on missing reverse-agent. Every step cold-starts a fresh `lambda.Invoke` (~30-90s × 12 steps = 12-min CI symptom). Per "no fallbacks" rule. Fix: drop Path B from default flow; fail-loud if reverse-agent not registered. |
| 1047 | P1 | `backends/cloudrun-functions/backend_delegates.go::ExecStart` | Path B (`execStartViaInvoke`) is the default for non-interactive exec; should invert to "Path A preferred, Path B fallback" for FaaS-family consistency with Lambda + AZF. Fix: same shape as Lambda after BUG-1046. |
| 1048 | P1 | `backends/{lambda,cloudrun-functions,azure-functions}/backend_impl.go::ContainerStart` | `ContainerStart` returns as soon as the cloud function is Active, but doesn't wait for the in-container reverse-agent bootstrap to dial back. First `docker exec` races the dial-back and either falls to Path B (Lambda) or fails NotImplemented (AZF). Fix: block until `reverseAgents.Resolve(id)` succeeds OR timeout (default 90s). |
| 1049 | P2 | `backends/core/storage_*.go` + per-FaaS `server.go` | `memory` driver (tmpfs) currently registered with 64 MiB default — too small for CI workspace. Should be 2 GiB default (configurable via `SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB`) AND should become the default storage backing for Lambda + GCF + AZF (currently efs-ephemeral / gcs-sync / azure-files-ephemeral respectively). Persistent backings stay registered as opt-in. |
| 1050 | P2 | `backends/{lambda,cloudrun-functions}` | `execStartViaInvoke` should be exposed as an opt-in `ExecDriver` (`SOCKERLESS_<BACKEND>_EXEC_DRIVER=invoke`) for operators who want stateless-per-step semantics. Currently it's a hardcoded code path with no opt-in mechanism. Once exposed as a driver, the default `ExecDriver` slot stays "reverse-agent only." |
| 1051 | P2 | tmpfs exhaustion path | When tmpfs fills (CI workspace exceeds `SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB`), the write fails with `ENOSPC`. Currently the bootstrap surfaces this as a generic "exec failed exit 1" without operator guidance. Fix: bootstrap detects ENOSPC, returns an exec envelope with explicit message "tmpfs exhausted at NN MiB; raise SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB or switch SOCKERLESS_<BACKEND>_STORAGE_BACKING to a persistent driver." |
| 1052 | P2 | Cleanup-path observability | `_ = s.Drivers.Network.Disconnect(...)` and `_, _ = s.aws.Lambda.DeleteFunction(...)` in `backends/lambda/backend_impl.go:912,929`, `backends/cloudrun-functions/backend_impl.go:681,850`, `backends/cloudrun/backend_impl.go:798`. Best-effort cleanup is fine (matches Docker semantics) but the errors should be logged at WARN level so operators can see resource leaks. Currently invisible. |
| 1053 | P2 | FaaS pod lifetime > platform max | When a pod exceeds the platform's max invocation duration (Lambda 15min, Gen2/AZF 60min), the bootstrap's WS closes and the next `docker exec` currently returns a generic 500. Fix: bootstrap signals "lifetime exceeded" cleanly; sockerless returns `&api.ServerError{Message: "container N exceeded FaaS pod lifetime (N min); use ECS / ACA / Cloud Run Services for longer pods"}`. No transparent re-invoke. |

## Sub-task layout (P168.0..P168.10)

Severity-ordered (1046 P0 first; the rest cluster into logical fix groups):

| Sub | Status | BUG(s) | What |
|---|---|---|---|
| **P168.0** | ◻ | — | Branch from `origin/main`; survey + 8 BUGs (1046–1053) filed in `BUGS.md`; continuity-doc opening. |
| **P168.1** | ◻ | 1046 | Lambda: drop `execStartViaInvoke` from the default `ExecStart` flow. New behaviour: `if !hasAgent { return ErrReverseAgentMissing }`. Keep `execStartViaInvoke` function in place (used by P168.4's opt-in driver). Add a unit test that hits ExecStart with no agent and asserts the new error. |
| **P168.2** | ◻ | 1047 | GCF: invert ExecStart to Path-A-preferred, Path-B-fallback → same shape as Lambda after P168.1 (drop Path B fallback entirely). For *interactive* (TTY+stdin) the behaviour is unchanged; for non-interactive the path is now WS-only. |
| **P168.3** | ◻ | 1048 | Lambda + GCF + AZF: `ContainerStart` blocks until `reverseAgents.Resolve(id)` succeeds OR `bootstrap_timeout_sec` elapses (default 90s, env-overridable). On timeout: `return &api.ServerError{Message: "reverse-agent bootstrap did not dial back within %s for container %s; check SOCKERLESS_CALLBACK_URL reachability from inside the function"}`. Removes the race that motivates P168.1's "did the agent show up yet" check at every exec. |
| **P168.4** | ◻ | 1050 | Promote `execStartViaInvoke` to an opt-in `ExecDriver`. Adds `core.ExecDriverInvoke` type that wraps the existing function. Wires `SOCKERLESS_<BACKEND>_EXEC_DRIVER=invoke` in lambda + gcf `server.go`. Default stays `reverseagent`. Docs note the trade-off (stateless per step; no process continuity across steps). |
| **P168.5** | ◻ | 1049 | Bump `MemoryDriver` default size to 2 GiB. Add `SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB` env var. Switch *default* `storage_backing` from `efs-ephemeral` / `gcs-sync` / `azure-files-ephemeral` to `memory` for the 3 FaaS backends. Persistent drivers stay registered as opt-in (`SOCKERLESS_<BACKEND>_STORAGE_BACKING=efs-ephemeral`, etc.). Update each backend's README. |
| **P168.6** | ◻ | 1051 | Bootstrap (sockerless-{lambda,gcf,azf}-bootstrap): wrap `os.WriteFile` / `exec.Command` paths to detect `ENOSPC`. On detection, return exec envelope with `{exit_code: 28, stderr: "tmpfs exhausted at <size> MiB; raise SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB or set SOCKERLESS_<BACKEND>_STORAGE_BACKING=<persistent driver>"}`. Exit 28 = ENOSPC's POSIX errno, conventional. |
| **P168.7** | ◻ | 1052 | Wrap each `_ = s.Drivers.Network.Disconnect(...)` and `_, _ = s.aws.Lambda.DeleteFunction(...)` etc. with `if err := ...; err != nil { s.Logger.Warn().Err(err).Str(...).Msg("cleanup failed (best-effort)") }`. Sweep across lambda + cloudrun + cloudrun-functions backends. Behaviour unchanged (still best-effort), just observable. |
| **P168.8** | ◻ | 1053 | Bootstrap monitors invocation deadline (Lambda: `context.RemainingTime`; GCF/AZF: equivalent timer). At T-30s, sends `{type: "lifetime_warning", remaining_sec: 30}` over the reverse-agent WS. At T-5s, sends `{type: "lifetime_expired"}` and closes the WS. Sockerless's reverse-agent handler maps `lifetime_expired` → mark container as Stopped with reason `FaaSPodLifetimeExceeded`; next ExecStart returns `&api.ServerError{...}`. |
| **P168.9** | ◻ | — | E2E test: run a 12-step job against the lambda backend with reverse-agent reachable; assert total time ≪ 60s. Same against gcf + azf. Run a no-reverse-agent scenario; assert ContainerStart fails loud within timeout. |
| **P168.10** | ◻ | — | State save + push + open PR + codex review + watch CI + ping user for merge. |

## Files touched (estimate)

```
backends/lambda/backend_delegates.go        (P168.1 — drop Path B fallback)
backends/lambda/backend_impl.go             (P168.3 — bootstrap wait; P168.7 — log cleanup)
backends/lambda/exec_invoke.go              (P168.4 — wrap as opt-in driver)
backends/lambda/server.go                   (P168.4 — wire env; P168.5 — default tmpfs)
backends/lambda/README.md                   (P168.5 — doc default change)
backends/cloudrun-functions/backend_delegates.go  (P168.2 — drop Path B default)
backends/cloudrun-functions/backend_impl.go (P168.3 + P168.7)
backends/cloudrun-functions/exec_invoke.go  (P168.4)
backends/cloudrun-functions/server.go       (P168.4 + P168.5)
backends/cloudrun-functions/README.md       (P168.5)
backends/azure-functions/backend_delegates.go    (P168.3 — already Path A only; bootstrap wait)
backends/azure-functions/backend_impl.go    (P168.3)
backends/azure-functions/server.go          (P168.5)
backends/azure-functions/README.md          (P168.5)
backends/core/storage_driver.go             (P168.5 — MemoryDriver default size)
agent/cmd/sockerless-lambda-bootstrap/main.go    (P168.6 + P168.8)
agent/cmd/sockerless-gcf-bootstrap/main.go       (P168.6 + P168.8)
agent/cmd/sockerless-azf-bootstrap/main.go       (P168.6 + P168.8)
backends/core/handle_*.go                   (P168.8 — handle lifetime_expired signal from WS)
backends/{lambda,cloudrun-functions,cloudrun}/backend_impl.go  (P168.7 — sweep)
docs/POD_MATERIALIZATION.md                 (P168.5 — update Backing default columns)
specs/CLOUD_RESOURCE_MAPPING.md             (P168.5 + P168.8 — update Lambda/GCF/AZF rows)
tests/runners/                              (P168.9 — new e2e tests)
```

## Acceptance bar

- All 8 BUGs (1046–1053) closed in this PR.
- `go test ./...` green in every touched Go module.
- E2E: 12-step CI job on Lambda / GCF / AZF backend completes in <60s wall-clock (down from ~12 min — that's the headline result that makes this phase worth doing).
- No silent fallbacks: code search for `if hasAgent` / `if rwc, err := execStartViaInvoke` returns 0 hits at top-level dispatch paths.
- Operator-visible env vars documented in each per-backend README:
  - `SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB` (default 2048)
  - `SOCKERLESS_<BACKEND>_BOOTSTRAP_TIMEOUT_SEC` (default 90)
  - `SOCKERLESS_<BACKEND>_STORAGE_BACKING` (default `memory`, opt-in `efs-ephemeral` / `gcs-sync` / `azure-files-ephemeral` / `pd-ephemeral`)
  - `SOCKERLESS_<BACKEND>_EXEC_DRIVER` (default `reverseagent`, opt-in `invoke`)
- 11 standard CI checks green per push.
- User merges PR.

## Out of scope

- Long-lived backends (docker, ecs, cloudrun-Services, aca) unchanged. They already use one container/task/revision per job.
- Storage *implementations* unchanged (existing efs-ephemeral / gcs-sync / azure-files-ephemeral / pd-ephemeral drivers all stay). Only the default changes.
- AZF supervisor-in-overlay pattern unchanged (not relevant to exec dispatch).
- Pod materialization (deferred-network-pod) unchanged.
- Anything that requires a separate VM / instance per the user's standing rule.

## Risks

1. **2 GiB tmpfs may not fit on smaller Lambda memory configs.** Lambda function memory is configurable 128 MB – 10 GB; tmpfs draws from the same memory pool. If operator's Lambda is configured at 1 GB and tmpfs default is 2 GB, allocation fails. **Mitigation:** at bootstrap startup, clamp tmpfs size to `min(SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB, function_memory_mib - 256)`. Surface the actual size in `docker inspect` so operator sees what they got.

2. **90s bootstrap timeout may not cover image-based Lambda cold-start with EFS + VPC** in worst case. Real-world reports of 2-3 min cold starts exist for >1 GB images + EFS attach. **Mitigation:** make the timeout configurable; document the real cold-start expectations per backend.

3. **Operator running an existing CI job suddenly sees tmpfs instead of EFS.** Backwards-incompatible default change. **Mitigation:** call out loudly in PR description + CHANGELOG. Operators who explicitly set the env var to a persistent backing are unaffected; only operators relying on the implicit default are affected. (For a project that explicitly states "no legacy support during active development," this is acceptable.)

4. **The reverse-agent WS connection drops mid-job** for reasons unrelated to FaaS lifetime (e.g. transient network blip). Currently the next `docker exec` would silently fall to Path B (Lambda). After P168.1, it would fail loud. **Mitigation:** the bootstrap implements WS reconnect with exponential backoff (likely already exists; verify). Sockerless's reverse-agent registry is updated only when the WS is healthy.

## Open questions still pending user confirmation

The 7 from the prior chat exchange. Tabulated above. None of them require code archaeology; they're product decisions.

After user confirms (or overrides) the 7, this plan locks and P168.0 starts.
