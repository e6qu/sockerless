---
name: adaptor-fidelity-check
description: Verify a sockerless component change against its real reference adaptor (docker CLI / aws CLI / gcloud / az / Terraform / gh CLI / Docker SDK). Use whenever editing files under backends/, simulators/, bleephub/, or anything that other tools speak to over the wire. Catches drift between what we implement and what real adaptors send.
---

# Adaptor-fidelity check

Every sockerless component is paired with an external **reference adaptor** (per `docs/RUNNERS.md`, `bleephub/README.md`, and the per-backend READMEs). The adaptor is the validation harness, the user-facing utility, and the source of truth for what "correct" means. This skill enforces that pairing.

## When this skill applies

| Component | Adaptor(s) |
|---|---|
| `backends/docker` | docker CLI, podman CLI, Docker Go SDK |
| `backends/ecs` | docker CLI/SDK + aws CLI/SDK + Terraform aws |
| `backends/lambda` | docker CLI/SDK + aws CLI/SDK + Terraform aws |
| `backends/cloudrun` | docker CLI/SDK + gcloud + GCP Go SDK + Terraform google |
| `backends/cloudrun-functions` | docker CLI/SDK + gcloud + Go SDK + Terraform google |
| `backends/aca` | docker CLI/SDK + az + Azure Go SDK + Terraform azurerm |
| `backends/azure-functions` | docker CLI/SDK + az + Azure Go SDK + Terraform azurerm |
| `simulators/aws` | aws CLI + AWS Go SDK + Terraform aws |
| `simulators/gcp` | gcloud + GCP Go SDK + Terraform google |
| `simulators/azure` | az + Azure Go SDK + Terraform azurerm |
| `bleephub` | gh CLI + smart-HTTP git + `actions/runner` |

## The check

Before you commit any change to a wire-facing handler or response shape, answer all six:

### 1. Identify the request shape the real adaptor sends

Run the real adaptor against a known-good upstream (real cloud, real `github.com`, real Docker daemon) with verbose logging:

```bash
docker --debug run ...
aws --debug ec2 describe-instances 2>&1
gcloud --log-http run jobs list
az --debug containerapp env list
gh api --include /repos/admin/demo
terraform plan -refresh=true  # then inspect .terraform/plugin*.log
```

Copy the exact path + method + headers + body the adaptor emits. **This is the spec.** Not what the model thinks; what the adaptor actually does.

### 2. Diff against the sockerless handler

Open the corresponding handler in the codebase. Compare field-by-field:
- Path template (`POST /v1.44/containers/{id}/wait` — case-sensitive).
- Query params (`condition=removed` vs `condition=next-exit`).
- Body shape (`{"private": "false"}` (string!) vs `{"private": false}`).
- Response headers (`Content-Type: application/json; charset=utf-8` — yes, charset matters).
- Response shape (HATEOAS URLs, optional fields, null vs missing).
- Status codes (400 vs 422 vs 409 for conflict).

If any of these differs from the adaptor's emission, that's a real bug. File it in BUGS.md before fixing.

### 3. Round-trip a test through the real adaptor

A test that doesn't drive the real adaptor doesn't count. Examples that DO count:

- `bleephub/test/run-gh-test.sh` — real `gh` binary against running bleephub in Docker.
- `tests/` — real Docker Go SDK against running backend.
- `simulators/aws/sdk-tests/` — real AWS Go SDK against running simulator.
- `simulators/<cloud>/terraform-tests/` — real Terraform provider against running simulator.

Tests that DON'T count:

- Mocked-everything tests where the adaptor never speaks to the binary.
- "Manual integration" tests that aren't in a Makefile target or CI.
- Tests against fixtures captured once and never re-validated.

### 4. Check the cross-cloud invariant

If you found a bug in one backend's handler, check the same handler shape in:
- The other backends in the same cloud (Pattern B: shared in `*-common`).
- The other clouds.
- The simulator handler if applicable.

Repeat per `MEMORY.md` § cross-cloud-sweep. Fix all in the same commit.

### 5. Confirm the change preserves the contract

After your edit, re-run the real adaptor (step 1) against your modified sockerless. Does the adaptor's behaviour against sockerless match its behaviour against the real upstream?

If not, the change is wrong. Iterate.

### 6. Document the contract

Update the component's README (per Phase 157 doc shape):
- Reference adaptor + min version.
- Validation: test path + last-green date.
- Sample command + **real captured output** (run it, paste it, don't guess).
- Out-of-scope: what the adaptor exercises that you didn't implement.

## Failure modes this skill catches

- "I'll mock the cloud API to test this" — pattern 2.
- "The aws SDK probably sends `Action=DescribeTasks` as a query param" — pattern 6, verify don't guess.
- "Looks like the test passes" — but the test uses fixtures captured 8 months ago, not a live `gh` call (pattern 3).
- "I only changed the response shape for ECS; should be fine" — but `lambda` and `aca` share the same handler (pattern 12).
- "I'll add a `null` check in the parser" — when the real adaptor never sends null in that field (pattern 22).

## Quick references

- `docs/VIBE_CODING.md` — full anti-pattern catalogue.
- `docs/RUNNERS.md` — runner ↔ sockerless wiring guide.
- `specs/BLEEPHUB_GITHUB_API_PARITY.md` — GitHub API contract for bleephub.
- `specs/CLOUD_RESOURCE_MAPPING.md` — Docker→cloud mapping per backend.
- Per-component README — adaptor + validation + wiring + sample.

## Output

When this skill fires, name the reference adaptor in one sentence ("docker SDK / aws CLI / gcloud / gh CLI"), the validation entry point (test path), and what you're about to verify. Then verify. Don't proceed to writing code until step 1 (real adaptor request shape) is captured.
