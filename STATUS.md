# Sockerless — Status

**102 phases closed. 770 bugs tracked — 769 fixed, 0 open, 1 false positive (BUG-747 audit umbrella). Branch `main`.**

See [PLAN.md](PLAN.md) (roadmap), [BUGS.md](BUGS.md) (bug log), [WHAT_WE_DID.md](WHAT_WE_DID.md) (narrative), [specs/](specs/) (architecture).

## Recent merges

| PR | Phases | Landed |
|---|---|---|
| #115 | 96 / 98 / 98b / 99 / 100 / 101 / 102 + 13-bug audit sweep (BUG-756–769) | 2026-04-24 |
| #114 | 91 (ECS EFS volumes) + BUG-735/736/737 | 2026-04-22 |
| #113 | 87 / 88 (CR Services, ACA Apps) + 89 (stateless audit) + 90 (no-fakes sweep) | 2026-04-21 |
| #112 | 86 (sim parity + Lambda agent-as-handler + live-AWS ECS validation) | 2026-04-20 |

Per-phase detail in [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Pending

- **Live-cloud runbooks**: GCP (Phase 87) + Azure (Phase 88) + Lambda track. Code closed; need scripted equivalents of `scripts/phase86/*.sh`.
- **BUG-721**: SSM `acknowledge` format still wrong for live AWS agent; backend dedupes retransmitted frames as a workaround. Needs live-AWS testing to fix for real.

## Test counts (as of PR #115)

| Category | Count |
|---|---|
| Core unit | 310 |
| Cloud SDK/CLI | AWS 68, GCP 64, Azure 57 |
| Sim-backend integration | 77 |
| GitHub E2E | 186 |
| GitLab E2E | 132 |
| Terraform | 75 |
| UI/Admin/bleephub | 512 |
| Lint (18 modules) | 0 |

## ECS live testing

6 rounds against `eu-west-1`. Round 6: Docker CLI pass, Podman pull+pods pass (container ops blocked by response format), Advanced 3/4. See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md).
