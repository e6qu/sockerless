# Agent Guidelines

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

The cloud simulators (`simulators/aws/`, `simulators/gcp/`, `simulators/azure/`) are **local reimplementations** of cloud provider services, not mocks, stubs, or fakes. They execute real logic:

- Jobs run. Functions execute. Timeouts fire. Logs are produced.
- Execution behavior is driven by the same cloud-native configuration that the real services honor — `replicaTimeout` for Azure ACA, task template `timeout` for GCP Cloud Run, `StopTask` for AWS ECS.
- There are no synthetic timers, hardcoded delays, or fake completion signals. If a cloud service doesn't have a native timeout mechanism (e.g., ECS tasks), neither does the simulator.
- Log entries are written to the same tables and log groups as the real services, queryable through the same APIs (KQL, Cloud Logging filters, CloudWatch).
- Every field the real API returns, the simulator returns. If a backend's CloudState expects `latestCreatedExecution` on a Cloud Run Job, the simulator must populate it.

When modifying simulators, always ask: "How does the real cloud service behave?" and implement that. Do not add simulator-specific environment variables, synthetic shortcuts, or approximate behaviors. Use the cloud's own configuration knobs.

The simulators run locally on a single machine today. The architecture is designed to eventually distribute execution across multiple machines, with the same API surface.

**Related docs:** [ARCHITECTURE.md](ARCHITECTURE.md), [agent/README.md](agent/README.md), [backends/README.md](backends/README.md)

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
