# DEPLOY-001: Create DEPLOYMENT.md

**Status:** DONE
**Phase:** 12 — Cloud Deployment Guide

## Description

Create a comprehensive deployment guide (`DEPLOYMENT.md`) documenting how to deploy each of the 6 backends to real cloud infrastructure using the terraform modules validated in Phase 11.

## What Was Done

Created `DEPLOYMENT.md` with:
- Architecture overview (frontend/backend/agent topology)
- Building binaries section (all 8 binaries + cross-compilation + agent Dockerfile)
- AWS section: ECS (21 resources) + Lambda (5 resources) with state bootstrap, deploy, validate
- GCP section: Cloud Run (13 resources) + GCF (7 resources) with state bootstrap, deploy, validate
- Azure section: ACA (18 resources) + AZF (11 resources) with state bootstrap, deploy, validate
- Complete terraform output → env var reference table for all 6 backends
- Validation section (quick test, act smoke test, health checks)
- Tear down section (terraform destroy + manual state backend cleanup)
- CI/CD integration patterns (GitHub Actions + GitLab CI)
- Cost estimates per cloud per backend

All env var mappings cross-checked against `tests/terraform-integration/run-test.sh` and `backends/*/config.go`.

## Files

- `DEPLOYMENT.md` — NEW (comprehensive deployment guide)
