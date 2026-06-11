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
