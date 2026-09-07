# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next

The `terraform-integration` job applies five environments — Amazon ECS,
Google Cloud Run, Cloud Run Functions, Azure Container Apps and Azure
Functions — against their simulators at sockerless-cloud v0.30.10 and runs
an act smoke through each backend; a red cell there is a module, harness
or simulator defect, not a flake. The Docker API clients speak
`github.com/moby/moby/client`.

Next, in order:

1. sockerless-cloud BUG-2991: the AWS simulator's CodeBuild build
   environment runs docker steps (the engine socket, as ECS tasks get)
   with a Docker configuration whose credential helper answers the
   simulator's ECR login server and routes it to the simulator's own
   `/v2/`, the way its Cloud Build and ACR Tasks already do; covered by an
   SDK test that builds and pushes an image and pulls it back.
2. BUG-2978, on the release that carries it: delete the Lambda backend's
   `EndpointURL != ""` branch and the local-docker fallback, build the
   overlay for the function's architecture through CodeBuild, export
   `SOCKERLESS_LAMBDA_CODEBUILD_PROJECT`, `SOCKERLESS_LAMBDA_BUILD_BUCKET`
   and `SOCKERLESS_LAMBDA_OVERLAY_ECR_REPO` from the harness's Lambda
   module outputs, and return the Lambda cell to the matrix.

What remains after that is gated outside this repository:

- BUG-2925 (the UI CI stall) stays open until its cause is proven; the
  original stall has not recurred.
- BUG-1075: live-cloud validation beyond AWS Lambda needs real-cloud
  credentials for Google Cloud Run, Azure Container Apps, Azure Functions,
  Lambda service mesh and Azure identity. The modules provision what a live
  run needs (the `sockerless-overlay` repository, the Cloud KMS-encrypted
  buckets, the Azure CLI-driven destroy sweep).

Durable rules for the next change:

- The coordinate rule: a backend reads a registry coordinate once into its
  `Config` and every registry operation takes it from there; no helper reads
  `SOCKERLESS_*_ENDPOINT` from the environment on its own. Every simulator
  stack exports `SOCKERLESS_GCP_AR_ENDPOINT` pointing at the simulator,
  scheme included.
- A stateless backend's cloud resource tags are the only carrier of a
  container's Docker identity (name, labels, tty) after a start; write them
  through `core.TagSet` on every create path.
- A destroy-time sweep requires its cloud CLI and fails the destroy on a
  failing list or delete; it never hides the CLI behind `2>/dev/null`.

Simulator pins: sockerless-cloud releases with exactly one `vX.Y.Z` tag
(release-please); Go pins reference release commits (pseudo-versions) and
checkout/git-context pins reference the tag. v0.30.10 (commit
`791211658b76c59780c0c4cd499daa97c81867c3`) is the current pin everywhere:
`tests/go.mod` (three simulators, `ui-auth`, `realexec`, `sim`), every
harness `ARG SOCKERLESS_CLOUD_VERSION`, `deploy/compose.build.yaml` and
`.github/workflows/live-tests-lambda.yml`. Verify a release with
sockerless-cloud's `scripts/verify-release-complete.sh <tag>` before pinning
it, and bump every pin in one PR.
