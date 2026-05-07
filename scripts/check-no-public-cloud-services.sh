#!/bin/bash
# Phase 122g hardening: enforce that no Cloud Run / Cloud Functions
# resources in terraform or backend Go specs grant invoke access to
# allUsers / allAuthenticatedUsers, AND that long-lived sockerless
# Services default to ingress=internal.
#
# User directive 2026-05-03: "all access must be authenticated and also
# authorized from specific github project". This lint codifies the
# no-public-exposure rule so a future PR can't regress it silently.
#
# Fails the commit / CI run if any of the patterns below appear outside
# explicitly-allowlisted lines.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

fail=0

# -------- Forbidden: allUsers / allAuthenticatedUsers as an invoker --
# `roles/run.invoker` granted to allUsers is the canonical way to
# expose a Cloud Run Service to the public internet without auth.
# Same pattern for Cloud Functions (`roles/cloudfunctions.invoker`).
matches=$(grep -rEn '"allUsers"|"allAuthenticatedUsers"' \
  --include='*.tf' --include='*.go' --include='*.yaml' --include='*.yml' \
  terraform/ backends/ 2>/dev/null | \
  grep -E 'run\.invoker|cloudfunctions\.invoker' || true)
if [ -n "$matches" ]; then
  echo "FAIL: forbidden allUsers/allAuthenticatedUsers grants to run.invoker / cloudfunctions.invoker found:"
  echo "$matches"
  fail=1
fi

# -------- Forbidden: ingress=all on long-lived sockerless services --
# The dispatcher + per-runner Cloud Run Services are outbound pollers;
# they must NOT have public ingress. Per-step short-lived sockerless-svc-*
# are created at runtime via Go and already default to internal — checked
# in backends/cloudrun/servicespec.go via TestBuildServiceSpec_Shape.
matches=$(grep -rEn 'ingress\s*=\s*"all"' \
  --include='*.tf' \
  terraform/modules/cloudrun/ terraform/modules/gcf/ 2>/dev/null || true)
if [ -n "$matches" ]; then
  echo "FAIL: terraform-managed Cloud Run / GCF services should default to ingress=internal:"
  echo "$matches"
  fail=1
fi

if [ $fail -ne 0 ]; then
  echo ""
  echo "Phase 122g rule: no Cloud Run / GCF service may be exposed to the public internet."
  echo "Either:"
  echo "  - Set ingress=internal (preferred — caller must reach via VPC connector + auth)"
  echo "  - Use Workload Identity Federation with attribute.repository=='e6qu/sockerless'"
  echo "    if you genuinely need GitHub-OIDC-mediated public ingress (open a phase task)"
  exit 1
fi

echo "no-public-cloud-services check: PASS"
