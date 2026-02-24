# simulator-gcp-terraform-tests

Integration tests that run `terraform apply` and `terraform destroy` against the GCP simulator. Verifies that the simulator implements enough of the GCP API surface for the Terraform Google provider to provision and tear down resources.

## Running

```sh
cd simulators/gcp/terraform-tests
go test -v ./...
```

The test harness (`helpers_test.go`) handles simulator binary build, port allocation, server startup, Terraform init/apply/destroy, and shutdown. No external services required.

## Prerequisites

- Go 1.23+
- `terraform` CLI installed and on `PATH`
- The `simulators/gcp/` parent module (built automatically by `TestMain`)

## How it works

1. `TestMain` builds the GCP simulator binary and starts it on a free port
2. Tests write Terraform configurations to a temp directory
3. `terraform init` downloads the Google provider
4. `terraform apply -auto-approve` provisions resources against the simulator
5. Test assertions verify the Terraform state
6. `terraform destroy -auto-approve` tears down resources
