# Sockerless — Status

**89 phases (757 tasks). 737 bugs tracked — 734 fixed, 3 open (BUG-735/736/737), 1 false positive. Branch `post-phase86-continuation`.**

See [PLAN.md](PLAN.md) for the roadmap, [BUGS.md](BUGS.md) for the bug log (+ open-bug descriptions), [WHAT_WE_DID.md](WHAT_WE_DID.md) for the narrative, [specs/](specs/) for architecture specs.

## Phase roll-up

| Phase | Scope | Status |
|---|---|---|
| 86 | Simulator parity (AWS + GCP + Azure) + Lambda agent-as-handler | Closed 2026-04-20 (PR #112). Phase C live-AWS validated. |
| 87 | Cloud Run Jobs → Services (internal ingress + VPC connector) | Closed in code 2026-04-21. Live-GCP pending. |
| 88 | ACA Jobs → Apps (internal ingress) | Closed in code 2026-04-21. Live-Azure pending. |
| 89 | Stateless-backend audit — cloud resource mapping, `resolve*State`, cloud-derived `ListImages` / `ListPods`, `resolveNetworkState` | Closed 2026-04-21. |
| 90 | No-fakes/no-fallbacks audit — workarounds, placeholders, silent substitutions all elevated to bugs | In progress 2026-04-21. BUG-729/730/731/732/733/734 fixed; BUG-735/736/737 open. |
| 91-94 | Real per-cloud volume provisioning (EFS / Filestore-or-GCS / Azure Files), simulator slices + backend wiring | Queued. Designs + per-backend actions in `specs/CLOUD_RESOURCE_MAPPING.md`. |

Detail per phase in [WHAT_WE_DID.md](WHAT_WE_DID.md). Open work items queued in [DO_NEXT.md](DO_NEXT.md).

## Test counts

| Category | Count |
|---|---|
| Core unit | 310 |
| Cloud SDK/CLI | AWS 68, GCP 64, Azure 57 |
| Sim-backend integration | 75 |
| GitHub E2E | 186 |
| GitLab E2E | 132 |
| Terraform | 75 |
| UI/Admin/bleephub | 512 |
| Lint (18 modules) | 0 issues |

## ECS live testing

6 rounds against real AWS ECS Fargate (`eu-west-1`). Round 6: Docker CLI all pass, Podman pull+pods pass (container ops blocked by response format), Advanced 3/4. See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md). Phase 87/88 live-cloud validation runbooks still to be written (GCP/Azure equivalents of `scripts/phase86/*.sh`).
