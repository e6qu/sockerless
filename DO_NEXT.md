# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/ratchet-up-14` — **gate-audit: measure + drive IAM to 100% + boyscout fixes (BUG-2213).** Audited the conformance gate against every vendored Smithy model + cross-referenced what the sim implements, all spec-validated (0 divergences) + real SDK/CLI.

- **The audit's verdict:** of the 38 vendored `aws-sdk-go-v2` Smithy models, **37 were already gated — IAM (`AWSIdentityManagementV20100508`) was the only unmeasured AWS service** (implemented via awsQuery at 74/176 but never in the gate). The other non-modeled register funcs (Dashboard/DDBPartiQL/HostMetadata/MulticastGroup/UI) are sim-internal or parts of already-gated services.
- **IAM ratcheted 74→176/176 (100%) and gated** in `serviceCoverageFloor` (awsQuery, unversioned, like STS). The 102 missing ops, across 5 agents: role permissions-boundary + instance-profile tags + managed-policy versions/entities/context-keys; UpdateUser/Group + login profiles + access-key-last-used + user tags + account-password-policy; virtual MFA devices + SSH keys + signing certs + service-specific creds; SAML/OIDC providers + server certs + account aliases; credential report + account-summary/auth-details + service-last-accessed + organizations-root + delegation requests + outbound-web-identity-federation.
- **Twenty-seven AWS services now at 100%.**
- **Boyscout (BUG-2212):** a focused audit of the older IAM slice found 5 real fail-loud gaps, all fixed: PutRolePolicy + UpdateAssumeRolePolicy stored the policy/trust document without `parseIAMPolicy` (now raise `MalformedPolicyDocument`); DeleteUser / DeleteRole / DeleteInstanceProfile deleted principals that still had attachments (access keys / inline+attached policies / group memberships / login profile; inline+attached policies / instance-profile membership; an attached role) instead of raising `DeleteConflict` — now they fail loud, verified regression-free across the full IAM + cross-service suites (existing tests already detach before delete). Also extended `ListPolicyVersions` to enumerate the non-default versions CreatePolicyVersion adds (it had hardcoded a single default version). Two findings classified non-bugs (a dead var; idiomatic `crypto/rand.Read` error ignore).
- Tests: aws sim/sdk/cli build/lint(0)/unit green; contract + cli-shard + all conformance + IAM spec-validator (0 divergences) pass; new IAM CLI tests green vs latest `aws` CLI (0 Invalid-choice).

**Next candidates:** the gate audit is complete — every vendored Smithy model is gated, and nearly every measured service is at its faithful max (the only gaps are dedicated-endpoint / SaaS-connector ops the regional sim can't host: S3 WriteGetObjectResponse + the 2 S3 Express ops; Glue's 3 connector ops). The conformance-ratchet arc is essentially closed. Highest-value next work: the **live-cloud track (BUG-1075)** — validate Cloud Run Services / ACA Apps / AZF cloud-DNS / Lambda service-mesh / ACA-AZF Azure AD against authenticated real clouds — or vendor + gate any *new* AWS service slice the project adds. Open GitHub issues: #394 (azuread upstream-blocked).

## Working agreement

The full before/after-task continuity-file workflow, the no-fakes rules, and branch/PR hygiene live in [AGENTS.md](AGENTS.md). In short: read `STATUS.md`/`DO_NEXT.md` first; run the narrowest meaningful tests for the touched area; file bugs before fixing; update the continuity files in the same commit as the code; rebase on `origin/main` before pushing; never merge the PR.

Narrowest-test recipes for the common surfaces:

```bash
# Simulator SDK probe
cd simulators/<cloud>/sdk-tests && GOWORK=off CGO_ENABLED=0 go test -tags noui -run '<pat>' -timeout 15m .
# Simulator module unit tests + lint
cd simulators/<cloud> && make unit-test
# A backend's unit tests
cd backends/<name> && GOWORK=off go test ./...
# bleephub runner topology harness (self-contained)
make bleephub-runner-docker-test
```
