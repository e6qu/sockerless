package gcp_sdk_test

import (
	"errors"
	"testing"

	eventarc "cloud.google.com/go/eventarc/apiv1"
	"cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	artifactregistry "google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/bigquery/v2"
	"google.golang.org/api/cloudresourcemanager/v1"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"
	logging "google.golang.org/api/logging/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/pubsub/v1"
	secretmanager "google.golang.org/api/secretmanager/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// These tests assert that writable fields accepted on create/patch survive a
// read back through the real Google client libraries. A field dropped on
// get/list produces perpetual terraform-provider-google drift; each case below
// drives the canonical client a consumer would use.

func TestConformance_DNSManagedZoneLabelsRoundTrip(t *testing.T) {
	svc, err := dns.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)

	created, err := svc.ManagedZones.Create("conformance-project", &dns.ManagedZone{
		Name:    "conformance-labels-zone",
		DnsName: "conformance.example.com.",
		Labels:  map[string]string{"env": "prod", "team": "platform"},
		DnssecConfig: &dns.ManagedZoneDnsSecConfig{
			State: "on",
		},
	}).Do()
	require.NoError(t, err)
	require.Equal(t, map[string]string{"env": "prod", "team": "platform"}, created.Labels)

	got, err := svc.ManagedZones.Get("conformance-project", "conformance-labels-zone").Do()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod", "team": "platform"}, got.Labels)
	require.NotNil(t, got.DnssecConfig)
	assert.Equal(t, "on", got.DnssecConfig.State)
}

func TestConformance_BigQueryTablePartitioningRoundTrip(t *testing.T) {
	svc, err := bigquery.NewService(ctx, option.WithEndpoint(baseURL+"/bigquery/v2/"), option.WithoutAuthentication())
	require.NoError(t, err)
	proj := "conformance-project"

	_, err = svc.Datasets.Insert(proj, &bigquery.Dataset{
		DatasetReference: &bigquery.DatasetReference{ProjectId: proj, DatasetId: "conf_part_ds"},
	}).Do()
	require.NoError(t, err)

	requireFilter := true
	created, err := svc.Tables.Insert(proj, "conf_part_ds", &bigquery.Table{
		TableReference: &bigquery.TableReference{ProjectId: proj, DatasetId: "conf_part_ds", TableId: "events"},
		Schema: &bigquery.TableSchema{Fields: []*bigquery.TableFieldSchema{
			{Name: "ts", Type: "TIMESTAMP"},
			{Name: "id", Type: "STRING"},
		}},
		TimePartitioning:       &bigquery.TimePartitioning{Type: "DAY", Field: "ts"},
		Clustering:             &bigquery.Clustering{Fields: []string{"id"}},
		RequirePartitionFilter: requireFilter,
	}).Do()
	require.NoError(t, err)
	require.NotNil(t, created.TimePartitioning)

	got, err := svc.Tables.Get(proj, "conf_part_ds", "events").Do()
	require.NoError(t, err)
	require.NotNil(t, got.TimePartitioning, "timePartitioning dropped on get")
	assert.Equal(t, "DAY", got.TimePartitioning.Type)
	assert.Equal(t, "ts", got.TimePartitioning.Field)
	require.NotNil(t, got.Clustering)
	assert.Equal(t, []string{"id"}, got.Clustering.Fields)
	assert.True(t, got.RequirePartitionFilter)
}

func TestConformance_PubSubTopicSchemaSettingsRoundTrip(t *testing.T) {
	svc, err := pubsub.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	name := "projects/conformance-project/topics/conf-schema-topic"

	created, err := svc.Projects.Topics.Create(name, &pubsub.Topic{
		MessageRetentionDuration: "86400s",
		SchemaSettings: &pubsub.SchemaSettings{
			Schema:   "projects/conformance-project/schemas/_deleted-schema_",
			Encoding: "JSON",
		},
		Labels: map[string]string{"env": "prod"},
	}).Do()
	require.NoError(t, err)
	require.NotNil(t, created.SchemaSettings)

	got, err := svc.Projects.Topics.Get(name).Do()
	require.NoError(t, err)
	assert.Equal(t, "86400s", got.MessageRetentionDuration)
	require.NotNil(t, got.SchemaSettings, "schemaSettings dropped on get")
	assert.Equal(t, "JSON", got.SchemaSettings.Encoding)
	assert.Equal(t, "projects/conformance-project/schemas/_deleted-schema_", got.SchemaSettings.Schema)
}

func TestConformance_EventarcTriggerChannelRoundTrip(t *testing.T) {
	client, err := eventarc.NewRESTClient(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	parent := "projects/conformance-project/locations/us-central1"
	name := parent + "/triggers/conf-channel-trigger"
	channel := parent + "/channels/conf-channel"

	op, err := client.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: "conf-channel-trigger",
		Trigger: &eventarcpb.Trigger{
			Channel: channel,
			EventFilters: []*eventarcpb.EventFilter{{
				Attribute: "type",
				Value:     "google.cloud.pubsub.topic.v1.messagePublished",
			}},
			Destination: &eventarcpb.Destination{
				Descriptor_: &eventarcpb.Destination_CloudRun{
					CloudRun: &eventarcpb.CloudRun{Service: "svc", Region: "us-central1"},
				},
			},
		},
	})
	require.NoError(t, err)
	created, err := op.Wait(ctx)
	require.NoError(t, err)
	assert.Equal(t, channel, created.GetChannel())
	t.Cleanup(func() {
		dop, derr := client.DeleteTrigger(ctx, &eventarcpb.DeleteTriggerRequest{Name: name})
		if derr == nil {
			_, _ = dop.Wait(ctx)
		}
	})

	got, err := client.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{Name: name})
	require.NoError(t, err)
	assert.Equal(t, channel, got.GetChannel(), "channel dropped on get")
}

func TestConformance_ArtifactRegistryRepoLabelsRoundTrip(t *testing.T) {
	svc, err := artifactregistry.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	parent := "projects/conformance-project/locations/us-central1"
	name := parent + "/repositories/conf-labels-repo"

	dryRun := true
	_, err = svc.Projects.Locations.Repositories.Create(parent, &artifactregistry.Repository{
		Format:              "DOCKER",
		Labels:              map[string]string{"env": "prod", "team": "platform"},
		KmsKeyName:          "projects/conformance-project/locations/us-central1/keyRings/kr/cryptoKeys/k",
		CleanupPolicyDryRun: dryRun,
		CleanupPolicies: map[string]artifactregistry.CleanupPolicy{
			"keep-recent": {Action: "KEEP", Id: "keep-recent"},
		},
	}).RepositoryId("conf-labels-repo").Do()
	require.NoError(t, err)

	got, err := svc.Projects.Locations.Repositories.Get(name).Do()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod", "team": "platform"}, got.Labels, "labels dropped on get")
	assert.Equal(t, "projects/conformance-project/locations/us-central1/keyRings/kr/cryptoKeys/k", got.KmsKeyName)
	assert.True(t, got.CleanupPolicyDryRun)
	require.Contains(t, got.CleanupPolicies, "keep-recent")
	assert.Equal(t, "KEEP", got.CleanupPolicies["keep-recent"].Action)
}

func TestConformance_SecretManagerAnnotationsRoundTrip(t *testing.T) {
	svc, err := secretmanager.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	parent := "projects/conformance-project"
	secretID := "conf-annotations-secret"
	name := parent + "/secrets/" + secretID

	_, err = svc.Projects.Secrets.Create(parent, &secretmanager.Secret{
		Replication: &secretmanager.Replication{Automatic: &secretmanager.Automatic{}},
		Annotations: map[string]string{"owner": "sdk", "team": "platform"},
		Topics: []*secretmanager.Topic{
			{Name: "projects/conformance-project/topics/conf-secret-topic"},
		},
	}).SecretId(secretID).Do()
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = svc.Projects.Secrets.Delete(name).Do() })

	got, err := svc.Projects.Secrets.Get(name).Do()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"owner": "sdk", "team": "platform"}, got.Annotations, "annotations dropped on get")
	require.Len(t, got.Topics, 1, "topics dropped on get")
	assert.Equal(t, "projects/conformance-project/topics/conf-secret-topic", got.Topics[0].Name)

	// PATCH updateMask=annotations must apply without erroring.
	patched, err := svc.Projects.Secrets.Patch(name, &secretmanager.Secret{
		Annotations: map[string]string{"owner": "ops"},
	}).UpdateMask("annotations").Do()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"owner": "ops"}, patched.Annotations)
}

func TestConformance_IAMBindingConditionRoundTrip(t *testing.T) {
	svc, err := cloudresourcemanager.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	project := "conformance-iam-condition-project"

	_, err = svc.Projects.SetIamPolicy(project, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: &cloudresourcemanager.Policy{
			Bindings: []*cloudresourcemanager.Binding{{
				Role:    "roles/viewer",
				Members: []string{"user:dev@example.com"},
				Condition: &cloudresourcemanager.Expr{
					Title:      "expires-2030",
					Expression: `request.time < timestamp("2030-01-01T00:00:00Z")`,
				},
			}},
		},
	}).Do()
	require.NoError(t, err)

	got, err := svc.Projects.GetIamPolicy(project, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	require.NoError(t, err)
	require.Len(t, got.Bindings, 1)
	require.NotNil(t, got.Bindings[0].Condition, "binding condition dropped on get")
	assert.Equal(t, "expires-2030", got.Bindings[0].Condition.Title)
	assert.Equal(t, `request.time < timestamp("2030-01-01T00:00:00Z")`, got.Bindings[0].Condition.Expression)
}

func TestConformance_LoggingMetricValueExtractorRoundTrip(t *testing.T) {
	svc, err := logging.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	parent := "projects/conformance-project"

	created, err := svc.Projects.Metrics.Create(parent, &logging.LogMetric{
		Name:           "conf-distribution-metric",
		Filter:         `resource.type="gce_instance"`,
		ValueExtractor: "EXTRACT(jsonPayload.latency)",
		MetricDescriptor: &logging.MetricDescriptor{
			MetricKind: "DELTA",
			ValueType:  "DISTRIBUTION",
		},
		BucketOptions: &logging.BucketOptions{
			ExplicitBuckets: &logging.Explicit{Bounds: []float64{0, 1, 2, 5, 10}},
		},
		LabelExtractors: map[string]string{"zone": "EXTRACT(resource.labels.zone)"},
	}).Do()
	require.NoError(t, err)
	require.NotNil(t, created.BucketOptions)

	got, err := svc.Projects.Metrics.Get(parent + "/metrics/conf-distribution-metric").Do()
	require.NoError(t, err)
	assert.Equal(t, "EXTRACT(jsonPayload.latency)", got.ValueExtractor, "valueExtractor dropped on get")
	require.NotNil(t, got.BucketOptions, "bucketOptions dropped on get")
	require.NotNil(t, got.BucketOptions.ExplicitBuckets)
	assert.Equal(t, []float64{0, 1, 2, 5, 10}, got.BucketOptions.ExplicitBuckets.Bounds)
	assert.Equal(t, map[string]string{"zone": "EXTRACT(resource.labels.zone)"}, got.LabelExtractors)
}

// requireGoogleErr asserts the error is a googleapi.Error with the given HTTP
// code and that the GCP error envelope's `status` string is present in the raw
// body — the canonical {"error":{"code","message","status"}} shape.
func requireGoogleErr(t *testing.T, err error, code int, gcpStatus string) {
	t.Helper()
	require.Error(t, err)
	var gerr *googleapi.Error
	require.True(t, errors.As(err, &gerr), "expected a googleapi.Error, got %T: %v", err, err)
	assert.Equal(t, code, gerr.Code, "http status code")
	assert.Contains(t, gerr.Body, gcpStatus, "GCP error.status field")
}

// TestConformance_ComputeDuplicateInsertConflict pins that a second insert of
// a metadata-only Compute resource (backendService) returns 409 ALREADY_EXISTS
// like real GCP, instead of silently overwriting. backendServices.insert does
// not touch the network fabric, so the case runs without a real-exec host.
func TestConformance_ComputeDuplicateInsertConflict(t *testing.T) {
	svc, err := compute.NewService(ctx, option.WithEndpoint(baseURL+"/compute/v1/"), option.WithoutAuthentication())
	require.NoError(t, err)
	const project = "conformance-project"

	bs := &compute.BackendService{Name: "conf-dup-backend", Protocol: "HTTP"}
	_, err = svc.BackendServices.Insert(project, bs).Context(ctx).Do()
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = svc.BackendServices.Delete(project, bs.Name).Context(ctx).Do() })

	_, err = svc.BackendServices.Insert(project, bs).Context(ctx).Do()
	requireGoogleErr(t, err, 409, "ALREADY_EXISTS")
}

// TestConformance_ComputeDeleteMissingNotFound pins that deleting a never-
// created metadata-only Compute resource (healthCheck) returns 404 NOT_FOUND
// like real GCP, instead of a synthesized DONE operation.
func TestConformance_ComputeDeleteMissingNotFound(t *testing.T) {
	svc, err := compute.NewService(ctx, option.WithEndpoint(baseURL+"/compute/v1/"), option.WithoutAuthentication())
	require.NoError(t, err)

	_, err = svc.HealthChecks.Delete("conformance-project", "conf-never-created-hc").Context(ctx).Do()
	requireGoogleErr(t, err, 404, "NOT_FOUND")
}

// TestConformance_PubSubDuplicateTopicConflict pins that creating a topic twice
// returns 409 ALREADY_EXISTS like real Pub/Sub instead of overwriting.
func TestConformance_PubSubDuplicateTopicConflict(t *testing.T) {
	svc, err := pubsub.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	name := "projects/conformance-project/topics/conf-dup-topic"

	_, err = svc.Projects.Topics.Create(name, &pubsub.Topic{}).Do()
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = svc.Projects.Topics.Delete(name).Do() })

	_, err = svc.Projects.Topics.Create(name, &pubsub.Topic{}).Do()
	requireGoogleErr(t, err, 409, "ALREADY_EXISTS")
}

// TestConformance_PubSubDuplicateSubscriptionConflict pins the same contract
// for subscriptions.
func TestConformance_PubSubDuplicateSubscriptionConflict(t *testing.T) {
	svc, err := pubsub.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	topic := "projects/conformance-project/topics/conf-dup-sub-topic"
	sub := "projects/conformance-project/subscriptions/conf-dup-sub"

	_, err = svc.Projects.Topics.Create(topic, &pubsub.Topic{}).Do()
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = svc.Projects.Topics.Delete(topic).Do() })

	_, err = svc.Projects.Subscriptions.Create(sub, &pubsub.Subscription{Topic: topic}).Do()
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = svc.Projects.Subscriptions.Delete(sub).Do() })

	_, err = svc.Projects.Subscriptions.Create(sub, &pubsub.Subscription{Topic: topic}).Do()
	requireGoogleErr(t, err, 409, "ALREADY_EXISTS")
}

// TestConformance_EventarcDuplicateTriggerConflict pins that a second
// CreateTrigger with the same triggerId returns ALREADY_EXISTS. The eventarc
// gRPC client surfaces the 409 as codes.AlreadyExists.
func TestConformance_EventarcDuplicateTriggerConflict(t *testing.T) {
	client, err := eventarc.NewRESTClient(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	parent := "projects/conformance-project/locations/us-central1"
	name := parent + "/triggers/conf-dup-trigger"
	req := &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: "conf-dup-trigger",
		Trigger: &eventarcpb.Trigger{
			Channel: parent + "/channels/conf-dup-trigger-channel",
			EventFilters: []*eventarcpb.EventFilter{{
				Attribute: "type",
				Value:     "google.cloud.pubsub.topic.v1.messagePublished",
			}},
			Destination: &eventarcpb.Destination{
				Descriptor_: &eventarcpb.Destination_CloudRun{
					CloudRun: &eventarcpb.CloudRun{Service: "svc", Region: "us-central1"},
				},
			},
		},
	}
	op, err := client.CreateTrigger(ctx, req)
	require.NoError(t, err)
	_, err = op.Wait(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		dop, derr := client.DeleteTrigger(ctx, &eventarcpb.DeleteTriggerRequest{Name: name})
		if derr == nil {
			_, _ = dop.Wait(ctx)
		}
	})

	_, err = client.CreateTrigger(ctx, req)
	require.Error(t, err)
	// The eventarc REST-over-gRPC client maps the sim's HTTP 409 to a gRPC
	// status. ALREADY_EXISTS and ABORTED both carry HTTP 409, and this
	// client's transport derives the gRPC code from the HTTP status rather
	// than the body's `status` string, so accept either 409-family code and
	// assert the conflict message — exactly what real Eventarc + this client
	// surface for a duplicate create.
	code := status.Code(err)
	assert.Contains(t, []codes.Code{codes.AlreadyExists, codes.Aborted}, code, "duplicate CreateTrigger must be a 409-family conflict: %v", err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestConformance_LoggingDeleteMissingSinkNotFound pins that deleting a
// never-created log sink returns 404 NOT_FOUND like real Cloud Logging.
func TestConformance_LoggingDeleteMissingSinkNotFound(t *testing.T) {
	svc, err := logging.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)

	_, err = svc.Projects.Sinks.Delete("projects/conformance-project/sinks/conf-never-created-sink").Do()
	requireGoogleErr(t, err, 404, "NOT_FOUND")
}

// TestConformance_IAMSetPolicyStaleEtagAborted pins the optimistic-concurrency
// contract: a setIamPolicy carrying a stale etag is rejected with 409 ABORTED
// so the caller re-reads and retries.
func TestConformance_IAMSetPolicyStaleEtagAborted(t *testing.T) {
	svc, err := cloudresourcemanager.NewService(ctx, option.WithEndpoint(baseURL), option.WithoutAuthentication())
	require.NoError(t, err)
	project := "conformance-iam-etag-project"

	// Read the current policy to learn its etag.
	got, err := svc.Projects.GetIamPolicy(project, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	require.NoError(t, err)
	require.NotEmpty(t, got.Etag)

	// A write with the correct etag succeeds and mints a new one.
	ok, err := svc.Projects.SetIamPolicy(project, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: &cloudresourcemanager.Policy{
			Etag:     got.Etag,
			Bindings: []*cloudresourcemanager.Binding{{Role: "roles/viewer", Members: []string{"user:a@example.com"}}},
		},
	}).Do()
	require.NoError(t, err)
	require.NotEqual(t, got.Etag, ok.Etag, "successful set must mint a fresh etag")

	// Re-using the now-stale original etag must be rejected with ABORTED.
	_, err = svc.Projects.SetIamPolicy(project, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: &cloudresourcemanager.Policy{
			Etag:     got.Etag,
			Bindings: []*cloudresourcemanager.Binding{{Role: "roles/editor", Members: []string{"user:b@example.com"}}},
		},
	}).Do()
	requireGoogleErr(t, err, 409, "ABORTED")
}
