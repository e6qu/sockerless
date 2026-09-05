# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next

The branch `consolidate-shared-backend-components` moved every duplicated
component of the six cloud backends into `backends/core`, `agent/envelope`,
or the owning `*-common` module (see [AGENTS.md](AGENTS.md) § "Shared code
has three homes"). When it merges:

- Watch its CI run. Hosted runners exercise the six integration harnesses
  against the pinned simulators (buffered attach, shared volumes, network
  pods, DNS discovery, volume listing) on the shared implementations; a
  failure there is a behavioural difference the consolidation introduced
  and is fixed in core or the owning common module, never by re-adding a
  per-backend copy.
- Keep new code in one of the three homes. A helper whose body would be
  identical in a sibling backend after renaming the cloud is a defect at
  review time.

Registry work, in order:

- BUG-2943 is timed: the Google Artifact Registry data plane in
  sockerless-cloud authenticates every `/v2/` request and requires the
  repository to exist, and the Cloud Run and Cloud Run Functions harnesses
  push anonymously into a repository they never create. Fix them in the
  release that carries it, alongside BUG-2946 (the Artifact Registry
  endpoint coordinate is split across two settings that agree only by
  accident).
- BUG-2945 blocks reading `docker images` through `core.OCIListImages`
  against the Microsoft Azure simulator, which serves no `GET /v2/_catalog`.
  When sockerless-cloud serves it and the pin here advances, the Azure
  Container Registry round trip in `backends/azure-common` should read
  through `core.OCIListImages` rather than the repository's tag endpoint.
- BUG-2944: `BaseServer.ImagePull` fetches image metadata anonymously and
  discards the credential it was handed; thread it through or refuse a
  registry that is not anonymously readable.

Larger items:

- BUG-2922 (Docker Engine advisories → moby/moby client migration) is the
  largest open local item, scoped to the Docker passthrough backend.
- BUG-1075: live-cloud validation beyond AWS Lambda.

Simulator pins: sockerless-cloud releases with exactly one `vX.Y.Z` tag
(release-please); Go pins reference release commits (pseudo-versions) and
checkout/git-context pins reference the tag. v0.9.2 is the current pin
everywhere. Verify a release with sockerless-cloud's
`scripts/verify-release-complete.sh <tag>` before pinning it, and bump every
pin in one PR: `tests/go.mod` (`make -C tests upgrade-deps`), the
`ARG SOCKERLESS_CLOUD_VERSION` defaults across the harness Dockerfiles, the
`context:` tag in `deploy/compose.build.yaml`, and the pinned ref in
`.github/workflows/live-tests-lambda.yml`.
