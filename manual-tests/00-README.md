# Manual tests

Per-cloud manual test sweeps against live infrastructure. Each runbook walks the docker / podman CLI surface end-to-end against a real cloud account, captures pass/fail per row, and files any divergence as a bug. Used to validate live-cloud parity beyond what the simulators cover.

## When to run

- Before tagging a release that crosses a cloud-API surface change.
- After landing a backend that's been worked-on with sim-only tests.
- Periodically as a regression sweep when the live-AWS / live-GCP / live-Azure tracks have gone untouched for more than a few weeks.

Sim-only changes don't need a manual test pass — the SDK + CLI + terraform tests under `simulators/<cloud>/{sdk-tests,cli-tests,terraform-tests}/` cover wire-format parity, and the per-commit sim parity rule (PLAN.md principle #10) keeps drift out.

## Structure

| File | Scope |
|---|---|
| [01-infrastructure.md](01-infrastructure.md) | Per-cloud terraform live envs — apply, env-var rendering, teardown |
| [02-aws-runbook.md](02-aws-runbook.md) | ECS Fargate + Lambda — Tracks A–J |
| [03-gcp-runbook.md](03-gcp-runbook.md) | Cloud Run Jobs/Services + Cloud Run Functions — placeholder, queued |
| [04-azure-runbook.md](04-azure-runbook.md) | ACA Jobs/Apps + Azure Functions — placeholder, queued |

## Workflow

1. Pick the runbook for the backend under test.
2. Provision live infra per [01-infrastructure.md](01-infrastructure.md) — destroy is self-sufficient (the `null_resource sockerless_runtime_sweep` per backend module ensures `terragrunt destroy` succeeds without manual cleanup).
3. Walk every row in the track table. Record pass / fail / NotImpl in the test row's status column for that round.
4. Each fail or unexpected NotImpl → file in [BUGS.md](../BUGS.md) before fixing, fix in-phase per the no-defer rule (PLAN.md principle #9), then re-test.
5. Tear down live infra (`terragrunt destroy` per environment).

## Cross-links

- Roadmap: [PLAN.md](../PLAN.md)
- Status: [STATUS.md](../STATUS.md)
- Resume pointer: [DO_NEXT.md](../DO_NEXT.md)
- Bug log: [BUGS.md](../BUGS.md)
- Architecture: [specs/SOCKERLESS_SPEC.md](../specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](../specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](../specs/BACKEND_STATE.md)
- Sim parity matrix: [specs/SIM_PARITY_MATRIX.md](../specs/SIM_PARITY_MATRIX.md)
