# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

PR #246 recorded the foundational simulator service audit requested after PR #245. The audit lives in `specs/SIM_FOUNDATIONAL_AUDIT.md` and covers object storage, managed data stores, DNS, queues, event routing, stream/event ingestion, VM/EC2-like compute, VPC/networking, NAT/egress, gateways, and managed load balancers across AWS/GCP/Azure.

The audit found core object storage and queue/message systems present across the three sims, with AWS S3/DynamoDB/Route 53/Cloud Map/SQS/SNS, GCP GCS/Pub/Sub/Cloud DNS, and Azure Blob/File/Queue/Table/Service Bus/Private DNS implemented. It also found real missing foundational cloud slices: AWS EventBridge, GCP Eventarc, Azure Event Grid, AWS Kinesis, Azure Event Hubs, GCP BigQuery, GCP Firestore/Datastore, Azure Cosmos DB, EC2/GCE/Azure VM lifecycle APIs, managed load balancers across all clouds, Azure public DNS, and uneven public-IP/NAT parity. BUG-1197..1207 track those gaps.

The PR also closed the two CI regressions found while landing the audit. BUG-1208 made the bleephub invalid-JWT test mutate decoded signature bytes before re-encoding, so it cannot accidentally leave the signature valid through base64url tail changes. BUG-1209 made the GCF FaaS smoke harness reuse local Docker tags across its subprocess TestMain run instead of rebuilding Alpine from Public ECR and risking registry 429s.

## Stage plan

Current phase: idle after the audit. The next implementation pass should start from the audit findings. VM/EC2-like support should expose the public EC2/GCE/Azure VM APIs while using Firecracker or another real local microVM runtime only as internal simulator machinery. Event routing is the next cross-cloud service slice after that: AWS EventBridge + GCP Eventarc + Azure Event Grid in one PR, with SDK/CLI/Terraform coverage for every added public operation.

Issue #243 finding: Azure ARM resource responses were inconsistent with the simulator's cloud-facing contract. Storage and Key Vault derived endpoint hosts from the incoming ARM request, but Service Bus, Redis, APIM, PostgreSQL Flexible Server, and Container Apps still returned production cloud suffixes. The fix derives those endpoint fields from the simulator request host while preserving Azure-shaped field names and host patterns; Service Bus listKeys connection strings were updated with the same host derivation.

Issue #244 finding: Container Apps Jobs and Apps passed `Architecture: "linux/arm64"` to Docker for every started container, including sidecars. That made the real container start fail on amd64 hosts when the local image manifest was amd64. The fix resolves each image and inspects its local manifest platform before calling `StartContainerSync`, matching the Cloud Run Services pattern.

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
