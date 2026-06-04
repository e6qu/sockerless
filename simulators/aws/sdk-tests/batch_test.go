package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchtypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func batchClient() *batch.Client {
	return batch.NewFromConfig(sdkConfig(), func(o *batch.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestBatch_ComputeEnvironment_SDK(t *testing.T) {
	c := batchClient()

	create, err := c.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String("batch-sdk-ce"),
		Type:                   batchtypes.CETypeManaged,
		State:                  batchtypes.CEStateEnabled,
		ComputeResources: &batchtypes.ComputeResource{
			Type:     batchtypes.CRTypeEc2,
			MinvCpus: aws.Int32(0),
			MaxvCpus: aws.Int32(256),
			Subnets:  []string{"subnet-00000001"},
		},
		ServiceRole: aws.String("arn:aws:iam::123456789012:role/aws-batch-service-role"),
		Tags:        map[string]string{"env": "sdk"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(create.ComputeEnvironmentArn))
	t.Cleanup(func() {
		_, _ = c.DeleteComputeEnvironment(ctx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String("batch-sdk-ce"),
		})
	})

	describe, err := c.DescribeComputeEnvironments(ctx, &batch.DescribeComputeEnvironmentsInput{
		ComputeEnvironments: []string{"batch-sdk-ce"},
	})
	require.NoError(t, err)
	require.Len(t, describe.ComputeEnvironments, 1)
	ce := describe.ComputeEnvironments[0]
	assert.Equal(t, "batch-sdk-ce", aws.ToString(ce.ComputeEnvironmentName))
	assert.Equal(t, batchtypes.CEStateEnabled, ce.State)
	assert.Equal(t, batchtypes.CEStatusValid, ce.Status)

	_, err = c.UpdateComputeEnvironment(ctx, &batch.UpdateComputeEnvironmentInput{
		ComputeEnvironment: aws.String("batch-sdk-ce"),
		State:              batchtypes.CEStateDisabled,
	})
	require.NoError(t, err)

	describe, err = c.DescribeComputeEnvironments(ctx, &batch.DescribeComputeEnvironmentsInput{
		ComputeEnvironments: []string{"batch-sdk-ce"},
	})
	require.NoError(t, err)
	assert.Equal(t, batchtypes.CEStateDisabled, describe.ComputeEnvironments[0].State)
}

func TestBatch_JobQueue_SDK(t *testing.T) {
	c := batchClient()

	// Need a compute environment first
	_, err := c.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String("batch-sdk-ce-for-q"),
		Type:                   batchtypes.CETypeManaged,
		State:                  batchtypes.CEStateEnabled,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteComputeEnvironment(ctx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String("batch-sdk-ce-for-q"),
		})
	})

	ceArn := "arn:aws:batch:us-east-1:123456789012:compute-environment/batch-sdk-ce-for-q"
	create, err := c.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
		JobQueueName: aws.String("batch-sdk-jq"),
		State:        batchtypes.JQStateEnabled,
		Priority:     aws.Int32(10),
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{Order: aws.Int32(1), ComputeEnvironment: aws.String(ceArn)},
		},
		Tags: map[string]string{"env": "sdk"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(create.JobQueueArn))
	t.Cleanup(func() {
		_, _ = c.DeleteJobQueue(ctx, &batch.DeleteJobQueueInput{
			JobQueue: aws.String("batch-sdk-jq"),
		})
	})

	describe, err := c.DescribeJobQueues(ctx, &batch.DescribeJobQueuesInput{
		JobQueues: []string{"batch-sdk-jq"},
	})
	require.NoError(t, err)
	require.Len(t, describe.JobQueues, 1)
	assert.Equal(t, "batch-sdk-jq", aws.ToString(describe.JobQueues[0].JobQueueName))
	assert.Equal(t, batchtypes.JQStateEnabled, describe.JobQueues[0].State)

	_, err = c.UpdateJobQueue(ctx, &batch.UpdateJobQueueInput{
		JobQueue: aws.String("batch-sdk-jq"),
		State:    batchtypes.JQStateDisabled,
	})
	require.NoError(t, err)

	describe, err = c.DescribeJobQueues(ctx, &batch.DescribeJobQueuesInput{
		JobQueues: []string{"batch-sdk-jq"},
	})
	require.NoError(t, err)
	assert.Equal(t, batchtypes.JQStateDisabled, describe.JobQueues[0].State)
}

func TestBatch_JobDefinition_SDK(t *testing.T) {
	c := batchClient()

	reg, err := c.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String("batch-sdk-jd"),
		Type:              batchtypes.JobDefinitionTypeContainer,
		ContainerProperties: &batchtypes.ContainerProperties{
			Image:  aws.String("public.ecr.aws/docker/library/alpine:3"),
			Vcpus:  aws.Int32(1),
			Memory: aws.Int32(512),
		},
		Tags: map[string]string{"env": "sdk"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(reg.JobDefinitionArn))
	assert.EqualValues(t, 1, aws.ToInt32(reg.Revision))
	t.Cleanup(func() {
		_, _ = c.DeregisterJobDefinition(ctx, &batch.DeregisterJobDefinitionInput{
			JobDefinition: reg.JobDefinitionArn,
		})
	})

	describe, err := c.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitionName: aws.String("batch-sdk-jd"),
		Status:            aws.String("ACTIVE"),
	})
	require.NoError(t, err)
	require.Len(t, describe.JobDefinitions, 1)
	assert.Equal(t, "batch-sdk-jd", aws.ToString(describe.JobDefinitions[0].JobDefinitionName))
	assert.Equal(t, "ACTIVE", aws.ToString(describe.JobDefinitions[0].Status))
	assert.EqualValues(t, 1, aws.ToInt32(describe.JobDefinitions[0].Revision))
}

func TestBatch_JobSubmitDescribe_SDK(t *testing.T) {
	c := batchClient()

	// Setup compute environment and queue
	_, err := c.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String("batch-sdk-ce-job"),
		Type:                   batchtypes.CETypeManaged,
		State:                  batchtypes.CEStateEnabled,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteComputeEnvironment(ctx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String("batch-sdk-ce-job"),
		})
	})

	ceArn := "arn:aws:batch:us-east-1:123456789012:compute-environment/batch-sdk-ce-job"
	_, err = c.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
		JobQueueName: aws.String("batch-sdk-jq-job"),
		State:        batchtypes.JQStateEnabled,
		Priority:     aws.Int32(10),
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{Order: aws.Int32(1), ComputeEnvironment: aws.String(ceArn)},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteJobQueue(ctx, &batch.DeleteJobQueueInput{JobQueue: aws.String("batch-sdk-jq-job")})
	})

	reg, err := c.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String("batch-sdk-jd-job"),
		Type:              batchtypes.JobDefinitionTypeContainer,
		ContainerProperties: &batchtypes.ContainerProperties{
			Image:  aws.String("public.ecr.aws/docker/library/alpine:3"),
			Vcpus:  aws.Int32(1),
			Memory: aws.Int32(512),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeregisterJobDefinition(ctx, &batch.DeregisterJobDefinitionInput{
			JobDefinition: reg.JobDefinitionArn,
		})
	})

	submit, err := c.SubmitJob(ctx, &batch.SubmitJobInput{
		JobName:       aws.String("batch-sdk-job"),
		JobQueue:      aws.String("batch-sdk-jq-job"),
		JobDefinition: reg.JobDefinitionArn,
		Tags:          map[string]string{"run": "sdk"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(submit.JobId))

	describe, err := c.DescribeJobs(ctx, &batch.DescribeJobsInput{
		Jobs: []string{aws.ToString(submit.JobId)},
	})
	require.NoError(t, err)
	require.Len(t, describe.Jobs, 1)
	job := describe.Jobs[0]
	assert.Equal(t, aws.ToString(submit.JobId), aws.ToString(job.JobId))
	assert.Equal(t, "batch-sdk-job", aws.ToString(job.JobName))
	assert.Equal(t, batchtypes.JobStatusSucceeded, job.Status)
}
