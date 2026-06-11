# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/bleephub-sweep` |
| In-flight | **bleephub deep sweep** (one fat PR): OpenAPI response-shape validator + ratchet inside `go test ./bleephub` (723 emitter violations -> 0); deployment approvals modeled (BUG-1590); webhook org block (BUG-1618); persistence overhaul — 14 reload gaps incl. a P1 token-resurrection security fix + fail-loud git-storage mismatch + delete cascade (BUG-1692..1695); GraphQL/gh-CLI drift — pr/release/clone command set unbroken, /api/v3/meta, push-run workflow_id, NOT_FOUND fidelity (BUG-1696..1699); UI fixes + 49 vitest cases (BUG-1690/1691). gh docker harness 92/0. |
| Last merged | #540 docs: per-component READMEs + docs/specs indexes; broken smoke-test targets + stale UI dev proxies fixed (BUG-1647/1648). #539 simulators: conformance hardening Stages 2 G4-6. |
| Open GitHub issues | #394 remained upstream-blocked (BUG-1345). Re-check GitHub before doing any non-conformance issue work. |
| Bugs | 1699 filed - 1627 fixed - 33 open - 6 false positives (see [BUGS.md](BUGS.md)). 28 of the open bugs are the sim runtime spec-shape burn-down (BUG-1658..1685). |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread Terraform upstream; BUG-1584 AzureStack provider deprecation warning despite `metadata_host`; BUG-1590 bleephub run-approvals empty-success gap; BUG-1618 bleephub webhook `organization` block for org-owned repos. |
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
