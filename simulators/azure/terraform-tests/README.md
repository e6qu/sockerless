# simulator-azure-terraform-tests

Integration tests that run `terraform apply` and `terraform destroy` against the Azure simulator. Verifies that the simulator implements enough of the Azure ARM API surface for the Terraform AzureRM provider to provision and tear down resources.

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
4. `terraform init` downloads the AzureRM provider
5. `terraform apply -auto-approve` provisions resources against the simulator
6. Test assertions verify the Terraform state
7. `terraform destroy -auto-approve` tears down resources
