# Agent Guidelines

## Simulators are real implementations

The cloud simulators (`simulators/aws/`, `simulators/gcp/`, `simulators/azure/`) are **local reimplementations** of cloud provider services, not mocks, stubs, or fakes. They execute real logic:

- Jobs run. Functions execute. Timeouts fire. Logs are produced.
- Execution behavior is driven by the same cloud-native configuration that the real services honor — `replicaTimeout` for Azure ACA, task template `timeout` for GCP Cloud Run, `StopTask` for AWS ECS.
- There are no synthetic timers, hardcoded delays, or fake completion signals. If a cloud service doesn't have a native timeout mechanism (e.g., ECS tasks), neither does the simulator.
- Log entries are written to the same tables and log groups as the real services, queryable through the same APIs (KQL, Cloud Logging filters, CloudWatch).

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
- In-memory-only networks when VPC is available — bug. Create real security groups (done for ECS in BUG-584).
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

## No silent deferrals

When given a task, implement it fully. Do not silently skip, defer, or stub out parts of the work. If something seems too hard, ambiguous, or out of scope, ask the user — do not decide on your own to drop it. Returning `NotImplementedError` or leaving a TODO without explicit user approval is not acceptable.

Specifically:
- If a method or feature is requested, implement it for all relevant backends/clouds, not just some
- If you encounter a difficulty that tempts you to defer, ask a follow-up question instead
- "Best effort" does not mean "skip if inconvenient" — it means handle errors gracefully while still performing the operation
- Every cloud backend in a cloud family (container + FaaS) must have parity on cloud-specific operations unless the user explicitly says otherwise
