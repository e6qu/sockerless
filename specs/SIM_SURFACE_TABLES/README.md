# Sim surface tables

Per-service canonical-operation enumerations for every sim surface that has a closed, doc-listed operation table. Each row is the answer to: **what's the complete set of operations real consumers can call against this surface, and which are implemented + tested in the sim?**

Why these tables exist:

When a community-filed issue says *"the sim is missing operation X on surface Y,"* the fix that lands the named operation often misses the *rest of the table* — its siblings, its DELETE counterpart, its symmetric query-string variant. Issue #196 → #201 is the canonical example: PR #200 closed the object-level subresources the user named (`?uploads`, `?tagging`, `x-amz-copy-source`); the bucket-level PUT subresources (`?versioning`, `?lifecycle`, `?cors`, `?policy`, `?encryption`, …) all kept returning 409 because they weren't in the user's issue text. The user noticed, filed #201, and the same shape reopened — with extra round trips for everyone.

The cure is to never claim *"surface fixed"* on a closed-enumeration surface until every row in the canonical table is either implemented or has a wire-visible 501 NotImplemented gap.

## Schema

Each table is one MD file per surface. Columns:

| Operation | Verb + path | sim handler | sdk-test | tf-test | notes |
|---|---|---|---|---|---|

- **Operation** — canonical name from the cloud provider's REST docs (e.g. `PutBucketVersioning`).
- **Verb + path** — `PUT /{bucket}?versioning`, `POST /v1/projects/{p}/subscriptions/{s}:detach`, etc.
- **sim handler** — `✓ <file>:<line>` when implemented, `✗` when missing, `501` when stubbed.
- **sdk-test** — `✓ <file>::<func>` when covered by a real-SDK test, `✗` when missing.
- **tf-test** — `✓ <main.tf-resource>` when the matching terraform-provider resource is exercised in `simulators/<cloud>/terraform-tests/`, `n/a` if no provider resource exists, `✗` otherwise.
- **notes** — links to BUGs, deferred sub-tasks, real-cloud quirks worth remembering.

## Single rule

**Before claiming a surface "fixed" in a PR, every row gets reviewed.** Rows not covered in this PR either become explicit deferred sub-tasks in PLAN.md (carrying the BUG number) or get a 501-with-rationale stub. *No row may silently stay `✗`.*

The companion skill `.claude/skills/surface-table-completeness/SKILL.md` runs the per-PR enforcement.

## Index

- [aws-s3-bucket-subresources.md](aws-s3-bucket-subresources.md) — bucket-level subresources (`?versioning`, `?lifecycle`, `?cors`, …)
- [azure-kv-data-plane.md](azure-kv-data-plane.md) — Azure Key Vault data plane (secrets / keys / certificates)

New tables land here when a community-filed issue surfaces a closed enumeration we hadn't tabled yet. **Don't proactively populate every cloud surface in one go** — the table earns its keep by being the answer to a specific reopen / scope-miss; pre-populating becomes maintenance debt the moment a cloud provider adds a new op.
