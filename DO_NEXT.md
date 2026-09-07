# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next

The `terraform-integration` job applies five environments — Amazon ECS,
Google Cloud Run, Cloud Run Functions, Azure Container Apps and Azure
Functions — against their simulators at sockerless-cloud v0.30.10 and runs
an act smoke through each backend; a red cell there is a module, harness
or simulator defect, not a flake. The Docker API clients speak
`github.com/moby/moby/client`.

Next, in order — the AWS registry coordinate, mirroring what the Google
(`SOCKERLESS_GCP_AR_ENDPOINT`) and Azure (`SOCKERLESS_AZURE_ACR_ENDPOINT`)
paths already do, where a relocated registry's host is the host every
image reference carries, for the push and for the workload's pull alike:

1. sockerless-cloud BUG-2991, the simulator side:
   - The CodeBuild build environment runs docker steps: `privileged_mode`
     gives the environment the engine (the host engine's socket bound in,
     as real CodeBuild's docker daemon is), and a curated image name
     (`aws/codebuild/<name>:<tag>`) resolves to its public distribution,
     `public.ecr.aws/codebuild/<name>:<tag>`; an `ARM_CONTAINER` runs the
     aarch64 image natively.
   - The environment's Docker configuration carries a credential helper
     that answers the simulator's ECR login server and the simulator's
     own port (`*:<port>`) with the `AWS` / authorization-token credential,
     the way `cloudBuildDockerConfig` and `acrRunDockerConfig` do.
   - The Lambda and Amazon ECS hosts pull a reference that carries the
     simulator's own port from the simulator's registry with that
     credential (`sim.PullImageWithCredential`), as the Cloud Run host
     does, instead of resolving it to a local engine name.
   - Covered by an SDK test: a CodeBuild build whose buildspec logs in,
     builds and pushes to the simulator's ECR, then a Lambda function
     created from that reference runs it.
2. BUG-2978, here, on the release that carries it:
   - `SOCKERLESS_AWS_ECR_ENDPOINT` (default: the account's ECR login
     server) read once into `aws-common`; the Lambda overlay reference
     carries its host, the buildspec logs in to the reference's own
     registry (`${IMAGE_URI%%/*}`) rather than a hardcoded login server,
     and the overlay is built for the function's architecture on an
     `ARM_CONTAINER` when it is arm64.
   - Delete the `EndpointURL != ""` branch and the local-docker fallback in
     `backends/lambda`; a prebuilt overlay stays the only other mode.
   - The harness exports `SOCKERLESS_LAMBDA_CODEBUILD_PROJECT`,
     `SOCKERLESS_LAMBDA_BUILD_BUCKET`, `SOCKERLESS_LAMBDA_OVERLAY_ECR_REPO`
     and `SOCKERLESS_AWS_ECR_ENDPOINT` from the Lambda module's outputs and
     the simulator's address; the Lambda cell rejoins the matrix.

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
