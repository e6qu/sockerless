# Agent Guidelines

> `CLAUDE.md` is a symlink to this file. Edit `AGENTS.md`.

## Continuity files — read before, update after, write timeless

The continuity files are **`STATUS.md`, `PLAN.md`, `DO_NEXT.md`, `WHAT_WE_DID.md`, `BUGS.md`** (and only these — never invent new continuity docs). They are the project's memory across sessions and compactions. Treat them as a first-class deliverable of every task, not an afterthought.

**Before starting a task:** read `STATUS.md` and `DO_NEXT.md` to load the current state, the active branch, and what's next. If they disagree with `git status`/the actual branch, fix them first — a stale continuity file is a bug.

**After finishing a task (in the same commit as the code):** update `STATUS.md` (snapshot), `DO_NEXT.md` (what's next), the `BUGS.md` ledger (file before fixing; strike when fixed; keep the header counts exact), and add/extend the relevant `WHAT_WE_DID.md` entry. Commit code, tests, and continuity files together — never separately.

**Write them in the past tense, describing the end state — not a diary.** The continuity files, PR descriptions, commit messages, and `WHAT_WE_DID.md` entries must read correctly *at the moment the branch merges*. Describe what the branch *is* and what it *did to the codebase as a whole* — the merged result — not the blow-by-blow history of how you got there within the branch. Never write "first I tried X, then it failed, so I switched to Y"; write "Y does Z." A reader six months later sees only the merged diff and the prose next to it; both must be timeless and accurate then. The same rule governs the `BUGS.md` one-liners: state the defect and the fix as facts, not as a session narrative.

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

## Simulators are real implementations

The cloud simulators (`simulators/{aws,gcp,azure}/`) are **local reimplementations** of cloud services, not mocks. They run real logic: jobs run, functions execute, timeouts fire, logs are produced — driven by the same cloud-native config the real services honor (`replicaTimeout` for ACA, task template `timeout` for Cloud Run, `StopTask` for ECS). No synthetic timers, hardcoded delays, or fake completion signals; if a cloud service has no native timeout (e.g. ECS tasks), neither does the simulator. Logs go to the same tables/log groups, queryable through the same APIs (KQL, Cloud Logging, CloudWatch). Every field the real API returns, the simulator returns — if a backend's CloudState expects `latestCreatedExecution` on a Cloud Run Job, populate it.

Always ask "How does the real cloud service behave?" and implement that — use the cloud's own configuration knobs, never simulator-specific env vars or shortcuts. The simulators run on one machine today; the architecture targets distributing execution across machines with the same API surface.

### Simulator architecture — cloud-slice principle

Three principles govern every simulator change. They are load-bearing; a PR that violates any of them is a bug.

1. **The simulator is a cloud slice.** `simulators/aws/` implements the subset of AWS's real public API surface that sockerless depends on, at cloud-API fidelity. It is *not* an emulation of a single product — there is no "Lambda simulator" or "ECS simulator" in isolation. If sockerless uses Lambda + ECS + ECR + CloudWatch + Cloud Map + EC2 + STS + IAM + S3 from AWS, the AWS simulator implements slices of all of them. Same for GCP and Azure.

2. **One simulator binary per cloud.** All AWS service slices live in `simulators/aws/` (single Go module, one `simulator-aws` binary, one shared `sim.Server` mux). Adding a new service slice = a new `registerX(srv)` call + handler file in the existing per-cloud binary. Never a new binary per product.

3. **Cloud-API fidelity.** Match the real cloud's error shapes, response headers, async operation semantics, path templates, and HTTP status codes exactly. When the cloud's contract doesn't cover something, neither does the simulator — don't invent simulator-specific env vars, synthetic shortcuts, or approximate behaviors. "How does the real cloud service behave?" is the authoritative question; the simulator answers it by implementing the same API the cloud does.

**How to add a new slice:**
1. Read the cloud's public API reference for the service (e.g. `docs.aws.amazon.com/lambda/…`).
2. Create `simulators/<cloud>/<service>.go` with handlers matching the cloud's endpoints, error codes, and response shapes.
3. In `simulators/<cloud>/main.go` or equivalent, call `register<Service>(srv)` so the new slice mounts on the shared mux.
4. Add SDK + CLI + Terraform tests per the testing contract below — the pre-commit hook enforces this.

**What "cloud-API fidelity" rules out:**
- Stdout-as-response shortcuts (where the simulator returns whatever the user-process printed instead of the real cloud's response shape).
- In-memory TODO placeholders that claim "we'll call the SDK later".
- Embedding AWS's `aws-lambda-rie` or similar third-party local emulators inside test images — that bypasses our cloud slice; the simulator IS the cloud from the container's perspective.
- Synthetic disambiguation (custom headers, custom env vars) that real cloud bootstraps wouldn't produce.
- **Any sockerless-aware or runner-aware special-casing.** The sim must be faithful to the real cloud and provide *no* special / fake functionality on top to make a sockerless backend or a GitHub/GitLab runner harness work. If a backend needs something (e.g. ACA needs an ACR-Tasks-built bootstrap-overlay image the host engine then pulls), implement the *real* cloud API faithfully and have the backend/host use it exactly as a real client would — never a sim-side hook keyed on "this is for sockerless." If it can't be done through faithful cloud APIs, find the real cloud primitive that does; don't special-case the sim.

**What it does allow:**
- Ephemeral sidecar listeners (e.g. per-Lambda-invocation listener on a free port) as long as the container-facing contract matches the cloud.
- Docker user-defined networks as the implementation mechanism behind Cloud Map / Cloud DNS / Private DNS — Docker's embedded DNS is just how the simulator realizes the cloud's DNS contract locally.

### Simulator fidelity — testing contract

Every simulator endpoint must be exercisable via all three real-world client surfaces, in the same commit that registers the endpoint:

1. **SDK** — the official cloud SDK for Go (`aws-sdk-go-v2/*`, `cloud.google.com/go/*`, `github.com/Azure/azure-sdk-for-go/*`). Tests live in `simulators/<cloud>/sdk-tests/`.
2. **CLI** — the vendor CLI (`aws`, `gcloud`, `az`) shelled out via `runCLI`. Tests in `simulators/<cloud>/cli-tests/`.
3. **Terraform** — the official provider resource that wraps the endpoint. Tests in `simulators/<cloud>/terraform-tests/` (extend `main.tf` and rely on the existing apply/destroy harness).

The pre-commit hook `scripts/check-simulator-tests.sh` blocks any commit that adds a `r.Register("OpName", …)` line without touching at least one file in the three test dirs that references the operation. Endpoints that genuinely aren't exposed via SDK/CLI/terraform (e.g. Lambda Runtime API routes that the function *container* polls, not an SDK) go on `simulators/<cloud>/tests-exempt.txt` — one operation per line.

There is no "just land it and add tests later." If you edit a simulator, the tests ship with it.

### A sim test differs from a cloud test ONLY in coordinates

A backend (or an integration test) talking to a simulator must use the **same code and the same identifiers** it uses against the real cloud, differing **only in coordinates** — the endpoint URL(s) and credentials. **Never** add an `if sim` / `if target == "sim"` branch, a sim-only env var, or any sim-aware behaviour to backend or test code. Such a special case is a *fake test*: it proves the sim-special path works, not that the real client path does.

If the sim needs to be reachable somewhere a real client reaches a cloud host (e.g. a registry at `<region>-docker.pkg.dev` / `<acr>.azurecr.io`), express that as a **coordinate** the backend already honours for both cloud and sim — for the overlay registry that's the `SOCKERLESS_GCP_AR_ENDPOINT` / `SOCKERLESS_AZURE_ACR_ENDPOINT` registry-endpoint coordinate (default = the real registry; a harness sets it to the sim's `/v2/` address). The full pattern, with the faithful build→push→pull it supports, is documented in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) § "Faithful build → push → pull". This is the same rule as § "Simulators are real implementations" (no sockerless-aware sim functionality) and § "No stubs… no synthetic behavior", applied to the *backend and test* code rather than the sim internals.

**Related docs:** [ARCHITECTURE.md](ARCHITECTURE.md), [agent/README.md](agent/README.md), [backends/README.md](backends/README.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

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

## Never create more than one PR — one branch, one PR

All work goes on a single branch and a single PR — even several independent concerns or consumer issues in one session. Never open a second PR while one is open. If two ever exist, **consolidate** their work into one PR (merge the branches together); never close one to "fix the count", never open another, never game the check. **Closing a PR without merging it abandons and deletes that work for good** — it is never a way to park work for later. Enforced by `scripts/check-single-open-pr.sh` (pre-commit + the `single-open-pr` CI job).

## Never dismiss a problem as "unrelated"

Any problem you notice — failing/flaky test, build or lint warning, dropped field, wrong status code, suspicious log — gets one of two outcomes: **fix it on the spot** (strongly preferred), or, if you truly can't now, **file it in `BUGS.md`** (area, symptom, suspected cause, fix shape). Noticing it and moving on is forbidden. "Pre-existing", "not caused by my change", "not my job" are not exits.

"Unrelated" must be *earned with evidence*, never assumed — and especially not by an agent whose context resets across compactions and sessions, so it often can't see that a failure shares a helper, wire format, or store invariant with its change. Even a genuinely orthogonal failure still gets fixed or filed. This is a vibe-coded codebase: unfixed problems compound fast and hide the next one. Proactivity is required, not optional.

## Never merge PRs

Create PRs with `gh pr create`. Never run `gh pr merge`. The user handles all merges.

## Branch hygiene

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

If you think a fallback is needed, **ask the user**. Never add one silently. The answer will usually be: make the dependency explicit and fail clearly when it's missing.

## No silent deferrals

When given a task, implement it fully. Do not silently skip, defer, or stub out parts of the work. If something seems too hard, ambiguous, or out of scope, ask the user — do not decide on your own to drop it. Returning `NotImplementedError` or leaving a TODO without explicit user approval is not acceptable.

Specifically:
- If a method or feature is requested, implement it for all relevant backends/clouds, not just some
- If you encounter a difficulty that tempts you to defer, ask a follow-up question instead
- "Best effort" does not mean "skip if inconvenient" — it means handle errors gracefully while still performing the operation
- Every cloud backend in a cloud family (container + FaaS) must have parity on cloud-specific operations unless the user explicitly says otherwise
