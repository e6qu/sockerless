# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next

The branch `bump-sockerless-cloud-v0.30.3-registry-fixes` pinned the
simulators at sockerless-cloud v0.30.3 — the release whose Google Artifact
Registry data plane authenticates every `/v2/` request and requires the
repository to exist — and closed the three registry defects that release
made real: the split Artifact Registry coordinate (BUG-2946), the anonymous
metadata fetch in the shared `ImagePull` (BUG-2944), and the Google harnesses
that pushed anonymously into repositories they never created (BUG-2943). When
it merges:

- Watch its CI run. The Cloud Run and Cloud Run Functions harnesses run
  there against the enforcing registry for the first time; a failure in the
  overlay build, the `docker-hub` pull, or the tag probe is a fidelity gap
  between the harness provisioning (`gcp-common/registrytest`) and what the
  simulator's data plane accepts, and is fixed in the harness or reported to
  sockerless-cloud — never by relaxing the credential.
- Keep the coordinate rule: a backend reads a registry coordinate once into
  its `Config` and every registry operation takes it from there; no helper
  reads `SOCKERLESS_*_ENDPOINT` from the environment on its own.

Remaining bugs, in order:

- BUG-2950: the AWS simulator's `reclaimOrphanedSubnet` can remove a live
  run's idle VPC network when another simulator-aws process allocates a
  slice; seen once locally, fixed in sockerless-cloud, then advance the pin.
- BUG-2945 blocks reading `docker images` through `core.OCIListImages`
  against the Microsoft Azure simulator, which serves no `GET /v2/_catalog`
  at v0.30.3 either. When sockerless-cloud serves it and the pin here
  advances, the Azure Container Registry round trip in
  `backends/azure-common` should read through `core.OCIListImages` rather
  than the repository's tag endpoint.
- BUG-2922 (Docker Engine advisories → moby/moby client migration) is the
  largest open local item, scoped to the Docker passthrough backend.
- BUG-2925 (the UI CI stall) stays open until its cause is proven.
- BUG-1075: live-cloud validation beyond AWS Lambda. The Google modules now
  create the `sockerless-overlay` repository the overlay path pushes to,
  which a live Cloud Run or Cloud Run Functions run needs.

Simulator pins: sockerless-cloud releases with exactly one `vX.Y.Z` tag
(release-please); Go pins reference release commits (pseudo-versions) and
checkout/git-context pins reference the tag. v0.30.3 (commit
`24dd57dfc0c684443ad7e0703f33652e6804ccb9`) is the current pin everywhere.
Verify a release with sockerless-cloud's
`scripts/verify-release-complete.sh <tag>` before pinning it, and bump every
pin in one PR: `tests/go.mod` (the three simulator modules and `ui-auth` at
the release commit — the deleted `v0.1.0` bootstrap tag outranks every
pseudo-version, so a `ui-auth` pin left at `v0.1.0` builds the new
simulators against the old package), the `ARG SOCKERLESS_CLOUD_VERSION`
defaults across the harness Dockerfiles, the `context:` tag in
`deploy/compose.build.yaml`, and the pinned ref in
`.github/workflows/live-tests-lambda.yml`.
