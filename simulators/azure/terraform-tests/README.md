# simulator-azure-terraform-tests

Integration tests that run `terraform apply` and `terraform destroy` against the Azure simulator. Verifies that the simulator implements enough of the Azure ARM API surface for real Terraform providers to provision and tear down resources.

Resources covered (azurerm — sim ships custom cloud metadata + OAuth2 token endpoint + JWKS so azurerm bootstraps against the sim instead of real Azure):
- `azurerm_resource_group`
- `azurerm_virtual_network` / `azurerm_subnet`
- `azurerm_network_security_group` / `azurerm_network_security_rule`
- `azurerm_storage_account` (Azure Files / runner shared volumes; a second account backs the Function App)
- `azurerm_storage_container` / `azurerm_storage_table` (storage data plane)
- `azurerm_key_vault` + access policy + secret / key / certificate (runner credential storage, control + data plane)
- `azurerm_container_registry` (Standard)
- `azurerm_user_assigned_identity`
- `azurerm_public_ip` / `azurerm_public_ip_prefix` / `azurerm_nat_gateway` + associations / `azurerm_lb` + backend pool + probe + rule
- `azurerm_network_interface` + `azurerm_linux_virtual_machine`
- `azurerm_private_dns_zone` / `azurerm_dns_zone` + A record
- `azurerm_eventgrid_topic` / domain / domain topic / system topic
- `azurerm_eventhub_namespace` + eventhub + consumer group + authorization rule
- `azurerm_servicebus_namespace` + queue
- `azurerm_cosmosdb_account` + SQL database + container + table
- `azurerm_redis_cache` + `azurerm_redis_firewall_rule`
- `azurerm_log_analytics_workspace`
- `azurerm_application_insights`
- `azurerm_container_app_environment` + `azurerm_container_app` + `azurerm_container_app_job` (the ACA runner backend host + workload + job primitives)
- `azurerm_logic_app_workflow` / `azurerm_container_group`
- `azurerm_service_plan` + `azurerm_linux_function_app` (the AZF runner backend host + workload)
- `azurerm_api_management` + API + product + subscription

## Running

These tests require Terraform and Docker. On Linux, direct `go test` runs Terraform locally. On macOS, the harness delegates the same test command into the shared Linux Docker test image because Go's Security.framework-backed trust store ignores `SSL_CERT_FILE`.

```sh
# Inside Docker (via the parent simulator Makefile)
cd simulators/azure
make docker-test

# Or directly; macOS delegates this command into Linux Docker
cd simulators/azure/terraform-tests
go test -v ./...
```

The test harness (`helpers_test.go`) handles simulator binary build, port allocation, simulator startup, Caddy HTTPS gateway startup, Terraform init/apply/destroy, and shutdown.

## Prerequisites

- Go 1.23+
- `terraform` CLI installed and on `PATH` for direct Linux runs; the shared Docker image supplies Terraform for macOS delegation
- `caddy` installed and on `PATH` for direct Linux runs; the shared Docker image supplies Caddy for macOS delegation
- Docker (required for Container Apps resources and for macOS Linux-container delegation)
- The `simulators/azure/` parent module (built automatically by `TestMain`)

## TLS requirement

The AzureRM Terraform provider hardcodes `https://` for metadata endpoint calls. The test harness starts the simulator on HTTP loopback, starts the repo's Caddy HTTPS gateway in front of it, and points Terraform at `https://azure.sockerless.localhost:<port>`. Terraform trusts Caddy's local CA via `SSL_CERT_FILE`.

## How it works

1. `TestMain` builds the Azure simulator binary and starts it on HTTP loopback
2. The harness starts Caddy with isolated per-test state and waits for its local CA
3. The harness verifies Azure metadata JSON through the HTTPS gateway
4. Tests write Terraform configurations to a temp directory
5. `terraform init` downloads the Terraform providers used by the test configuration
6. `terraform apply -auto-approve` provisions resources against the simulator
7. Test assertions verify the Terraform state
8. `terraform destroy -auto-approve` tears down resources
