package aws_sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRDS_Snapshot_Lifecycle exercises the snapshot family
// — CreateDBSnapshot → DescribeDBSnapshots → RestoreDBInstanceFromDBSnapshot
// → DeleteDBSnapshot — and asserts the state machine documented in
// RDSSnapshot's docstring. Real RDS transitions a snapshot through
// creating → available → deleted (or failed). The sim collapses
// creating → available into an inline-settle; this test locks the
// Status field is non-empty + reads back as "available".
func TestRDS_Snapshot_Lifecycle(t *testing.T) {
	c := rdsClient()
	ctx := context.Background()

	// Source instance.
	_, err := c.CreateDBInstance(ctx, &rds.CreateDBInstanceInput{
		DBInstanceIdentifier: aws.String("snap-src"),
		DBInstanceClass:      aws.String("db.t3.micro"),
		Engine:               aws.String("postgres"),
		MasterUsername:       aws.String("admin"),
		AllocatedStorage:     aws.Int32(20),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
			DBInstanceIdentifier: aws.String("snap-src"),
			SkipFinalSnapshot:    aws.Bool(true),
		})
	})

	// CreateDBSnapshot.
	createOut, err := c.CreateDBSnapshot(ctx, &rds.CreateDBSnapshotInput{
		DBSnapshotIdentifier: aws.String("snap-1"),
		DBInstanceIdentifier: aws.String("snap-src"),
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.DBSnapshot)
	assert.Equal(t, "available", aws.ToString(createOut.DBSnapshot.Status),
		"Status must be set (state-machine-completeness rule)")
	assert.Equal(t, "manual", aws.ToString(createOut.DBSnapshot.SnapshotType),
		"sim emits SnapshotType=manual for client-initiated snapshots")

	// DescribeDBSnapshots round-trips.
	desc, err := c.DescribeDBSnapshots(ctx, &rds.DescribeDBSnapshotsInput{
		DBSnapshotIdentifier: aws.String("snap-1"),
	})
	require.NoError(t, err)
	require.Len(t, desc.DBSnapshots, 1)
	assert.Equal(t, "snap-1", aws.ToString(desc.DBSnapshots[0].DBSnapshotIdentifier))
	assert.Equal(t, "available", aws.ToString(desc.DBSnapshots[0].Status))

	attrs, err := c.DescribeDBSnapshotAttributes(ctx, &rds.DescribeDBSnapshotAttributesInput{
		DBSnapshotIdentifier: aws.String("snap-1"),
	})
	require.NoError(t, err)
	require.NotNil(t, attrs.DBSnapshotAttributesResult)
	assert.Equal(t, "snap-1", aws.ToString(attrs.DBSnapshotAttributesResult.DBSnapshotIdentifier))

	// RestoreDBInstanceFromDBSnapshot creates a new instance from the
	// snapshot; the new instance inherits engine + version + storage.
	restoreOut, err := c.RestoreDBInstanceFromDBSnapshot(ctx, &rds.RestoreDBInstanceFromDBSnapshotInput{
		DBInstanceIdentifier: aws.String("snap-restored"),
		DBSnapshotIdentifier: aws.String("snap-1"),
		DBInstanceClass:      aws.String("db.t3.micro"),
	})
	require.NoError(t, err)
	require.NotNil(t, restoreOut.DBInstance)
	assert.Equal(t, "postgres", aws.ToString(restoreOut.DBInstance.Engine))
	t.Cleanup(func() {
		_, _ = c.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
			DBInstanceIdentifier: aws.String("snap-restored"),
			SkipFinalSnapshot:    aws.Bool(true),
		})
	})

	// DeleteDBSnapshot transitions to deleted + removes from store.
	delOut, err := c.DeleteDBSnapshot(ctx, &rds.DeleteDBSnapshotInput{
		DBSnapshotIdentifier: aws.String("snap-1"),
	})
	require.NoError(t, err)
	assert.Equal(t, "deleted", aws.ToString(delOut.DBSnapshot.Status),
		"final state-machine transition before removal")

	// Subsequent identifier-addressed Describe returns the real RDS
	// not-found fault; unfiltered list calls are the empty-list shape.
	_, err = c.DescribeDBSnapshots(ctx, &rds.DescribeDBSnapshotsInput{
		DBSnapshotIdentifier: aws.String("snap-1"),
	})
	assertAWSAPIErrorCode(t, err, "DBSnapshotNotFound")

	desc2, err := c.DescribeDBSnapshots(ctx, &rds.DescribeDBSnapshotsInput{})
	require.NoError(t, err)
	assert.Empty(t, desc2.DBSnapshots)
}
