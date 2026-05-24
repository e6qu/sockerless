# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

**Phase 176 — PR #200 open, CI running, awaiting user merge.** Branch `phase-176-community-issues`, 11 commits on top of `origin/main`. PR URL: https://github.com/e6qu/sockerless/pull/200.

PR #192 (Phase 175) merged at `ca11405` closed BUG-1120..1133 + 3 GitHub issues (#189/#190/#191). The user re-tested against the merged build and reopened **#190** (path-style storage dispatch didn't work — the prior fix required ARM registration that real Azurite users don't do) and filed 7 new issues (#193–#199).

Phase 176 closed all 8 (BUG-1134..1141) + every skill-audit finding from the in-PR validation pass + the path-style dispatcher test-state contamination uncovered by running the full Azure SDK suite.

BUGS.md: **1141 filed · 1139 fixed · 2 open.** Only BUG-1075 (live-cloud, deprioritized) + BUG-1104 (audit-cadence meta) remain Open.

## What landed

- BUG-1134 — permissive path-style storage with `hasAzureStorageSignal` protocol-marker discriminator (`x-ms-version` / `x-ms-date` / `x-ms-blob-type` / `restype` / `comp` / `SharedKey`). Non-storage co-tenants (IMDS / MSI / Monitor / dataCollectionRules) keep their own routes on the shared sim port.
- BUG-1135 — KV WrapHandler issues `401 + WWW-Authenticate: Bearer authorization=..., resource="https://vault.azure.net"` on unauthenticated probes. Non-RSA key generation upgraded to real ecdsa + crypto/elliptic + rand.Read; `AQAB` placeholder dropped.
- BUG-1136 — RDS + ElastiCache populate `EngineVersion` from per-engine GA default maps when the request leaves it empty.
- BUG-1137 — Real Service Bus REST data plane with persistent message store. SendMessage 201, ReceiveAndDelete 200/204, PeekLock 201 + body + Location pointing at `/messages/{id}/{token}`, CompleteLock DELETE 204. Topic + Subscription equivalents.
- BUG-1138 — `s3_subresources.go` query-key dispatcher: full multipart cycle (Initiate / UploadPart / Complete / Abort / List), Object Tagging CRUD, CopyObject via `x-amz-copy-source`, multi-object delete.
- BUG-1139 — Real `GET /v1/operations` cross-service projection from `crOperations` store with AIP-151 filter + name-prefix support; GCS catch-all tightened to refuse unknown bucket segments.
- BUG-1140 — `:compose` POST handler concatenates source-object bytes; resumable + multipart Init parse `name` from body or query; `mediaLink` / `selfLink` hard-coded https via `gcsObjectMetadata`; resumable chunk PUT registered with `openStreamingBody` body wrap.
- BUG-1141 — `lambda_subresources.go` registers PublishVersion / ListVersions / Alias CRUD / AddPermission / GetPolicy / RemovePermission / FunctionUrlConfig CRUD under restJson1; CreateAlias + UpdateAlias validate FunctionVersion against PublishVersion history.

Plus the in-PR audit pass closed:
- KV non-RSA placeholder → real EC/oct key generation.
- SB PeekLock Location → includes `/messages/` segment; SB SendMessage drop dead `sbLockTokenGuids`.
- Storage blob.go drop dead `knownStorageAccount` helper.
- engine_version_test.go drop dead `var _ = ...AvailabilityZone{}` silencers + imports.
- S3 multipart 3 silent `io.ReadAll(_)` → IncompleteBody 400 on body-decode failure.
- Lambda subresources `json.Marshal` errors checked.
- GCS multipart metadata decode → fail-loud 400 INVALID_ARGUMENT.
- Path-style storage dispatcher contamination → `hasAzureStorageSignal` protocol discriminator.
- ServiceBus PeekLock test → parse Location via `url.Parse` + assert host/path + replay with `Host = locURL.Host`.

## Test results

All three SDK suites green locally: AWS 222s (full), Azure 23s, GCP 124s. Full Azure suite (which had been masking 10 cross-test failures pre-fix) now clean.

## Next steps

1. Watch `gh pr checks 200` for CI completion across all 22 jobs.
2. On green CI → ping user for merge.
3. After user merges:
   - Sync local main: `git checkout main && git pull origin main`.
   - Delete the merged branch locally + on origin.
   - Reset DO_NEXT / STATUS / WHAT_WE_DID to next phase if there is one; otherwise close to "idle, awaiting new community-filed issues."

## Invariants snapshot (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule per phase; never more than 1 PR open.
- File BUGs *before* fixing.
- No fakes / no fallbacks / no silent shims — fail loud or ask, never silently degrade.
- Driver pluggability preserved: one driver per dimension.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative.

## Session-resume checklist

1. `git fetch origin` + `gh pr view 200 --json mergeable,state,statusCheckRollup` to learn whether the PR is still open, merged, or closed.
2. If still open: `gh pr checks 200` to see CI state; report to the user.
3. If merged: `git checkout main && git pull origin main`; reset continuity docs to the next phase or "idle."
4. Read STATUS.md + this file + BUGS.md § Open before doing anything else.
5. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before any code change.
