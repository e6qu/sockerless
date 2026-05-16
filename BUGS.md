# Known Bugs

**1005 filed · 995 fixed · 10 open · 1 false positive.**

Standing rule: every CI / live-cloud failure lands here with a one-liner *before* any fix attempt. Workarounds, fakes, placeholders, silent fallbacks, skips, and incomplete implementations are all bugs and get the same treatment. Per-bug fix detail beyond the one-liner: `git log <commit>` or the linked PR.

Live status (cells, branch, milestone) lives in [STATUS.md](STATUS.md). Vibe-pattern numbers reference `docs/VIBE_CODING.md`.

## Open

| ID | Sev | Area | Pattern | One-liner |
|----|-----|------|---------|-----------|
| 995 | P1 | `backends/core/handle_extended.go`, `handle_images.go`, `handle_libpod.go` | 11 / 12 | HTTP handlers read `s.Store.*` directly instead of dispatching through `s.self.<Method>`. Affected: `handleSystemDf` (volumes + images + containers branches), `collectAllContainers`, `handleContainerList` fallback (`CloudState == nil`), `handleImagePrune`, `handleLibpodContainerList`. Siblings of BUG-991 / BUG-992; cloud backend overrides never reached from these HTTP paths. Fix: delegate to `s.self.SystemDf`, `s.self.ImagePrune`, `s.self.ContainerList`, etc. |
| 1001 | P1 | `bleephub/gh_issues_graphql.go`, `gh_pulls_graphql.go` | 9 | GraphQL resolvers for ProjectV2 + PR-review-thread fields wired to `alwaysNil` / `emptyList` / `alwaysEmptyString` — returns fake data instead of real not-found errors. Examples: `ProjectV2ItemFieldSingleSelectValue.optionId/name` always-nil; 11 PR comment editor / reviewer / suggested-edits fields always-nil. Fix: implement real lookups against the bleephub store, or return GraphQL field-level errors per the spec — never fake data. |
| 998 | P1 | `backends/core/handle_images.go::decodeRegistryAuth` | 1 | Returns `("", "")` on base64 *or* JSON decode failure. Callers can't distinguish "no auth header" from "malformed auth header" — real Docker daemon returns 400 on malformed. Fix: split the function so empty-header is the only success-on-empty path; propagate decode errors to the HTTP handler. |
| 1002 | P2 | `simulators/azure/acr.go` Replications list | 9 | `GET /registries/{r}/replications` returns empty array if `{r}` doesn't exist instead of `ResourceNotFound`. Real Azure returns the 404 envelope. Fix: verify parent registry exists; return the Azure error shape with `code: "RegistryNotFound"`. |
| 996 | P2 | `simulators/{aws,gcp,azure}/*.go` | 1 | ~18 sites of `_ = sim.ReadJSON(r, &req)` swallow JSON-decode errors. Some handlers genuinely have an optional body; others should error. Fix: per-site audit — mandatory body → propagate parse error; optional body → switch to `io.Copy(io.Discard, r.Body)` with a `why` comment so the choice is explicit. |
| 994 | P2 | repo-wide | 8 | ~60 production code comments reference phase numbers (`// Phase 87b`) or BUG IDs (`// BUG-944`). Memory rule + guiding principle 14: that metadata belongs in commits / PRs / BUGS.md, not in source comments — they rot. Fix: sweep `backends/`, `simulators/`, `bleephub/`, `cmd/`, `api/`, `agent/`, `github-runner-dispatcher-*/` — drop the phase / bug reference; preserve the *why* when load-bearing. |
| 999 | P2 | `backends/core/tags.go::TagSet.InstanceID` | 8 | Field marked `Deprecated: use Cluster instead for stateless model` but has 27+ active callers across backends. Either the deprecation is real (migrate callers and remove the field) or it's stale (drop the misleading comment). Fix: audit callers; pick one. |
| 1004 | P2 | `bleephub/store.go::SeedDefaultUser` | 8 | Seeded admin user keeps a `bph_`-prefixed token "for backwards compatibility with existing tests + integrations" while `CreateToken` now mints `ghp_`. Rule: if real GitHub does X, bleephub does X — and real GitHub doesn't issue `bph_`. Fix: switch seeded token to `ghp_`; update fixture references. |
| 1005 | P3 | `bleephub/workflows.go::~410` | 5 | `if foundJob.Def != nil && foundJob.Def.Strategy != nil && foundJob.Def.Strategy.FailFast != nil` 3-deep nil chain when resolving matrix fail-fast. Fix: normalise on YAML parse (default `Strategy` + `FailFast` to non-nil zero values) so the runtime path is single-deref. |
| 1003 | P3 | `simulators/gcp/artifactregistry.go::buildOCIHandler` | 14 | Single-call-site helper called once at line 223; the abstraction has no second consumer. Fix: inline. |

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

995 bugs filed and fixed across phases 86–161.

- **997** (Phase 161) — 18 sites across `bleephub/{store,store_repos,gh_apps_store,gh_apps_user_tokens,gh_oauth}.go` silently swallowed persistence writes via `_ = st.persist.Put(...)` / `_ = st.persist.Delete(...)`. Fix: add `Persistence.MustPut` + `Persistence.MustDelete` (`log.Fatalf` on write failure, matching `MustNewPersistence`'s open-time invariant). Sweep every site to the Must* variants.
- **1000** (Phase 161) — `bleephub/auth.go::handleOAuthToken` previously returned a valid 1-year `alg:none` JWT for any input — auth-bypass. Fix: validate `grant_type`, `client_assertion_type`, and the RS256 `client_assertion` JWT signature against the agent's registered RSA public key (Modulus + Exponent from `POST /_apis/v1/Agent/{poolId}`). JWT `iss` must match a known agent ClientID; `exp` honored. OAuth-envelope errors (400 / 401). Tests rewritten to drive a real RSA keypair + signed assertion (success / missing-assertion → 400 / unknown-client → 401), replacing the prior Pattern-3 implementation-coupled test that asserted only `access_token != nil` on an empty body.
- **993** (Phase 160) — Close-then-bind port-allocator race in `simulators/gcp/sdk-tests/{helpers_test.go,quota_test.go}`. Pattern was `ln := Listen(:0); port = ln.Addr().Port; ln.Close(); ln2 := Listen(:0); grpcPort = ln2.Addr().Port; ln2.Close()` — between `ln.Close()` and `ln2 := Listen()` the OS could re-assign the just-freed port to `ln2`, so the sim's HTTP and gRPC servers both wanted the same port and the second `bind()` failed with "address already in use". Surfaced on PR #160 CI 2026-05-16 (`TestSDK_RegionalCPUQuota_RejectsCloudFunctionsDeploy`). Fix: allocate both listeners while both are open, then close both — simultaneous-open listeners cannot collide by construction. Verified locally with `count=3` reruns; ports are now guaranteed sequential.
- **992** (Phase 158) — Sibling of BUG-991. `handleImageList` in `backends/core/handle_images.go` read `s.Store.Images.List()` directly with extensive filter logic, never calling `s.self.ImageList()`. For passthrough backends (docker) this returned `[]` even when the upstream daemon had images. Fixed by replacing the 100-line in-handler filtering with a thin delegate to `s.self.ImageList(opts)` — each backend's override knows where the truth lives (docker → upstream daemon; cloud backends → `ImageManager` which merges Store + cloud registry). Verified: `docker images` against `backends/docker` now returns the upstream daemon's images. Volumes + networks already delegated correctly; no other list handler affected. Surfaced during BUG-991 investigation on 2026-05-13.
- **991** (Phase 158) — Classic fallback-hiding-bug. `docker run --rm` against `backends/docker` returned `error waiting for container: No such container` because `handleContainerWait`'s non-CloudState branch checked `s.Store.Containers.Get(id)` directly and short-circuited to 200/StatusCode=0 on `condition=removed`. The wait fires *before* start in the docker CLI's foreground flow, so the local Store lookup races and lies. Fixed by replacing the Store-direct branch with `s.self.ContainerInspect(ref)` (which delegates to upstream on passthrough backends) + `s.self.ContainerWait` for the actual block. Also removed the parallel `condition=removed → StatusCode: 0` fallback in `BaseServer.ContainerWait` itself — callers wanting "already removed = success" semantics must `Inspect` first themselves, never return success on a missing resource. Surfaced 2026-05-13 during Phase 157 docs sample-capture; the symptom directly motivated Phase 158's vibe-coding-anti-pattern doc + skill work.

Phases 154 / 155 / 156 / 157 closed zero new bugs — broad GitHub API sweep + docs refresh + component-adaptor sweep shipped without surfacing regressions. The `google.golang.org/api` v0.278.0 → v0.279.0 bump on PR #156 was upstream dep drift flagged by the `check-latest-deps` pre-push hook, not a sockerless bug.

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
