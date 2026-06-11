package aws_cli_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// amplifyWaitJobStatus polls get-job until the job reaches the wanted
// status (the sim drives jobs through PENDING → RUNNING → SUCCEED on a
// short synthetic timer).
func amplifyWaitJobStatus(t *testing.T, appID, branch, jobID, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		out := runCLI(t, awsCLI("amplify", "get-job",
			"--app-id", appID,
			"--branch-name", branch,
			"--job-id", jobID,
			"--output", "json"))
		var result struct {
			Job struct {
				Summary struct {
					Status string `json:"status"`
				} `json:"summary"`
			} `json:"job"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &result))
		if result.Job.Summary.Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s never reached %s (last status %s)", jobID, want, result.Job.Summary.Status)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

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
			AppId      string `json:"appId"`
		} `json:"webhook"`
	}
	require.NoError(t, json.Unmarshal([]byte(whOut), &whResult))
	require.NotEmpty(t, whResult.Webhook.WebhookId)
	require.NotEmpty(t, whResult.Webhook.WebhookUrl)
	require.Equal(t, appID, whResult.Webhook.AppId)

	// Job — settles through the synthetic PENDING → RUNNING → SUCCEED
	// pipeline.
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
	jobID := jobResult.JobSummary.JobId
	amplifyWaitJobStatus(t, appID, "main", jobID, "SUCCEED")

	artifactsOut := runCLI(t, awsCLI("amplify", "list-artifacts",
		"--app-id", appID,
		"--branch-name", "main",
		"--job-id", jobID,
		"--max-results", "1",
		"--output", "json"))
	var artifactsResult struct {
		Artifacts []struct {
			ArtifactId       string `json:"artifactId"`
			ArtifactFileName string `json:"artifactFileName"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(artifactsOut), &artifactsResult))
	require.Len(t, artifactsResult.Artifacts, 1)
	require.NotEmpty(t, artifactsResult.Artifacts[0].ArtifactId)
	require.NotEmpty(t, artifactsResult.Artifacts[0].ArtifactFileName)

	artifactOut := runCLI(t, awsCLI("amplify", "get-artifact-url",
		"--artifact-id", artifactsResult.Artifacts[0].ArtifactId,
		"--output", "json"))
	var artifactResult struct {
		ArtifactId  string `json:"artifactId"`
		ArtifactUrl string `json:"artifactUrl"`
	}
	require.NoError(t, json.Unmarshal([]byte(artifactOut), &artifactResult))
	require.Equal(t, artifactsResult.Artifacts[0].ArtifactId, artifactResult.ArtifactId)
	require.NotEmpty(t, artifactResult.ArtifactUrl)

	logsOut := runCLI(t, awsCLI("amplify", "generate-access-logs",
		"--app-id", appID,
		"--domain-name", createResult.App.DefaultDomain,
		"--output", "json"))
	var logsResult struct {
		LogUrl string `json:"logUrl"`
	}
	require.NoError(t, json.Unmarshal([]byte(logsOut), &logsResult))
	require.NotEmpty(t, logsResult.LogUrl)

	// Stopping the finished job is rejected.
	_, err := awsCLI("amplify", "stop-job",
		"--app-id", appID,
		"--branch-name", "main",
		"--job-id", jobID,
		"--output", "json").CombinedOutput()
	require.Error(t, err)

	// A job stopped inside its run window lands CANCELLED.
	secondJobOut := runCLI(t, awsCLI("amplify", "start-job",
		"--app-id", appID, "--branch-name", "main",
		"--job-type", "RELEASE",
		"--output", "json"))
	var secondJobResult struct {
		JobSummary struct {
			JobId string `json:"jobId"`
		} `json:"jobSummary"`
	}
	require.NoError(t, json.Unmarshal([]byte(secondJobOut), &secondJobResult))
	secondJobID := secondJobResult.JobSummary.JobId
	stopOut := runCLI(t, awsCLI("amplify", "stop-job",
		"--app-id", appID,
		"--branch-name", "main",
		"--job-id", secondJobID,
		"--output", "json"))
	var stopResult struct {
		JobSummary struct {
			JobId  string `json:"jobId"`
			Status string `json:"status"`
		} `json:"jobSummary"`
	}
	require.NoError(t, json.Unmarshal([]byte(stopOut), &stopResult))
	require.Equal(t, secondJobID, stopResult.JobSummary.JobId)
	require.Equal(t, "CANCELLED", stopResult.JobSummary.Status)

	for _, id := range []string{jobID, secondJobID} {
		runCLI(t, awsCLI("amplify", "delete-job",
			"--app-id", appID,
			"--branch-name", "main",
			"--job-id", id,
			"--output", "json"))
	}

	_, err = awsCLI("amplify", "get-job",
		"--app-id", appID,
		"--branch-name", "main",
		"--job-id", jobID,
		"--output", "json").CombinedOutput()
	require.Error(t, err)

	// Cleanup
	runCLI(t, awsCLI("amplify", "delete-webhook", "--webhook-id", whResult.Webhook.WebhookId))
	runCLI(t, awsCLI("amplify", "delete-app", "--app-id", appID))
}

func TestAmplify_Deployment_Flow(t *testing.T) {
	name := "cli-dep-" + time.Now().Format("150405.000000")
	out := runCLI(t, awsCLI("amplify", "create-app", "--name", name, "--output", "json"))
	var createResult struct {
		App struct {
			AppId string `json:"appId"`
		} `json:"app"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &createResult))
	appID := createResult.App.AppId
	defer runCLI(t, awsCLI("amplify", "delete-app", "--app-id", appID))
	runCLI(t, awsCLI("amplify", "create-branch",
		"--app-id", appID, "--branch-name", "main", "--output", "json"))

	depOut := runCLI(t, awsCLI("amplify", "create-deployment",
		"--app-id", appID, "--branch-name", "main", "--output", "json"))
	var depResult struct {
		JobId          string            `json:"jobId"`
		ZipUploadUrl   string            `json:"zipUploadUrl"`
		FileUploadUrls map[string]string `json:"fileUploadUrls"`
	}
	require.NoError(t, json.Unmarshal([]byte(depOut), &depResult))
	require.NotEmpty(t, depResult.JobId)
	require.NotEmpty(t, depResult.ZipUploadUrl)

	// Upload a real site zip to the presigned URL the way the console /
	// amplify tooling does.
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	indexFile, err := zw.Create("index.html")
	require.NoError(t, err)
	_, err = indexFile.Write([]byte("<html>cli deployed " + depResult.JobId + "</html>"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	zipBytes := zipBuf.Bytes()
	putReq, err := http.NewRequest(http.MethodPut, depResult.ZipUploadUrl, bytes.NewReader(zipBytes))
	require.NoError(t, err)
	putReq.Header.Set("Content-Type", "application/zip")
	putResp, err := http.DefaultClient.Do(putReq)
	require.NoError(t, err)
	defer putResp.Body.Close()
	require.Equal(t, http.StatusOK, putResp.StatusCode)

	startOut := runCLI(t, awsCLI("amplify", "start-deployment",
		"--app-id", appID, "--branch-name", "main",
		"--job-id", depResult.JobId,
		"--output", "json"))
	var startResult struct {
		JobSummary struct {
			JobId   string `json:"jobId"`
			JobType string `json:"jobType"`
		} `json:"jobSummary"`
	}
	require.NoError(t, json.Unmarshal([]byte(startOut), &startResult))
	require.Equal(t, depResult.JobId, startResult.JobSummary.JobId)
	require.Equal(t, "MANUAL", startResult.JobSummary.JobType)

	amplifyWaitJobStatus(t, appID, "main", depResult.JobId, "SUCCEED")

	// The uploaded zip is the job artifact, byte for byte.
	artifactsOut := runCLI(t, awsCLI("amplify", "list-artifacts",
		"--app-id", appID, "--branch-name", "main",
		"--job-id", depResult.JobId, "--output", "json"))
	var artifactsResult struct {
		Artifacts []struct {
			ArtifactId string `json:"artifactId"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(artifactsOut), &artifactsResult))
	require.Len(t, artifactsResult.Artifacts, 1)
	urlOut := runCLI(t, awsCLI("amplify", "get-artifact-url",
		"--artifact-id", artifactsResult.Artifacts[0].ArtifactId, "--output", "json"))
	var urlResult struct {
		ArtifactUrl string `json:"artifactUrl"`
	}
	require.NoError(t, json.Unmarshal([]byte(urlOut), &urlResult))
	resp, err := http.Get(urlResult.ArtifactUrl)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	gotBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, zipBytes, gotBytes)

	// Hosting smoke: the deployed branch serves on its defaultDomain child
	// host (curl-style loopback request with a Host: header — the
	// cross-platform pattern, *.amplifyapp.com does not resolve locally).
	hostReq, err := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	require.NoError(t, err)
	hostReq.Host = "main." + appID + ".amplifyapp.com"
	hostResp, err := http.DefaultClient.Do(hostReq)
	require.NoError(t, err)
	defer hostResp.Body.Close()
	require.Equal(t, http.StatusOK, hostResp.StatusCode)
	hostBody, err := io.ReadAll(hostResp.Body)
	require.NoError(t, err)
	require.Equal(t, "<html>cli deployed "+depResult.JobId+"</html>", string(hostBody))
	require.Contains(t, hostResp.Header.Get("Content-Type"), "text/html")

	// start-deployment without jobId or sourceUrl is rejected.
	_, err = awsCLI("amplify", "start-deployment",
		"--app-id", appID, "--branch-name", "main",
		"--output", "json").CombinedOutput()
	require.Error(t, err)
}
