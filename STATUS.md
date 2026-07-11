# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/bleephub-codeql-variant-query-pack-objects` |
| Branch purpose | Follow-up Bleephub storage hardening after merged #786. The branch moved CodeQL variant-analysis query-pack tarballs out of SQLite metadata and into the configured S3-compatible object byte store, extended persisted startup/local-development documentation to name those query packs as required object-backed service bytes, and made controller-repository deletion fail loudly before metadata deletion when required query-pack object cleanup fails. |
| Current fixes | BUG-2489 was fixed on this branch. CodeQL variant-analysis rows now persist controller, actor, language, target, status, query-pack size, and object-key metadata while uploaded query-pack tarballs live in object storage whenever object storage is configured. Public query-pack downloads read the object store, persistent stores fail loudly without `BLEEPHUB_OBJECT_S3_BUCKET`, and controller-repository deletion purges query-pack objects before deleting repository metadata. |
| Last merged (#786) | **Store Bleephub service bytes in objects and harden public ingestion**. It moved release assets, GitHub Packages file bytes, GitHub Container Registry blobs, GitHub CodeQL database archives, and artifact attestation Sigstore bundles to object-backed storage; made relevant public byte routes read object storage; hardened repository deletion cleanup; moved public/official-client setup and security alert ingestion away from operator-only routes; and fixed incidental dependency and simulator test issues. |
| Last merged (#785) | **Clean Bleephub Codespaces, require Actions object storage, and harden AWS SDK CI**. It made repository deletion clean real Codespace runtime/workspace state before deleting repository records, hardened the AWS simulator SDK CI shard against hosted-runner disk exhaustion, and made persisted Bleephub require object-backed GitHub Actions artifacts, dependency caches, and runner logs. |
| Last merged (#784) | **Purge Bleephub repository durable state**. It made repository rename, transfer, deletion, and deployment deletion keep persisted repository-owned state coherent so old repository names and IDs do not leave durable issue, pull request, notification, Projects v2, Actions allowlist, code-security, deployment, environment, Pages deployment, team access, artifact metadata, source import, dependency graph, SBOM export, enterprise Dependabot access, label, milestone, or reaction state that can attach to later reuse. |
| Last merged (#783) | **Harden Bleephub GitHub service, UI, and runtime fidelity**. It continued the #781/#782 Bleephub fidelity arc across public GitHub REST/GraphQL/UI/runner behavior, persisted state, checked entropy, Actions runtime data, Docker-backed official-client harnesses, and incidental AWS simulator CI hardening. |
| Last merged (#782) | **Persist Bleephub repository metadata and permissions from real state**. It wired repository license metadata, feature flags, merge settings, Pages capability, pushed/archive timestamps, template provenance, and permissions to real persisted state, and rebalanced AWS Command Line Interface simulator shards without changing required check names. |
| Older context | #781 moved Actions artifacts/caches/logs to object storage and advanced GitHub Apps, Actions, storage, and repository fidelity; #778 closed the actionable open GitHub issues other than upstream-blocked AzureAD; #774 redesigned the Bleephub UI into a functional GitHub clone; #773 added fuzz/load/concurrency coverage; and #665-#700 built the simulator conformance-gate and service-ratchet foundation. Older detail lives in PR descriptions and `git log`. |
| Open GitHub issues | #394 azuread Terraform Graph override - upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header: 2489 filed, 2445 fixed, 3 open, 16 false positives. Open: BUG-2441, BUG-1345, and BUG-1075. |
| Local Docker | Available through Docker CLI compatibility backed by Podman 6.0.1; `docker version`, `docker ps`, `docker run --rm alpine:3 true`, and `make bleephub-gh-docker-test` passed on this branch. |
| Live infra | None up. |

## What's Next

- Continue the active branch by replacing Bleephub fake, shallow, internal-only, or compatibility-only behavior with real GitHub-compatible REST/GraphQL/UI/runner behavior backed by SQLite metadata plus git/object storage.
- Prefer classes of fixes: public GitHub endpoints over `/internal/*`, real git/object-store state over fabricated IDs/SHAs/timestamps, fail-loud errors over fallbacks, and official client compatibility over hand-built harness behavior.
- Keep the open gaps visible: BUG-2441 current `knip`/Node deprecation warning, BUG-1075 live-cloud validation, and BUG-1345 AzureAD Terraform provider upstream endpoint support.

## Invariants

- Never auto-merge PRs; the user handles merges.
- At most one PR open at a time. Put all work in the single in-progress PR.
- Rebase PR branches on `origin/main` before pushing; sync local `main` after.
- File a concrete `BUGS.md` entry before fixing a discovered defect.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Do not bypass, remove, ignore, stash around, or unstage around pre-commit/pre-push hooks.
- External identity stays GitHub/GitHub Enterprise Server-shaped: public paths, fields, `GITHUB_*` variables, runner contract, and user-facing UI text.
- Simulators remain real local cloud application programming interface slices, one binary per cloud, with official software development kit, command-line interface, and Terraform coverage where those surfaces exist.

## Environment Notes

- Simulator ports: AWS 4566, GCP 4567, Azure 4568.
- Azure Terraform tests are Docker-only because the AzureRM provider needs the local HTTPS gateway.
- Linux network-fabric tests require `CAP_NET_ADMIN`, iproute2, and nftables; off-Linux they skip through the realexec capability gate.
- Bleephub runner topology harnesses: `make bleephub-runner-docker-test` and `make bleephub-runner-docker-test-aca`; `BLEEPHUB_TEST_FROM=N` starts at a numbered test.
