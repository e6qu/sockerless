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
| `GET /acm/email-validation/{token}` | ✓ `simulators/aws/acm.go:255::handleACMEmailValidation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.RequestCertificate` | ✓ `simulators/aws/acm.go:257::handleACMRequestCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.DescribeCertificate` | ✓ `simulators/aws/acm.go:258::handleACMDescribeCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.DeleteCertificate` | ✓ `simulators/aws/acm.go:259::handleACMDeleteCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ListCertificates` | ✓ `simulators/aws/acm.go:260::handleACMListCertificates` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.AddTagsToCertificate` | ✓ `simulators/aws/acm.go:261::handleACMAddTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.RemoveTagsFromCertificate` | ✓ `simulators/aws/acm.go:262::handleACMRemoveTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ListTagsForCertificate` | ✓ `simulators/aws/acm.go:263::handleACMListTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ImportCertificate` | ✓ `simulators/aws/acm.go:264::handleACMImportCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.UpdateCertificateOptions` | ✓ `simulators/aws/acm.go:265::handleACMUpdateOptions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ResendValidationEmail` | ✓ `simulators/aws/acm.go:266::handleACMResendValidationEmail` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.RenewCertificate` | ✓ `simulators/aws/acm.go:267::handleACMRenewCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.GetCertificate` | ✓ `simulators/aws/acm.go:268::handleACMGetCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ExportCertificate` | ✓ `simulators/aws/acm.go:269::handleACMExportCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.RevokeCertificate` | ✓ `simulators/aws/acm.go:270::handleACMRevokeCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.GetAccountConfiguration` | ✓ `simulators/aws/acm.go:271::handleACMGetAccountConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.PutAccountConfiguration` | ✓ `simulators/aws/acm.go:272::handleACMPutAccountConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.SearchCertificates` | ✓ `simulators/aws/acm.go:273::handleACMSearchCertificates` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.TagResource` | ✓ `simulators/aws/acm.go:274::handleACMTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.UntagResource` | ✓ `simulators/aws/acm.go:275::handleACMUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CertificateManager.ListTagsForResource` | ✓ `simulators/aws/acm.go:276::handleACMListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
