# Known Bugs

**985 filed · 985 fixed · 0 open · 1 false positive.**

Standing rule: every CI / live-cloud failure lands here with a one-liner before any fix attempt. Workarounds, fakes, placeholders, silent fallbacks, skips, and incomplete implementations are all bugs and get the same treatment. Per-bug fix detail beyond the one-liner: `git log <commit>` or the linked PR.

Live status (cells, branch, milestone) lives in [STATUS.md](STATUS.md).

## Open

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|

## False positives

| Area | Finding | Why it's not a bug |
|------|---------|--------------------|
| `backends/aca/azure.go::fakeCredential` | Returns literal `"fake-token"` against simulator endpoints. | Sims don't verify bearer tokens — would require real Azure AD endpoint not emulated. Credential wired only via `newAzureClientsWithEndpoint` (sim path); production uses `azidentity.NewDefaultAzureCredential`. |

## Class-of-bug rules (carried forward)

- **Backend ↔ host primitive must match (P0).** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF. Cross-pollution is a critical architectural error.
- **No fakes / no fallbacks / no skips.** Synthetic exit codes, silent shims, fake-data fallbacks, conditional `t.Skip` for missing config — all file as bugs and get real fixes. Tests run or fail loud; never skip silently.
- **Cross-cloud sweep on every find.** When a pattern is found in one backend, the same code paths in the other 5 backends / 3 sims get checked in the same commit.
- **Pattern B for cloud-specific drivers.** Within a cloud, drivers consolidate into `*-common`; cross-cloud duplication is fine and expected.

## Resolved history

985 bugs filed and fixed across phases 86–135 + Phase 84 (PRs #112–134, plus the Phase 84 PR). PRs #135–#141 (phases 121b finish, 78, 79, 80, 81, 82, 83) added no new bugs; Phase 84 surfaced 1. Per-bug detail in `git log` / linked PR. Recent ranges:

- **985** (Phase 84) — sim shared `NewServer` silently fell back to in-memory storage when `SIM_PERSIST=true` but `OpenDB` failed. Operator-requested persistence must fail loud; in-memory fallback masks misconfiguration (bad path, perms, full disk) and produces silent data loss across restarts. Fix: `NewServer` returns `(*Server, error)`, callers `log.Fatalf` on persistence open failure.
- **975–984** (PR #129) — Phase 135 sim host model + native arm64 CI runners.
- **973–974** (PR #128) — Sim test stability (`Eventually` polling).
- **949 + 972** (PR #123) — GCF `os/exec` workload → `sim.StartContainerSync`; cloudrun/gcf AR-proxy gate on `endpointURL`.
- **877–971** (PR #123) — FaaS pod overlays; 4 GCP runner cells GREEN; cloud-faithful GCP sim; storage-backing driver pilot.
- **845–876** (PR #122) — Phase 110: 4 AWS runner cells GREEN.
- **820–844 + 802** (PR #120) — Driver framework migration + cloud-native typed drivers.
- **786–819** (PR #118) — Round-8/9 AWS sweep; stateless invariant; real layer mirror.
- **770–785** (PR #117) — Round-7 AWS sweep.
- **661–769** (PRs #112–115) — Sim parity; stateless backends; FaaS invocation tracking; reverse-agent exec/cp/diff; Docker pod synthesis.
