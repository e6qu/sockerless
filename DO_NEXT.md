# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

Main is idle after the GCS metadata validation and persistence-guard fixes for issues #239, #240, and #241. GCS metadata writes now reject invalid `customTime` and overlong `contentLanguage` values, redundant metadata cloning was removed, and a source-level guard prevents future direct GCS object-store writes outside `persistGCSObject`.

## Stage plan

Current phase: none. Start the next pass by syncing `main`, listing open GitHub issues, filing each real issue in `BUGS.md`, then creating a fresh branch from `origin/main`.

Issue #239 finding: PR #238 made GCS object metadata durable and observable but did not validate accepted metadata fields. The fix implements validation where the public docs are explicit: `customTime` must parse as RFC 3339 and `contentLanguage` must be at most 100 characters. Invalid metadata now returns `400 INVALID_ARGUMENT` across multipart upload, resumable upload init/finalization, compose, `copyTo`, and `rewriteTo`.

Issue #240 finding: `gcsObjectResource.applyTo` and `persistGCSObject` both cloned custom metadata in the normal write flow. The fix marks metadata that was already cloned from the request resource and leaves `persistGCSObject` as the store-boundary clone for any uncloned map, removing the redundant clone without weakening isolation.

Issue #241 finding: PR #238 centralized GCS object writes through `persistGCSObject`, but future direct `objects.Put` calls could bypass the disk-backed byte write and metadata normalization. The fix adds a source-level guard test that fails if GCS object store writes occur outside `persistGCSObject`.

Issue #236 finding: the GCS `rewriteTo` / `copyTo` endpoints were real public JSON API surfaces, and callers can supply a destination object resource body with metadata beyond `contentType`. The fix persists custom metadata plus HTTP metadata fields, applies destination-over-source precedence for supplied fields, inherits absent fields from the source object, and returns the stored fields from metadata reads and download headers.

Issue #237 finding: upload, resumable upload, compose, and copy/rewrite all performed the same object-byte write, checksum, timestamp, and store-update work independently. The fix routes those paths through one persistence helper so future object metadata changes update one real write path.

Issue #232 finding: Azure Blob Copy Blob is a public data-plane `PUT` selected by `x-ms-copy-source`, not a multipart-copy detail. The fix branches before Put Blob, resolves host-style and path-style source URLs with escaped names, copies the real stored source bytes, returns Azure copy ID/status headers, and preserves source metadata unless destination metadata is supplied.

Issue #233 finding: GCS object copy is a public JSON API surface. The fix implements canonical `rewriteTo` and legacy `copyTo` routes in the existing object POST handler, backed by real object bytes. `rewriteTo` completes synchronously with `done: true` for same-simulator copies and returns SDK-compatible string byte counts.

Issue #234 finding: GCS object listing is documented as lexicographic by object name. The fix sorts `items[]` after filtering and also sorts delimiter-produced `prefixes[]` for stable directory-style listings.

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
3. If a PR merged: sync `main`, delete merged branch, prune remotes, refresh continuity docs to idle.
4. If fresh issues filed: `gh issue list --state open --limit 30`; file each as a BUG in BUGS.md before any fix attempt.
5. Read `.claude/skills/avoid-vibe-slop/SKILL.md` before any code change.

## Reference for next reopen / new issue

If a community-filed issue surfaces against a closed enumeration (subresources, ops on a single service, paged List, state-bearing resource), the routine is:

1. Identify the surface table at `specs/SIM_SURFACE_TABLES/<surface>.md`. If none exists, create one before any fix.
2. File a BUG in BUGS.md citing which row(s) the issue covers + which siblings should be checked.
3. Fix the named row + every reasonable sibling (`surface-table-completeness` rule).
4. SDK test uses the canonical client (no raw `net/http` where an SDK exists; for List* use a Pager).
5. For reopens: BUG entry MUST include the three postmortem fields (what test passed but should have failed / what SDK code path was missed / what new canonical-client test catches the regression).
