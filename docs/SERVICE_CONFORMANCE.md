# Service conformance — a repeatable process for "is this slice actually complete?"

This is the method for taking any simulator service slice from "we think it works"
to a **measured, enforced conformance number**, so gaps are visible and an agent
(human or LLM) can fix them mechanically instead of a consumer discovering them.

It exists because "100% coverage" claims kept being wrong: audits read the *code*
(and inherited its blind spots) and graded themselves, with no authoritative
checklist and no external oracle. The two techniques that actually work — an
**enumerated checklist from spec** and a **differential against an external
oracle** — are the backbone here. The AWS Identity and Access Management (IAM) policy engine is the worked example
(`simulators/aws/iam_conformance_test.go`, `testdata/iam_conformance_vectors.json`,
`sdk-tests/iam_conformance_differential_test.go`).

## The protocol

1. **Enumerate the authoritative surface, from the spec, into a checklist.**
   Pull the complete list from the canonical source — the SDK/botocore model, the
   service's API reference, the policy grammar reference — not from memory or the
   existing code. For a policy engine that's every condition operator, every
   global *and* service condition key, policy variables, Principal forms, the
   evaluation algorithm. The checklist *is* the definition of 100%. Encode it as
   data: a catalog of `{name, supported bool, …}` rows. Each row is `supported`
   with a test, or explicitly `supported:false` with a reason — never silently
   absent.

2. **Coverage = checklist rows backed by an assertion, reported as a fraction.**
   For every `supported` row, a probe/test must exercise it (e.g. the IAM operator
   catalog carries a probe vector per operator; `TestIAMConformance_Operators`
   asserts each one evaluates, and that every *unsupported* one safely no-matches
   rather than silently granting). Report "N/M supported", not "audit found
   nothing".

3. **A ratchet locks the gap set.** A test asserts the count/set of `supported:false`
   rows equals a checked-in number (`TestIAMConformance_Ratchet`). Implement a gap →
   flip the row + decrement; add a newly-discovered spec item → it fails until you
   classify it. The failure message *is* the live non-conformity report. This is
   what stops a half-done thing from being re-declared "done".

4. **A golden corpus of end-to-end vectors, in one shared file.**
   `(policy/request, context) → expected outcome` cases live in
   `testdata/<svc>_conformance_vectors.json`, derived from the spec's own examples
   and real consumer artifacts. The in-process gate runs them through the engine
   directly; the differential (below) runs the *same* file through the public API.

5. **A differential against an external oracle — coordinates-only.**
   Run the corpus through the real public API (`SimulateCustomPolicy` for IAM) and
   compare to the expected outcome. The test differs only in *coordinates*: by
   default it hits the **sim** (proving the public wire path agrees with the
   engine); point it at the real cloud (`SOCKERLESS_IAM_ORACLE=aws`, ambient creds)
   and the same assertions validate the corpus against **ground truth**. This is
   the layer that catches silent-wrong — exactly as the DynamoDB/Cosmos/Firestore
   emulator differentials do for data-plane CRUD.

6. **Drive from real consumer artifacts.** The fastest path to real coverage is to
   take the consumer's actual policies/requests and assert every operator, key, and
   action in them is honored. Real usage beats imagination as a completeness oracle
   (issue #661 — `aws:ResourceTag`/`ecs:cluster` — came from a real consumer, not us).

7. **Adversarial pass against the spec, not the code.** A reviewer whose only inputs
   are the checklist and the test list, asked "which checklist rows have no test?".
   Reading-the-code reviews inherit the code's blind spots; this doesn't.

8. **Honest language.** Retire "100%/complete". Say "covers X; not yet: Y (listed in
   the registry, ratcheted)". The number is the claim.

## Measuring AWS service operation coverage

`simulators/aws/service_conformance_test.go` measures every AWS service against its
vendored Smithy model. How a service's *implemented* op set is read depends on its
wire protocol:

- **awsJson (`X-Amz-Target`)** — read from the JSON router's registered targets.
- **awsQuery / ec2Query (`Action=…`)** — read from the query router. The versioned
  buckets are keyed by each service's own API `Version`, so they attribute
  themselves. The *unversioned* `Register` form (Amazon EC2, AWS IAM, AWS STS, and
  Amazon CloudWatch's query surface) lands in the router's `""` bucket, which carries
  no service identity at all; ownership there is *measured* by running each service's
  registration entry point (`legacyQueryRegistrars`) against a fresh router and
  recording what it mounts. An action no registrar claims is credited to no service —
  never to every service, which would let one service's actions count as another's
  coverage. `TestServiceConformance_LegacyQueryAttribution` fails when an action in
  that bucket has no owner, and `TestServiceConformance_LegacyQueryShadowing` fails
  when a versioned service leaves an operation to be answered by another service's
  unversioned handler.
- **REST (path + method)** — the op name is a constant passed to
  `cloudTrailRecordedREST("Op", "<source>.amazonaws.com", …)` at registration, so it
  is captured in the `restRegisteredOps` registry (keyed by CloudTrail event source)
  and read back via `restConformanceSources`. Amazon S3 is the exception: its op name
  is composed dynamically from method + path + subresource, so it has a bespoke
  enumeration harness (`s3ImplementedOps`).

Two ratchet shapes lock the result:

- **Exact missing-list** (`serviceConformanceCatalog`, `TestServiceConformance_Ratchet`)
  — the precise set of unimplemented ops, for services tracked op-by-op. Implement one
  → remove it; the model grows → it's flagged as newly-missing.
- **Coverage floor** (`serviceCoverageFloor`, `TestServiceConformance_CoverageFloor`)
  — the implemented-op *count*, for the awsQuery/ec2Query giants (EC2 769 ops, RDS,
  Glue, …) and the REST services, where an exact list of hundreds of gaps would bloat
  the catalog. The count must *equal* the floor: a drop is a regression; implementing
  more ratchets the floor up.

## Applying it to a new service

- Add `simulators/<cloud>/<svc>_conformance_test.go` with the catalog(s) + the three
  tests (coverage, golden-corpus, ratchet).
- Add `testdata/<svc>_conformance_vectors.json` (shared corpus).
- Add `sdk-tests/<svc>_conformance_differential_test.go` driving the public API,
  defaulting to the sim, switchable to the real cloud by an `..._ORACLE` env var.
- The ratchet's non-conformity list is the work queue: each `supported:false` /
  `populated:false` row is a discrete, mechanically-fixable task.
