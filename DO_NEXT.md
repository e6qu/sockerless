# Do Next

Snapshot pointer for the next session. Updated after every task per user directive.

## Right now (in-flight)

**Phase 86 Phase C — live-AWS session 2** on branch `post-phase86-continuation`. Plan: `~/.claude/plans/purring-sprouting-dusk.md`. AWS account 729079515331 (eu-west-1 + us-east-1).

Progress vs. plan:

| Phase | Status | Notes |
|---|---|---|
| 0 Preflight | done | Scripts fixed, state buckets bootstrapped (`sockerless-tf-state` eu-west-1, `sockerless-terraform-state` + `sockerless-terraform-locks` us-east-1), binaries built, creds verified. |
| 1 ECS infra up | done | terragrunt apply 34/34 in eu-west-1 (~2min). Outputs at `/tmp/ecs-out.json`. |
| 2 ECS smoke | partial-pass | 2.1 PASS (~33s cold), 2.2 PASS, 2.3 PASS via FQDN (short-name FAIL by design — BUG-711), 2.4 FAIL (BUG-717 SSM proto). 10 bugs filed + 5 fully fixed + 4 minimum-fixed + 1 deferred. |
| 3 Lambda infra up | pending | After ECS smoke completes. |
| 4 Lambda baseline | pending |  |
| 5 Lambda agent (CONDITIONAL) | pending | Decision point on tunnel strategy. |
| 6 E2E live tests | pending | Narrow smoke first, widen if time. |
| 7 Teardown | pending | Lambda first then ECS. Hard requirement before session ends. |
| 8 Doc updates + commit | pending | Final state save. |

## Open bugs (all from this session, fixes landed on branch)

| ID | Sev | Status |
|---|---|---|
| 708 | Low | open — workaround in place (per-prefix skip-cache memo + INF demote); proper credential plumbing for ECR pull-through cache deferred |
| 709 | High | fix landed on branch (waitForOperation polling now sleeps); cross-cloud sweep done (ECS-only — Azure SDK PollUntilDone sleeps, GCP SDK handles polling); needs unit test |
| 710 | Med | fix landed on branch (defaults all moved to :3375); cross-cloud sweep done (all 7 backends + CLI + READMEs + examples + http-trace); needs CI to confirm |
| 711 | High | minimum fix landed (DnsSearchDomains stripped); cross-cloud sweep done (ECS-only — other backends use FQDN-style DNS without explicit search domains); long-term mechanism for short-name DNS resolution still TBD |
| 712 | High | fix landed on branch (cloudNetworkCreate idempotent for ECS); cross-cloud sweep found BUG-713 in cloudrun |
| 713 | High | fix landed on branch (cloudrun cloudNetworkCreate now reuses existing zone on 409); cross-cloud sweep done (Azure naturally idempotent via PUT, Lambda/GCF/AZF have no cloud-side network create); needs unit test |
| 714 | High | fix landed on branch (ECS now registers in Cloud Map with the ENI IP after task RUNNING, not the in-memory placeholder); cross-cloud sweep found BUG-715 + BUG-716 |
| 715 | High | minimum fix landed in cloudrun (skip DNS register on placeholder IP); proper fix deferred — Cloud Run Jobs lack addressable per-execution IPs |
| 716 | High | minimum fix landed in aca (skip DNS register on placeholder IP); proper fix deferred — ACA Jobs lack addressable per-execution IPs |
| 717 | High | open — SSM Session Manager binary protocol not decoded; docker exec output garbled. Substantial implementation; deferred. ECS-only per cross-cloud sweep. |

## Live AWS state right now

- `sockerless-backend-ecs` running locally on `:3375` (PID 94554, started 2026-04-20 14:36).
- Sockerless infra up in eu-west-1: VPC `vpc-0991a9275f0aa033f`, ECS cluster `sockerless-live`, ECR `sockerless-live`, NAT gateway running (~$0.05/hr).
- No orphan Cloud Map namespaces, security groups, or running tasks (verified post-cleanup).
- Lambda infra in us-east-1 — not yet provisioned.

## After Phase C closes

Candidates for the next session (order TBD by user):

- **Phase 68** Multi-Tenant Backend Pools (P68-002 → 010). Pool registry, request router, concurrency limiter, pool lifecycle, metrics, RR scheduling, resource limits, tests, state save.
- **Phase 78** UI Polish — dark mode, design tokens, error UX, container detail modal, auto-refresh, perf audit, a11y, E2E smoke, docs.
- **BUG-708 proper fix** — wire `SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN` into `CreatePullThroughCacheRule` so docker-hub auth works.
- **BUG-711 proper fix** — pick a mechanism for short-name cross-container DNS that survives awsvpc's restrictions (DHCP options vs. resolv.conf injection vs. Service Connect).
- **Tests for in-flight Phase-C fixes** — unit tests for waitForOperation sleep, idempotent cloudNetworkCreate, port-default sweep.

## Session budget snapshot (from plan)

| Scope | Time | Cost |
|---|---|---|
| Narrow (Phases 0-4 + 6 smoke + 7) | ~1h 30min | ~$0.15 |
| Full matrix (+ 5, + 6 full) | ~3h | ~$0.50 |
