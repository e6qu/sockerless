package aws_sdk_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func glueClient() *glue.Client {
	return glue.NewFromConfig(sdkConfig(), func(o *glue.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestGlue_DatabaseCRUD_SDK(t *testing.T) {
	c := glueClient()

	_, err := c.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{
			Name:        aws.String("glue-sdk-db"),
			Description: aws.String("sdk test database"),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteDatabase(ctx, &glue.DeleteDatabaseInput{Name: aws.String("glue-sdk-db")})
	})

	get, err := c.GetDatabase(ctx, &glue.GetDatabaseInput{Name: aws.String("glue-sdk-db")})
	require.NoError(t, err)
	assert.Equal(t, "glue-sdk-db", aws.ToString(get.Database.Name))

	list, err := c.GetDatabases(ctx, &glue.GetDatabasesInput{})
	require.NoError(t, err)
	found := false
	for _, db := range list.DatabaseList {
		if aws.ToString(db.Name) == "glue-sdk-db" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestGlue_TableCRUD_SDK(t *testing.T) {
	c := glueClient()

	_, err := c.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{
			Name: aws.String("glue-sdk-tbl-db"),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteTable(ctx, &glue.DeleteTableInput{
			DatabaseName: aws.String("glue-sdk-tbl-db"),
			Name:         aws.String("glue-sdk-table"),
		})
		_, _ = c.DeleteDatabase(ctx, &glue.DeleteDatabaseInput{Name: aws.String("glue-sdk-tbl-db")})
	})

	_, err = c.CreateTable(ctx, &glue.CreateTableInput{
		DatabaseName: aws.String("glue-sdk-tbl-db"),
		TableInput: &gluetypes.TableInput{
			Name: aws.String("glue-sdk-table"),
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Location:     aws.String("s3://bucket/prefix/"),
				InputFormat:  aws.String("org.apache.hadoop.mapred.TextInputFormat"),
				OutputFormat: aws.String("org.apache.hadoop.hive.ql.io.HiveIgnoreKeyTextOutputFormat"),
			},
		},
	})
	require.NoError(t, err)

	get, err := c.GetTable(ctx, &glue.GetTableInput{
		DatabaseName: aws.String("glue-sdk-tbl-db"),
		Name:         aws.String("glue-sdk-table"),
	})
	require.NoError(t, err)
	assert.Equal(t, "glue-sdk-table", aws.ToString(get.Table.Name))
	assert.Equal(t, "glue-sdk-tbl-db", aws.ToString(get.Table.DatabaseName))

	tables, err := c.GetTables(ctx, &glue.GetTablesInput{
		DatabaseName: aws.String("glue-sdk-tbl-db"),
	})
	require.NoError(t, err)
	require.Len(t, tables.TableList, 1)
	assert.Equal(t, "glue-sdk-table", aws.ToString(tables.TableList[0].Name))
}

func TestGlue_DatabaseUpdate_SDK(t *testing.T) {
	c := glueClient()

	_, err := c.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{
			Name:        aws.String("glue-sdk-updb"),
			Description: aws.String("before"),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteDatabase(ctx, &glue.DeleteDatabaseInput{Name: aws.String("glue-sdk-updb")})
	})

	_, err = c.UpdateDatabase(ctx, &glue.UpdateDatabaseInput{
		Name: aws.String("glue-sdk-updb"),
		DatabaseInput: &gluetypes.DatabaseInput{
			Name:        aws.String("glue-sdk-updb"),
			Description: aws.String("after"),
			Parameters:  map[string]string{"owner": "data-eng"},
		},
	})
	require.NoError(t, err)

	get, err := c.GetDatabase(ctx, &glue.GetDatabaseInput{Name: aws.String("glue-sdk-updb")})
	require.NoError(t, err)
	assert.Equal(t, "after", aws.ToString(get.Database.Description))
	assert.Equal(t, "data-eng", get.Database.Parameters["owner"])
}

func TestGlue_TableUpdateAndBatchDelete_SDK(t *testing.T) {
	c := glueClient()
	db := "glue-sdk-tblupd-db"

	_, err := c.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{Name: aws.String(db)},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.BatchDeleteTable(ctx, &glue.BatchDeleteTableInput{
			DatabaseName:   aws.String(db),
			TablesToDelete: []string{"t1", "t2"},
		})
		_, _ = c.DeleteDatabase(ctx, &glue.DeleteDatabaseInput{Name: aws.String(db)})
	})

	for _, name := range []string{"t1", "t2"} {
		_, err = c.CreateTable(ctx, &glue.CreateTableInput{
			DatabaseName: aws.String(db),
			TableInput: &gluetypes.TableInput{
				Name:              aws.String(name),
				StorageDescriptor: &gluetypes.StorageDescriptor{Location: aws.String("s3://bucket/" + name + "/")},
			},
		})
		require.NoError(t, err)
	}

	_, err = c.UpdateTable(ctx, &glue.UpdateTableInput{
		DatabaseName: aws.String(db),
		TableInput: &gluetypes.TableInput{
			Name:              aws.String("t1"),
			TableType:         aws.String("EXTERNAL_TABLE"),
			StorageDescriptor: &gluetypes.StorageDescriptor{Location: aws.String("s3://bucket/t1-new/")},
		},
	})
	require.NoError(t, err)

	get, err := c.GetTable(ctx, &glue.GetTableInput{DatabaseName: aws.String(db), Name: aws.String("t1")})
	require.NoError(t, err)
	assert.Equal(t, "EXTERNAL_TABLE", aws.ToString(get.Table.TableType))
	assert.Equal(t, "s3://bucket/t1-new/", aws.ToString(get.Table.StorageDescriptor.Location))

	bd, err := c.BatchDeleteTable(ctx, &glue.BatchDeleteTableInput{
		DatabaseName:   aws.String(db),
		TablesToDelete: []string{"t1", "t2", "missing"},
	})
	require.NoError(t, err)
	require.Len(t, bd.Errors, 1)
	assert.Equal(t, "missing", aws.ToString(bd.Errors[0].TableName))

	tables, err := c.GetTables(ctx, &glue.GetTablesInput{DatabaseName: aws.String(db)})
	require.NoError(t, err)
	assert.Empty(t, tables.TableList)
}

func TestGlue_PartitionLifecycle_SDK(t *testing.T) {
	c := glueClient()
	db := "glue-sdk-part-db"
	tbl := "glue-sdk-part-tbl"

	_, err := c.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{Name: aws.String(db)},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteTable(ctx, &glue.DeleteTableInput{DatabaseName: aws.String(db), Name: aws.String(tbl)})
		_, _ = c.DeleteDatabase(ctx, &glue.DeleteDatabaseInput{Name: aws.String(db)})
	})

	_, err = c.CreateTable(ctx, &glue.CreateTableInput{
		DatabaseName: aws.String(db),
		TableInput: &gluetypes.TableInput{
			Name:              aws.String(tbl),
			StorageDescriptor: &gluetypes.StorageDescriptor{Location: aws.String("s3://bucket/part/")},
			PartitionKeys:     []gluetypes.Column{{Name: aws.String("dt"), Type: aws.String("string")}},
		},
	})
	require.NoError(t, err)

	// CreatePartition (single).
	_, err = c.CreatePartition(ctx, &glue.CreatePartitionInput{
		DatabaseName: aws.String(db),
		TableName:    aws.String(tbl),
		PartitionInput: &gluetypes.PartitionInput{
			Values:            []string{"2024-01-01"},
			StorageDescriptor: &gluetypes.StorageDescriptor{Location: aws.String("s3://bucket/part/dt=2024-01-01/")},
		},
	})
	require.NoError(t, err)

	// BatchCreatePartition (multiple).
	bcp, err := c.BatchCreatePartition(ctx, &glue.BatchCreatePartitionInput{
		DatabaseName: aws.String(db),
		TableName:    aws.String(tbl),
		PartitionInputList: []gluetypes.PartitionInput{
			{Values: []string{"2024-01-02"}, StorageDescriptor: &gluetypes.StorageDescriptor{Location: aws.String("s3://bucket/part/dt=2024-01-02/")}},
			{Values: []string{"2024-01-01"}}, // already exists -> Errors entry
		},
	})
	require.NoError(t, err)
	require.Len(t, bcp.Errors, 1)
	assert.Equal(t, []string{"2024-01-01"}, bcp.Errors[0].PartitionValues)

	// GetPartition.
	gp, err := c.GetPartition(ctx, &glue.GetPartitionInput{
		DatabaseName:    aws.String(db),
		TableName:       aws.String(tbl),
		PartitionValues: []string{"2024-01-01"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"2024-01-01"}, gp.Partition.Values)
	assert.Equal(t, db, aws.ToString(gp.Partition.DatabaseName))
	assert.Equal(t, tbl, aws.ToString(gp.Partition.TableName))

	// GetPartitions.
	gps, err := c.GetPartitions(ctx, &glue.GetPartitionsInput{
		DatabaseName: aws.String(db),
		TableName:    aws.String(tbl),
	})
	require.NoError(t, err)
	assert.Len(t, gps.Partitions, 2)

	// BatchGetPartition (one present, one missing).
	bgp, err := c.BatchGetPartition(ctx, &glue.BatchGetPartitionInput{
		DatabaseName: aws.String(db),
		TableName:    aws.String(tbl),
		PartitionsToGet: []gluetypes.PartitionValueList{
			{Values: []string{"2024-01-02"}},
			{Values: []string{"2099-12-31"}},
		},
	})
	require.NoError(t, err)
	require.Len(t, bgp.Partitions, 1)
	assert.Equal(t, []string{"2024-01-02"}, bgp.Partitions[0].Values)
	require.Len(t, bgp.UnprocessedKeys, 1)
	assert.Equal(t, []string{"2099-12-31"}, bgp.UnprocessedKeys[0].Values)

	// UpdatePartition (in place).
	_, err = c.UpdatePartition(ctx, &glue.UpdatePartitionInput{
		DatabaseName:       aws.String(db),
		TableName:          aws.String(tbl),
		PartitionValueList: []string{"2024-01-01"},
		PartitionInput: &gluetypes.PartitionInput{
			Values:     []string{"2024-01-01"},
			Parameters: map[string]string{"rows": "100"},
		},
	})
	require.NoError(t, err)
	gp, err = c.GetPartition(ctx, &glue.GetPartitionInput{
		DatabaseName:    aws.String(db),
		TableName:       aws.String(tbl),
		PartitionValues: []string{"2024-01-01"},
	})
	require.NoError(t, err)
	assert.Equal(t, "100", gp.Partition.Parameters["rows"])

	// DeletePartition (single).
	_, err = c.DeletePartition(ctx, &glue.DeletePartitionInput{
		DatabaseName:    aws.String(db),
		TableName:       aws.String(tbl),
		PartitionValues: []string{"2024-01-01"},
	})
	require.NoError(t, err)
	_, err = c.GetPartition(ctx, &glue.GetPartitionInput{
		DatabaseName:    aws.String(db),
		TableName:       aws.String(tbl),
		PartitionValues: []string{"2024-01-01"},
	})
	require.Error(t, err)
	var notFound *gluetypes.EntityNotFoundException
	assert.ErrorAs(t, err, &notFound)

	// BatchDeletePartition (one present, one missing).
	bdp, err := c.BatchDeletePartition(ctx, &glue.BatchDeletePartitionInput{
		DatabaseName: aws.String(db),
		TableName:    aws.String(tbl),
		PartitionsToDelete: []gluetypes.PartitionValueList{
			{Values: []string{"2024-01-02"}},
			{Values: []string{"2099-12-31"}},
		},
	})
	require.NoError(t, err)
	require.Len(t, bdp.Errors, 1)
	assert.Equal(t, []string{"2099-12-31"}, bdp.Errors[0].PartitionValues)

	gps, err = c.GetPartitions(ctx, &glue.GetPartitionsInput{
		DatabaseName: aws.String(db),
		TableName:    aws.String(tbl),
	})
	require.NoError(t, err)
	assert.Empty(t, gps.Partitions)
}

func TestGlue_GetPartitionIndexes_SDK(t *testing.T) {
	c := glueClient()

	_, err := c.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{Name: aws.String("glue-sdk-index-db")},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteTable(ctx, &glue.DeleteTableInput{
			DatabaseName: aws.String("glue-sdk-index-db"),
			Name:         aws.String("glue-sdk-index-table"),
		})
		_, _ = c.DeleteDatabase(ctx, &glue.DeleteDatabaseInput{Name: aws.String("glue-sdk-index-db")})
	})

	_, err = c.CreateTable(ctx, &glue.CreateTableInput{
		DatabaseName: aws.String("glue-sdk-index-db"),
		TableInput: &gluetypes.TableInput{
			Name: aws.String("glue-sdk-index-table"),
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Location: aws.String("s3://bucket/indexed/"),
			},
			PartitionKeys: []gluetypes.Column{{Name: aws.String("dt"), Type: aws.String("string")}},
		},
	})
	require.NoError(t, err)

	indexes, err := c.GetPartitionIndexes(ctx, &glue.GetPartitionIndexesInput{
		DatabaseName: aws.String("glue-sdk-index-db"),
		TableName:    aws.String("glue-sdk-index-table"),
	})
	require.NoError(t, err)
	assert.NotNil(t, indexes.PartitionIndexDescriptorList)
	assert.Empty(t, indexes.PartitionIndexDescriptorList)
}

func TestGlue_JobCRUD_SDK(t *testing.T) {
	c := glueClient()
	s3c := s3Client()
	bucket := "glue-sdk-scripts"
	_, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String("script.py")})
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})
	_, err = s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("script.py"),
		Body:   bytes.NewReader([]byte("import sys\nprint('glue-sdk-ready')\nsys.exit(0)\n")),
	})
	require.NoError(t, err)

	workers := 2
	create, err := c.CreateJob(ctx, &glue.CreateJobInput{
		Name:            aws.String("glue-sdk-job"),
		Description:     aws.String("sdk test job"),
		Role:            aws.String("arn:aws:iam::123456789012:role/glue-role"),
		GlueVersion:     aws.String("4.0"),
		WorkerType:      gluetypes.WorkerTypeG1x,
		NumberOfWorkers: aws.Int32(int32(workers)),
		Command: &gluetypes.JobCommand{
			Name:           aws.String("pythonshell"),
			ScriptLocation: aws.String("s3://" + bucket + "/script.py"),
		},
		DefaultArguments: map[string]string{
			"--job-language": "python",
		},
		Tags: map[string]string{"env": "sdk"},
	})
	require.NoError(t, err)
	assert.Equal(t, "glue-sdk-job", aws.ToString(create.Name))
	t.Cleanup(func() {
		_, _ = c.DeleteJob(ctx, &glue.DeleteJobInput{JobName: aws.String("glue-sdk-job")})
	})

	get, err := c.GetJob(ctx, &glue.GetJobInput{JobName: aws.String("glue-sdk-job")})
	require.NoError(t, err)
	assert.Equal(t, "glue-sdk-job", aws.ToString(get.Job.Name))
	assert.Equal(t, "sdk test job", aws.ToString(get.Job.Description))

	// The Job shape has no Tags member — create-time tags ride GetTags.
	jobTags, err := c.GetTags(ctx, &glue.GetTagsInput{
		ResourceArn: aws.String("arn:aws:glue:us-east-1:123456789012:job/glue-sdk-job"),
	})
	require.NoError(t, err)
	assert.Equal(t, "sdk", jobTags.Tags["env"])

	list, err := c.GetJobs(ctx, &glue.GetJobsInput{})
	require.NoError(t, err)
	found := false
	for _, j := range list.Jobs {
		if aws.ToString(j.Name) == "glue-sdk-job" {
			found = true
		}
	}
	assert.True(t, found)

	runResp, err := c.StartJobRun(ctx, &glue.StartJobRunInput{
		JobName: aws.String("glue-sdk-job"),
		Arguments: map[string]string{
			"--input": "s3://bucket/data/",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(runResp.JobRunId))

	var run *glue.GetJobRunOutput
	require.Eventually(t, func() bool {
		run, err = c.GetJobRun(ctx, &glue.GetJobRunInput{
			JobName: aws.String("glue-sdk-job"),
			RunId:   runResp.JobRunId,
		})
		require.NoError(t, err)
		return run.JobRun.JobRunState == gluetypes.JobRunStateSucceeded
	}, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, aws.ToString(runResp.JobRunId), aws.ToString(run.JobRun.Id))
	assert.Equal(t, gluetypes.JobRunStateSucceeded, run.JobRun.JobRunState)

	runs, err := c.GetJobRuns(ctx, &glue.GetJobRunsInput{
		JobName: aws.String("glue-sdk-job"),
	})
	require.NoError(t, err)
	require.Len(t, runs.JobRuns, 1)

	// GetJobRuns for a non-existent job must raise EntityNotFoundException, not
	// silently return an empty list (matching real Glue).
	_, err = c.GetJobRuns(ctx, &glue.GetJobRunsInput{
		JobName: aws.String("glue-sdk-job-does-not-exist"),
	})
	require.Error(t, err)
	var notFound *gluetypes.EntityNotFoundException
	assert.ErrorAs(t, err, &notFound)
}

func TestGlue_Tags_SDK(t *testing.T) {
	c := glueClient()

	create, err := c.CreateJob(ctx, &glue.CreateJobInput{
		Name: aws.String("glue-sdk-tag-job"),
		Role: aws.String("arn:aws:iam::123456789012:role/glue-role"),
		Command: &gluetypes.JobCommand{
			Name:           aws.String("glueetl"),
			ScriptLocation: aws.String("s3://bucket/script.py"),
		},
	})
	require.NoError(t, err)
	_ = create
	t.Cleanup(func() {
		_, _ = c.DeleteJob(ctx, &glue.DeleteJobInput{JobName: aws.String("glue-sdk-tag-job")})
	})

	jobARN := "arn:aws:glue:us-east-1:123456789012:job/glue-sdk-tag-job"

	_, err = c.TagResource(ctx, &glue.TagResourceInput{
		ResourceArn: aws.String(jobARN),
		TagsToAdd:   map[string]string{"tier": "gold"},
	})
	require.NoError(t, err)

	tags, err := c.GetTags(ctx, &glue.GetTagsInput{
		ResourceArn: aws.String(jobARN),
	})
	require.NoError(t, err)
	assert.Equal(t, "gold", tags.Tags["tier"])

	_, err = c.UntagResource(ctx, &glue.UntagResourceInput{
		ResourceArn:  aws.String(jobARN),
		TagsToRemove: []string{"tier"},
	})
	require.NoError(t, err)

	tags, err = c.GetTags(ctx, &glue.GetTagsInput{ResourceArn: aws.String(jobARN)})
	require.NoError(t, err)
	assert.Empty(t, tags.Tags["tier"])
}
