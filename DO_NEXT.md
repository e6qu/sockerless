# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

Phase 179 in flight on `phase-179-community-issues`. Two reopens (#209 / #210 — postmortems in BUG-1174 / 1175 / 1176) + three new community-filed issues (#213 / #214 / #215). 7 BUGs filed: 1174..1180.

## Stage plan

1. **Stage A — shared scaffolding.**
   - GCP per-resource IAM-verb dispatcher (AIP-141: `getIamPolicy` / `setIamPolicy` / `testIamPermissions`). Single shared helper registered on every IAM-bearing resource type.
   - Azure listKeys helper: `base64(sha256(resourceID|key-kind))` — 44-char deterministic, mirrors real-Azure shape.
2. **Stage B — #209 fixes.** Memorystore `:upgrade` lookup key fix + Pub/Sub topics + subscriptions `:getIamPolicy` / `:testIamPermissions` via Stage A dispatcher.
3. **Stage C — #210.** Replace Azure Redis listKeys placeholder with Stage A helper; sweep sibling services (Service Bus / EventHub / Cosmos DB listKeys) for the same placeholder.
4. **Stage D — #213.** Azure Resources Tags API — `PATCH .../Microsoft.Resources/tags/default` + per-resource `PATCH` for tags-only updates.
5. **Stage E — #214.** Service Bus `authorizationRules` family at namespace + queue + topic level. Auto-provision `RootManageSharedAccessKey` on namespace PUT.
6. **Stage F — #215.** AWS IAM `CreatePolicy` + `CreateInstanceProfile` lifecycle + API Gateway v1 method/integration response handlers.
7. **Stage G — continuity-doc reset** (final commit).

## Standing invariants (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule per phase; never more than 1 PR open.
- File BUGs in BUGS.md *before* any fix attempt.
- No fakes / no fallbacks / no silent shims.
- Every reopen carries a postmortem trail (`.claude/skills/reopen-postmortem/SKILL.md`).
- Every closed-enumeration surface has a `specs/SIM_SURFACE_TABLES/<name>.md` with no silent ✗ rows.
- Every List* op has a paged-iterator test (`sim-canonical-config-test` rule).
- Every stateful resource type has a state-machine assertion (`sim-state-machine-completeness`).
- `make hooks` on every fresh clone (wires `mux-overlap-scan` + gofmt + golangci-lint + …).

## Session-resume checklist

1. `git fetch origin && gh pr list --state open && git status`.
2. If a phase PR is open: `gh pr checks <N>`; report state.
3. If merged: sync `main`, delete merged branch, prune remotes, refresh continuity docs to idle.
4. If fresh issues filed: `gh issue list --state open --limit 30`; file each as a BUG in BUGS.md before any fix attempt.
5. Read `.claude/skills/avoid-vibe-slop/SKILL.md` before any code change.

## Reference for next reopen / new issue

If a community-filed issue surfaces against a closed enumeration (subresources, ops on a single service, paged List, state-bearing resource), the routine is:

1. Identify the surface table at `specs/SIM_SURFACE_TABLES/<surface>.md`. If none exists, create one before any fix.
2. File a BUG in BUGS.md citing which row(s) the issue covers + which siblings should be checked.
3. Fix the named row + every reasonable sibling (`surface-table-completeness` rule).
4. SDK test uses the canonical client (no raw `net/http` where an SDK exists; for List* use a Pager).
5. For reopens: BUG entry MUST include the three postmortem fields (what test passed but should have failed / what SDK code path was missed / what new canonical-client test catches the regression).
