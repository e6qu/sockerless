// Package gcpcells holds the Phase 120 live-GCP runner cell harness.
// The cell tests are build-tag-gated under `gcp_runner_live` so the
// default `go test ./...` doesn't try to dispatch real workflows;
// see harness_test.go for the per-cell test bodies and
// manual-tests/04-gcp-runner-cells.md for the operator runbook.
package gcpcells
