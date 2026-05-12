# Known Bugs

**990 filed · 990 fixed · 0 open · 1 false positive.**

Standing rule: every CI / live-cloud failure lands here with a one-liner *before* any fix attempt. Workarounds, fakes, placeholders, silent fallbacks, skips, and incomplete implementations are all bugs and get the same treatment. Per-bug fix detail beyond the one-liner: `git log <commit>` or the linked PR.

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
- **No legacy-shim fallbacks during development.** Bleephub isn't carrying old behavior — if real GitHub does X, bleephub does X. Don't add a "legacy bph_ token shape" path when the right answer is "match GitHub's `ghp_`".
- **Cross-cloud sweep on every find.** When a pattern is found in one backend, the same code paths in the other 5 backends / 3 sims get checked in the same commit.
- **Pattern B for cloud-specific drivers.** Within a cloud, drivers consolidate into `*-common`; cross-cloud duplication is fine and expected.
- **Doc-only fixes are unsafe when the cloud rejects the config.** BUG-944/987: a documented MountOptions requirement that Cloud Run rejects is no fix. Verify cloud-acceptable, not just sockerless-controllable.
- **HTTP 500 reserved for unexpected panics.** Never return 5xx as a designed failure path; use exit-code header / envelope.
- **External test fixtures must use the real client.** The `gh` CLI test harness uses real `gh repo create` / `gh issue create` against bleephub, not `gh api $URL -f key=val` URL hackery. If bleephub rejects what real GitHub accepts, fix bleephub — don't bend the test.

## Resolved history (compressed)

990 bugs filed and fixed across phases 86–156.

Phases 154 / 155 / 156 closed zero new bugs — broad GitHub API sweep + docs refresh shipped without surfacing regressions. The `google.golang.org/api` v0.278.0 → v0.279.0 bump on PR #156 was upstream dep drift flagged by the `check-latest-deps` pre-push hook, not a sockerless bug.

- **988 + 990** (Phase 153 P153.13) — `gh repo list` + `gh issue list` rejected GraphQL enum names (`CREATED_AT`, `DESC`, `PUBLIC`, `OWNER`). Bleephub declared the args as `String`; gh sends them as enums. Fixed by adding `RepositoryPrivacy` / `RepositoryAffiliation` / `RepositoryOrderField` / `OrderDirection` / `IssueOrderField` / `IssueOrderDirection` enums + adding `repositoryOwner(login)` polymorphic query that gh's repo list uses.
- **989** (Phase 153 P153.13) — `gh issue view` failed because `issueOrPullRequest` returned just `Issue`, not a union with `PullRequest`; PR type missed `milestone`/`comments(last:)`; `PRCommentConnection` missed `nodes`; Issue.milestone resolver returned nil-typed empty map triggering Milestone.number NonNull; Issue.projectItems unimplemented (gh queries Projects v2 as a second round-trip). Fixed by declaring a real `IssueOrPullRequest` union, adding the missing PR fields, returning explicit nil for missing milestones, and adding empty-connection stubs for the Projects v2 surface. Per-bug detail in `git log` / linked PR. Recent ranges:

- **987** (Phase 92, PR #151) — `Backing: gcs-fuse` on cloudrun + gcf produced silently broken cross-task workspaces. Cache-TTL gcsfuse mount flags rejected by Cloud Run; deregister `GCSFuseDriver` and reject `BackingGCSFuse` in the translator with a concrete pointer at `gcs-sync`. Closes the documentation-fix-without-enforcement gap from BUG-944.
- **985–986** (Phase 84, PR #142) — sim shared `NewServer` + `MakeStore[T]` silently fell back to in-memory storage when persistence-open failed. Operator-requested persistence must fail loud. Fix: return error / `log.Fatalf`.
- **975–984** (PR #129) — Phase 135 sim host model + native arm64 CI runners.
- **973–974** (PR #128) — Sim test stability (`Eventually` polling).
- **949 + 972** (PR #123) — GCF `os/exec` workload → `sim.StartContainerSync`; cloudrun/gcf AR-proxy gate on `endpointURL`.
- **877–971** (PR #123) — FaaS pod overlays; 4 GCP runner cells GREEN; cloud-faithful GCP sim; storage-backing driver pilot.
- **845–876** (PR #122) — Phase 110: 4 AWS runner cells GREEN.
- **820–844 + 802** (PR #120) — Driver framework migration + cloud-native typed drivers.
- **786–819** (PR #118) — Round-8/9 AWS sweep; stateless invariant; real layer mirror.
- **770–785** (PR #117) — Round-7 AWS sweep.
- **661–769** (PRs #112–115) — Sim parity; stateless backends; FaaS invocation tracking; reverse-agent exec/cp/diff; Docker pod synthesis.
- **86–660** — Foundation work prior to live-cloud cell validation.
