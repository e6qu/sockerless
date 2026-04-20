# Azure Simulator Parity

Which Azure service slices `simulators/azure/` covers, for which surfaces,
and where the runner path relies on each one.

**Status legend:**

- ✔ **implemented** — simulator covers what sockerless uses, with SDK + CLI + terraform tests (or documented exemption).
- ◐ **partial** — some endpoints missing; not on sockerless's runner path.
- ✖ **not implemented** — sockerless uses this but simulator doesn't cover it. Tracked in BUGS.md.
- N/A — sockerless doesn't use this slice.

Current bug count: 707 total / 707 fixed / 0 open on the Azure side of Phase 86.

## Runner-path slices

| Slice | Status | File(s) | Runner usage |
|---|---|---|---|
| **Azure Container Apps (Jobs)** — `armappcontainers` Jobs CRUD, JobsExecutions start/list/get/cancel, template CRUD | ✔ | `containerapps.go` | ACA backend runs every sockerless-managed container as a Job execution. |
| **ACA environments** (BUG-701 Azure) — managedEnvironments CRUD; PUT auto-creates backing Docker network `sim-env-<envName>`; job-start resolves env + passes `NetworkAliases=[jobName]` through `ContainerConfig` so cross-job DNS works via Docker embedded DNS | ✔ | `containerappsenv.go` | Cross-job hostname resolution. |
| **Azure Functions** (via App Service Plans) | ✔ | `functions.go` + `appserviceplan.go` | Azure Functions backend — function containers run with Functions runtime. |
| **Azure Container Registry (control plane + OCI Distribution v2)** — Registries CRUD, name availability check, replications list; `/v2/` manifest + blob + upload endpoints | ✔ | `acr.go` | ACA + Azure Functions backends push overlay images here. |
| **ACR Cache Rules** (BUG-706) — cacheRules CRUD (PUT/GET/LIST/DELETE) as `registries/{registry}/cacheRules/{rule}` sub-resource; real `armcontainerregistry.CacheRulesClient` round-trips | ✔ | `acr.go` | ACA backend's `ResolveAzureImageURIWithCache` matches source-repository prefix (exact + `/*` wildcard) and rewrites Docker Hub refs → `<acrName>.azurecr.io/<target>:<tag>`. |
| **Private DNS Zones** (BUG-702) — zones CRUD, record sets CRUD (A records), virtual network links | ✔ | `dns.go` | ACA service discovery — per-network zone `skls-<name>.local`. |
| **Network Security Groups (NSG)** (BUG-703) — networkSecurityGroups CRUD + `securityRules` sub-resource CRUD (PUT/GET/LIST/DELETE) consistent with parent's `Properties.SecurityRules` | ✔ | `network.go` | ACA backend creates per-network NSG + allow rules on container connect. |
| **Virtual Network / Subnets** — virtualNetworks CRUD, subnets CRUD, subnet ↔ NSG association | ✔ | `network.go` | VPC-level container placement. |
| **Log Analytics Workspaces** (control plane) + **Log Query (azquery)** — workspace query; HTTP fallback (`httpLogsClient`) when SDK's BearerTokenPolicy rejects non-TLS endpoints | ✔ | `monitor.go` + `insights.go` | `docker logs` against ACA reads here. |
| **Managed Identity** (user-assigned identities) — identities CRUD | ✔ | `managedidentity.go` | ACA + Azure Functions workload identity. |
| **Azure Files shares** — shares CRUD; ACA + Functions mount volumes via SMB | ✔ | `files.go` | Docker volume backing. |
| **Authorization (RBAC roleAssignments)** — roleAssignments CRUD, roleDefinitions read | ✔ | `authorization.go` | Managed-identity → resource RBAC. |
| **Resource Groups** — CRUD | ✔ | `resourcegroups.go` | Every resource lives in a resource group. |
| **Subscription + Metadata** — subscription Get, ARM metadata endpoint | ✔ | `subscription.go` + `metadata.go` | SDK client bootstrap. |

## Out-of-scope (N/A) slices

Sockerless doesn't touch these, so the simulator doesn't implement them:

- Azure Kubernetes Service, Azure Service Bus, Cosmos DB, Azure SQL, Azure Cache for Redis, Event Grid, Event Hubs, Front Door, Application Gateway, Key Vault (beyond managed-identity integration), Traffic Manager.

## Known limitations

- **Terraform coverage**: The simulator terraform-tests use the `azurestack` provider (the only Azure-compatible TF provider that accepts an `arm_endpoint` override for custom HTTP endpoints). `azurestack` doesn't expose newer resource types — notably: ACA resources (`azurestack_container_app_*`), ACA environment, ACR cache rules, managed identities. Where a resource lacks `azurestack` coverage, the simulator's SDK + CLI tests carry the parity proof. All current slices are covered by at least one of {SDK, CLI, terraform}.

## Exit check

No ✖ rows. All runner-path slices are implemented with SDK + CLI tests (and terraform where the provider supports the resource); gaps are documented via `simulators/azure/tests-exempt.txt`.
