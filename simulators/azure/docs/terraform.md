# Using the Azure simulator with Terraform

## Prerequisites

- Terraform installed (`terraform version`)
- Simulator running with **TLS enabled** on `https://localhost:4568`

## TLS requirement

The Azure Terraform providers (`azurestack`, `azurerm`) hardcode `https://` for metadata endpoint calls. The simulator must run with TLS:

```sh
SIM_TLS_CERT=server-cert.pem SIM_TLS_KEY=server-key.pem ./simulator-azure
```

Terraform must trust the CA that signed the server certificate:

```sh
export SSL_CERT_FILE=/path/to/ca.pem
```

> **macOS limitation:** Go 1.20+ on macOS uses Security.framework for TLS and ignores `SSL_CERT_FILE`. Azure Terraform tests are Docker-only (Linux). On macOS, use the CLI or SDK approach instead.

### Generating self-signed certificates

For local testing, generate a CA and server certificate:

```sh
# Generate CA key and certificate
openssl ecparam -genkey -name prime256v1 -out ca-key.pem
openssl req -new -x509 -key ca-key.pem -out ca.pem -days 1 -subj "/CN=Test CA"

# Generate server key and certificate signed by the CA
openssl ecparam -genkey -name prime256v1 -out server-key.pem
openssl req -new -key server-key.pem -out server.csr -subj "/CN=localhost"
openssl x509 -req -in server.csr -CA ca.pem -CAkey ca-key.pem -CAcreateserial \
  -out server-cert.pem -days 1 \
  -extfile <(printf "subjectAltName=DNS:localhost,IP:127.0.0.1")

# Start simulator with TLS
SIM_TLS_CERT=server-cert.pem SIM_TLS_KEY=server-key.pem ./simulator-azure

# Tell Terraform to trust the CA
export SSL_CERT_FILE=$(pwd)/ca.pem
```

## Provider configuration

Use the `hashicorp/azurerm` provider through the simulator's custom Azure cloud metadata and OAuth2 endpoints for the cloud resources sockerless exercises. The test suite also retains `azurestack` coverage for Azure Stack-compatible ARM resources.

```hcl
terraform {
  required_providers {
    azurestack = {
      source  = "hashicorp/azurestack"
      version = "~> 1.0"
    }
  }
}

provider "azurestack" {
  arm_endpoint    = "https://localhost:4568"
  client_id       = "test-client-id"
  client_secret   = "test-client-secret"
  tenant_id       = "11111111-1111-1111-1111-111111111111"
  subscription_id = "00000000-0000-0000-0000-000000000001"

  skip_provider_registration = true

  features {}
}
```

`skip_provider_registration = true` prevents provider-registration calls that are outside the simulator slice.

## Environment variables

Set these before running `terraform`:

```sh
export ARM_CLIENT_ID=test-client-id
export ARM_CLIENT_SECRET=test-client-secret
export ARM_TENANT_ID=11111111-1111-1111-1111-111111111111
export ARM_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000001
export ARM_ENDPOINT=https://localhost:4568
export SSL_CERT_FILE=/path/to/ca.pem
```

## Example resources

```hcl
resource "azurestack_resource_group" "main" {
  name     = "my-rg"
  location = "eastus"
}

resource "azurestack_virtual_network" "main" {
  name                = "my-vnet"
  resource_group_name = azurestack_resource_group.main.name
  location            = azurestack_resource_group.main.location
  address_space       = ["10.0.0.0/16"]
}

resource "azurestack_subnet" "main" {
  name                 = "my-subnet"
  resource_group_name  = azurestack_resource_group.main.name
  virtual_network_name = azurestack_virtual_network.main.name
  address_prefix       = "10.0.1.0/24"
}

resource "azurestack_dns_zone" "main" {
  name                = "example.local"
  resource_group_name = azurestack_resource_group.main.name
}
```

## Running

```sh
# Using a variable for the endpoint
terraform init
terraform apply -auto-approve -var="endpoint=https://localhost:4568"
terraform destroy -auto-approve -var="endpoint=https://localhost:4568"
```

With a `variables.tf`:

```hcl
variable "endpoint" {
  description = "Simulator endpoint URL"
  type        = string
  default     = "https://localhost:4568"
}
```

## Supported resources

The simulator supports the Azure API operations that these Terraform resources use:

| Category | Resources |
|----------|-----------|
| Resource Groups | `azurestack_resource_group` |
| Networking | `azurestack_virtual_network`, `azurestack_subnet`, `azurestack_network_security_group` |
| DNS | `azurestack_dns_zone` |
| Storage | `azurestack_storage_account` |

The automated terraform tests cover both Azure Stack-compatible ARM resources and AzureRM resources that sockerless depends on: ACA managed environments, ACA Jobs/Apps, ACR, managed identity, Private DNS, Log Analytics, Application Insights, App Service plans, Linux Function Apps, and Storage Accounts.

## Notes

- TLS is required. Azure Terraform providers use HTTPS for metadata and ARM endpoint calls.
- All state is in-memory and resets when the simulator restarts.
- The OAuth2 endpoint accepts local-test credentials and returns Azure-shaped token responses.
- `skip_provider_registration = true` is required to prevent provider registration API calls.
- Docker-only on macOS due to TLS trust limitations (see TLS requirement section above).
