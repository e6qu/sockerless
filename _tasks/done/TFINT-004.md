# TFINT-004: Azure Terraform Integration (ACA + AZF)

**Status:** DONE
**Phase:** 11 — Full Terraform Integration Testing

## Description

Run the full ACA and AZF terraform modules against the Azure simulator. Fix simulator API gaps.

## Results

- **ACA:** 18 resources apply + destroy cleanly
- **AZF:** 11 resources apply + destroy cleanly

## Simulator Fixes

- Container App Environment: `customDomainConfiguration` + `peerAuthentication` fields (nil pointer crash in provider)
- Storage data plane: Subdomain routing via dnsmasq + TLS wildcard SANs + Host-header middleware
- Storage share: Data plane existence checks (404 before create), ACL support
- Workspace: `features`/`sku` struct fields, `sharedKeys` endpoint
- App Service Plan: Dual casing (`serverFarms` / `serverfarms`) for go-azure-sdk compatibility
- Application Insights: Billing features GET/PUT endpoint, 200 status for creates
- Metadata: `suffixes.storage` includes port for URL parsing
