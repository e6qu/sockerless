# simulator-aws-terraform-tests

Integration tests that run `terraform apply` and `terraform destroy` against the AWS simulator. Verifies that the simulator implements enough of the AWS API surface for the Terraform AWS provider to provision and tear down resources.

## Running

```sh
cd simulators/aws/terraform-tests
go test -v ./...
```

The test harness (`helpers_test.go`) handles simulator binary build, port allocation, server startup, Terraform init/apply/destroy, and shutdown. No external services required.

## Prerequisites

- Go 1.23+
- `terraform` CLI installed and on `PATH`
- The `simulators/aws/` parent module (built automatically by `TestMain`)

## How it works

1. `TestMain` builds the AWS simulator binary and starts it on a free port
2. Tests write Terraform configurations to a temp directory
3. `terraform init` downloads the AWS provider
4. `terraform apply -auto-approve` provisions resources against the simulator
5. Test assertions verify the Terraform state
6. `terraform destroy -auto-approve` tears down resources
