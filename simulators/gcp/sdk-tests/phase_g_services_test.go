package gcp_sdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/bigtableadmin/v2"
	"google.golang.org/api/dataflow/v1b3"
	"google.golang.org/api/option"
	"google.golang.org/api/spanner/v1"
)

func TestSpanner_InstanceDatabaseSessionSDK(t *testing.T) {
	svc, err := spanner.NewService(ctx, option.WithEndpoint(baseURL+"/spanner/"), option.WithoutAuthentication())
	require.NoError(t, err)

	op, err := svc.Projects.Instances.Create("projects/test-project", &spanner.CreateInstanceRequest{
		InstanceId: "sdk-spanner",
		Instance: &spanner.Instance{
			DisplayName: "SDK Spanner",
			NodeCount:   1,
			Labels:      map[string]string{"env": "sdk"},
		},
	}).Do()
	require.NoError(t, err)
	assert.True(t, op.Done)

	inst, err := svc.Projects.Instances.Get("projects/test-project/instances/sdk-spanner").Do()
	require.NoError(t, err)
	assert.Equal(t, "READY", inst.State)

	dbOp, err := svc.Projects.Instances.Databases.Create("projects/test-project/instances/sdk-spanner", &spanner.CreateDatabaseRequest{
		CreateStatement: "CREATE DATABASE `sdkdb`",
	}).Do()
	require.NoError(t, err)
	assert.True(t, dbOp.Done)

	db, err := svc.Projects.Instances.Databases.Get("projects/test-project/instances/sdk-spanner/databases/sdkdb").Do()
	require.NoError(t, err)
	assert.Equal(t, "READY", db.State)

	session, err := svc.Projects.Instances.Databases.Sessions.Create(db.Name, &spanner.CreateSessionRequest{
		Session: &spanner.Session{Labels: map[string]string{"kind": "sdk"}},
	}).Do()
	require.NoError(t, err)
	assert.Contains(t, session.Name, "/sessions/")

	sessions, err := svc.Projects.Instances.Databases.Sessions.List(db.Name).Do()
	require.NoError(t, err)
	require.Len(t, sessions.Sessions, 1)
	assert.Equal(t, session.Name, sessions.Sessions[0].Name)
}

func TestDataflow_RegionalJobSDK(t *testing.T) {
	svc, err := dataflow.NewService(ctx, option.WithEndpoint(baseURL+"/"), option.WithoutAuthentication())
	require.NoError(t, err)

	job, err := svc.Projects.Locations.Jobs.Create("test-project", "us-central1", &dataflow.Job{
		Name:   "sdk-dataflow-job",
		Type:   "JOB_TYPE_BATCH",
		Labels: map[string]string{"env": "sdk"},
	}).Do()
	require.NoError(t, err)
	assert.Equal(t, "JOB_STATE_RUNNING", job.CurrentState)

	got, err := svc.Projects.Locations.Jobs.Get("test-project", "us-central1", job.Id).Do()
	require.NoError(t, err)
	assert.Equal(t, "sdk-dataflow-job", got.Name)

	list, err := svc.Projects.Locations.Jobs.List("test-project", "us-central1").Do()
	require.NoError(t, err)
	require.Len(t, list.Jobs, 1)
	assert.Equal(t, job.Id, list.Jobs[0].Id)
}

func TestBigtable_InstanceClusterTableSDK(t *testing.T) {
	svc, err := bigtableadmin.NewService(ctx, option.WithEndpoint(baseURL+"/"), option.WithoutAuthentication())
	require.NoError(t, err)

	op, err := svc.Projects.Instances.Create("projects/test-project", &bigtableadmin.CreateInstanceRequest{
		InstanceId: "sdk-bt",
		Instance: &bigtableadmin.Instance{
			DisplayName: "SDK Bigtable",
			Type:        "PRODUCTION",
			Labels:      map[string]string{"env": "sdk"},
		},
		Clusters: map[string]bigtableadmin.Cluster{
			"sdk-bt-c1": {
				Location:           "projects/test-project/locations/us-central1-a",
				ServeNodes:         1,
				DefaultStorageType: "SSD",
			},
		},
	}).Do()
	require.NoError(t, err)
	assert.True(t, op.Done)

	inst, err := svc.Projects.Instances.Get("projects/test-project/instances/sdk-bt").Do()
	require.NoError(t, err)
	assert.Equal(t, "READY", inst.State)

	cluster, err := svc.Projects.Instances.Clusters.Get("projects/test-project/instances/sdk-bt/clusters/sdk-bt-c1").Do()
	require.NoError(t, err)
	assert.Equal(t, "READY", cluster.State)

	table, err := svc.Projects.Instances.Tables.Create("projects/test-project/instances/sdk-bt", &bigtableadmin.CreateTableRequest{
		TableId: "sdk_table",
		Table: &bigtableadmin.Table{
			ColumnFamilies: map[string]bigtableadmin.ColumnFamily{
				"cf": {},
			},
		},
	}).Do()
	require.NoError(t, err)
	assert.Equal(t, "projects/test-project/instances/sdk-bt/tables/sdk_table", table.Name)

	list, err := svc.Projects.Instances.Tables.List("projects/test-project/instances/sdk-bt").Do()
	require.NoError(t, err)
	require.Len(t, list.Tables, 1)
	assert.Equal(t, table.Name, list.Tables[0].Name)
}
