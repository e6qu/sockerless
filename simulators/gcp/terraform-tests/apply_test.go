package gcp_tf_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTerraformApplyDestroy provisions the full GCP-sim coverage stack
// (compute network + disk + subnet + firewall + regional address + Cloud NAT +
// global HTTP load balancer, public + private DNS zones, Artifact Registry,
// Cloud Run v2 Service + Job, Cloud Functions v2, Pub/Sub, Cloud Build trigger,
// Cloud Storage bucket + object, Cloud Logging sink/metric, BigQuery dataset/table, Firestore
// document, Eventarc trigger, Secret Manager, IAM service account) in a single
// terraform apply round-trip and asserts the cross-resource references
// converged.
//
// Slices exercised against the simulator:
//   - compute.googleapis.com (networks + disks + subnetworks + firewalls +
//     addresses + routers + router NATs + healthChecks + backendServices +
//     urlMaps + targetHttpProxies + globalForwardingRules)
//   - dns.googleapis.com (public + private managedZones)
//   - artifactregistry.googleapis.com (Docker repository)
//   - run.googleapis.com v2 (Service + Job)
//   - cloudfunctions.googleapis.com v2 (Function)
//   - eventarc.googleapis.com (Trigger)
//   - pubsub.googleapis.com (Topic + Subscription)
//   - cloudbuild.googleapis.com (Trigger)
//   - storage.googleapis.com (bucket + object)
//   - logging.googleapis.com (project sink + metric)
//   - bigquery.googleapis.com (dataset + table)
//   - firestore.googleapis.com (document)
//   - secretmanager.googleapis.com (secret + version)
//   - iam.googleapis.com (service account — via iam_beta_custom_endpoint;
//     terraform-provider-google routes the resource through iambeta.NewClient
//     which uses iam_beta_custom_endpoint, NOT iam_custom_endpoint)
func TestTerraformApplyDestroy(t *testing.T) {
	init := terraformCmd("init")
	init.Stdout = nil
	init.Stderr = nil
	out, err := init.CombinedOutput()
	require.NoError(t, err, "terraform init failed:\n%s", out)

	apply := terraformCmd("apply", "-auto-approve", "-var", "secret_label_env=dev")
	out, err = apply.CombinedOutput()
	require.NoError(t, err, "terraform apply failed:\n%s", out)

	outputs := readOutputs(t)

	diskLink := outputs.must(t, "compute_disk_self_link")
	require.Contains(t, diskLink, "/zones/us-central1-a/disks/tf-test-disk",
		"compute disk self_link must round-trip the zone+name; got %s", diskLink)

	arRepoID := outputs.must(t, "ar_repo_id")
	require.Contains(t, arRepoID, "projects/test-project/locations/us-central1/repositories/tf-ar-docker",
		"AR repo id must be the canonical projects/{p}/locations/{l}/repositories/{id} path; got %s", arRepoID)

	arRemoteRepoID := outputs.must(t, "ar_remote_repo_id")
	require.Contains(t, arRemoteRepoID, "projects/test-project/locations/us-central1/repositories/docker-hub",
		"AR remote repo id must be the canonical projects/{p}/locations/{l}/repositories/{id} path; got %s", arRemoteRepoID)

	crServiceURI := outputs.must(t, "cloud_run_v2_service_uri")
	// Real Cloud Run returns https://<service>-<hash>.<region>.run.app; the
	// sim returns its local invocation URL (http://host:port/v2-services-invoke/...).
	// Both must include the service name so callers can target it.
	require.Contains(t, crServiceURI, "tf-crv2-svc",
		"Cloud Run v2 service URI must reference the service name; got %s", crServiceURI)

	crJobID := outputs.must(t, "cloud_run_v2_job_id")
	require.Contains(t, crJobID, "projects/test-project/locations/us-central1/jobs/tf-crv2-job",
		"Cloud Run v2 job id must round-trip the full resource path; got %s", crJobID)

	gcfFunctionID := outputs.must(t, "cloudfunctions2_function_id")
	require.Contains(t, gcfFunctionID, "projects/test-project/locations/us-central1/functions/tf-gcfv2-function",
		"Cloud Functions v2 id must round-trip the full resource path; got %s", gcfFunctionID)

	eventarcID := outputs.must(t, "eventarc_trigger_id")
	require.Contains(t, eventarcID, "projects/test-project/locations/us-central1/triggers/tf-eventarc-trigger",
		"Eventarc trigger id must round-trip the full resource path; got %s", eventarcID)

	pubsubTopicID := outputs.must(t, "pubsub_topic_id")
	require.Equal(t, "projects/test-project/topics/tf-pubsub-topic", pubsubTopicID,
		"Pub/Sub topic id must be the canonical project topic path; got %s", pubsubTopicID)

	pubsubSubscriptionID := outputs.must(t, "pubsub_subscription_id")
	require.Equal(t, "projects/test-project/subscriptions/tf-pubsub-subscription", pubsubSubscriptionID,
		"Pub/Sub subscription id must be the canonical project subscription path; got %s", pubsubSubscriptionID)

	cloudBuildTriggerID := outputs.must(t, "cloudbuild_trigger_id")
	require.Contains(t, cloudBuildTriggerID, "projects/test-project/locations/us-central1/triggers/",
		"Cloud Build trigger id must include the regional trigger path; got %s", cloudBuildTriggerID)

	bucketURL := outputs.must(t, "storage_bucket_url")
	require.True(t, strings.HasPrefix(bucketURL, "gs://tf-test-bucket-"),
		"GCS bucket url must be a gs:// URL; got %s", bucketURL)

	logSinkID := outputs.must(t, "logging_project_sink_id")
	require.Contains(t, logSinkID, "projects/test-project/sinks/tf-log-sink",
		"Logging project sink id must include canonical project sink path; got %s", logSinkID)

	logMetricID := outputs.must(t, "logging_metric_id")
	require.Equal(t, "tf-log-metric", logMetricID,
		"Logging metric id must match terraform-provider-google's metric resource ID shape; got %s", logMetricID)

	secretVersionID := outputs.must(t, "secret_version_id")
	require.Contains(t, secretVersionID, "projects/test-project/secrets/tf-test-secret/versions/",
		"Secret version ID must include the canonical secret path; got %s", secretVersionID)

	secretLabelEnv := outputs.must(t, "secret_label_env")
	require.Equal(t, "dev", secretLabelEnv,
		"Secret Manager labels must round-trip through terraform state; got env=%s", secretLabelEnv)

	subnetID := outputs.must(t, "subnet_id")
	require.Contains(t, subnetID, "projects/test-project/regions/us-central1/subnetworks/tf-test-subnet",
		"subnet id must include the canonical region+name path; got %s", subnetID)

	firewallID := outputs.must(t, "firewall_id")
	require.Contains(t, firewallID, "projects/test-project/global/firewalls/tf-test-fw-allow-ssh",
		"firewall id must include the canonical global path; got %s", firewallID)

	natAddress := outputs.must(t, "nat_address")
	require.True(t, strings.HasPrefix(natAddress, "34."),
		"regional NAT address must receive an external IPv4 address; got %s", natAddress)

	routerNATName := outputs.must(t, "router_nat_name")
	require.Equal(t, "tf-router-nat", routerNATName,
		"router NAT name must round-trip through terraform state; got %s", routerNATName)

	lbHCID := outputs.must(t, "lb_health_check_id")
	require.Contains(t, lbHCID, "projects/test-project/global/healthChecks/tf-lb-hc",
		"load-balancer health check id must include the canonical global path; got %s", lbHCID)

	lbBackendID := outputs.must(t, "lb_backend_service_id")
	require.Contains(t, lbBackendID, "projects/test-project/global/backendServices/tf-lb-backend",
		"load-balancer backend service id must include the canonical global path; got %s", lbBackendID)

	lbURLMapID := outputs.must(t, "lb_url_map_id")
	require.Contains(t, lbURLMapID, "projects/test-project/global/urlMaps/tf-lb-url-map",
		"load-balancer URL map id must include the canonical global path; got %s", lbURLMapID)

	lbProxyID := outputs.must(t, "lb_target_http_proxy_id")
	require.Contains(t, lbProxyID, "projects/test-project/global/targetHttpProxies/tf-lb-http-proxy",
		"load-balancer target HTTP proxy id must include the canonical global path; got %s", lbProxyID)

	lbForwardingIP := outputs.must(t, "lb_forwarding_rule_ip")
	require.True(t, strings.HasPrefix(lbForwardingIP, "34."),
		"load-balancer global forwarding rule must receive an external IPv4 address; got %s", lbForwardingIP)

	gcsObjLink := outputs.must(t, "gcs_object_self_link")
	require.Contains(t, gcsObjLink, "tf-test-artifact.txt",
		"GCS object self_link must reference the object name; got %s", gcsObjLink)
	require.Contains(t, gcsObjLink, "tf-test-bucket-",
		"GCS object self_link must reference the parent bucket; got %s", gcsObjLink)

	saEmail := outputs.must(t, "service_account_email")
	require.Equal(t, "tf-test-runner-sa@test-project.iam.gserviceaccount.com", saEmail,
		"service-account email must match the canonical {account_id}@{project}.iam.gserviceaccount.com shape; got %s", saEmail)

	saName := outputs.must(t, "service_account_name")
	require.Equal(t, "projects/test-project/serviceAccounts/tf-test-runner-sa@test-project.iam.gserviceaccount.com", saName,
		"service-account name must include the canonical projects/{project}/serviceAccounts/{email} resource path; got %s", saName)

	bqTableID := outputs.must(t, "bigquery_table_id")
	require.Contains(t, bqTableID, "/datasets/tf_test_dataset/tables/events",
		"BigQuery table id must include dataset/table; got %s", bqTableID)

	fsDocName := outputs.must(t, "firestore_document_name")
	require.Contains(t, fsDocName, "projects/test-project/databases/(default)/documents/tf-users/alice",
		"Firestore document name must round-trip the canonical document path; got %s", fsDocName)

	destroy := terraformCmd("destroy", "-auto-approve", "-var", "secret_label_env=dev")
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
