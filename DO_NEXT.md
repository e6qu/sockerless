# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next

The branch `tf-int-harness-credentials` made the Terraform integration
harness run every environment against its simulator in CI, and its cells
found four defects the other harnesses had never reached — the reverse-agent
`docker cp` destination (BUG-2959), the Amazon ECS `docker exec` exit status
(BUG-2960), the working directory of a Cloud Run pod Service before its
workload runs (BUG-2961) and an ECS exec whose working directory is missing
(BUG-2962) — plus the simulator's dropped ECS `workingDirectory`
(sockerless-cloud BUG-2981, released in v0.30.7). The pin moved from v0.9.2
to v0.30.7, which also carries the Cloud Run and Cloud Functions hosts
pulling as the project's service agent (BUG-2951), the owner-aware subnet
reclaim (BUG-2950) and Azure Container Registry's `GET /v2/_catalog`
(BUG-2945). When the branch merges:

- Watch its CI run; every harness runs against v0.30.7, and the
  `terraform-integration` job applies each Terraform environment against its
  simulator — a red cell there is a module or harness defect, not a flake.
- Keep the coordinate rule: a backend reads a registry coordinate once into
  its `Config` and every registry operation takes it from there; no helper
  reads `SOCKERLESS_*_ENDPOINT` from the environment on its own. Every
  simulator stack (smoke, GitLab, e2e, `make stack-*`) exports
  `SOCKERLESS_GCP_AR_ENDPOINT` pointing at the simulator, scheme included.

Simulator-side work, in sockerless-cloud (one open pull request there at a
time):

- BUG-2952: retest the Azure Container Apps file-share mount at v0.30.7.
- BUG-2945: read `docker images` through `core.OCIListImages` in the Azure
  round trip now that the catalog is served.
- BUG-2957: the Lambda host pulls its image from the simulator's own ECR
  instead of resolving a pull-through-cache reference to a local name; then
  the Lambda cell rejoins the `terraform-integration` matrix.

Remaining local items:

- BUG-2922 (Docker Engine advisories → moby/moby client migration) is the
  largest open local item, scoped to the Docker passthrough backend.
- BUG-2925 (the UI CI stall) stays open until its cause is proven.
- BUG-1075: live-cloud validation beyond AWS Lambda. The Google modules now
  create the `sockerless-overlay` repository the overlay path pushes to,
  which a live Cloud Run or Cloud Run Functions run needs.

Simulator pins: sockerless-cloud releases with exactly one `vX.Y.Z` tag
(release-please); Go pins reference release commits (pseudo-versions) and
checkout/git-context pins reference the tag. v0.30.7 (commit
`a0155d2fbc7a0ffe7c0a769e2bce94d3eef75d39`) is the current pin everywhere.
Verify a release with sockerless-cloud's
`scripts/verify-release-complete.sh <tag>` before pinning it, and bump every
pin in one PR.
