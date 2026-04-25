# Sockerless — Status

**102 phases closed. 800 bugs tracked — 798 fixed, 2 open (BUG-789/798 SSM frame parsing, BUG-795 podman-list pod members), 1 false positive. Branch `round-8-bug-sweep` open.**

See [PLAN.md](PLAN.md) (roadmap), [BUGS.md](BUGS.md) (bug log), [WHAT_WE_DID.md](WHAT_WE_DID.md) (narrative), [specs/](specs/) (architecture).

## Recent merges

| PR | Phases | Landed |
|---|---|---|
| #117 | Round-7 live-AWS sweep — 16 bugs fixed (BUG-770..785) | 2026-04-25 |
| #116 | State docs + manual-testing runbook refresh | 2026-04-25 |
| #115 | 96 / 98 / 98b / 99 / 100 / 101 / 102 + 13-bug audit sweep (BUG-756–769) | 2026-04-24 |
| #114 | 91 (ECS EFS volumes) + BUG-735/736/737 | 2026-04-22 |
| #113 | 87 / 88 (CR Services, ACA Apps) + 89 (stateless audit) + 90 (no-fakes sweep) | 2026-04-21 |
| #112 | 86 (sim parity + Lambda agent-as-handler + live-AWS ECS validation) | 2026-04-20 |

Per-phase detail in [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Round-8 (this branch — pending PR)

Two-round live-AWS sweep against `eu-west-1`. **278 tests across rounds 1+2** (142 + 136 in round-2 v2). 13 bugs filed and fixed (BUG-786, 787, 788, 790, 791, 792, 793, 794, 796, 797, 799, 800 + accepted-gaps classification for ECS commit / pause / ContainerResize / ImageSave / ImageSearch / streaming stats). 2 bugs remain open as P1-or-later (789/798 SSM frame parsing, 795 podman-list pod members).

Headline fixes:
- **Phase 89 stateless invariant restored** — registry persistence at `./sockerless-registry.json` removed (BUG-800); recovery now skips STOPPED tasks (BUG-799); 11 stale registry files swept from the tree.
- **BUG-788 registry-to-registry layer mirror** — new `core.FetchLayerBlob` + `Store.ImageManifestLayers` populated by `ImagePull` / `ImageLoad` / `ContainerCommit`; `OCIPush` preserves source compressed digests verbatim. Verified live: pulled image pushed back to ECR.
- **BUG-790 sync `docker stop`** — new `waitForTaskStopped` blocks until ECS reports STOPPED so immediate `docker rm` succeeds.
- **BUG-794 cross-network isolation** — per-network SG is the sole authority for containers with `--network X`; default SG only applies to networkless containers.
- **BUG-786 rmi alias-entry sweep** — `ImageRemove` rewrites every `Store.Images` entry whose `Value.ID` matches.
- **Spec doc** — `specs/CLOUD_RESOURCE_MAPPING.md` now reflects landed phases (91–94, 96, 102) and lists 9 maintainer-approved acceptable gaps.

## Pending

- **BUG-789/798**: SSM exec returns -1 on live AWS even with `ExecuteCommandAgent` readiness wait. Sim-backed tests pass; needs WS-frame capture against live exec session to diagnose ack-format mismatch on `ExecuteCommand` path.
- **BUG-795**: `podman ps --filter` doesn't return pod-attached containers; `podman pod inspect` does see them.
- **Live-cloud runbooks**: GCP (Phase 87) + Azure (Phase 88) + Lambda track. Code closed; need scripted equivalents of `scripts/phase86/*.sh`.
- **Phase 103** (overlay-rootfs): queued; replace `find -newer /proc/1` heuristic with overlayfs-based diff/cp/export for FaaS+CR+ACA.
- **BUG-721**: SSM `acknowledge` format still wrong for live AWS agent. Sim-side ack format is correct (BUG-729 closed it for sim); live remains broken.

## Test counts

| Category | Count |
|---|---|
| Core unit | 312 (BUG-786 + 788 fixes added Entries() / FetchLayerBlob tests; persistence tests removed for BUG-800) |
| Cloud SDK/CLI | AWS 68, GCP 64, Azure 57 |
| Sim-backend integration | 77 |
| GitHub E2E | 186 |
| GitLab E2E | 132 |
| Terraform | 75 |
| UI/Admin/bleephub | 512 |
| Lint (18 modules) | 0 |
| Round-8 live-AWS manual sweep | 278 tests, 274 pass, 4 expected fails (404-on-not-found + BUG-799 ghost which the new binary fixes) |

## ECS live testing

8 rounds against `eu-west-1`. Round 8: 142 tests (round 1) + 136 (round 2 post-fixes, including stateless-recovery I-track all-pass) + 26 final retest verifying all bug fixes end-to-end. See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md).
