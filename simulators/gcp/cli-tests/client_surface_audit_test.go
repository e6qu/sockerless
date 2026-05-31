package gcp_cli_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_IAMServiceAccounts(t *testing.T) {
	out := runCLI(t, gcloudCLI("iam", "service-accounts", "create", "cli-audit-sa",
		"--display-name=CLI Audit SA",
		"--format=json",
		"--quiet"))
	assert.Contains(t, out, "cli-audit-sa@test-project.iam.gserviceaccount.com")

	list := runCLI(t, gcloudCLI("iam", "service-accounts", "list", "--format=json"))
	assert.Contains(t, list, "cli-audit-sa@test-project.iam.gserviceaccount.com")

	runCLI(t, gcloudCLI("iam", "service-accounts", "delete",
		"cli-audit-sa@test-project.iam.gserviceaccount.com",
		"--quiet"))
}

func TestCLI_PubSubTopicSubscriptionLifecycle(t *testing.T) {
	runCLI(t, gcloudCLI("pubsub", "topics", "create", "cli-audit-topic", "--format=json", "--quiet"))
	t.Cleanup(func() {
		_ = gcloudCLI("pubsub", "topics", "delete", "cli-audit-topic", "--quiet").Run()
	})

	runCLI(t, gcloudCLI("pubsub", "subscriptions", "create", "cli-audit-sub",
		"--topic=cli-audit-topic",
		"--format=json",
		"--quiet"))
	t.Cleanup(func() {
		_ = gcloudCLI("pubsub", "subscriptions", "delete", "cli-audit-sub", "--quiet").Run()
	})

	runCLI(t, gcloudCLI("pubsub", "topics", "publish", "cli-audit-topic", "--message=hello"))
	pulled := runCLI(t, gcloudCLI("pubsub", "subscriptions", "pull", "cli-audit-sub",
		"--limit=1",
		"--auto-ack",
		"--format=json"))
	var messages []struct {
		Message struct {
			Data string `json:"data"`
		} `json:"message"`
	}
	require.NoError(t, json.Unmarshal([]byte(pulled), &messages))
	require.Len(t, messages, 1)
	data, err := base64.StdEncoding.DecodeString(messages[0].Message.Data)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestCLI_APIGatewayApis(t *testing.T) {
	runCLI(t, gcloudCLI("api-gateway", "apis", "create", "cli-audit-api",
		"--display-name=CLI Audit API",
		"--async",
		"--format=json",
		"--quiet"))
	t.Cleanup(func() {
		_ = gcloudCLI("api-gateway", "apis", "delete", "cli-audit-api", "--async", "--quiet").Run()
	})

	list := runCLI(t, gcloudCLI("api-gateway", "apis", "list", "--format=json"))
	assert.Contains(t, list, "cli-audit-api")
}

func TestCLI_CloudBuildTriggers(t *testing.T) {
	configPath := filepath.Join(tmpDir, "cloudbuild-trigger.json")
	err := os.WriteFile(configPath, []byte(`{
  "name": "cli-audit-build-trigger",
  "filename": "cloudbuild.yaml",
  "triggerTemplate": {
    "repoName": "cli-repo",
    "branchName": "main"
  }
}`), 0o644)
	require.NoError(t, err)

	created := runCLI(t, gcloudCLI("builds", "triggers", "create", "manual",
		"--trigger-config="+configPath,
		"--format=json",
		"--quiet"))
	assert.Contains(t, created, "cli-audit-build-trigger")

	list := runCLI(t, gcloudCLI("builds", "triggers", "list", "--format=json"))
	assert.Contains(t, list, "cli-audit-build-trigger")

	var trigger struct {
		ID string `json:"id"`
	}
	parseJSON(t, created, &trigger)
	require.NotEmpty(t, trigger.ID)
	runCLI(t, gcloudCLI("builds", "triggers", "delete", trigger.ID, "--quiet"))
}
