# Sim surface — aws-acm

Surface registered in `simulators/aws/acm.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CertificateManager.RequestCertificate` | ✓ `simulators/aws/acm.go:116::handleACMRequestCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.DescribeCertificate` | ✓ `simulators/aws/acm.go:117::handleACMDescribeCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.DeleteCertificate` | ✓ `simulators/aws/acm.go:118::handleACMDeleteCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ListCertificates` | ✓ `simulators/aws/acm.go:119::handleACMListCertificates` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.AddTagsToCertificate` | ✓ `simulators/aws/acm.go:120::handleACMAddTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.RemoveTagsFromCertificate` | ✓ `simulators/aws/acm.go:121::handleACMRemoveTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ListTagsForCertificate` | ✓ `simulators/aws/acm.go:122::handleACMListTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ImportCertificate` | ✓ `simulators/aws/acm.go:123::handleACMImportCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.UpdateCertificateOptions` | ✓ `simulators/aws/acm.go:124::handleACMUpdateOptions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ResendValidationEmail` | ✓ `simulators/aws/acm.go:125::handleACMResendValidationEmail` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.RenewCertificate` | ✓ `simulators/aws/acm.go:126::handleACMRenewCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
