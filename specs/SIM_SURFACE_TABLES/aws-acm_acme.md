# Sim surface — aws-acm_acme

Surface registered in `simulators/aws/acm_acme.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /acme/{endpoint}/directory` | ✓ `simulators/aws/acm_acme.go:196::handleACMEDataPlane` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `HEAD /acme/{endpoint}/new-nonce` | ✓ `simulators/aws/acm_acme.go:197::handleACMEDataPlane` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /acme/{endpoint}/new-nonce` | ✓ `simulators/aws/acm_acme.go:198::handleACMEDataPlane` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /acme/{endpoint}/{resource}` | ✓ `simulators/aws/acm_acme.go:199::handleACMEDataPlane` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /acme/{endpoint}/{resource}/{id}` | ✓ `simulators/aws/acm_acme.go:200::handleACMEDataPlane` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.CreateAcmeEndpoint` | ✓ `simulators/aws/acm_acme.go:176::handleACMCreateAcmeEndpoint` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.DeleteAcmeEndpoint` | ✓ `simulators/aws/acm_acme.go:177::handleACMDeleteAcmeEndpoint` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.DescribeAcmeEndpoint` | ✓ `simulators/aws/acm_acme.go:178::handleACMDescribeAcmeEndpoint` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.ListAcmeEndpoints` | ✓ `simulators/aws/acm_acme.go:179::handleACMListAcmeEndpoints` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.UpdateAcmeEndpoint` | ✓ `simulators/aws/acm_acme.go:180::handleACMUpdateAcmeEndpoint` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.CreateAcmeDomainValidation` | ✓ `simulators/aws/acm_acme.go:181::handleACMCreateAcmeDomainValidation` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.DeleteAcmeDomainValidation` | ✓ `simulators/aws/acm_acme.go:182::handleACMDeleteAcmeDomainValidation` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.DescribeAcmeDomainValidation` | ✓ `simulators/aws/acm_acme.go:183::handleACMDescribeAcmeDomainValidation` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.ListAcmeDomainValidations` | ✓ `simulators/aws/acm_acme.go:184::handleACMListAcmeDomainValidations` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.UpdateAcmeDomainValidation` | ✓ `simulators/aws/acm_acme.go:185::handleACMUpdateAcmeDomainValidation` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.CreateAcmeExternalAccountBinding` | ✓ `simulators/aws/acm_acme.go:186::handleACMCreateAcmeExternalAccountBinding` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.DeleteAcmeExternalAccountBinding` | ✓ `simulators/aws/acm_acme.go:187::handleACMDeleteAcmeExternalAccountBinding` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.DescribeAcmeExternalAccountBinding` | ✓ `simulators/aws/acm_acme.go:188::handleACMDescribeAcmeExternalAccountBinding` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.GetAcmeExternalAccountBindingCredentials` | ✓ `simulators/aws/acm_acme.go:189::handleACMGetAcmeExternalAccountBindingCredentials` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.ListAcmeExternalAccountBindings` | ✓ `simulators/aws/acm_acme.go:190::handleACMListAcmeExternalAccountBindings` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.RevokeAcmeExternalAccountBinding` | ✓ `simulators/aws/acm_acme.go:191::handleACMRevokeAcmeExternalAccountBinding` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.DescribeAcmeAccount` | ✓ `simulators/aws/acm_acme.go:192::handleACMDescribeAcmeAccount` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.ListAcmeAccounts` | ✓ `simulators/aws/acm_acme.go:193::handleACMListAcmeAccounts` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `Action CertificateManager.RevokeAcmeAccount` | ✓ `simulators/aws/acm_acme.go:194::handleACMRevokeAcmeAccount` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
