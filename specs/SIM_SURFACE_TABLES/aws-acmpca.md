# Sim surface — aws-acmpca

Surface registered in `simulators/aws/acmpca.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action ACMPrivateCA.CreateCertificateAuthority` | ✓ `simulators/aws/acmpca.go:120::handlePrivateCACreate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.CreateCertificateAuthorityAuditReport` | ✓ `simulators/aws/acmpca.go:121::handlePrivateCACreateAudit` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.CreatePermission` | ✓ `simulators/aws/acmpca.go:122::handlePrivateCACreatePermission` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.DeleteCertificateAuthority` | ✓ `simulators/aws/acmpca.go:123::handlePrivateCADelete` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.DeletePermission` | ✓ `simulators/aws/acmpca.go:124::handlePrivateCADeletePermission` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.DeletePolicy` | ✓ `simulators/aws/acmpca.go:125::handlePrivateCADeletePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.DescribeCertificateAuthority` | ✓ `simulators/aws/acmpca.go:126::handlePrivateCADescribe` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.DescribeCertificateAuthorityAuditReport` | ✓ `simulators/aws/acmpca.go:127::handlePrivateCADescribeAudit` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.GetCertificate` | ✓ `simulators/aws/acmpca.go:128::handlePrivateCAGetCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.GetCertificateAuthorityCertificate` | ✓ `simulators/aws/acmpca.go:129::handlePrivateCAGetAuthorityCertificate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.GetCertificateAuthorityCsr` | ✓ `simulators/aws/acmpca.go:130::handlePrivateCAGetCSR` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.GetPolicy` | ✓ `simulators/aws/acmpca.go:131::handlePrivateCAGetPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.ImportCertificateAuthorityCertificate` | ✓ `simulators/aws/acmpca.go:132::handlePrivateCAImport` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.IssueCertificate` | ✓ `simulators/aws/acmpca.go:133::handlePrivateCAIssue` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.ListCertificateAuthorities` | ✓ `simulators/aws/acmpca.go:134::handlePrivateCAList` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.ListPermissions` | ✓ `simulators/aws/acmpca.go:135::handlePrivateCAListPermissions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.ListTags` | ✓ `simulators/aws/acmpca.go:136::handlePrivateCAListTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.PutPolicy` | ✓ `simulators/aws/acmpca.go:137::handlePrivateCAPutPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.RestoreCertificateAuthority` | ✓ `simulators/aws/acmpca.go:138::handlePrivateCARestore` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.RevokeCertificate` | ✓ `simulators/aws/acmpca.go:139::handlePrivateCARevoke` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.TagCertificateAuthority` | ✓ `simulators/aws/acmpca.go:140::handlePrivateCATag` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.UntagCertificateAuthority` | ✓ `simulators/aws/acmpca.go:141::handlePrivateCAUntag` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ACMPrivateCA.UpdateCertificateAuthority` | ✓ `simulators/aws/acmpca.go:142::handlePrivateCAUpdate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
