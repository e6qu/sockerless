# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next

The `terraform-integration` job applies five environments — Amazon ECS,
Google Cloud Run, Cloud Run Functions, Azure Container Apps and Azure
Functions — against their simulators at sockerless-cloud v2.0.9 and runs
an act smoke through each backend; a red cell there is a module, harness
or simulator defect, not a flake.

Next, in order:

1. sockerless-cloud: a CodeBuild build environment gets its project's
   service role credentials through the container credentials endpoint
   (`AWS_CONTAINER_CREDENTIALS_FULL_URI` at the simulator's address), as
   real CodeBuild does; today it takes the instance metadata endpoint's
   fixed `ASIASIMACCESSKEY`, which the simulator's IAM refuses.
2. BUG-2978, here, on the release that carries it once it has aged 24
   hours: bump every pin and add `lambda` to the `terraform-integration`
   matrix. The backend, the Lambda module and the harness are ready.
3. The next simulator bump past v2.0.9 gives each App Service and Container
   Apps workload its own `IDENTITY_HEADER`; the Azure harnesses (the ACA and
   Azure Functions integration suites, `smoke-tests/run.sh`, the Terraform
   integration harness, `tests/sim_harness_test.go`) need a credential the
   simulator authenticates for a client outside it.
4. BUG-2980: the simulator-backed end-to-end harnesses configure the AWS
   backends as `tests/terraform-integration/run-test.sh` does.

What remains is gated outside this repository:

- BUG-1075: live-cloud validation needs the accounts and secrets its row
  lists, plus ACA and Azure Functions runner images and GCP runner images
  aligned with the live Terraform outputs.

Durable rules for the next change:

- The coordinate rule: a backend reads a registry coordinate
  (`SOCKERLESS_AWS_ECR_ENDPOINT`, `SOCKERLESS_GCP_AR_ENDPOINT`,
  `SOCKERLESS_AZURE_ACR_ENDPOINT`) once into its `Config` and every registry
  operation takes it from there; no helper reads it from the environment on
  its own, and `core.RegistryCoordinateHost` / `core.RegistryCoordinateURL`
  are the one reading of its value. Every simulator stack exports the
  coordinate pointing at the simulator, scheme included.
- Every AWS Lambda function runs a bootstrap overlay: built through AWS
  CodeBuild for the function's architecture, or prebuilt by the operator.
  There is no bare-image or local-docker path, against the simulator or the
  cloud.
- Bun installs the UI workspace and runs its scripts; the tools the scripts
  call (Vite, Vitest, tsc) run on Node. No script uses `bun --bun`.
- A harness reads Terraform outputs with `jq -e`, so a missing output fails
  the run, and a failed `terragrunt destroy` fails it too.
- A stateless backend's cloud resource tags are the only carrier of a
  container's Docker identity (name, labels, tty) after a start; write them
  through `core.TagSet` on every create path.
- A destroy-time sweep requires its cloud CLI and fails the destroy on a
  failing list or delete; it never hides the CLI behind `2>/dev/null`.

Simulator pins: sockerless-cloud releases with exactly one `vX.Y.Z` tag
(release-please); Go pins reference release commits (pseudo-versions) and
checkout/git-context pins reference the tag. v2.0.9 (commit
`2f60a2aaa0a9c006aa4541d20ff2a2c65977b33a`) is the current pin everywhere:
`tests/go.mod` (three simulators, `ui-auth`, `realexec`, `sim`), every
harness `ARG SOCKERLESS_CLOUD_VERSION`, `deploy/compose.build.yaml` and
`.github/workflows/live-tests-lambda.yml`. Verify a release with
sockerless-cloud's `scripts/verify-release-complete.sh <tag>` before pinning
it, and bump every pin in one PR.
