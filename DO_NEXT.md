# Do Next

Snapshot pointer for the next session. Updated after every task.

## Branch state

`post-phase86-continuation` — PR #113. Every outstanding bug fixed. CI re-running after BUG-727/728/724 fixes.

## Pending work

See [PLAN.md](PLAN.md) for scope. Priority-ordered:

1. **Phase 87 live-GCP runbook** — validate Cloud Run Services path end-to-end. Needs GCP project + VPC connector.
2. **Phase 88 live-Azure runbook** — validate ACA Apps path end-to-end. Needs Azure subscription + managed environment with VNet integration.
3. **Phase 86 Lambda live track** — scripted already, deferred at Phase C closure for session-budget reasons.
4. **BUG-721** — nail down the SSM ack format AWS's agent accepts so the MessageID dedupe workaround in `ssmDecoder` can be removed. Live-AWS required.
5. **Phase 68 — Multi-Tenant Backend Pools** (P68-002 → 010). Orthogonal to the cloud-backend work.
6. **Phase 78 — UI Polish**.

## Operational state

- AWS: zero residue (state buckets + DDB lock table retained as cheap reusable infra).
- Local sockerless backend: stopped.
- No credentials in environment.
