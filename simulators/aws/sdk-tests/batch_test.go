package aws_sdk_test

import (
	"testing"
	"time"

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
			Image:   aws.String(containerCommandImage),
			Command: []string{"log", "batch-sdk-ready"},
			Vcpus:   aws.Int32(1),
			Memory:  aws.Int32(512),
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

// TestBatch_DescribeJobDefinitions_FilterByList verifies the jobDefinitions[]
// filter: DescribeJobDefinitions with an explicit list of name:revision / ARN
// returns only those definitions, not every registered one.
func TestBatch_DescribeJobDefinitions_FilterByList_SDK(t *testing.T) {
	c := batchClient()

	mk := func(name string) string {
		reg, err := c.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
			JobDefinitionName: aws.String(name),
			Type:              batchtypes.JobDefinitionTypeContainer,
			ContainerProperties: &batchtypes.ContainerProperties{
				Image:  aws.String(containerCommandImage),
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
		return aws.ToString(reg.JobDefinitionArn)
	}
	arnA := mk("batch-sdk-filter-a")
	mk("batch-sdk-filter-b")

	// Filter to only definition A by ARN — B must not appear.
	out, err := c.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitions: []string{arnA},
	})
	require.NoError(t, err)
	require.Len(t, out.JobDefinitions, 1)
	assert.Equal(t, "batch-sdk-filter-a", aws.ToString(out.JobDefinitions[0].JobDefinitionName))

	// Filter by name:revision form.
	out2, err := c.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitions: []string{"batch-sdk-filter-b:1"},
	})
	require.NoError(t, err)
	require.Len(t, out2.JobDefinitions, 1)
	assert.Equal(t, "batch-sdk-filter-b", aws.ToString(out2.JobDefinitions[0].JobDefinitionName))
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

	var job batchtypes.JobDetail
	require.Eventually(t, func() bool {
		describe, err := c.DescribeJobs(ctx, &batch.DescribeJobsInput{
			Jobs: []string{aws.ToString(submit.JobId)},
		})
		require.NoError(t, err)
		require.Len(t, describe.Jobs, 1)
		job = describe.Jobs[0]
		return job.Status == batchtypes.JobStatusSucceeded
	}, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, aws.ToString(submit.JobId), aws.ToString(job.JobId))
	assert.Equal(t, "batch-sdk-job", aws.ToString(job.JobName))
	assert.Equal(t, batchtypes.JobStatusSucceeded, job.Status)
	assert.EqualValues(t, 0, aws.ToInt32(job.Container.ExitCode))
}

func TestBatch_SchedulingPolicy_SDK(t *testing.T) {
	c := batchClient()

	create, err := c.CreateSchedulingPolicy(ctx, &batch.CreateSchedulingPolicyInput{
		Name: aws.String("batch-sdk-sp"),
		FairsharePolicy: &batchtypes.FairsharePolicy{
			ShareDecaySeconds:  aws.Int32(3600),
			ComputeReservation: aws.Int32(50),
			ShareDistribution: []batchtypes.ShareAttributes{
				{ShareIdentifier: aws.String("teamA"), WeightFactor: aws.Float32(0.5)},
			},
		},
		Tags: map[string]string{"env": "sdk"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(create.Arn))
	require.Equal(t, "batch-sdk-sp", aws.ToString(create.Name))
	arn := aws.ToString(create.Arn)
	t.Cleanup(func() {
		_, _ = c.DeleteSchedulingPolicy(ctx, &batch.DeleteSchedulingPolicyInput{Arn: aws.String(arn)})
	})

	describe, err := c.DescribeSchedulingPolicies(ctx, &batch.DescribeSchedulingPoliciesInput{
		Arns: []string{arn},
	})
	require.NoError(t, err)
	require.Len(t, describe.SchedulingPolicies, 1)
	sp := describe.SchedulingPolicies[0]
	assert.Equal(t, "batch-sdk-sp", aws.ToString(sp.Name))
	require.NotNil(t, sp.FairsharePolicy)
	assert.EqualValues(t, 3600, aws.ToInt32(sp.FairsharePolicy.ShareDecaySeconds))
	assert.EqualValues(t, 50, aws.ToInt32(sp.FairsharePolicy.ComputeReservation))
	require.Len(t, sp.FairsharePolicy.ShareDistribution, 1)
	assert.Equal(t, "teamA", aws.ToString(sp.FairsharePolicy.ShareDistribution[0].ShareIdentifier))

	list, err := c.ListSchedulingPolicies(ctx, &batch.ListSchedulingPoliciesInput{})
	require.NoError(t, err)
	found := false
	for _, lp := range list.SchedulingPolicies {
		if aws.ToString(lp.Arn) == arn {
			found = true
		}
	}
	assert.True(t, found, "created scheduling policy should appear in ListSchedulingPolicies")

	_, err = c.UpdateSchedulingPolicy(ctx, &batch.UpdateSchedulingPolicyInput{
		Arn: aws.String(arn),
		FairsharePolicy: &batchtypes.FairsharePolicy{
			ShareDecaySeconds:  aws.Int32(7200),
			ComputeReservation: aws.Int32(25),
		},
	})
	require.NoError(t, err)

	describe, err = c.DescribeSchedulingPolicies(ctx, &batch.DescribeSchedulingPoliciesInput{Arns: []string{arn}})
	require.NoError(t, err)
	require.Len(t, describe.SchedulingPolicies, 1)
	require.NotNil(t, describe.SchedulingPolicies[0].FairsharePolicy)
	assert.EqualValues(t, 7200, aws.ToInt32(describe.SchedulingPolicies[0].FairsharePolicy.ShareDecaySeconds))

	// Resource-level tags on the scheduling policy ARN.
	_, err = c.TagResource(ctx, &batch.TagResourceInput{
		ResourceArn: aws.String(arn),
		Tags:        map[string]string{"team": "platform"},
	})
	require.NoError(t, err)
	tags, err := c.ListTagsForResource(ctx, &batch.ListTagsForResourceInput{ResourceArn: aws.String(arn)})
	require.NoError(t, err)
	assert.Equal(t, "platform", tags.Tags["team"])
	assert.Equal(t, "sdk", tags.Tags["env"])
	_, err = c.UntagResource(ctx, &batch.UntagResourceInput{
		ResourceArn: aws.String(arn),
		TagKeys:     []string{"team"},
	})
	require.NoError(t, err)
	tags, err = c.ListTagsForResource(ctx, &batch.ListTagsForResourceInput{ResourceArn: aws.String(arn)})
	require.NoError(t, err)
	_, has := tags.Tags["team"]
	assert.False(t, has, "untagged key should be gone")

	_, err = c.DeleteSchedulingPolicy(ctx, &batch.DeleteSchedulingPolicyInput{Arn: aws.String(arn)})
	require.NoError(t, err)
	describe, err = c.DescribeSchedulingPolicies(ctx, &batch.DescribeSchedulingPoliciesInput{Arns: []string{arn}})
	require.NoError(t, err)
	assert.Empty(t, describe.SchedulingPolicies)
}

func TestBatch_JobQueueWithSchedulingPolicy_SDK(t *testing.T) {
	c := batchClient()

	sp, err := c.CreateSchedulingPolicy(ctx, &batch.CreateSchedulingPolicyInput{
		Name: aws.String("batch-sdk-jq-sp"),
		FairsharePolicy: &batchtypes.FairsharePolicy{
			ShareDistribution: []batchtypes.ShareAttributes{
				{ShareIdentifier: aws.String("default"), WeightFactor: aws.Float32(1.0)},
			},
		},
	})
	require.NoError(t, err)
	spArn := aws.ToString(sp.Arn)
	t.Cleanup(func() {
		_, _ = c.DeleteSchedulingPolicy(ctx, &batch.DeleteSchedulingPolicyInput{Arn: aws.String(spArn)})
	})

	_, err = c.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String("batch-sdk-jq-sp-ce"),
		Type:                   batchtypes.CETypeManaged,
		State:                  batchtypes.CEStateEnabled,
		ComputeResources: &batchtypes.ComputeResource{
			Type:     batchtypes.CRTypeEc2,
			MinvCpus: aws.Int32(0),
			MaxvCpus: aws.Int32(16),
			Subnets:  []string{"subnet-00000001"},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteComputeEnvironment(ctx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String("batch-sdk-jq-sp-ce"),
		})
	})

	cq, err := c.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
		JobQueueName:        aws.String("batch-sdk-jq-sp"),
		Priority:            aws.Int32(1),
		SchedulingPolicyArn: aws.String(spArn),
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{Order: aws.Int32(1), ComputeEnvironment: aws.String("batch-sdk-jq-sp-ce")},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(cq.JobQueueArn))
	t.Cleanup(func() {
		_, _ = c.DeleteJobQueue(ctx, &batch.DeleteJobQueueInput{JobQueue: aws.String("batch-sdk-jq-sp")})
	})

	dq, err := c.DescribeJobQueues(ctx, &batch.DescribeJobQueuesInput{
		JobQueues: []string{"batch-sdk-jq-sp"},
	})
	require.NoError(t, err)
	require.Len(t, dq.JobQueues, 1)
	assert.Equal(t, spArn, aws.ToString(dq.JobQueues[0].SchedulingPolicyArn))
}
