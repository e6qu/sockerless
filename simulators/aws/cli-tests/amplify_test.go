package aws_cli_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAmplify_App_Lifecycle(t *testing.T) {
	name := "cli-app-" + time.Now().Format("150405.000000")
	out := runCLI(t, awsCLI("amplify", "create-app",
		"--name", name,
		"--description", "cli test",
		"--platform", "WEB",
		"--output", "json",
	))
	var createResult struct {
		App struct {
			AppId         string `json:"appId"`
			Name          string `json:"name"`
			DefaultDomain string `json:"defaultDomain"`
		} `json:"app"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &createResult))
	require.NotEmpty(t, createResult.App.AppId)
	require.Equal(t, name, createResult.App.Name)
	appID := createResult.App.AppId

	runCLI(t, awsCLI("amplify", "get-app", "--app-id", appID, "--output", "json"))
	runCLI(t, awsCLI("amplify", "list-apps", "--output", "json"))

	// Branch
	brOut := runCLI(t, awsCLI("amplify", "create-branch",
		"--app-id", appID, "--branch-name", "main", "--output", "json"))
	var brResult struct {
		Branch struct {
			BranchName string `json:"branchName"`
		} `json:"branch"`
	}
	require.NoError(t, json.Unmarshal([]byte(brOut), &brResult))
	require.Equal(t, "main", brResult.Branch.BranchName)

	// Webhook
	whOut := runCLI(t, awsCLI("amplify", "create-webhook",
		"--app-id", appID, "--branch-name", "main",
		"--description", "cli webhook", "--output", "json"))
	var whResult struct {
		Webhook struct {
			WebhookId  string `json:"webhookId"`
			WebhookUrl string `json:"webhookUrl"`
		} `json:"webhook"`
	}
	require.NoError(t, json.Unmarshal([]byte(whOut), &whResult))
	require.NotEmpty(t, whResult.Webhook.WebhookId)
	require.NotEmpty(t, whResult.Webhook.WebhookUrl)

	// Job
	jobOut := runCLI(t, awsCLI("amplify", "start-job",
		"--app-id", appID, "--branch-name", "main",
		"--job-type", "RELEASE",
		"--output", "json"))
	var jobResult struct {
		JobSummary struct {
			JobId  string `json:"jobId"`
			Status string `json:"status"`
		} `json:"jobSummary"`
	}
	require.NoError(t, json.Unmarshal([]byte(jobOut), &jobResult))
	require.NotEmpty(t, jobResult.JobSummary.JobId)

	// Cleanup
	runCLI(t, awsCLI("amplify", "delete-webhook", "--webhook-id", whResult.Webhook.WebhookId))
	runCLI(t, awsCLI("amplify", "delete-app", "--app-id", appID))
}
