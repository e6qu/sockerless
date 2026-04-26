# Azure runbook — ACA Jobs/Apps + Azure Functions

**Status:** placeholder. Live-Azure track is queued in [PLAN.md](../PLAN.md). The terraform live env under `terraform/environments/aca/live/` and `azure-functions/live/` still needs to be added; once it's in place the runbook below gets fleshed out track-by-track.

The shape mirrors [02-aws-runbook.md](02-aws-runbook.md) — same docker / podman CLI surface, same track structure (A core, B podman, C advanced, D function-specific, E peer comms, F pods, G compose, H podman compose, I stateless, J runner integration). The cloud-API parity (every SDK call sockerless makes against Container Apps / App Service / Storage / Log Analytics / Private DNS / Network Security Groups) is already exercised against the Azure simulator under `simulators/azure/{sdk-tests,cli-tests,terraform-tests}/`; the live runbook is to verify the same surface against real Azure.

## Prerequisites (when ready)

- Azure subscription with Container Apps + App Service quota.
- Managed Environment with VNet integration (required when `SOCKERLESS_ACA_USE_APP=1` for peer reachability).
- Service principal with `Container Apps Contributor` + `Storage Account Contributor` + `Log Analytics Reader`.
- `az` CLI authenticated.

## Cross-links

- Sim coverage: [specs/SIM_PARITY_MATRIX.md](../specs/SIM_PARITY_MATRIX.md) § Azure — 28/28 cloud-API rows ✓
- Backend code: `backends/aca/`, `backends/azure-functions/`
- Sim handlers: `simulators/azure/containerapps.go`, `simulators/azure/containerapps_apps.go`, `simulators/azure/functions.go`
