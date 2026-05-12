# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

State [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · architecture [specs/](specs/).

This file keeps narrative — *why* each phase, what was surprising, what blocked. Per-bug detail in [BUGS.md](BUGS.md); code-level detail in `git log`.

## 2026-05-12 — Phase 153 bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compat (`docs-cleanup-actionable` branch, PR #153 in flight)

13-sub-task phase. 12 commits shipped, P153.13 in flight. Beyond the original "parity" scope, the user folded in two more pieces at the end:

- **SQLite persistence (P153.12)** — `BLEEPHUB_PERSIST=true` gates a write-through SQLite layer for users / tokens / apps / oauth_apps / installations / installation_tokens / user_to_server_tokens / refresh_tokens / repos. Fail-loud on open failure (BUG-985/986 pattern). Git storage stays in-memory (separate refactor when needed).
- **Real `gh` CLI compatibility (P153.13)** — the test must use `gh repo create` / `gh issue create` / `gh pr create` / `gh release create` directly against bleephub, not `gh api $URL -f` URL hackery.

  The first instinct (changing `-f` to `-F` in the test) was wrong. Real GitHub accepts both — `gh api -f private=false` sends `{"private": "false"}` (string-coerced), and GitHub's Rails layer coerces it to a bool. Bleephub strictly rejecting the string is the bug, not the test. Fixed via new `flexBool` / `flexInt` / `flexInt64` / `flexIntSlice` types in `gh_request_decode.go` that decode either typed or string-coerced JSON values; applied to every bool/int field in the request structs that `gh` CLI might hit. **No fallback shims** — this is the GitHub API spec, just made explicit on the bleephub side.

  Test harness rewrite: `bleephub/test/run-gh-test.sh` calls native `gh` commands. `gh auth login --hostname localhost --with-token` registers bleephub as a known GHES host; subsequent `gh repo create`, `gh issue create`, `gh issue view`, `gh issue list`, `gh repo view`, `gh repo list` all target bleephub directly. The `api` helper stays for endpoints with no native `gh` verb (apps/{slug}, /applications/{cid}/token, suspend, OAuth Apps mgmt).

  `make bleephub-gh-docker-test` wires the real harness entrypoint — builds `bleephub/Dockerfile.gh-test` (bleephub + gh 2.92.0 + jq + git + a self-signed TLS cert), runs the harness inside the container. Used to be orphaned (script existed but no make target invoked it); now the canonical end-to-end check before any PR claim.



Audit on 2026-05-12 found bleephub's GitHub Apps surface was happy-path complete (manifest → JWT → installation token → ghs_ usage) but missing seven layers: a public app lookup, suspend/unsuspend, org/user installation lookup, `installation/repositories`, `/applications/{client_id}/*` token mgmt, repo + app-level webhook redelivery, the Checks API, plus semantic gaps (permission enforcement on installation tokens, repo_selection=selected support, webhook installation field + headers, app-targeted events, token-prefix disambiguation, HATEOAS url fields). 10 sub-task commits on the same branch close those gaps; SQLite persistence (P153.12) and the gh-CLI smoke harness extension (P153.13) round it out.

**GitHub Apps and OAuth Apps are separate concepts.** The user pushed back early when I conflated them. They share an OAuth-flow shape but the data model is distinct: GitHub Apps have a JWT-signing key, installations on users/orgs, fine-grained permissions, and `ghs_`/`ghu_` tokens; OAuth Apps are client_id+client_secret pairs that mint classic `gho_` tokens with `repo`/`read:org`-style scopes. Both register under `Store` (`Apps` vs `OAuthApps`); both can be Basic-authenticated on `/applications/{client_id}/*`; the same authenticateClientCreds helper resolves the client_id against both tables, so the family handles either kind transparently. UI surfaces them in separate tabs.

**Token prefix disambiguation is load-bearing.** Real GH uses distinct prefixes (`ghp_`/`gho_`/`ghu_`/`ghs_`/`ghr_`/`github_pat_`) because clients commonly branch on prefix to decide what auth shape they're holding. Bleephub previously minted `bph_…` for everything except `ghs_`, so probot/octokit detection routines would misclassify. Middleware now recognises each prefix; default mint switched from `bph_` → `ghp_`. The seeded admin's static `bph_…` token is preserved as a backwards-compat path — existing tests + the `bleephub-test` Docker harness rely on it.

**Permission enforcement is a decorator, not a middleware.** Real GH's gate is per-endpoint per-permission-pair, not "ghs_ token can do anything within its app's perms". `requirePerm(scope, level, next)` wraps each write-class handler at registration time so the scope is local + readable next to the route. PAT bypass is deliberate — real GH treats PATs as full-scope. The ghu_/gho_ paths dispatch to either installation-perms (for App user-to-server) or a classic-scopes-to-fine-grained mapping (for OAuth App user tokens, e.g. `repo` covers `contents:write`+`issues:write`+`pull_requests:write`).

**Installation tokens are immutable snapshots.** A test caught the subtlety: bumping an installation's permissions doesn't retroactively bump tokens it already minted. Mirrors real GH. Operators (or sims) must re-mint to pick up new perms — recorded as a test invariant rather than an implementation choice that could drift later.

**App-level webhooks are a separate channel from per-repo hooks.** A GitHub App owns one webhook URL (`App.WebhookURL`); per-repo hooks (existing) own their own. AppHookDeliveries is keyed by app ID; per-repo HookDeliveries is keyed by hook ID. HookID < 0 is the sentinel marking an app-level delivery — read by doDeliverAttempt to populate X-GitHub-Hook-Installation-Target-Type=integration. installation/installation_repositories events emit through deliverAppWebhook (3-attempt exp-backoff, same shape as deliverWebhook). All four header families now ship (X-GitHub-Event, X-GitHub-Delivery, X-GitHub-Hook-ID, X-GitHub-Hook-Installation-Target-{Type,ID}); HMAC sig fires both SHA-256 (`X-Hub-Signature-256`) and SHA-1 (`X-Hub-Signature`) so legacy clients work.

**UI redesign was surprisingly small.** Three tabs (GitHub Apps · Installations · OAuth Apps) replacing two. Per-installation Suspend/Unsuspend/Delete buttons hit new sim-only mgmt endpoints — the web UI can't sign JWTs, so the App's `PUT /app/installations/{id}/suspended` would 401. The sim-only mgmt path (`/api/v3/bleephub/installations/{id}/suspend|unsuspend`) sits beside the JWT path: same store mutation, same event emission, different auth shape (PAT instead of JWT). Created-secrets dialog shows PEM + client_id + client_secret + webhook_secret once in a code block with a warning, then closes and flushes.

**Phase 153 lives on `docs-cleanup-actionable`.** Started as a docs cleanup PR (STATUS/PLAN/DO_NEXT/BUGS/WHAT_WE_DID trim) and the user asked to continue the implementation on the same branch. Coincidence that the PR number is 153 — but useful, since the spec doc `specs/BLEEPHUB_GITHUB_API_PARITY.md` and the phase entry in PLAN.md both reference Phase 153 already.

## 2026-05-10 — Phase 87d closeout + Phase 92 bundle (`phase-87d-92-observability-closeout-gcs-sync` branch)

Six commits bundling the Phase 87 closeout (three real gaps the audit surfaced after PR #150 merged) with the next planned volume-work entry, Phase 92.

**Phase 87d — three closeouts that turned out load-bearing.** I'd flagged these as "candidates for follow-up" in the PR #150 audit but the user pulled them into a single bundle when planning the next phase. Each turned out small + concrete:

1. **Trace propagation was leaking at the boundary.** Admin and bleephub had `otelhttp.NewHandler` wrapping their incoming muxes since Phase 87b — so admin's request handlers correctly recorded a span. But when admin then made an outgoing call to a backend (proxy, health probe, rollup), it used a vanilla `&http.Client{}` and the trace context never serialized into a `traceparent` header. Backend's `otelhttp.NewHandler` got a context-less request and started a fresh trace. Every admin → component hop was a chain break. Fix: wrap each `&http.Client{}` site (7 in admin + 2 in bleephub) with `otelhttp.NewTransport(http.DefaultTransport)` and set the global `TraceContext + Baggage` propagator inside each of the 5 `InitObservability` implementations. Once the global is set, otelhttp.NewTransport reads from it automatically — no per-call code changes.

2. **MeterProvider was a no-op all along.** `otelhttp.NewHandler` auto-instruments HTTP request count / duration / size **if a MeterProvider is set**. Phase 87b/c set up TracerProvider + LoggerProvider but skipped MeterProvider. The free instrumentation was being emitted into a noop meter. Fix: add MeterProvider with the OTLP HTTP exporter inside InitObservability — same shape as TracerProvider. Also wire `runtime.Start` from `go.opentelemetry.io/contrib/instrumentation/runtime` so each binary emits Go runtime metrics (goroutines, GC pauses, heap size). No custom counters this round (confirmed zero existing counter call-sites in the codebase via grep).

3. **No e2e check that any of this actually worked.** `make stack-observability-up` brought up the Otel collector + VictoriaLogs + Jaeger, but nothing asserted that with the stack up + a backend running with `OTEL_EXPORTER_OTLP_ENDPOINT` set, telemetry actually landed. I'd been eyeballing the dashboards. Added `make stack-observability-validate` which polls VictoriaLogs (`/select/logsql/query?query=service.name:"<svc>"`) and Jaeger (`/api/traces?service=<svc>`) until both return ≥1 record or the timeout expires. Manual operator-grade target — not in CI since CI doesn't bring up the stack.

**Phase 92 — BUG-944 was a documentation lie.** The original BUG-944 fix was "MountOptions are mandatory for runner workspaces". The required options included `metadata-cache:ttl-secs=0` and `metadata-cache:negative-ttl-secs=0` to defeat gcsfuse's 5s negative-cache that hides freshly-written files from sibling containers. Problem: Cloud Run wraps gcsfuse and **rejects** those flags as "Unsupported or unrecognized flag for Cloud Storage volume". Only `implicit-dirs / o / file-mode / dir-mode / uid / gid` are accepted. So the documented fix was unenforceable on Cloud Run; deploys silently went through with the unsafe defaults and runner-task → sub-task script handoffs failed with stale `_temp/event.json`. The real fix is to abandon gcs-fuse on Cloud Run entirely. `gcs-sync` already exists (per-exec tar/untar, no FUSE, strong consistency); Phase 92 just deregisters `GCSFuseDriver` on cloudrun + gcf and replaces the translator's BackingGCSFuse case with a reject arm pointing at gcs-sync. The driver code stays in `backends/gcp-common/storage_gcsfuse.go` for hypothetical future backends without the flag-allowlist constraint, but it's no longer wired anywhere live.

**The deeper observation.** BUG-944 is a clean example of "documentation fixes" being unsafe when the underlying primitive doesn't accept the documented configuration. Future cross-cloud audits should distinguish between "sockerless can configure this" (operator-controllable) and "the cloud accepts this" (cloud-controllable). When the cloud doesn't accept the configuration sockerless needs, that's the reject-with-pointers signal — not a TODO comment.

## 2026-05-10 — Phase 87c — zerolog → OTel logs bridge across all 12 components (`phase-87c-zerolog-otel-bridge` branch, PR #150)

Two implementation commits + state save. Closes the observability story for every sockerless process: each log line now flows through BOTH stderr (operator-visible via `ConsoleWriter`) AND the OTel logs SDK when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. With the env var unset, behaviour is identical to today — preserves the components-decoupled invariant.

**The user pushed back on splitting (again).** I had been carving Phase 87c into 87c (backends) + 87c.1 (sims+admin+bleephub) since they live in separate Go modules. The user said "do not split phase 87 into further sub/micro-phases" and "keep all phase 87 on a single PR". Right call — even though the modules are separate, the work is mechanical mirroring of the same bridge code; reviewer load on one PR is the same as two PRs minus the re-review cost.

**Three bridge variants ended up shipping.** Looked the same on paper but the binaries differ:

1. `backends/core` (used by 7 backends) — full `Observability{LogWriter, Shutdown}` + `OTelLogWriter` parsing zerolog JSON line-by-line. zerolog level → OTel severity, `message` → body, other keys → attributes.
2. `simulators/{aws,gcp,azure}/shared/otel.go` + `bleephub/otel.go` — same shape mirrored, since neither imports `backends/core` (separate Go modules with `replace` directives or none at all). `Config.LogWriter` field threaded through `NewServer` lets the sim main.go assign the writer before the simulator's logger is built.
3. `cmd/sockerless-admin/otel.go` — admin uses **stdlib `log`**, not zerolog. Stdlib emits flat text lines, not JSON. Added `TextLogWriter` that records each line at INFO severity with the trimmed text as body. Wired via `log.SetOutput(io.MultiWriter(os.Stderr, TextLogWriter))`.

**The shared/ go.mod gotcha.** `simulators/{aws,gcp,azure}/shared/` each have their own `go.mod` (module `github.com/sockerless/simulator`, with the parent sim module using a `replace` directive). First commit hit golangci-lint failures because `go mod tidy` in the parent sim module didn't pull the new OTel logs deps into the shared/ module's go.sum. Had to run `go mod tidy` in each shared/ submodule explicitly. Same lesson as Phase 87b but I'd forgotten the structure — now noted.

**Components-decoupled invariant preserved.** Emission gated entirely on `OTEL_EXPORTER_OTLP_ENDPOINT`. No admin or UI dep injected into any backend / sim / bleephub. Each separate Go module owns its own bridge code.

## 2026-05-10 — Phase 91 (consolidated) — Lambda framework + GCP PD reject + ECR Gallery (`phase-91c-lambda-backingspec-migration` branch)

Two implementation commits + state save. Per user direction, all remaining Phase 91 work consolidated here — no more sub-phase splits.

**The user pushed back on splitting.** I had been carving Phase 91 into 91 / 91b / 91c / 91c.1 / 91d sub-phases as the audit revealed each cloud's distinct shape. The user said "stop splitting phase 91 into sub-phases, keep it all on a single PR". Right call — by the time I'd shipped 91 (cloudrun+gcf BackingMemory) and 91b (ECS+ACA+AZF BackingMemory), the remaining work (Lambda framework migration, cloudrun+gcf BackingPDEphemeral reject) was small enough to coexist on one branch.

**Three pieces in this PR:**

1. **Lambda framework migration.** `fileSystemConfigsForBinds` previously built `FileSystemConfig{Arn, LocalMountPath}` inline from `accessPointForVolume(...)` + `accessPointARN(...)`. The migration: collapse the canonical AP ARN through `storageBackings.Resolve(BackingEFSEphemeral) → driver.CloudSpec(SharedVolumeRef{...}) → translateBackingSpecToLambda(spec)`. Lambda-specific constraints (single-FSC-per-function, `/mnt/[A-Za-z0-9_.\-]+` path constraint, sub-path collapse) stay in `fileSystemConfigsForBinds` as caller-side aggregation rules; they don't belong in the per-spec translator. Lambda joins the other 5 backends in unified framework dispatch.

2. **BackingPDEphemeral rejection on cloudrun + gcf.** Cloud Run Services don't expose Compute Engine PD as a first-class volume primitive — `runpb.Volume` has EmptyDir / Secret / CloudSqlInstance / Gcs / Nfs but no PD. Real implementation would require a Cloud Run Admin API surface that doesn't exist for Services today (the spec notes this at line 567). Translator rejects loudly with concrete pointers: `Backing: gcs-fuse` (MountOptions per BUG-944) for cross-task workspace sharing, `Backing: gcs-sync` for per-step granularity, GCE-backend bookmark for real PD attach. GCF rejection mirrors cloudrun (Gen2 sits on Cloud Run Services).

3. **ECR Public Gallery as Docker Hub alternative.** Cloudrun + gcf integration TestMain hit Docker Hub anonymous-pull rate limits during local + CI runs. User pointed at `public.ecr.aws/docker/library/*` which mirrors the Docker Library images without the 100-pulls-per-6h anonymous quota. Multi-stage Dockerfile FROM lines + `docker pull` calls switched. Saved the operator hint as `feedback_ecr_gallery_alt.md` for future similar throttling cases.

**The deeper observation.** Phases 91 + 91b + 91c together prove that a "cloud-agnostic backing model" is honest only when it maps to actual cloud-native primitives. `BackingMemory` is the easiest case — Cloud Run / Cloud Run Functions / ACA all expose EmptyDir-style tmpfs as a first-class volume type, so the dispatch is clean. ECS exposes the same idea at a different layer (LinuxParameters.Tmpfs[] on container-def, not Volumes on task-def) → reject loudly. AZF doesn't expose tmpfs at all → reject loudly. Lambda is the framework outlier (`fileSystemConfigsForBinds` predates BackingSpec); migration in this PR brings it into the fold. `BackingPDEphemeral` is even more honest: Cloud Run Services lack the primitive entirely, so the operator's request can't be honored on-cloud — explicit rejection with alternatives is more useful than silent fallback.

## 2026-05-10 — Phase 91b BackingMemory on ECS / ACA / AZF (`phase-91b-backingmemory-ecs-lambda` branch)

One implementation commit + state save. Continues Phase 91's BackingMemory work across the AWS + Azure backends. The pattern that emerged was per-cloud divergent — exactly why splitting was the right call.

**ACA: clean cloud-native match.** Azure Container Apps revisions support `StorageTypeEmptyDir` as a first-class volume type (the SDK enum literally enumerates it alongside `StorageTypeAzureFile`). One-line addition to the translator's switch arm; no architectural friction.

**ECS: explicit reject.** ECS task-def Volumes don't expose a tmpfs primitive at all — RAM-backed mounts live at `ContainerDefinition.LinuxParameters.Tmpfs[]` (container-def, not task-def). Two layers, two different shapes. The choice was: silently substitute `Host{}` (disk-backed) volume with a misleading "memory" label, OR reject loudly. Chose rejection — the operator's expectation when picking `Backing: memory` is RAM, not disk; lying about that would surprise them at runtime when the cache they thought was in RAM was actually paging through disk. Error message points at the right primitive (`LinuxParameters.Tmpfs`) + the alternative (`Backing: emptyDir` for disk-backed task-scoped scratch).

**AZF: explicit reject.** Azure Functions WebApps storage surface (`AzureStorageInfoValue`) is BYOS-only — no tmpfs primitive at that layer. Per-invocation `/tmp` is the closest analogue but isn't a Docker-style mount. Same rejection logic as ECS, pointer to `/tmp`.

**Lambda: deferred.** Lambda's volume path predates the BackingSpec framework — `volumes.go::fileSystemConfigsForBinds` builds `lambdatypes.FileSystemConfig` directly from `awscommon.EFSManager` without ever calling `storageBackings.Resolve`. Wiring `BackingMemory` requires first migrating Lambda to the framework. That's a separate refactor PR (Phase 91c) and shouldn't be bundled with translator extensions — different blast radius.

**The deeper observation.** Phase 91 + 91b together prove the cloud-agnostic backing model only goes as deep as the cloud-native primitives it maps onto. `BackingMemory` works cleanly on Cloud Run / Cloud Run Functions / ACA because all three expose `EmptyDir{Memory}` as a first-class volume type. ECS exposes the same idea at a different layer (LinuxParameters), and AZF doesn't expose it at all. The rejection arms aren't a failure of the framework — they're the framework being honest that the operator's request can't be honored on that cloud, with concrete pointers at what to do instead.

**5 new tests** across ECS / ACA / AZF: ECS rejection error contains the right pointers; ECS EFS path still works (regression guard); ACA EmptyDir maps cleanly; ACA AzureFile path still works; AZF rejection points at `/tmp`.

## 2026-05-10 — Phase 91 BackingMemory translator on cloudrun + gcf (`phase-91-pd-ephemeral-volumes` branch)

One implementation commit + state save. Audit-driven scope.

**The audit reframed the phase.** Branch was created as `phase-91-pd-ephemeral-volumes` to match PLAN.md's "lift the runner-task `emptyDir` fallback to real-workload provisioning of `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`". Reading the existing code revealed that two of those three are already wired:

- `efs-ephemeral` was wired on ECS in Phase 127. Lambda's inline EFS path (predating the BackingSpec framework) is functionally equivalent.
- `azure-files-ephemeral` was wired on ACA + AZF in Phase 127.
- `pd-ephemeral` on Cloud Run was always a bookmark. The spec at line 567-568 calls this out: Cloud Run Services lack first-class PD volume attach; real implementation requires sockerless to manage Compute Engine `disks.create`/`attach`/`delete` per task. Multi-day cloud-API work, deferred to Phase 91d.

The actual gap the audit surfaced: `BackingMemory` (Phase 127) had its driver registered in all 6 backends — `core.NewMemoryDriver(64)` — but no per-backend volume translator handled the `case core.BackingMemory` arm. An operator setting `Backing: memory` on a SharedVolume hit "unsupported backing kind" from the translator's default case despite the driver framework claiming support. Framework-vs-translator mismatch.

**This PR's deliverable.** Close the gap on cloudrun + gcf — the two backends most likely to use a memory backing for runner-task workspaces. Adds `case core.BackingMemory:` to `runpbVolumeFromBackingSpec` in both translators. Mapping: Cloud Run's tmpfs primitive is `EmptyDir{Medium: MEMORY}`; `spec.Memory.SizeMB > 0` forwards to the `SizeLimit` field as `<N>Mi` (Cloud Run's accepted format); `SizeMB == 0` leaves SizeLimit blank so the cloud uses the container's memory limit as the cap.

**Why split 91/91b/91c/91d** instead of one big PR. Each cloud's tmpfs primitive differs. ECS exposes tmpfs at the *container-def* layer (`LinuxParameters.Tmpfs[]`), not the *task-def* layer where Volumes live — wiring on ECS is cross-layer plumbing, not a parallel translator addition. Lambda has no volume primitive for RAM mounts at all (`/tmp` is per-invocation scratch, 512 MB–10 GB). ACA + AZF need their own translator extensions. Per-cloud separation keeps reviews focused.

**5 tests** assert SizeMB→SizeLimit composition, SizeMB=0 → no SizeLimit, nil Memory spec → EmptyDir without limit. Pure-function translator; no live-cloud calls.

## 2026-05-10 — Phase 87b component-side OTel SDK wiring (`phase-87b-component-otel-wiring` branch)

Two implementation commits + state save. Trace emission for every Go binary in the project — backends, sims, admin. Logs already worked from Phase 87 via the collector's filelog receiver scraping `.stack-pids/*.log`.

**Discovered during the audit.**

- `backends/core/server.go` already wraps the mux with `otelhttp.NewHandler` (line 501) — so all 7 backends *had* HTTP-level instrumentation, but the spans were going into a no-op tracer because only `backends/docker/cmd/main.go` actually called `core.InitTracer`. The 6 other backends just needed the 4-line init at startup. Cheapest possible win.
- Sims had OTel as transitive deps (pulled in by something else) but didn't use them. Adding `otelhttp.NewHandler` at the outermost middleware layer + a new `shared/otel.go` per cloud + 4 lines in each main.go was the full work.
- Admin's go.mod had zero OTel deps — separate Go module without backend-core. Two paths: introduce a shared `pkg/otel` module, or duplicate the helper in admin. Duplication won (matches the per-cloud `shared/` pattern; small code).
- bleephub was already fully wired (`InitTracer` + `otelhttp.NewHandler`) since the Phase 86 baseline — no changes needed.

**Helper-duplication choice.** The phase plan in DO_NEXT.md offered two options: shared `pkg/otel` package or per-module duplication. Picked duplication. Reasons: each `InitTracer` is ~30 LOC including imports — small enough that DRY-ing it across module boundaries pulls in more complexity than it saves. The shape is identical across all 4 copies (backends/core, sim/aws/shared, sim/gcp/shared, sim/azure/shared, admin), and divergence is unlikely because the function only knows about `OTEL_EXPORTER_OTLP_ENDPOINT` + service name. If someone wants to extend it later (say, register a zerolog hook per Phase 87c), they touch all 4 — same blast radius as a shared module.

**Why otelhttp.NewHandler is outermost.** The wrap order matters. By placing it at the outermost layer, per-request spans cover the full middleware stack — auth, logging, request-id, route handling — instead of just the post-routing path. Operators see complete latency including any expensive middleware work. Existing middlewares run *inside* the span, so their structured log output naturally correlates by trace ID once Phase 87c bridges zerolog into OTel logs.

**No-op safety.** All four `InitTracer` helpers return a no-op shutdown function when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset. So binaries that never see the env var (default operator workflow without `make stack-observability-up`) pay zero runtime cost — no exporter goroutines, no batching, no extra allocations beyond the otelhttp middleware's built-in noop tracer fast path.

**What this phase explicitly does NOT do.** No zerolog → OTel logs bridge (Phase 87c, optional — filelog receiver covers logs already). No metrics export (counters / histograms). No custom span attributes beyond what otelhttp emits.

## 2026-05-10 — Phase 87 observability — Stack A first PR (`phase-87-observability` branch)

Four implementation commits + state save. The original Phase 87 plan listed 6 sub-steps; reading the existing code split it into two PRs along a natural seam:

- **First PR (this branch)**: ship the *stack* (otel-collector + VictoriaLogs + Jaeger) + admin UI integration + docs. Logs work day-1 via the collector's filelog receiver scraping `.stack-pids/*.log`, no binary changes.
- **Phase 87b (follow-up)**: wire the OTel SDK into each component's `main.go` so traces emit to OTLP. zerolog→OTel logs bridge so OTLP-mode operators don't depend on the filelog fallback.

**Why split.** The component-side wiring means touching ~12 main.go files and either adding a shared `pkg/otel` module or duplicating the helper across the existing per-module `shared/` pattern. Doing it alongside the stack would have ballooned the PR. Splitting also lets operators get value from the stack (filelog-scraped logs in VictoriaLogs) before any binary touch.

**Filelog receiver is the cleverness.** The collector's filelog receiver watches `.stack-pids/*.log` and ships every line to VictoriaLogs tagged with `service.name = <pidfile-basename>`. So even without component-side OTLP wiring, every sockerless binary's stdout is searchable in VictoriaLogs grouped by service. Phase 86's file-tail-based diagnostic panel and the operator-grade VictoriaLogs UI feed off the same source. When Phase 87b lands, the filelog path is the redundant fallback (collector dedupes by source).

**Why a static observability config endpoint.** `GET /api/v1/observability` reads `OTEL_LOGS_DASHBOARD` / `OTEL_TRACES_DASHBOARD` at admin boot. The UI fetches it once with a 5-min staleTime. Operators bring up the observability stack with `make stack-observability-up`, then export the dashboard URLs before starting admin. Empty / unset → UI hides the chips → file-tail-only experience from Phase 86. Either URL set → chips render with the instance name as a query filter.

**URL-filter shape.** VictoriaLogs canonical filter is `service.name=<value>`; Jaeger canonical is `service=<value>`. Both overridable via `OTEL_LOGS_SERVICE_PARAM` / `OTEL_TRACES_SERVICE_PARAM` for operators using a custom collector pipeline that writes the service identifier to a different attribute.

**Make-target shape.** `make stack-observability-{up,down,status}` mirrors the existing `stack-up` pattern but writes PIDs into `.stack-pids/observability/` instead of `.stack-pids/`. The two stacks are independent — `stack-down` doesn't kill the observability stack, and the observability stack survives a cell `stack-down` so debugging across stack restarts is uninterrupted.

**State directories** under `.sockerless-state/observability/{logs,traces}/` align with Phase 84's convention. `make purge-state-all` (Phase 84) wipes them alongside other instance state.

**Stack swap is make-work.** The brief in PLAN.md noted that the same component-side OTel SDK works against OpenObserve (AGPL) or SigNoz (MIT) — only `make/stack.mk` changes. This PR doesn't pin the operator to Stack A; future PRs can ship `make stack-observability-openobserve-{up,down,status}` etc. as parallel targets. Components emit OTLP either way.

**What this PR explicitly does NOT do.** No SDK wiring on components. No metrics export (sockerless's existing zerolog setup doesn't carry per-binary counters/histograms today; that's a Phase 87b+ design question). No alerting / paging integration. No dashboards beyond what VictoriaLogs/Jaeger ship by default.

## 2026-05-10 — Phase 86 health + supervision surface (`phase-86-health-supervision` branch)

Three implementation commits + state save. The brief from PLAN.md said "mark unhealthy on ANY of: process exit / non-2xx /v1/health / probe timeout" — reading the existing code showed half of it already worked (signal-0 PID probe + `/v1/health` polling + 1 s timeout from Phase 79 step 7). The actual Phase 86 work was fixing two gaps:

1. **No exit-code capture.** `start-component` ran the binary and recorded the PID, full stop. When the binary exited — operator-driven kill or crash — the only signal was the PID file pointing at a dead PID. The UI couldn't distinguish "operator stopped this cleanly" from "this crashed at 12:00:00 with exit 137".
2. **No diagnostic surface for unhealthy rows.** The TopologyPage's per-row `StatusBadge` showed "unhealthy" but offered no way to see why short of clicking through to logs.

**Exit-code capture.** Wrote a watcher-subshell pattern in the `start-component` make target:

```sh
( cd $$dir && \
    env $$envline ./$$bin $$flag :$(PORT) > $$logfile 2>&1 &
    bin_pid=$$!
    echo $$bin_pid > $$pidfile
    ( wait $$bin_pid; code=$$?
      printf '%d %s\n' $$code "$(date)" > $$exitfile ) &
)
```

The pidfile still points at the binary (so the existing SIGHUP / SIGTERM paths via `reload-component` and `stop-component` still target the binary directly). The watcher waits in the background and writes the exit record only when the binary actually terminates. Stale exit records are cleared at the start of each `start-component` so we don't see yesterday's exit reading after a successful restart.

**CrashedSinceStart distinction.** When the binary dies on its own, the watcher writes `.exit` and the pidfile is left in place — `readInstanceStatus` sees `pid > 0 && !alive && exit != nil` and flags `CrashedSinceStart=true`. When the operator runs `stop-component`, the make target removes the pidfile, so the same logic produces `pid=0 && !alive` (the watcher's exit record may still arrive afterwards, but nothing flags it as a crash). This separates "this thing died unexpectedly" from "operator stopped this cleanly" without any change to stop-component's contract.

**5-second probe.** Bumped the `probeHealth` timeout from 1 s to 5 s. Operator-grade reality: a backend doing real work may not answer `/v1/health` inside a second while completing in-flight requests. 5 s matches the brief and the existing `--no-skip` philosophy (don't lie about health when the answer is "give me a moment").

**Diagnostic panel.** New `<UnhealthyDiagnosticPanel>` mounts inside InstanceRow when `shouldRender(status)` is true:

- `health === "unhealthy"`
- `crashed_since_start`
- `!running && pid > 0` (process gone but no exit record — the watcher missed something or was killed alongside the binary)

Healthy / cleanly-stopped rows mount nothing — the diagnostic poll fires only on actually-broken instances, so the cost is bounded to the rows the operator cares about.

The panel surfaces the failing-signal header (with prose-y reason), exit info if present, the health_detail line (e.g. "HTTP 503"), the last 50 log lines via the new bundled `/diagnostics` endpoint (one fetch instead of chaining `/status` + `/logs`), and three actions: deep link to live tail, deep link to project console, refresh.

**Diagnostic endpoint shape.** `GET /api/v1/topology/.../diagnostics?lines=N` returns `{status, log_lines, log_path}`. Default N=50, cap 1000. Reuses the Phase 81 `readLastLines` helper. The cap stops degenerate `?lines=99999` queries; an operator who needs more opens the live tail at `/ui/topology/:p/:i/logs`.

**What this phase explicitly does NOT do.** No auto-restart (deferred — Phase 86 is the *surface*, not the recovery). No paging / alerting integration (operator-driven). No multi-instance health rollup (that's the natural Phase 87 observability shape). No ContainerRow-level health probe (component-decoupled invariant: admin reads what components already expose).

## 2026-05-10 — Phase 85 config edit + hot reload (`phase-85-config-edit-hot-reload` branch)

Two implementation commits + state save. Tight scope by design — the original Phase 85 plan listed four discrete pieces (annotation, edit endpoint, reload endpoint, UI), but the first three are tightly coupled (metadata informs response, response drives UX) so they ship as one commit; the UI is its own commit because TypeScript / vitest scaffolding lives in a different package.

**Why not extend the existing InstanceForm.** The first instinct was "edit-mode InstanceForm gains hot/restart badges and the post-save Reload prompt". That muddles the contract: InstanceForm handles the full-instance edit (name/kind locked, port/cloud/sim editable) which has a different invariant from a config-only edit. The metadata-driven badges only make sense when the operator is changing Config — anywhere else they're noise. So a separate `<ConfigEditModal>` component, triggered by a new "config" button on each InstanceRow, keeps the two flows orthogonal.

**Curated metadata, not introspection.** Admin owns a static `ConfigKeyMeta` table. 3 hot-reloadable keys (SIM_LOG_LEVEL, SOCKERLESS_LOG_LEVEL, SIM_PULL_POLICY — log levels + pull policy, all things components re-read from env per-request); 14 annotated restart-required keys (binding addresses, persistence dirs, cloud resource layout); unknown keys default to restart-required (the safe default).

The metadata lives admin-side, NOT on the component. Per the components-decoupled invariant, components don't grow a "describe my config" endpoint. The cost: admin's metadata drifts behind component reality between releases — when a new SOCKERLESS_X key shows up, admin treats it as restart-required until someone annotates it. The benefit: zero coupling, clear ownership of the operator's mental model.

**ClassifyChanges shape.** `ClassifyChanges(prev, next map[string]string) (hot, restart []string)` — handles added, removed, and changed keys uniformly. A removed key counts as a change of its annotation; an unchanged key is skipped. Sort the slices so the UI gets stable output.

**Reload semantics.** `POST .../reload` re-renders `.stack-pids/<n>.env` and shells `make reload-component NAME=<n>`, which `kill -HUP`s the recorded PID. The component side may or may not handle SIGHUP — Phase 85's contract is "signal sent", not "config absorbed". Reload of a dead PID is an error (stop + start would be the operator's recourse). Re-rendering the env file always happens, so a follow-up restart picks up the latest values whether the component absorbs SIGHUP or not.

**Restart trumps reload in the UI.** If any restart-required key changed, the post-save footer offers Restart as the primary, with "Reload (partial)" as an escape hatch — reload alone wouldn't pick up restart-required changes. If only hot keys changed, just Reload. If nothing changed (operator hit Save without editing anything), just Close.

**Test coverage.** Backend: 9 metadata unit tests (annotation lookup, classification across all four cases — hot, restart, mixed, removed) + 7 endpoint tests (metadata GET, PUT classification + persistence + identity-noop, 404, 400, reload 503/404). UI: 6 vitest cases (rows render with badges, all three save outcomes, Reload click flow, save-error inline + toast).

**What this phase explicitly does NOT do.** No SIGHUP handling on the components — that's per-binary work, deferred. No InstanceForm refactor. No automatic reload-on-save (operator confirms which action to take in the post-save footer).

## 2026-05-10 — Phase 84 per-instance state isolation + BUG-985 (`phase-84-instance-state-isolation` branch)

Three implementation commits + state save. The phase brief was "make multiple sim instances of the same cloud coexist with isolated state across restarts" — the work split into one bug-fix and one wiring task once I started reading the existing code.

**BUG-985 surfaced during the audit.** Before touching anything, I read `simulators/aws/shared/server.go` and `config.go` to understand the persistence layer. Found this in `NewServer`:

```go
if cfg.Persist {
    db, err := OpenDB(dataDir)
    if err != nil {
        logger.Error().Err(err).Msg("...falling back to in-memory")
    } else {
        srv.db = db
        ...
    }
}
```

The operator set `SIM_PERSIST=true` because they want durable state. If `OpenDB` fails (bad path, missing fs perms, full disk), the simulator silently runs in-memory and loses everything across restarts. Per the no-fallbacks principle this is a bug. Filed as BUG-985 and fixed in the same patch — `NewServer` now returns `(*Server, error)`, sim main.go calls `log.Fatalf`. Mirrored across all three sims (`shared/` is duplicated per cloud — they're not the same Go package, so the fix lands in three identical-shape commits-worth of changes folded into one diff).

A similar latent issue lived at `MakeStore` (line 132-141 of `state_sqlite.go`) — silent fallback to in-memory when `NewSQLiteStore` fails per-table. Filed as BUG-986 and folded into the same PR after the user asked for the "out of scope" items to ship together. Failure mode would have been *half-persistent state* across a restart: some tables survive, some silently drop back to memory, no operator signal. Fix: `MakeStore` calls `log.Fatalf` on `NewSQLiteStore` failure. Signature unchanged so the 106 call sites across the three sims aren't touched — every caller is at sim init time, so `log.Fatalf` is the equivalent of a startup error with the failing table name visible in the message.

**Admin SIM_DATA_DIR injection.** `InstanceLifecycle.Start(ctx, project, inst, simPort)` now writes `SIM_DATA_DIR=<repo>/.sockerless-state/<project>/<instance>/` into `.stack-pids/<n>.env` for sim instances. The path scheme matches the spec: project-scoped + instance-scoped, so two sim-aws instances under different projects don't collide. New helpers:

- `managedEnvFor(project, inst, stateRoot)` — admin-synthesised env entries per kind. Sim gets `SIM_DATA_DIR`; backend / bleephub get nothing (their state isn't filesystem-scoped).
- `mergeConfig(managed, operator)` — overlays operator-provided `Instance.Config` on top of admin defaults. Operator wins so a field operator who wants state on `/mnt/big-disk/` can override.

Decision: admin does NOT inject `SIM_PERSIST=true`. Per the components-decoupled invariant, persistence is a behaviour choice the operator makes — admin only fills in path coordination concerns. The result: a sim launched without `SIM_PERSIST=true` runs in-memory regardless of `SIM_DATA_DIR` (which becomes a no-op), and the operator's intent is unambiguous in `sockerless.yaml`.

**Cross-cloud isolation tests.** 5 cases × 3 clouds = 15 tests, in each cloud's `shared/state_isolation_test.go`:

1. `TestPersistenceIsolatedAcrossDataDirs` — two SQLite stores at admin-shaped paths (`<root>/<project>/<instance>/`), write to one, verify no leak.
2. `TestPersistenceSurvivesReopen` — close + reopen the DB at the same path; entries persist. Combined with #1 this is what makes per-instance state usable: stop+restart picks up where it left off without leaking to neighbours.
3. `TestNewServerPersistFailLoud` — BUG-985 regression guard. `DataDir` under a regular file (mkdir → ENOTDIR) returns an error; server is nil.
4. `TestNewServerPersistHappy` — happy path: persistence on, writable dir, `srv.DB() != nil`.
5. `TestNewServerNoPersist` — persistence off, no disk touch, `DB()` returns nil.

The test file is duplicated in each cloud's `shared/` because each is its own Go package (importable as `github.com/sockerless/simulator` from outside but distinct compilation units). The cross-cloud sweep workflow rule lands here.

**Operator workflow target.** Initially I left `make purge-state` out of scope ("operators wipe `.sockerless-state/` directly"), but folded it in alongside BUG-986 once the user asked for the deferred items to ship together:

- `make purge-state PROJECT=<p> NAME=<i>` — wipe one instance's state dir.
- `make purge-state-all` — wipe everything under `.sockerless-state/`.

PROJECT + NAME both required on the single-instance form so a stray invocation can't nuke an unrelated dir; the clean-slate workflow goes through `purge-state-all` explicitly. `stop-component` still leaves state untouched — that's the design — and `purge-state` is the explicit opposite.

**What this phase still does NOT do.** No refactor of the `Persist` / `OpenDB` paths (the architectural shape stays — only the failure mode changes). No `start-component` change in `make/components.mk` — it already sources `.stack-pids/<n>.env`, so admin's env file write is sufficient.

## Older closed phases (compressed)

Narratives older than Phase 84 collapse to one-liners. Per-commit detail in `git log <PR-number>`; load-bearing decisions promoted into [STATUS.md](STATUS.md) invariants or `docs/`.

| PR | Phases | Headline |
|---|---|---|
| #141 | 83 | Shared `ResourceListPage<T>` in `@sockerless/ui-core`; 13 sim pages refactored across simulator-aws / gcp / azure; legacy `/ui/resources` + `/ui/projects/:name` + `/ui/projects/:name/logs` admin pages retired. |
| #140 | 81 + 82 | SSE log endpoint (`/api/v1/topology/projects/{p}/instances/{i}/logs?follow=1`), instance proxy endpoint, single-instance log tail UI, combined timeline + API console UI. Cloud-resources rollup endpoint + UI grouped by instance/cloud/service/flat with failed-sources banner. |
| #139 | 80 | Admin UI `/ui/topology`: project + instance tree, per-instance status polling, Start/Stop/Rebuild, per-kind add/edit instance modal, port registry. Replaced legacy ProjectsPage. |
| #138 | 79 | `sockerless.yaml` topology store; `TopologyManager`; CRUD + lifecycle REST; `make/components.mk` granular targets; port allocator; Phase 87 plan added; `specs/CLOUD_RESOURCE_MAPPING.md` consolidation (Docker→cloud quick reference, CI runner requirements, multi-system CI/CD comparison). |
| #137 | 78 + 79 step 1 | UI polish (dark mode toggle, Toast / InlineError, Modal + ContainerDetail, a11y, perf, READMEs) + admin `Instance` type + components-decoupled invariant established. |
| #135–136 | 121b | Azure sim cloud-faithful (Files data plane, AAD JWT); all-6-backends integration test harness restructured to `SOCKERLESS_TEST_TARGET=sim|cloud`; in-memory `BackingMemory` driver; driver consolidation pattern B (id-token, IAM role, Cloud DNS Zone, Cloud Map, Private DNS Zone); GCP sim Cloud Run invoke routing via sim handler; GCF envelope decode + label round-trip; native arm64 publish workflow (no QEMU). Phase 121b finish: network-discovery adapter consolidation, host-aliases everywhere, AZF cloud-dns + Lambda service-mesh wiring, Azure AD access driver, DNS↔NetworkDiscovery gating cleanup. |
| #134 | 127 | Storage backing driver expansion (`pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`) — per-cloud drivers in `*-common`, per-backend `storageBackings` registry. |
| #133 | 126 | Access driver (`AccessMechanism` enum + `AccessDriver` interface): cloudrun + GCF id-token, ECS + Lambda iam-role (SigV4), ACA + AZF none-internal. |
| #132 | 125 | DNS driver (`DNSMechanism` enum + `DNSDriver` interface): cloudrun cloud-dns-zone, ECS cloud-map, ACA private-dns-zone, FaaS NoOp. `SOCKERLESS_DNS_SEARCH_DOMAIN` per `ContainerCreate`. |
| #131 | 124 | Network discovery driver (`host-aliases` / `cloud-dns` / `service-mesh` / `nat-gateway-only`). Per-backend adapters; `cloudServiceRegister/Deregister/Resolve` migrated through the driver. |
| #130 | 128 | Runner job timeout (two-layer: bootstrap timer + cloud-native cap). `SOCKERLESS_JOB_TIMEOUT_SECONDS`. |
| #129 | 135 | Sim host model: workloads dispatch through Docker honouring explicit `Architecture`; per-cloud-product host-metadata services; static no-`os/exec`-of-workload check; native `ubuntu-24.04-arm` CI. 12 bugs closed. |
| #128 | 134 | Makefile standardization + per-app leaf Makefiles + stack orchestration. |
| #127 | 129#4 + 130–132 | Orphan pod-Service GC; sim parity prep; bleephub workflows + OAuth REST + UI. |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #122–123 | 110 + 118 + 120–123 | 8/8 runner cells GREEN; FaaS pod overlays; cloud-faithful GCP sim; storage-backing driver pilot. |
| #117–121 | 109 + Round-7/8/9 | Live-AWS bug sweep; strict cloud-API fidelity audit. |
| #112–115 | 86–102 | Sim parity; stateless backends; real volumes; FaaS invocation tracking; reverse-agent exec/cp/diff/commit/pause; Docker pod synthesis. |

Per-bug detail in [BUGS.md](BUGS.md). Per-commit detail in `git log`.
