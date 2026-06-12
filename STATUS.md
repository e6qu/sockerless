# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/bleephub-actions-complete` |
| In-flight | **Complete GitHub Actions support in bleephub** (one fat PR): server-side workflow engine (on: filters/activity types incl. PR synchronize, schedule crons, reusable workflows with gate/collector expansion, real expression engine with full contexts), secrets+variables at repo/org/environment with the real sealed-box contract, checks integration (jobs→check runs/suites, workflow_run/workflow_job/check_run/check_suite events, mergeable_state + required-status-check merge gating), real per-step timeline records + log storage (runner wrapper-decode root cause), runs-on label routing, org runners + honest busy, run attempts (same run id, archived attempts, rerun-failed carry-over), workflow enable/disable, full GitHub-style Actions UI (runs/steps/live logs/dispatch/approvals/secrets-vars management/PR checks), gh harness 115/0; official-runner harness de-bitrotted + ALL 9 e2e tests green in CI (`sim (bleephub actions/runner)`) — surfaced+fixed BUG-1740 (broker assigned busy runners) and BUG-1741 (step templates sent as literal tokens); azure-sim Entra OIDC fixes for issues #548/#547 (client_secret_basic, login_hint-bound auth codes — BUG-1742/1743). BUG-1724..1743 filed+fixed. |
| Last merged | #546 Amplify full support + bleephub apps/orgs hardening (BUG-1706..1723). #545 launch-hygiene batch (spec-validate armed on tf jobs, XML validation). |
| Open GitHub issues | #394 remained upstream-blocked (BUG-1345). Re-check GitHub before doing any non-conformance issue work. |
| Bugs | 1743 filed - 1701 fixed - 2 open - 7 false positives (see [BUGS.md](BUGS.md)). Open: BUG-1075 (live-cloud cells), BUG-1345 (azuread upstream). |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1345 azuread Terraform upstream. |
| Live infra | None up. |

## Simulator conformance + hardening

The active arc is deep behavioural conformance of the AWS/GCP/Azure simulators
against the real official clients (SDK/CLI/Terraform) for the implemented
slices, plus Go type and simulator-UI hardening. Methodology and per-stage
status live in [PLAN.md](PLAN.md) § Current Work; the narrative is in
[WHAT_WE_DID.md](WHAT_WE_DID.md); per-bug detail is in [BUGS.md](BUGS.md).

- Stages 1-6 are complete. Stage 1 (AWS) + Stage 2 batches G1-G3 (GCP) merged in
  #537/#538; Stage 2 G4 (GCP missing ops), Stage 3 (Azure), Stage 4 (Go type
  hardening), Stage 5 (simulator UI hardening), and Stage 6 (CI sim-module unit
  tests) are on the current branch (PR #539), with BUG-1646 (bleephub gh-CLI
  sub-issue GraphQL drift) and an azure tf-test timeout flake fix.
- Documented deferrals (not faked, tracked in BUGS.md / PLAN.md): GCP
  cloudbuild/dataflow server-assigned-id name-collision 409; the GCP synthetic
  compute operation store (cannot 404 a bogus op name without fabricating one);
  Azure long-tail list `nextLink` for small fixed collections
  (EventHub/EventGrid/LogicApps/storage-ARM/RG); surface-table regeneration
  (the seed script over-generates — left as-is, gate green).

## Bleephub state

Bleephub parity, durable storage (SQLite/PostgreSQL persistence, filesystem and
S3/MinIO git content storage, git HTTP auth/permissions), the GitHub-style UI
restyle, and the GitHub-API fidelity sweep are merged (#534-#536). Remaining
open bleephub fidelity gaps are tracked in [BUGS.md](BUGS.md): BUG-1590
(run/environment pending approvals returns an empty success rather than modeling
deployment protection) and BUG-1618 (the top-level `organization` block is not
emitted on issue/PR/push webhook payloads for org-owned repos). External
identity must stay GitHub/GHES-shaped: public API paths, request/response
fields, runner and `GITHUB_*` variables, and client-facing UI text use the
GitHub names real clients expect; bleephub-specific names are acceptable only
for internal code or operator-only management surfaces.

## Invariants

- Never auto-merge PRs; the user handles merges.
- Rebase PR branches on `origin/main` before pushing.
- File concrete BUG entries before fixing discovered defects.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Simulators implement real cloud API slices in one binary per cloud.
- Every simulator public endpoint ships with official SDK, vendor CLI, and Terraform coverage where those surfaces exist.
- Coverage authorities are [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md) and [specs/SIM_SURFACE_TABLES](specs/SIM_SURFACE_TABLES).

## Environment Notes

- AzureRM custom metadata discovery still needs HTTPS through the local Caddy gateway: `make stack-https-{up,status,ca,down}`.
- AWS and GCP Terraform providers accept localhost custom endpoints directly.
- Simulator ports: AWS 4566, GCP 4567, Azure 4568.
- Linux network-fabric tests require `CAP_NET_ADMIN`, `iproute2`, and nftables; off-Linux tests skip through the realexec capability gate.
