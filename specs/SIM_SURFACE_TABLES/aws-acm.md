# Sim surface — aws-acm

Surface registered in `simulators/aws/acm.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CertificateManager.RequestCertificate` | ✓ `simulators/aws/acm.go:116::handleACMRequestCertificate` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.DescribeCertificate` | ✓ `simulators/aws/acm.go:117::handleACMDescribeCertificate` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.DeleteCertificate` | ✓ `simulators/aws/acm.go:118::handleACMDeleteCertificate` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.ListCertificates` | ✓ `simulators/aws/acm.go:119::handleACMListCertificates` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.AddTagsToCertificate` | ✓ `simulators/aws/acm.go:120::handleACMAddTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.RemoveTagsFromCertificate` | ✓ `simulators/aws/acm.go:121::handleACMRemoveTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.ListTagsForCertificate` | ✓ `simulators/aws/acm.go:122::handleACMListTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.ImportCertificate` | ✓ `simulators/aws/acm.go:123::handleACMImportCertificate` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.UpdateCertificateOptions` | ✓ `simulators/aws/acm.go:124::handleACMUpdateOptions` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.ResendValidationEmail` | ✓ `simulators/aws/acm.go:125::handleACMResendValidationEmail` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CertificateManager.RenewCertificate` | ✓ `simulators/aws/acm.go:126::handleACMRenewCertificate` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
