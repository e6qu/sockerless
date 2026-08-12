# Agent Guidelines

> `CLAUDE.md` is a symlink to this file. Edit `AGENTS.md`.

## Continuity files — read before, update after, write timeless and in past tense

The continuity files are **`STATUS.md`, `PLAN.md`, `DO_NEXT.md`, `WHAT_WE_DID.md`, `BUGS.md`** (and only these — never invent new continuity docs). They are the project's memory across sessions and compactions. Treat them as a first-class deliverable of every task, not an afterthought.

**Before starting a task:** read `STATUS.md` and `DO_NEXT.md` to load the current state, the active branch, and what's next. If they disagree with `git status`/the actual branch, fix them first — a stale continuity file is a bug.

**After finishing a task (in the same commit as the code):** update the continuity files together with the code and tests. Never commit continuity files separately from the work they describe.
- `STATUS.md`: snapshot of the merged state.
- `DO_NEXT.md`: what is next now that this work is done.
- `BUGS.md`: strike fixed bugs and keep the header counts exact.
- `WHAT_WE_DID.md`: add or extend the entry for this change.

**Write them in the past tense, describing the end state — not a diary.** The continuity files, PR descriptions, commit messages, and `WHAT_WE_DID.md` entries must read correctly *at the moment the branch merges*. Describe what the branch *is* and what it *did to the codebase as a whole* — the merged result — not the blow-by-blow history of how you got there within the branch. Never write "first I tried X, then it failed, so I switched to Y"; write "Y does Z." A reader six months later sees only the merged diff and the prose next to it; both must be timeless and accurate then. The same rule governs the `BUGS.md` one-liners: state the defect and the fix as facts, not as a session narrative.

**Update continuity docs as the final step of a task, after the code and tests are green, and always in past tense.** Their job is to describe the merged state, so writing them before the implementation is finished almost always leaves stale claims. Treat them as a post-implementation review of what actually shipped. When the PR merges, the continuity files must already say what the codebase *is*, not what it *will be* or what you *plan* to do.

**Never create a PR that contains only continuity-file edits.** A standalone continuity PR creates a period where `STATUS.md`/`DO_NEXT.md` describe a state the code has not yet reached, and it forces readers to cross-reference a future PR to understand the current branch. The only allowed exception is a deliberate, user-requested rotation pass whose entire purpose is to reconcile stale continuity files before the next task begins. Even then, that pass must be rare and explicitly described as such. If you are tempted to open a continuity-only PR, stop: instead, update the continuity files as part of the next code PR, or ask the user how to proceed.

**Keep them streamlined.** Old, merged, irrelevant detail belongs in `git log` and PR descriptions, not accreting forever in the continuity files. When a file grows stale, compress the history to a compact summary and keep the live sections (current state, next work, open bugs, durable rules) sharp and actionable.

## No stubs. No fakes. No mocks. No synthetic behavior. Ever.

This is the single most important rule. Every piece of code in this project — backends, simulators, tests, CI — must do real work or not exist. There is no middle ground.

**Stubs and fakes are bugs.** Not shortcuts. Not placeholders. Not "good enough for now." They are defects that hide real problems, create false confidence, and accumulate into architectural rot. If you are tempted to stub something out, stop and ask the user instead.

This applies to:
- **Backends**: Every Docker API method must perform real cloud operations or return `NotImplementedError` with user approval. No synthetic responses, no hardcoded values, no in-memory stand-ins for cloud state.
- **Simulators**: Every API endpoint must behave like the real cloud service. If the real API returns labels, the simulator returns labels. If the real API tracks execution state, the simulator tracks execution state.
- **Tests**: Tests run against real simulators or real backends. No mock objects, no fake HTTP responses, no simulated cloud behavior.
- **CI**: Smoke tests exercise real API flows end-to-end. If a test can't work without a feature, implement the feature — don't mock around it.

If you find yourself writing any of the following, you are writing a bug:
- `return nil, nil` or `return &SomeStruct{}` as a "temporary" response
- Reading from `Store.Containers` in a cloud backend (the cloud is the source of truth)
- Hardcoded values where cloud metadata should be queried
- `// TODO: implement` without filing a bug and telling the user
- A function that "works" by ignoring its inputs and returning a canned response
- Fallbacks that silently degrade to local/in-memory behavior

## Simulators live in the sockerless-cloud repository

The cloud simulators (AWS, Google Cloud, Microsoft Azure), their SDK/CLI/Terraform test suites, console SPAs, and vendored API specifications live in [github.com/e6qu/sockerless-cloud](https://github.com/e6qu/sockerless-cloud) — see its `AGENTS.md` for the simulator-side rules (cloud-slice principle, testing contract, fidelity discipline).

This repository consumes them as **pinned Go modules**: `tests/go.mod` pins the version via `tool` directives on `github.com/e6qu/sockerless-cloud/simulator-<cloud>`; the test harnesses, backend integration tests, and stack targets build the binaries from that pin (`make install-simulators` → `tests/.build/`), and the harness Docker images `go install` the same modules at the `SOCKERLESS_CLOUD_VERSION` build arg. When bumping the simulator version, bump every pin in the same PR: `tests/go.mod`, the Dockerfile `ARG SOCKERLESS_CLOUD_VERSION` defaults, the git context tag in `deploy/compose.build.yaml`, and the pinned ref in `.github/workflows/live-tests-*.yml`. sockerless-cloud cuts releases with exactly one `vX.Y.Z` tag per release (release-please; no per-module tags, no `latest`), so Go-module pins use the release **commit** (`go get github.com/e6qu/sockerless-cloud/simulator-<cloud>@<release-commit>` records a pseudo-version; the bootstrap `*/v0.1.0` module tags were deleted; those versions survive only in the Go module proxy cache), while checkout- and git-context pins use the `vX.Y.Z` tag directly.

The simulators remain **real implementations** — local reimplementations of cloud services, never mocks — and the two coordinates rules below continue to govern the code in *this* repository that talks to them.


### A sim test differs from a cloud test ONLY in coordinates

A backend (or an integration test) talking to a simulator must use the **same code and the same identifiers** it uses against the real cloud, differing **only in coordinates** — the endpoint URL(s) and credentials. **Never** add an `if sim` / `if target == "sim"` branch, a sim-only env var, or any sim-aware behaviour to backend or test code. Such a special case is a *fake test*: it proves the sim-special path works, not that the real client path does.

If the sim needs to be reachable somewhere a real client reaches a cloud host (e.g. a registry at `<region>-docker.pkg.dev` / `<acr>.azurecr.io`), express that as a **coordinate** the backend already honours for both cloud and sim — for the overlay registry that's the `SOCKERLESS_GCP_AR_ENDPOINT` / `SOCKERLESS_AZURE_ACR_ENDPOINT` registry-endpoint coordinate (default = the real registry; a harness sets it to the sim's `/v2/` address). The full pattern, with the faithful build→push→pull it supports, is documented in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) § "Faithful build → push → pull". This is the same rule as § "Simulators are real implementations" (no sockerless-aware sim functionality) and § "No stubs… no synthetic behavior", applied to the *backend and test* code rather than the sim internals.

**Related docs:** [ARCHITECTURE.md](ARCHITECTURE.md), [agent/README.md](agent/README.md), [backends/README.md](backends/README.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

### A simulator console UI differs from a real-cloud console ONLY in coordinates

The same rule governs the simulator **console UIs**. The distinction that makes it work: **the console's Shauth authentication layer is the *console's own*, not the *simulator's*** — it is co-served with the console today and deployable with the console against a real cloud. The **cloud data plane** — how the console reaches cloud resources and cloud credentials — is the only thing that touches the cloud, and it must run **identically against the simulator and the real cloud**, reading **only real cloud APIs**, differing **only in coordinates** (the endpoint base URL(s) it points at). There must be **no `if sim` / `if real` branch, no sim-only endpoint on the data plane, and no fallback of any kind** (a fallback hides bugs and leaves dead or functionally dead code). A console that reads a sockerless-invented endpoint the real cloud does not serve — an `/sim/v1/*` dashboard route, or an `/auth/cloud-token` credential broker baked into the sim — is coupling the data plane to the simulator; pointing that same console at the real cloud would then require a special path — forbidden.

- **Data plane — cloud resources:** the console reads the cloud's real resource APIs (e.g. Cloud Run `GET /v2/.../jobs`) at a configured base URL that is the console's own origin when embedded in the sim and the real cloud host otherwise. Never a `/sim/v1/*` endpoint.
- **Data plane — cloud credentials:** the console federates the operator's Shauth assertion into cloud credentials through the **cloud's own federation primitive** — Google Cloud Workforce Identity Federation (`POST {sts}/v1/token`), AWS `AssumeRoleWithWebIdentity`, Microsoft Entra → Azure Resource Manager token — at a configured coordinate. The workforce pool / role / federation resource is provisioned the way an administrator provisions it (Terraform / the real API), and its identifier reaches the console as a coordinate; never a sim-side auto-provisioning hook. Never a sim-specific credential broker.
- **Authentication (Shauth SSO) — the console's own layer:** the console is its own OpenID Connect relying party (server-side session, front- and back-channel logout, the `data-shauth-*` marker contract). This layer belongs to the console and is unchanged by which cloud the data plane points at; it hands the SPA the operator's assertion, which the SPA then feeds to the cloud's real federation endpoint. It is not part of the simulator and is not a data-plane coupling.

The sim implementing a real cloud auth/federation API (STS token exchange, `AssumeRoleWithWebIdentity`) is correct — that is *the cloud*, at the sim's coordinate. A sockerless endpoint the cloud never offers, on the data plane, is not.

## All synthetic behavior is a bug

Any fake, synthetic, hardcoded, or placeholder behavior in backends is a **bug**, not a feature or acceptable shortcut. No exceptions. Examples:

- Synthetic image metadata (fake Cmd, fake sizes, fake layer hashes) — bug. Fetch the real config from the registry.
- Synthetic IP addresses (172.17.0.x) that don't correspond to real ENI IPs — bug. Use the actual task IP.
- Synthetic container stats (fake CPU/memory numbers) — bug. Get real metrics from CloudWatch or the agent.
- Synthetic process lists from `docker top` — bug. Query the real container via the agent.
- Synthetic events stream (empty) — bug. Emit real events from actual state transitions.
- Synthetic disk usage numbers — bug. Calculate from real image/container/volume data.
- In-memory-only volumes when EFS is configured — bug. Wire up EFS.
- In-memory-only networks when VPC is available — bug. Create real security groups.
- Hardcoded CPU/memory (256/512) instead of honoring container resource requests — bug.
- Placeholder progress bars during image pull — bug. Report real progress or omit.

If the real implementation is not feasible today, file a bug and track it. Do not silently fall back to synthetic behavior. When you encounter synthetic behavior in the codebase, treat it as a bug to fix, not as intended behavior to preserve.

## Always fix CI failures and test failures

If CI fails or tests fail, fix the issue — even if the failure is "pre-existing" and not caused by the current change. We do not tolerate broken CI on any branch. If adding a module to lint or expanding test coverage reveals old issues, fix them in the same PR.

## Never ignore or work around a pre-commit / pre-push failure

A pre-commit or pre-push hook failure — even one that looks incidental, cosmetic, or unrelated to your change — is flagging a real problem. **Fix the underlying problem the hook points at.** Never bypass it: no `--no-verify`, no commenting the hook out, no narrowing its scope, no editing the staged set to dodge it, no force-pushing past it. If a hook "fixed" something for you (formatting, a badge update, a generated file), commit what it changed — don't discard it.

If you believe a hook is genuinely wrong or that something should be ignored, **stop and ask the user** — describe the failure and why you think it's a false positive, and let them decide. Deciding on your own to skip, suppress, or route around a hook failure is forbidden.

The **one** sanctioned `--no-verify` is the badge auto-commit: `scripts/update-readme-badges.sh`, running as the pre-push hook, auto-commits a refreshed `README.md` (and only `README.md` — a single deterministic, generated file the badge job owns) with `--no-verify`, then fails the push so you re-run it and the badge commit goes out too. That bypass is safe because the commit carries nothing the hooks could meaningfully validate. It does not license `--no-verify` anywhere else.

## Always bundle dependency updates into the open PR

When `scripts/check-latest-deps.sh` reports drift — from the pre-push hook or
the `check-deps` CI job — **upgrade the drifted modules and commit them into
the pull request already in flight**. This is settled practice here, not a
judgement call: do it as part of the work, without asking.

Never propose a separate dependency-refresh pull request, never present the
choice as a trade-off, and never defer the upgrade to a later branch. One PR
is open at a time (see below), so a standalone dependency PR cannot exist
without blocking the actual work, and the freshness check gates every push —
drift found mid-branch either lands on that branch or nothing lands at all.

Run `make upgrade-deps` in each drifted module; a module without that target
takes `GOWORK=off go get -u <module>@latest && go mod tidy`. Then re-run the
check, build and test every upgraded module, and run the SDK test suites when a
cloud SDK moved, so the upgrade is verified against real request and response
wire shapes rather than a compile. Fresh drift often appears between a local
run and CI — upgrade again and push.

## Never create more than one PR — one branch, one PR

All work goes on a single branch and a single PR — even several independent concerns or consumer issues in one session. Never open a second PR while one is open. If two ever exist, **consolidate** their work into one PR (merge the branches together); never close one to "fix the count", never open another, never game the check. **Closing a PR without merging it abandons and deletes that work for good** — it is never a way to park work for later. Enforced by `scripts/check-single-open-pr.sh` (pre-commit + the `single-open-pr` CI job).

## Never dismiss a problem as "unrelated"

Any problem you notice — failing/flaky test, build or lint warning, dropped field, wrong status code, suspicious log — gets one of two outcomes: **fix it on the spot** (strongly preferred), or, if you truly can't now, **file it in `BUGS.md`** (area, symptom, suspected cause, fix shape). Noticing it and moving on is forbidden. "Pre-existing", "not caused by my change", "not my job" are not exits.

This is the **Boy Scout rule**, and it is a hard rule: in this project there
is no safe category of "unrelated" defect. A symptom in one component can
share credentials, wire formats, state, lifecycle, deployment coordinates, or
an assumption with another component even when that relationship is not yet
visible. Agents have limited working context; treating an observation as
unrelated because its connection is not immediately remembered is therefore a
reliability failure. Investigate the relationship first, then fix the defect
where the evidence leads. A change is not complete while a noticed failure is
being merely described, excused, or left for a hypothetical later pass.

When a real external dependency prevents an immediate repair, record the
concrete evidence, owner boundary, and intended repair in `BUGS.md`, tell the
user, and resume it as soon as the dependency changes. Do not use a tracking
entry to hide an inconvenient investigation, to narrow a test, or to preserve
a broken behavior. Tests, observability, deployment configuration, UI paths,
documentation, and operational hygiene are all part of the same product and
are subject to this rule.

"Unrelated" must be *earned with evidence*, never assumed — and especially not by an agent whose context resets across compactions and sessions, so it often can't see that a failure shares a helper, wire format, or store invariant with its change. Even a genuinely orthogonal failure still gets fixed or filed. This is a vibe-coded codebase: unfixed problems compound fast and hide the next one. Proactivity is required, not optional.

## Never merge PRs

Create PRs with `gh pr create`. Never run `gh pr merge`. The user handles all merges.

## Branch hygiene

Remote repository state is authoritative. Before inspecting or editing a
repository, fetch the freshest `origin/main` and the active pull-request branch
or ref, then compare the local worktree and `HEAD` with those fetched refs.
Never assume a local branch, cached pull-request checkout, or continuity file is
current. Preserve dirty work deliberately while reconciling it with the fetched
remote; do not overwrite or discard it. Repeat this check before rebasing and
pushing so review and validation always cover the newest remote code.

Before pushing a PR branch, always rebase it on top of `origin/main`:

```
git fetch origin main
git rebase origin/main
```

After rebasing and pushing, sync local `main` with `origin/main`:

```
git checkout main
git pull origin main
```

This is an acceptance criterion for every task — a PR is not ready until the branch is rebased on `origin/main` and local `main` is in sync.

## No bug IDs in code comments

Do not reference bug IDs (e.g., `BUG-123`) in source code comments. Once a bug is fixed, the fix speaks for itself — the comment should describe *what* the code does, not *which bug prompted it*. If a bug is still open and the code is a workaround or partial fix, that belongs in `BUGS.md`, not in a code comment.

Good: `// Podman's libpod API sends "reference" instead of "fromImage"`
Bad: `// BUG-625: Podman's libpod API sends "reference" instead of "fromImage"`

Bug tracking belongs in `BUGS.md`, `STATUS.md`, and task files. Code comments describe intent and behavior.

## Use proper, fully-qualified service and feature names

Call every cloud service, product, and feature by its **real, fully-qualified name** in prose — code comments, docs, test names, and any user-facing string. Never an invented shorthand or a half-name. The feature AWS launched on 2025-11-21 is **"Amazon ECS Express Mode"**, not "ExpressGateway"; refer to it as *ECS Express Mode* (the feature/mode) — reserving *Express Gateway service* for the AWS API resource the feature exposes. The same rule governs every service: write *Amazon Elastic Container Service (ECS)*, *Google Cloud Run*, *Azure Container Apps* — expand an acronym on first use in a document, then use the short form.

The line between "name we choose" and "name the wire dictates":

- **Prose, doc titles, test/helper names, log lines, comments** — use the proper fully-qualified product/feature name. This is where the rule bites: a test named `…ExpressGateway…` for the ECS Express Mode feature is wrong; name it `…ECSExpressMode…`.
- **Real API operation names, SDK types, wire fields, CLI command names, package names** — keep verbatim, exactly as the cloud's SDK/CLI/API spells them (`CreateExpressGatewayService`, `ExpressGatewayServiceConfiguration`, `create-express-gateway-service`, the `ecs` package). These are the contract; renaming them diverges from the SDK and is a bug. A sim type that deliberately mirrors an SDK type keeps the SDK's spelling (say so in a comment).

The only exceptions are the two above (wire identifiers) and identifiers that would become absurdly long if expanded (you do not rename the `ecs` package to `elasticcontainerservice`). When in doubt, expand it.

## Assemble Docker abstractions from cloud primitives — on every backend

Sockerless backends are **providers of the Docker + Podman REST API assembled out of cloud primitives**. The north-star is **experiential parity**: a user's experience with containers, pods, networks, and volumes inside any backend must be *the same* as with regular local Docker / Podman — the cloud is the implementation, never a visible seam. Networks, multi-container pods, and volumes are *composed*, on *every* backend, FaaS included — each backend executing inside its own model (**ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, Cloud Run Functions in CRF, ACA in ACA, Azure Functions in AZF** — see "Backend ↔ host primitive must match"). There is no "this Docker concept has no cloud analog." If a cloud lacks a first-class equivalent, build one from the primitives it *does* have (Private DNS / Cloud Map / Cloud DNS for networks; multi-container Service revisions / ACA Apps / Fargate tasks, or per-member functions + DNS for pods; EFS / GCS / Azure-Files for volumes) plus the sockerless **agent** (direct or reverse). The agent fills gaps in a cloud's API surface — but it must **never break or bypass** the abstractions sockerless establishes (stateless, cloud-is-source-of-truth, Docker-API-faithful).

- **FaaS multi-container pods are our job to assemble, not to reject.** If a FaaS platform doesn't run multiple containers inside one function, compose the container-pod execution model (with its docker network) out of cloud primitives for that backend — native sidecar containers where the platform offers them, otherwise **a pod assembled from multiple functions (one per container)** wired together by cloud DNS/service-mesh and a shared workspace volume. A current fail-fast rejection (e.g. AZF) is honest *interim* behavior tracked as not-yet-assembled in [PLAN.md](PLAN.md) / [BUGS.md](BUGS.md) — never the intended end state.
- **The full pod abstraction, regardless of backend.** Whatever you get inside a real pod, the user gets — most importantly **localhost / shared-loopback networking between containers in the same pod** (a sibling reachable on `localhost:<port>`, matching Docker/Podman pod + Compose semantics), plus a shared workspace and lifecycle. Where the cloud primitive already shares a network namespace (multi-container Service revision / ACA App / Fargate task, native FaaS sidecars) this is intrinsic; where it doesn't (a pod assembled from separate functions), the sockerless **agent** assembles it — proxying `localhost:<port>` to the sibling member over the cloud network — without breaking the stateless / cloud-is-source-of-truth abstractions.
- Never describe a Docker concept as having "no cloud analog" / "no equivalent" / "impossible on this backend." A gap is *not-yet-assembled* — name the cloud primitive(s) + agent path that would compose it, and stage hard ones across PLAN.md phases.
- `docker network create` on a cloud backend records docker-shaped metadata (the `SyntheticNetworkDriver`); the real networking is provisioned by that backend's `NetworkCreate` cloud wrapper — never a local Linux kernel netns. AZF maps `NetworkCreate` to an Azure Private DNS zone exactly as ACA does. The authoritative per-cloud mapping lives in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) § Universal rules.

## Cloud backends must be stateless

Cloud backends (ECS, Lambda, Cloud Run, Cloud Run Functions, ACA, Azure Functions) maintain **zero local state** for containers, pods, networks, or volumes. The cloud provider is the single source of truth.

- No `Store.Containers` writes. No `Store.ContainerNames` as authority. No `Store.WaitChs` as primary mechanism.
- `docker ps` queries the cloud API. `docker inspect` queries the cloud API. `docker wait` polls the cloud API.
- Container metadata lives in cloud resource tags, not in local maps.
- The only acceptable local state is `PendingCreates` — a transient map for containers between `docker create` and `docker start`, before any cloud resource exists.
- If you restart the backend process, `docker ps` must return all running containers from the cloud. No recovery needed. No registry file needed.

No exceptions. No fallbacks. No "keep Store as backup." If making an operation stateless is hard, ask the user for help — do not silently add local state as a shortcut.

## Cloud backends must not use core engine methods directly

Cloud backends (all except Docker passthrough) must **never call `BaseServer` container lifecycle methods** (`BaseServer.ContainerStart`, `BaseServer.ContainerStop`, `Store.StopContainer`, `Store.ForceStopContainer`, `Store.RevertToCreated`, etc.). These methods operate on local in-memory state, which violates the stateless requirement.

Instead, cloud backends must:
- Call their cloud provider's API directly (ECS `RunTask`/`StopTask`, Lambda `Invoke`/`DeleteFunction`, etc.)
- Let the cloud API be the action and the source of truth
- Use `CloudStateProvider` to query current state when needed
- Implement every `api.Backend` method explicitly — no generated delegates

The only exception: the Docker passthrough backend, which delegates everything to the local Docker daemon via the Docker SDK.

**Enforcement**:
- **Compiler**: `var _ api.Backend = (*Server)(nil)` in every backend — missing methods cause build failure.
- **CI lint**: `scripts/check-cloud-backend-isolation.sh` verifies no cloud backend uses `Store.ResolveContainerID`, `Store.Containers`, `BaseServer` lifecycle methods, `SpawnAutoAgent`, or `StopAutoAgent`. Runs in pre-commit and CI.
- **No generated delegates**: `backend_delegates_gen.go` files are forbidden. Every method must be explicitly implemented in `backend_delegates.go` with proper container resolution via `ResolveContainerAuto`.

**Why**: Calling core engine methods creates hidden local state, breaks the stateless invariant, causes divergence between what the backend thinks and what the cloud knows, and makes backend restart lose track of containers.

## Auto-agent is for Docker passthrough only

`SpawnAutoAgent` and `StopAutoAgent` exist in `core/auto_agent.go` for the Docker passthrough backend. Cloud backends must **never** use auto-agent. It spawns local processes and reads `Store.Containers` — both stateless violations.

Cloud backends that need exec, archive, or attach must use a cloud-deployed agent (forward or reverse mode). If no agent is connected, return `NotImplementedError` — do not fall back to auto-agent.

## No fallbacks. No degraded modes. No "graceful" alternatives.

If a dependency is required, it is required. If Docker is needed, Docker must be present — do not silently fall back to a weaker execution mode. If a cloud API is the source of truth, do not fall back to local state when the API is slow or unavailable.

Fallbacks create two code paths. Two code paths means two sets of bugs, two behaviors to test, two mental models. The "fallback" path is always the one that rots first because it's exercised least.

If you think a fallback is needed, **ask the user**. Never add one silently. The answer will usually be: make the dependency explicit and fail clearly when it's the missing one.

## No skip-if-absent tests

Never write tests that `t.Skip` when an external tool, binary, or CLI is "not installed" (`exec.LookPath("cbt")`, `t.Skip` when Docker is missing, `t.Skip` when `gcloud` isn't on PATH, etc.). Skip-if-absent tests are deceptive: they report green in environments that lack the dependency (CI images without the tool, a fresh checkout, a contributor's laptop) while exercising nothing. They become dead code — silently skipped on every run, forgotten, and the surface they claim to cover rots unseen. A green checkmark that proves nothing is worse than a red checkmark that demands the dependency be installed.

The dependency is either required or it isn't:

- **Required** — install it in `TestMain` (build it from source via `go install pkg@version` into the test's tmp dir, `docker pull` the image, `go build` the helper binary — the project already does each of these for other tests) so the test runs unconditionally every time. If the install itself fails, `log.Fatalf` in `TestMain` — a clear, actionable failure, not a silent skip.
- **Not required** — delete the test. Don't keep a conditional that never fires.

The one narrow exception is a capability the *host kernel* cannot provide (e.g. Linux-only `CAP_NET_ADMIN` network-namespace tests on macOS/Darwin): there is no way to install a kernel capability into a foreign OS, so a `runtime.GOOS`-gated skip is honest (the test cannot run on this host, full stop) — not "we couldn't find the tool, oh well." That is fundamentally different from "cbt isn't on PATH today." Tool-absent skips are always wrong; kernel-capability skips are the only sanctioned form, and only because there is no alternative.

If you find an existing skip-if-absent test, treat it as a bug: either install the dependency in `TestMain` (preferred) or delete the test.

## No silent deferrals

When given a task, implement it fully. Do not silently skip, defer, or stub out parts of the work. If something seems too hard, ambiguous, or out of scope, ask the user — do not decide on your own to drop it. Returning `NotImplementedError` or leaving a TODO without explicit user approval is not acceptable.

Specifically:
- If a method or feature is requested, implement it for all relevant backends/clouds, not just some
- If you encounter a difficulty that tempts you to defer, ask a follow-up question instead
- "Best effort" does not mean "skip if inconvenient" — it means handle errors gracefully while still performing the operation
- Every cloud backend in a cloud family (container + FaaS) must have parity on cloud-specific operations unless the user explicitly says otherwise
