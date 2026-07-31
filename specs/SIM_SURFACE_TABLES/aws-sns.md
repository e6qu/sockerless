# Sim surface — aws-sns

Surface registered in `simulators/aws/sns.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateTopic` | ✓ `simulators/aws/sns.go:84::handleSNSCreateTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteTopic` | ✓ `simulators/aws/sns.go:85::handleSNSDeleteTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListTopics` | ✓ `simulators/aws/sns.go:86::handleSNSListTopics` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetTopicAttributes` | ✓ `simulators/aws/sns.go:87::handleSNSGetTopicAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetTopicAttributes` | ✓ `simulators/aws/sns.go:88::handleSNSSetTopicAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Subscribe` | ✓ `simulators/aws/sns.go:89::handleSNSSubscribe` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Unsubscribe` | ✓ `simulators/aws/sns.go:90::handleSNSUnsubscribe` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ConfirmSubscription` | ✓ `simulators/aws/sns.go:91::handleSNSConfirmSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetSubscriptionAttributes` | ✓ `simulators/aws/sns.go:92::handleSNSGetSubscriptionAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetSubscriptionAttributes` | ✓ `simulators/aws/sns.go:93::handleSNSSetSubscriptionAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListSubscriptions` | ✓ `simulators/aws/sns.go:94::handleSNSListSubscriptions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListSubscriptionsByTopic` | ✓ `simulators/aws/sns.go:95::handleSNSListSubscriptionsByTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddPermission` | ✓ `simulators/aws/sns.go:96::handleSNSAddPermission` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemovePermission` | ✓ `simulators/aws/sns.go:97::handleSNSRemovePermission` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Publish` | ✓ `simulators/aws/sns.go:98::handleSNSPublish` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PublishBatch` | ✓ `simulators/aws/sns.go:99::handleSNSPublishBatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TagResource` | ✓ `simulators/aws/sns.go:100::handleSNSTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UntagResource` | ✓ `simulators/aws/sns.go:101::handleSNSUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListTagsForResource` | ✓ `simulators/aws/sns.go:102::handleSNSListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreatePlatformApplication` | ✓ `simulators/aws/sns_mobile_sms.go:104::handleSNSCreatePlatformApplication` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeletePlatformApplication` | ✓ `simulators/aws/sns_mobile_sms.go:105::handleSNSDeletePlatformApplication` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetPlatformApplicationAttributes` | ✓ `simulators/aws/sns_mobile_sms.go:106::handleSNSGetPlatformApplicationAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetPlatformApplicationAttributes` | ✓ `simulators/aws/sns_mobile_sms.go:107::handleSNSSetPlatformApplicationAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListPlatformApplications` | ✓ `simulators/aws/sns_mobile_sms.go:108::handleSNSListPlatformApplications` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreatePlatformEndpoint` | ✓ `simulators/aws/sns_mobile_sms.go:111::handleSNSCreatePlatformEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteEndpoint` | ✓ `simulators/aws/sns_mobile_sms.go:112::handleSNSDeleteEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetEndpointAttributes` | ✓ `simulators/aws/sns_mobile_sms.go:113::handleSNSGetEndpointAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetEndpointAttributes` | ✓ `simulators/aws/sns_mobile_sms.go:114::handleSNSSetEndpointAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListEndpointsByPlatformApplication` | ✓ `simulators/aws/sns_mobile_sms.go:115::handleSNSListEndpointsByPlatformApplication` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateSMSSandboxPhoneNumber` | ✓ `simulators/aws/sns_mobile_sms.go:118::handleSNSCreateSMSSandboxPhoneNumber` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteSMSSandboxPhoneNumber` | ✓ `simulators/aws/sns_mobile_sms.go:119::handleSNSDeleteSMSSandboxPhoneNumber` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action VerifySMSSandboxPhoneNumber` | ✓ `simulators/aws/sns_mobile_sms.go:120::handleSNSVerifySMSSandboxPhoneNumber` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListSMSSandboxPhoneNumbers` | ✓ `simulators/aws/sns_mobile_sms.go:121::handleSNSListSMSSandboxPhoneNumbers` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetSMSSandboxAccountStatus` | ✓ `simulators/aws/sns_mobile_sms.go:122::handleSNSGetSMSSandboxAccountStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetSMSAttributes` | ✓ `simulators/aws/sns_mobile_sms.go:125::handleSNSGetSMSAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetSMSAttributes` | ✓ `simulators/aws/sns_mobile_sms.go:126::handleSNSSetSMSAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CheckIfPhoneNumberIsOptedOut` | ✓ `simulators/aws/sns_mobile_sms.go:127::handleSNSCheckIfPhoneNumberIsOptedOut` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListPhoneNumbersOptedOut` | ✓ `simulators/aws/sns_mobile_sms.go:128::handleSNSListPhoneNumbersOptedOut` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action OptInPhoneNumber` | ✓ `simulators/aws/sns_mobile_sms.go:129::handleSNSOptInPhoneNumber` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListOriginationNumbers` | ✓ `simulators/aws/sns_mobile_sms.go:130::handleSNSListOriginationNumbers` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutDataProtectionPolicy` | ✓ `simulators/aws/sns_mobile_sms.go:133::handleSNSPutDataProtectionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetDataProtectionPolicy` | ✓ `simulators/aws/sns_mobile_sms.go:134::handleSNSGetDataProtectionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
