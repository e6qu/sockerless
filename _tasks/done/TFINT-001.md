# TFINT-001: Test Infrastructure Scripts + Output Mapping

**Status:** DONE
**Phase:** 11 — Full Terraform Integration Testing

## Description

Create the test runner script and Docker image for running full terraform modules against simulators.

## Changes

- **`tests/terraform-integration/run-test.sh`** — Main orchestrator. Accepts backend name (ecs|lambda|cloudrun|gcf|aca|azf). Lifecycle: build simulator, start it, terragrunt apply, extract outputs, optionally start backend+frontend+act smoke test, terragrunt destroy, cleanup.
- **`tests/terraform-integration/Dockerfile`** — Docker image with Go 1.24, terraform 1.8.5, terragrunt 0.77.22, dnsmasq.
- Azure-specific: TLS cert generation (CA + server with wildcard SANs), dnsmasq for `*.localhost` DNS resolution.
- Output-to-env mapping for all 6 backends (SOCKERLESS_* env vars from terraform outputs).
