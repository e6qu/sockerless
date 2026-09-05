# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next

The branch `bump-sockerless-cloud-v0.30.3-registry-fixes` closed the three
registry defects that sockerless-cloud v0.30.3 — the release whose Google
Artifact Registry data plane authenticates every `/v2/` request and requires
the repository to exist — made real: the split Artifact Registry coordinate
(BUG-2946), the anonymous metadata fetch in the shared `ImagePull`
(BUG-2944), and the Google harnesses that pushed anonymously into
repositories they never created (BUG-2943). The pin itself stays at v0.9.2:
against v0.30.3 the harness provisioning worked end to end, but that release's
own Cloud Run and Cloud Functions hosts pull workload images with no
credential from the registry they now enforce (BUG-2951), and its Azure
Container Apps file-share mount fails the shared-volume writer (BUG-2952).
When the branch merges:

- Watch its CI run; every harness runs against v0.9.2 with the coordinate
  and credential changes.
- Keep the coordinate rule: a backend reads a registry coordinate once into
  its `Config` and every registry operation takes it from there; no helper
  reads `SOCKERLESS_*_ENDPOINT` from the environment on its own. Every
  simulator stack (smoke, GitLab, e2e, `make stack-*`) exports
  `SOCKERLESS_GCP_AR_ENDPOINT` pointing at the simulator, scheme included.

Simulator-side work, in sockerless-cloud (one open pull request there at a
time; another agent's is open now):

- BUG-2951: the Cloud Run and Cloud Functions hosts pull with the
  credential the simulator's own token service issues for the project's
  service agent, passed as the Docker SDK's `RegistryAuth`.
- BUG-2952: the Azure Container Apps file-share mount at v0.30.3.
- BUG-2950: `reclaimOrphanedSubnet` removes only networks of a dead run.
- BUG-2945: serve `GET /v2/_catalog` on the Azure simulator.

Then bump the pin here — `tests/go.mod` (the three simulators and `ui-auth`
at the release commit; the deleted `v0.1.0` bootstrap tag outranks every
pseudo-version, so a `ui-auth` pin left at `v0.1.0` builds the new
simulators against the old package), the `ARG SOCKERLESS_CLOUD_VERSION`
defaults across the harness Dockerfiles, the `context:` tag in
`deploy/compose.build.yaml`, and the pinned ref in
`.github/workflows/live-tests-lambda.yml` — and read `docker images` through
`core.OCIListImages` in the Azure Container Registry round trip.

Remaining local items:

- BUG-2922 (Docker Engine advisories → moby/moby client migration) is the
  largest open local item, scoped to the Docker passthrough backend.
- BUG-2925 (the UI CI stall) stays open until its cause is proven.
- BUG-1075: live-cloud validation beyond AWS Lambda. The Google modules now
  create the `sockerless-overlay` repository the overlay path pushes to,
  which a live Cloud Run or Cloud Run Functions run needs.

Simulator pins: sockerless-cloud releases with exactly one `vX.Y.Z` tag
(release-please); Go pins reference release commits (pseudo-versions) and
checkout/git-context pins reference the tag. v0.9.2 (commit
`723736a8a233e0f3ccb3d6ce0c209a94bfc9afc2`) is the current pin everywhere.
Verify a release with sockerless-cloud's
`scripts/verify-release-complete.sh <tag>` before pinning it, and bump every
pin in one PR.
