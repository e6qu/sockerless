package gcp_tf_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTerraformApplyDestroy provisions the full GCP-sim coverage stack
// (compute network + disk, public + private DNS zones, Artifact Registry,
// Cloud Run v2 Service + Job, Cloud Storage, Secret Manager) in a single
// terraform apply round-trip and asserts the cross-resource references
// converged.
//
// Slices exercised against the simulator:
//   - compute.googleapis.com (networks + disks)
//   - dns.googleapis.com (public + private managedZones)
//   - artifactregistry.googleapis.com (Docker repository)
//   - run.googleapis.com v2 (Service + Job)
//   - storage.googleapis.com (bucket)
//   - secretmanager.googleapis.com (secret + version)
func TestTerraformApplyDestroy(t *testing.T) {
	init := terraformCmd("init")
	init.Stdout = nil
	init.Stderr = nil
	out, err := init.CombinedOutput()
	require.NoError(t, err, "terraform init failed:\n%s", out)

	apply := terraformCmd("apply", "-auto-approve")
	out, err = apply.CombinedOutput()
	require.NoError(t, err, "terraform apply failed:\n%s", out)

	outputs := readOutputs(t)

	diskLink := outputs.must(t, "compute_disk_self_link")
	require.Contains(t, diskLink, "/zones/us-central1-a/disks/tf-test-disk",
		"compute disk self_link must round-trip the zone+name; got %s", diskLink)

	arRepoID := outputs.must(t, "ar_repo_id")
	require.Contains(t, arRepoID, "projects/test-project/locations/us-central1/repositories/tf-ar-docker",
		"AR repo id must be the canonical projects/{p}/locations/{l}/repositories/{id} path; got %s", arRepoID)

	crServiceURI := outputs.must(t, "cloud_run_v2_service_uri")
	// Real Cloud Run returns https://<service>-<hash>.<region>.run.app; the
	// sim returns its local invocation URL (http://host:port/v2-services-invoke/...).
	// Both must include the service name so callers can target it.
	require.Contains(t, crServiceURI, "tf-crv2-svc",
		"Cloud Run v2 service URI must reference the service name; got %s", crServiceURI)

	crJobID := outputs.must(t, "cloud_run_v2_job_id")
	require.Contains(t, crJobID, "projects/test-project/locations/us-central1/jobs/tf-crv2-job",
		"Cloud Run v2 job id must round-trip the full resource path; got %s", crJobID)

	bucketURL := outputs.must(t, "storage_bucket_url")
	require.True(t, strings.HasPrefix(bucketURL, "gs://tf-test-bucket-"),
		"GCS bucket url must be a gs:// URL; got %s", bucketURL)

	secretVersionID := outputs.must(t, "secret_version_id")
	require.Contains(t, secretVersionID, "projects/test-project/secrets/tf-test-secret/versions/",
		"Secret version ID must include the canonical secret path; got %s", secretVersionID)

	destroy := terraformCmd("destroy", "-auto-approve")
	out, err = destroy.CombinedOutput()
	require.NoError(t, err, "terraform destroy failed:\n%s", out)
}

type tfOutputs map[string]struct {
	Sensitive bool        `json:"sensitive"`
	Type      interface{} `json:"type"`
	Value     interface{} `json:"value"`
}

func (o tfOutputs) must(t *testing.T, key string) string {
	t.Helper()
	v, ok := o[key]
	require.True(t, ok, "output %q missing from terraform state", key)
	s, ok := v.Value.(string)
	require.True(t, ok, "output %q is not a string (got %T)", key, v.Value)
	require.NotEmpty(t, s, "output %q is empty", key)
	return s
}

func readOutputs(t *testing.T) tfOutputs {
	t.Helper()
	out, err := terraformCmd("output", "-json").CombinedOutput()
	require.NoError(t, err, "terraform output failed:\n%s", out)
	var outputs tfOutputs
	require.NoError(t, json.Unmarshal(out, &outputs))
	return outputs
}
