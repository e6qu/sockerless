# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 165 merged 2026-05-17 (PR #165, `288b76d3` on `origin/main`) — third vibe-slop sweep + sim test-pyramid expansion + continuity-doc compression + codex review. **9 BUGs closed**; **3 Open BUGs (1040/1041/1042)** staged for Phase 166.

No phase in flight. The next session starts by picking one of the three Phase 166 candidates (or a new direction the user surfaces).

Default "never auto-merge / user merges every PR" remains in force.

## Phase 166 candidates (file before fix, then pick)

| BUG | Sev | Area | What |
|---|---|---|---|
| **1040** | P1 | Azure terraform-tests | Research how to point `azurerm` provider at the sim (azurerm needs custom-cloud metadata + OAuth endpoint overrides; `azurestack`'s simple `arm_endpoint=...` doesn't work). Then add: ACR + Container App Environment + Container App + Container App Job + Function App + Service Plan + Application Insights + user_assigned_identity + private_dns_zone + Key Vault data-plane (keys/secrets). Sim implements all the ARM endpoints; the gap is provider wiring. |
| **1041** | P2 | GCP terraform-tests | Add `google_service_account` (needs sim-side IAM-Admin endpoint OR provider workaround — terraform-provider-google's IAM resources don't honour `iam_custom_endpoint`), `google_cloudfunctions2_function` (build_config needs real GCS source archive — multi-resource orchestration), Compute instance + instance_template, Cloud Build trigger, Logging sink + metric, Pub/Sub topic + subscription (sim probably doesn't model Pub/Sub yet — separate prereq). |
| **1042** | P0 | AWS terraform-tests | Add 5 sim handler stubs surfaced + reverted during Phase 165: S3 path-style routing fix, DynamoDB DescribeTable response shape (sim sets `TableStatus=ACTIVE` but provider polls 21 times waiting), KMS `TrentService.GetKeyPolicy` action (stub returning empty policy), SecretsManager `secretsmanager.GetResourcePolicy` (stub), SSM `AmazonSSM.ListTagsForResource` (stub returning empty tag list). Then add the resources to terraform-tests/main.tf. |

Recommended order: **1042 first** (highest sev P0; most-load-bearing for the runner pod lifecycle), then **1040** (widest Azure gap), then **1041** (GCP follow-ups).

## Session-resume checklist for a fresh session

1. `git fetch origin && git checkout main && git pull --ff-only`.
2. `git log --oneline -10` to see the last merged phases.
3. Read STATUS.md (73 lines, includes invariants) + this file + BUGS.md § Open (3 entries).
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before writing any code.
5. Pick a Phase 166 candidate (or accept a new direction from the user).
6. Branch off `origin/main`. File any new BUGs **before** the first fix commit.
7. `go test ./...` in every touched module per commit. `git log --oneline -1` after each commit to confirm SHA advanced (pre-commit hooks can roll back silently).
8. State save per commit: STATUS.md bug counts + DO_NEXT.md sub-task status + WHAT_WE_DID.md narrative + MEMORY.md.
9. Push per commit; verify CI green per push.

## Resumable tracks (longer-horizon)

### Track A — Live-cloud validation (one branch per cell)

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. Teardown self-sufficient per `feedback_teardown_aggressive.md`.

### Track B — UI / TypeScript vibe-slop sweep (carried from Phase 161)

Sibling pattern check on `ui/packages/*/src/`. Open if a Go-side sweep surfaces a parallel finding worth investigating.

### Track C — Phase 91d (bookmarked indefinitely)

Real `pd-ephemeral` on cloudrun + gcf. Cloud Run's `runpb.Volume` lacks a PD field. Don't reopen until cloud capability changes.

## Invariants snapshot (full list in STATUS.md + VIBE_CODING.md)

- Never auto-merge; user merges every PR.
- Single-branch rule: all in-flight work for one phase lands on one branch; many granular commits, one PR.
- File BUGs *before* fixing.
- Verify each significant chunk; don't batch fixes.
- Components decoupled from admin / UI.
- No fakes / no fallbacks / no silent shims.
- Persistence opt-in + fail-loud on both open AND write.
- HTTP handlers dispatch through `s.self.<Method>`; never read `s.Store` directly.
- No phase / BUG-ID references in code comments or test docstrings (BUG-994 / 1014 / 1026 / 1036).
- `gh` CLI is the reference adaptor for bleephub.
- `aws --debug` + SDK serializer source are the reference for sim handler wire shapes.
- Terraform provider call sequences differ materially from raw SDK — both test layers required.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative for cloud-mapping.
