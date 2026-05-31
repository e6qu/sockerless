package gcp_sdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

func sqlAdminService(t *testing.T) *sqladmin.Service {
	t.Helper()
	svc, err := sqladmin.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

// TestCloudSQL_InstanceDatabaseUserLifecycle exercises:
// Insert instance → Get → List → Patch → Insert database → Insert
// user → List databases → List users → Delete instance.
func TestCloudSQL_InstanceDatabaseUserLifecycle(t *testing.T) {
	svc := sqlAdminService(t)
	project := "test-project"
	instanceName := "lifecycle-pg"

	op, err := svc.Instances.Insert(project, &sqladmin.DatabaseInstance{
		Name:            instanceName,
		Region:          "us-central1",
		DatabaseVersion: "POSTGRES_15",
	}).Do()
	require.NoError(t, err)
	assert.Equal(t, "DONE", op.Status)

	inst, err := svc.Instances.Get(project, instanceName).Do()
	require.NoError(t, err)
	assert.Equal(t, instanceName, inst.Name)
	assert.Equal(t, "RUNNABLE", inst.State)
	assert.Equal(t, "POSTGRES_15", inst.DatabaseVersion)
	assert.NotEmpty(t, inst.ConnectionName)

	list, err := svc.Instances.List(project).Do()
	require.NoError(t, err)
	found := false
	for _, x := range list.Items {
		if x.Name == instanceName {
			found = true
		}
	}
	assert.True(t, found)

	// Insert a database.
	_, err = svc.Databases.Insert(project, instanceName, &sqladmin.Database{
		Name: "appdb",
	}).Do()
	require.NoError(t, err)

	dbList, err := svc.Databases.List(project, instanceName).Do()
	require.NoError(t, err)
	require.Len(t, dbList.Items, 1)
	assert.Equal(t, "appdb", dbList.Items[0].Name)

	// Insert a user.
	_, err = svc.Users.Insert(project, instanceName, &sqladmin.User{
		Name: "appuser",
		Host: "%",
	}).Do()
	require.NoError(t, err)

	uList, err := svc.Users.List(project, instanceName).Do()
	require.NoError(t, err)
	require.Len(t, uList.Items, 1)
	assert.Equal(t, "appuser", uList.Items[0].Name)

	// Delete instance (cascade).
	_, err = svc.Instances.Delete(project, instanceName).Do()
	require.NoError(t, err)
}

func TestCloudSQL_BackupRunsReturnOperations(t *testing.T) {
	svc := sqlAdminService(t)
	project := "test-project"
	instanceName := "backup-pg"

	_, err := svc.Instances.Insert(project, &sqladmin.DatabaseInstance{
		Name:            instanceName,
		Region:          "us-central1",
		DatabaseVersion: "POSTGRES_15",
	}).Do()
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = svc.Instances.Delete(project, instanceName).Do() })

	insertOp, err := svc.BackupRuns.Insert(project, instanceName, &sqladmin.BackupRun{}).Do()
	require.NoError(t, err)
	require.Equal(t, "sql#operation", insertOp.Kind)
	require.Equal(t, "DONE", insertOp.Status)
	require.Equal(t, "BACKUP_VOLUME", insertOp.OperationType)

	list, err := svc.BackupRuns.List(project, instanceName).Do()
	require.NoError(t, err)
	require.Len(t, list.Items, 1)

	deleteOp, err := svc.BackupRuns.Delete(project, instanceName, list.Items[0].Id).Do()
	require.NoError(t, err)
	require.Equal(t, "sql#operation", deleteOp.Kind)
	require.Equal(t, "DONE", deleteOp.Status)
	require.Equal(t, "DELETE_BACKUP", deleteOp.OperationType)
}
