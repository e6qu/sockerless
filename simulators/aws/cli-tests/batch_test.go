package aws_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch_ComputeEnvironment_CLI(t *testing.T) {
	out := runCLI(t, awsCLI("batch", "create-compute-environment",
		"--compute-environment-name", "batch-cli-ce",
		"--type", "MANAGED",
		"--state", "ENABLED",
		"--compute-resources", `{"type":"EC2","minvCpus":0,"maxvCpus":256,"subnets":["subnet-00000001"],"instanceRole":"arn:aws:iam::123456789012:instance-profile/ecsInstanceRole","instanceTypes":["m5.large"]}`,
	))
	var created struct {
		ComputeEnvironmentArn  string `json:"computeEnvironmentArn"`
		ComputeEnvironmentName string `json:"computeEnvironmentName"`
	}
	parseJSON(t, out, &created)
	require.NotEmpty(t, created.ComputeEnvironmentArn)
	t.Cleanup(func() {
		runCLI(t, awsCLI("batch", "delete-compute-environment",
			"--compute-environment", "batch-cli-ce"))
	})

	out = runCLI(t, awsCLI("batch", "describe-compute-environments",
		"--compute-environments", "batch-cli-ce"))
	var described struct {
		ComputeEnvironments []struct {
			ComputeEnvironmentName string `json:"computeEnvironmentName"`
			State                  string `json:"state"`
			Status                 string `json:"status"`
		} `json:"computeEnvironments"`
	}
	parseJSON(t, out, &described)
	require.Len(t, described.ComputeEnvironments, 1)
	ce := described.ComputeEnvironments[0]
	assert.Equal(t, "batch-cli-ce", ce.ComputeEnvironmentName)
	assert.Equal(t, "ENABLED", ce.State)
	assert.Equal(t, "VALID", ce.Status)

	runCLI(t, awsCLI("batch", "update-compute-environment",
		"--compute-environment", "batch-cli-ce",
		"--state", "DISABLED"))
	out = runCLI(t, awsCLI("batch", "describe-compute-environments",
		"--compute-environments", "batch-cli-ce"))
	parseJSON(t, out, &described)
	assert.Equal(t, "DISABLED", described.ComputeEnvironments[0].State)
}

func TestBatch_JobQueue_CLI(t *testing.T) {
	runCLI(t, awsCLI("batch", "create-compute-environment",
		"--compute-environment-name", "batch-cli-ce-q",
		"--type", "MANAGED",
		"--state", "ENABLED",
	))
	t.Cleanup(func() {
		runCLI(t, awsCLI("batch", "delete-compute-environment",
			"--compute-environment", "batch-cli-ce-q"))
	})

	ceArn := "arn:aws:batch:us-east-1:123456789012:compute-environment/batch-cli-ce-q"
	out := runCLI(t, awsCLI("batch", "create-job-queue",
		"--job-queue-name", "batch-cli-jq",
		"--state", "ENABLED",
		"--priority", "10",
		"--compute-environment-order", `[{"order":1,"computeEnvironment":"`+ceArn+`"}]`,
	))
	var created struct {
		JobQueueArn  string `json:"jobQueueArn"`
		JobQueueName string `json:"jobQueueName"`
	}
	parseJSON(t, out, &created)
	require.NotEmpty(t, created.JobQueueArn)
	t.Cleanup(func() {
		runCLI(t, awsCLI("batch", "delete-job-queue", "--job-queue", "batch-cli-jq"))
	})

	out = runCLI(t, awsCLI("batch", "describe-job-queues",
		"--job-queues", "batch-cli-jq"))
	var described struct {
		JobQueues []struct {
			JobQueueName string `json:"jobQueueName"`
			State        string `json:"state"`
		} `json:"jobQueues"`
	}
	parseJSON(t, out, &described)
	require.Len(t, described.JobQueues, 1)
	assert.Equal(t, "batch-cli-jq", described.JobQueues[0].JobQueueName)
	assert.Equal(t, "ENABLED", described.JobQueues[0].State)
}

func TestBatch_JobDefinition_CLI(t *testing.T) {
	out := runCLI(t, awsCLI("batch", "register-job-definition",
		"--job-definition-name", "batch-cli-jd",
		"--type", "container",
		"--container-properties", `{"image":"public.ecr.aws/docker/library/alpine:3","vcpus":1,"memory":512}`,
	))
	var reg struct {
		JobDefinitionArn  string `json:"jobDefinitionArn"`
		JobDefinitionName string `json:"jobDefinitionName"`
		Revision          int    `json:"revision"`
	}
	parseJSON(t, out, &reg)
	require.NotEmpty(t, reg.JobDefinitionArn)
	assert.Equal(t, 1, reg.Revision)
	t.Cleanup(func() {
		runCLI(t, awsCLI("batch", "deregister-job-definition",
			"--job-definition", reg.JobDefinitionArn))
	})

	out = runCLI(t, awsCLI("batch", "describe-job-definitions",
		"--job-definition-name", "batch-cli-jd",
		"--status", "ACTIVE"))
	var described struct {
		JobDefinitions []struct {
			JobDefinitionName string `json:"jobDefinitionName"`
			Revision          int    `json:"revision"`
			Status            string `json:"status"`
		} `json:"jobDefinitions"`
	}
	parseJSON(t, out, &described)
	require.Len(t, described.JobDefinitions, 1)
	assert.Equal(t, "batch-cli-jd", described.JobDefinitions[0].JobDefinitionName)
	assert.Equal(t, "ACTIVE", described.JobDefinitions[0].Status)
	assert.Equal(t, 1, described.JobDefinitions[0].Revision)
}

func TestBatch_SubmitJob_CLI(t *testing.T) {
	// Setup
	runCLI(t, awsCLI("batch", "create-compute-environment",
		"--compute-environment-name", "batch-cli-ce-job",
		"--type", "MANAGED",
		"--state", "ENABLED",
	))
	t.Cleanup(func() {
		runCLI(t, awsCLI("batch", "delete-compute-environment",
			"--compute-environment", "batch-cli-ce-job"))
	})

	ceArn := "arn:aws:batch:us-east-1:123456789012:compute-environment/batch-cli-ce-job"
	runCLI(t, awsCLI("batch", "create-job-queue",
		"--job-queue-name", "batch-cli-jq-job",
		"--state", "ENABLED",
		"--priority", "10",
		"--compute-environment-order", `[{"order":1,"computeEnvironment":"`+ceArn+`"}]`,
	))
	t.Cleanup(func() {
		runCLI(t, awsCLI("batch", "delete-job-queue", "--job-queue", "batch-cli-jq-job"))
	})

	out := runCLI(t, awsCLI("batch", "register-job-definition",
		"--job-definition-name", "batch-cli-jd-job",
		"--type", "container",
		"--container-properties", `{"image":"public.ecr.aws/docker/library/alpine:3","vcpus":1,"memory":512}`,
	))
	var reg struct {
		JobDefinitionArn string `json:"jobDefinitionArn"`
	}
	parseJSON(t, out, &reg)
	t.Cleanup(func() {
		runCLI(t, awsCLI("batch", "deregister-job-definition",
			"--job-definition", reg.JobDefinitionArn))
	})

	out = runCLI(t, awsCLI("batch", "submit-job",
		"--job-name", "batch-cli-job",
		"--job-queue", "batch-cli-jq-job",
		"--job-definition", reg.JobDefinitionArn,
	))
	var submitted struct {
		JobID   string `json:"jobId"`
		JobName string `json:"jobName"`
	}
	parseJSON(t, out, &submitted)
	require.NotEmpty(t, submitted.JobID)
	assert.Equal(t, "batch-cli-job", submitted.JobName)

	out = runCLI(t, awsCLI("batch", "describe-jobs",
		"--jobs", submitted.JobID))
	var described struct {
		Jobs []struct {
			JobID  string `json:"jobId"`
			Status string `json:"status"`
		} `json:"jobs"`
	}
	parseJSON(t, out, &described)
	require.Len(t, described.Jobs, 1)
	assert.Equal(t, submitted.JobID, described.Jobs[0].JobID)
	assert.Equal(t, "SUCCEEDED", described.Jobs[0].Status)

	out = runCLI(t, awsCLI("batch", "list-jobs",
		"--job-queue", "batch-cli-jq-job"))
	var listed struct {
		JobSummaryList []struct {
			JobID string `json:"jobId"`
		} `json:"jobSummaryList"`
	}
	parseJSON(t, out, &listed)
	found := false
	for _, j := range listed.JobSummaryList {
		if j.JobID == submitted.JobID {
			found = true
		}
	}
	assert.True(t, found)
}
