# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current Branch

Branch: `feat/sim-conformance-stage2-6` (PR #539).

This is the single working branch for the simulator conformance + hardening
continuation. Keep one PR open. See [PLAN.md](PLAN.md) § Current Work for the
stage structure and [BUGS.md](BUGS.md) (1640-1646) for per-bug detail.

## Last Completed

Simulator conformance + hardening, Stages 2 G4 through 6 (all complete on this
branch; Stage 1 and Stage 2 G1-G3 already merged in #537/#538):

- Stage 2 G4 — GCP missing ops (BUG-1640): GCS bucket/object PATCH, Spanner
  Instances.Patch, KMS CreateCryptoKeyVersion/UpdateCryptoKeyVersion/:restore,
  CloudFunctions v2 UpdateFunction/:generateUploadUrl, CloudBuild ListBuilds,
  Bigtable :modifyColumnFamilies + instance/cluster update, memorystore/Cloud
  SQL updateMask merge. Stage 2 complete.
- Stage 3 — Azure conformance (BUG-1641 round-trip drift, BUG-1642 missing
  ops/error fidelity/pagination). Stage 3 complete.
- Stage 4 — Go type hardening: `unconvert` + `wastedassign` linters added to
  `.golangci.yml`; one typed enum per sim (AWS `ECSTaskStatus`, GCP
  `ComputeInstanceStatus`, Azure `ACIContainerState`), wire bytes unchanged.
  Caught and fixed BUG-1643 (a Stage-2-G4 GCS metadata-PATCH regression that
  bypassed the persistence helper).
- Stage 5 — simulator UI hardening (BUG-1645): TS types aligned to the Go
  dashboard wire shapes; stringly enums narrowed to the values each server
  actually emits.
- Stage 6 — CI gap fixed (BUG-1644): a `unit-test` Makefile target plus a
  "Run module unit tests" step in the `sim` CI jobs, so in-module guard/unit
  tests now run in CI (the gap that let BUG-1643 ship green). Coverage-matrix
  gate verified green.
- Plus BUG-1646 (bleephub gh-CLI sub-issue GraphQL drift — the newer `gh issue
  view` selects fields the Issue type lacked; added them returning null/empty,
  not faked) and an azure tf-test timeout flake fix.

## Next

1. Review and merge PR #539 (user merges). Then sync local `main`.
2. Documented follow-ups (deferred, tracked in [BUGS.md](BUGS.md) / noted in the
   conformance work):
   - GCP cloudbuild/dataflow name-collision 409 (server-assigned ids — a
     different create contract).
   - GCP synthetic compute operation store (so a bogus operation name can 404
     instead of reporting DONE).
   - Azure long-tail list `nextLink` for the remaining small fixed collections
     (EventHub/EventGrid/LogicApps/storage-ARM/RG).
   - Surface-table regeneration (`specs/SIM_SURFACE_TABLES/`) — the seed script
     over-generates; revisit the generator before any bulk regen.
3. Re-audit per BUG-1104 cadence after this stage, and re-check GitHub issues.

## Handoff Protocol

Before starting work:

1. Read [STATUS.md](STATUS.md) and this file.
2. Confirm the active branch is `feat/sim-conformance-stage2-6`.
3. Check `git status --short --branch`.
4. If the previous session stopped mid-stage, inspect the modified files before
   editing anything.

After finishing a chunk of work:

1. Run the narrowest meaningful tests for the touched area. Simulator SDK probes:

```bash
cd simulators/<cloud>/sdk-tests && GOWORK=off CGO_ENABLED=0 go test -tags noui -run '<pat>' -timeout 15m .
```

2. Run the sim-module unit tests (the Stage 6 gap) and golangci-lint:

```bash
cd simulators/<cloud> && make unit-test
```

3. Update [STATUS.md](STATUS.md), this file, and add a short
   [WHAT_WE_DID.md](WHAT_WE_DID.md) entry for meaningful completed chunks.
4. File any new defect in [BUGS.md](BUGS.md) before fixing it.
5. Commit code, tests, and continuity docs together.

## Branch And PR Hygiene

- Keep one PR open for this branch.
- Before pushing the PR branch:

```bash
git fetch origin main
git rebase origin/main
```

- After pushing the branch and opening/updating the PR, return local `main` to
  the remote state after the user merges:

```bash
git checkout main
git pull origin main
```

Do not merge the PR from the agent session; the user handles merges.

## Rules That Matter Most

- No stubs, fakes, mocks, synthetic responses, discarded uploads, or silent
  fallback to memory when durable storage was requested.
- Bleephub should match real GitHub/GHES behavior for every API it exposes.
- Use official GitHub REST/OpenAPI, GraphQL, Actions cache/artifact, official
  runner behavior, and Git smart-HTTP behavior as references.
- For object storage, use a real S3-compatible client against S3/MinIO-shaped
  APIs. Do not create an in-memory object-store stand-in.
