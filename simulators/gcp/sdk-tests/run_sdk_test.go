package gcp_sdk_test

import (
	"testing"
	"time"

	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
)

func newJobsClient(t *testing.T) *run.JobsClient {
	t.Helper()
	client, err := run.NewJobsRESTClient(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return client
}

func newExecutionsClient(t *testing.T) *run.ExecutionsClient {
	t.Helper()
	client, err := run.NewExecutionsRESTClient(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return client
}

func TestSDK_CloudRun_CreateJob(t *testing.T) {
	client := newJobsClient(t)

	op, err := client.CreateJob(ctx, &runpb.CreateJobRequest{
		Parent: "projects/test-project/locations/us-central1",
		JobId:  "sdk-create-job",
		Job: &runpb.Job{
			Template: &runpb.ExecutionTemplate{
				Template: &runpb.TaskTemplate{
					Containers: []*runpb.Container{
						{Image: "gcr.io/test/sdk:latest"},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	job, err := op.Wait(ctx)
	require.NoError(t, err)

	assert.Contains(t, job.Name, "sdk-create-job")
	assert.NotEmpty(t, job.Uid)
	assert.Equal(t, int64(1), job.Generation)
}

func TestSDK_CloudRun_RunJob(t *testing.T) {
	jobsClient := newJobsClient(t)

	// Create job first
	createOp, err := jobsClient.CreateJob(ctx, &runpb.CreateJobRequest{
		Parent: "projects/test-project/locations/us-central1",
		JobId:  "sdk-run-job",
		Job: &runpb.Job{
			Template: &runpb.ExecutionTemplate{
				Template: &runpb.TaskTemplate{
					Containers: []*runpb.Container{
						{Image: "gcr.io/test/sdk:latest"},
					},
					Timeout: durationpb.New(1 * time.Second),
				},
			},
		},
	})
	require.NoError(t, err)
	_, err = createOp.Wait(ctx)
	require.NoError(t, err)

	// Run job
	runOp, err := jobsClient.RunJob(ctx, &runpb.RunJobRequest{
		Name: "projects/test-project/locations/us-central1/jobs/sdk-run-job",
	})
	require.NoError(t, err)

	exec, err := runOp.Wait(ctx)
	require.NoError(t, err)

	assert.Contains(t, exec.Name, "sdk-run-job")
	assert.NotEmpty(t, exec.Name)
}

func TestSDK_CloudRun_GetExecution(t *testing.T) {
	jobsClient := newJobsClient(t)
	execClient := newExecutionsClient(t)

	// Create and run job
	createOp, err := jobsClient.CreateJob(ctx, &runpb.CreateJobRequest{
		Parent: "projects/test-project/locations/us-central1",
		JobId:  "sdk-getexec-job",
		Job: &runpb.Job{
			Template: &runpb.ExecutionTemplate{
				Template: &runpb.TaskTemplate{
					Containers: []*runpb.Container{
						{Image: "gcr.io/test/sdk:latest"},
					},
					Timeout: durationpb.New(1 * time.Second),
				},
			},
		},
	})
	require.NoError(t, err)
	_, err = createOp.Wait(ctx)
	require.NoError(t, err)

	runOp, err := jobsClient.RunJob(ctx, &runpb.RunJobRequest{
		Name: "projects/test-project/locations/us-central1/jobs/sdk-getexec-job",
	})
	require.NoError(t, err)

	exec, err := runOp.Wait(ctx)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(2 * time.Second)

	// Get execution via SDK
	gotExec, err := execClient.GetExecution(ctx, &runpb.GetExecutionRequest{
		Name: exec.Name,
	})
	require.NoError(t, err)

	assert.Equal(t, exec.Name, gotExec.Name)
	assert.Equal(t, int32(1), gotExec.SucceededCount)
	assert.Equal(t, int32(0), gotExec.RunningCount)
	assert.NotNil(t, gotExec.CompletionTime)
}

func TestSDK_CloudRun_CancelExecution(t *testing.T) {
	jobsClient := newJobsClient(t)
	execClient := newExecutionsClient(t)

	// Create and run a long-running job (no command = sleeps for default timeout)
	createOp, err := jobsClient.CreateJob(ctx, &runpb.CreateJobRequest{
		Parent: "projects/test-project/locations/us-central1",
		JobId:  "sdk-cancel-job",
		Job: &runpb.Job{
			Template: &runpb.ExecutionTemplate{
				Template: &runpb.TaskTemplate{
					Containers: []*runpb.Container{
						{Image: "gcr.io/test/sdk:latest"},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	_, err = createOp.Wait(ctx)
	require.NoError(t, err)

	runOp, err := jobsClient.RunJob(ctx, &runpb.RunJobRequest{
		Name: "projects/test-project/locations/us-central1/jobs/sdk-cancel-job",
	})
	require.NoError(t, err)

	exec, err := runOp.Wait(ctx)
	require.NoError(t, err)

	// Cancel immediately
	cancelOp, err := execClient.CancelExecution(ctx, &runpb.CancelExecutionRequest{
		Name: exec.Name,
	})
	require.NoError(t, err)

	cancelledExec, err := cancelOp.Wait(ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(0), cancelledExec.RunningCount)
	assert.Equal(t, int32(1), cancelledExec.CancelledCount)
	assert.NotNil(t, cancelledExec.CompletionTime)
}

func TestSDK_CloudRun_DeleteJob(t *testing.T) {
	client := newJobsClient(t)

	// Create job
	createOp, err := client.CreateJob(ctx, &runpb.CreateJobRequest{
		Parent: "projects/test-project/locations/us-central1",
		JobId:  "sdk-delete-job",
		Job: &runpb.Job{
			Template: &runpb.ExecutionTemplate{
				Template: &runpb.TaskTemplate{
					Containers: []*runpb.Container{
						{Image: "gcr.io/test/sdk:latest"},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	_, err = createOp.Wait(ctx)
	require.NoError(t, err)

	// Delete job
	deleteOp, err := client.DeleteJob(ctx, &runpb.DeleteJobRequest{
		Name: "projects/test-project/locations/us-central1/jobs/sdk-delete-job",
	})
	require.NoError(t, err)

	deletedJob, err := deleteOp.Wait(ctx)
	require.NoError(t, err)
	assert.Contains(t, deletedJob.Name, "sdk-delete-job")
}

func TestSDK_CloudRun_ListJobs(t *testing.T) {
	client := newJobsClient(t)

	// Create two jobs with unique prefix
	for _, id := range []string{"sdk-list-job-a", "sdk-list-job-b"} {
		op, err := client.CreateJob(ctx, &runpb.CreateJobRequest{
			Parent: "projects/test-project/locations/us-central1",
			JobId:  id,
			Job: &runpb.Job{
				Template: &runpb.ExecutionTemplate{
					Template: &runpb.TaskTemplate{
						Containers: []*runpb.Container{
							{Image: "gcr.io/test/sdk:latest"},
						},
					},
				},
			},
		})
		require.NoError(t, err)
		_, err = op.Wait(ctx)
		require.NoError(t, err)
	}

	// List jobs
	it := client.ListJobs(ctx, &runpb.ListJobsRequest{
		Parent: "projects/test-project/locations/us-central1",
	})

	var names []string
	for {
		job, err := it.Next()
		if err == iterator.Done {
			break
		}
		require.NoError(t, err)
		names = append(names, job.Name)
	}

	// Should contain at least our two jobs
	foundA, foundB := false, false
	for _, n := range names {
		if n == "projects/test-project/locations/us-central1/jobs/sdk-list-job-a" {
			foundA = true
		}
		if n == "projects/test-project/locations/us-central1/jobs/sdk-list-job-b" {
			foundB = true
		}
	}
	assert.True(t, foundA, "sdk-list-job-a not found in list")
	assert.True(t, foundB, "sdk-list-job-b not found in list")
}
