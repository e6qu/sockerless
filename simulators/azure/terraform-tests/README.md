# simulator-azure-terraform-tests

Integration tests that run `terraform apply` and `terraform destroy` against the Azure simulator. Verifies that the simulator implements enough of the Azure ARM API surface for real Terraform providers to provision and tear down resources.

Resources covered (azurestack):
- `azurestack_resource_group`
- `azurestack_virtual_network` / `azurestack_subnet`
- `azurestack_network_security_group` / `azurestack_network_security_rule`
- `azurestack_storage_account` (Azure Files / runner shared volumes)
- `azurestack_key_vault` (runner credential storage)

Resources covered (azurerm — sim ships custom cloud metadata + OAuth2 token endpoint + JWKS so azurerm bootstraps against the sim instead of real Azure):
- `azurerm_resource_group`
- `azurerm_container_registry` (Standard)
- `azurerm_user_assigned_identity`
- `azurerm_private_dns_zone`
- `azurerm_log_analytics_workspace`
- `azurerm_application_insights`
- `azurerm_container_app_environment` + `azurerm_container_app` + `azurerm_container_app_job` (the ACA runner backend host + workload + job primitives)
- `azurerm_service_plan` + `azurerm_linux_function_app` (the AZF runner backend host + workload)
- `azurerm_storage_account` (azurerm-managed, used by Function App)

Not yet covered: Key Vault data-plane (keys/secrets). Data-plane requires per-vault subdomain routing and should be filed in BUGS.md before implementation work starts.

## Running

These tests require Docker (Linux only). On macOS, Go 1.20+ uses Security.framework for TLS and ignores `SSL_CERT_FILE`, so the Terraform provider cannot trust the self-signed CA.

```sh
# Inside Docker (via Makefile)
cd simulators/azure/terraform-tests
make docker-test

# Or directly (Linux only)
go test -v ./...
```

The test harness (`helpers_test.go`) handles simulator binary build, TLS certificate generation, port allocation, server startup, Terraform init/apply/destroy, and shutdown.

## Prerequisites

- Go 1.23+
- `terraform` CLI installed and on `PATH`
- Docker (for running on macOS, which delegates to a Linux container)
- The `simulators/azure/` parent module (built automatically by `TestMain`)

## TLS requirement

The AzureRM Terraform provider and `azurestack` provider hardcode `https://` for metadata endpoint calls. The test harness generates self-signed TLS certificates (CA + server cert) and starts the simulator with `SIM_TLS_CERT` / `SIM_TLS_KEY`. Terraform trusts the CA via `SSL_CERT_FILE`.

## How it works

1. `TestMain` generates a self-signed CA and server certificate
2. Builds the Azure simulator binary and starts it with TLS on a free port
3. Tests write Terraform configurations to a temp directory
4. `terraform init` downloads the Terraform providers used by the test configuration
5. `terraform apply -auto-approve` provisions resources against the simulator
6. Test assertions verify the Terraform state
7. `terraform destroy -auto-approve` tears down resources
