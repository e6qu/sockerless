# simulator-aws-terraform-tests

Integration tests that run `terraform apply` and `terraform destroy` against the AWS simulator. Verifies that the simulator implements enough of the AWS API surface for the Terraform AWS provider to provision and tear down resources.

## Running

```sh
cd simulators/aws/terraform-tests
go test -v ./...
```

To run the same provider flow through the optional Caddy HTTPS gateway:

```sh
cd simulators/aws
make terraform-https-test
```

The HTTPS target uses Caddy's `https://localhost:<ephemeral-port>` single-simulator route so the test does not depend on wildcard `.localhost` DNS support. It still uses Caddy TLS and passes the generated root CA to Terraform through `SSL_CERT_FILE`. On macOS the Make target runs the same test inside the shared Linux simulator test image so the real provider honors that CA file.

The test harness (`helpers_test.go`) handles simulator binary build, port allocation, server startup, Terraform init/apply/destroy, and shutdown. No external services required.

## Prerequisites

- Go 1.23+
- `terraform` CLI installed and on `PATH`
- The `simulators/aws/` parent module (built automatically by `TestMain`)
- `caddy` installed and on `PATH` for `make terraform-https-test`

## How it works

1. `TestMain` builds the AWS simulator binary and starts it on a free port
2. Tests write Terraform configurations to a temp directory
3. `terraform init` downloads the AWS provider
4. `terraform apply -auto-approve` provisions resources against the simulator
5. Test assertions verify the Terraform state
6. `terraform destroy -auto-approve` tears down resources
