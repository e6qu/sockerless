package aws_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeBuild_ProjectCRUD_CLI(t *testing.T) {
	out := runCLI(t, awsCLI("codebuild", "create-project",
		"--name", "cb-cli-project",
		"--source", `{"type":"NO_SOURCE"}`,
		"--artifacts", `{"type":"NO_ARTIFACTS"}`,
		"--environment", `{"type":"LINUX_CONTAINER","image":"aws/codebuild/standard:7.0","computeType":"BUILD_GENERAL1_SMALL"}`,
		"--service-role", "arn:aws:iam::123456789012:role/cb-role",
	))
	var created struct {
		Project struct {
			Name string `json:"name"`
			Arn  string `json:"arn"`
		} `json:"project"`
	}
	parseJSON(t, out, &created)
	assert.Equal(t, "cb-cli-project", created.Project.Name)
	require.NotEmpty(t, created.Project.Arn)
	t.Cleanup(func() {
		runCLI(t, awsCLI("codebuild", "delete-project", "--name", "cb-cli-project"))
	})

	out = runCLI(t, awsCLI("codebuild", "batch-get-projects", "--names", "cb-cli-project"))
	var getProjects struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
		ProjectsNotFound []string `json:"projectsNotFound"`
	}
	parseJSON(t, out, &getProjects)
	require.Len(t, getProjects.Projects, 1)
	assert.Equal(t, "cb-cli-project", getProjects.Projects[0].Name)
	assert.Empty(t, getProjects.ProjectsNotFound)

	out = runCLI(t, awsCLI("codebuild", "list-projects"))
	var list struct {
		Projects []string `json:"projects"`
	}
	parseJSON(t, out, &list)
	found := false
	for _, p := range list.Projects {
		if p == "cb-cli-project" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestCodeBuild_Build_CLI(t *testing.T) {
	runCLI(t, awsCLI("codebuild", "create-project",
		"--name", "cb-cli-build-proj",
		"--source", `{"type":"NO_SOURCE"}`,
		"--artifacts", `{"type":"NO_ARTIFACTS"}`,
		"--environment", `{"type":"LINUX_CONTAINER","image":"aws/codebuild/standard:7.0","computeType":"BUILD_GENERAL1_SMALL"}`,
		"--service-role", "arn:aws:iam::123456789012:role/cb-role",
	))
	t.Cleanup(func() {
		runCLI(t, awsCLI("codebuild", "delete-project", "--name", "cb-cli-build-proj"))
	})

	out := runCLI(t, awsCLI("codebuild", "start-build",
		"--project-name", "cb-cli-build-proj",
	))
	var startResult struct {
		Build struct {
			ID          string `json:"id"`
			BuildStatus string `json:"buildStatus"`
		} `json:"build"`
	}
	parseJSON(t, out, &startResult)
	require.NotEmpty(t, startResult.Build.ID)
	assert.Equal(t, "SUCCEEDED", startResult.Build.BuildStatus)

	out = runCLI(t, awsCLI("codebuild", "batch-get-builds", "--ids", startResult.Build.ID))
	var getBuilds struct {
		Builds []struct {
			ID          string `json:"id"`
			BuildStatus string `json:"buildStatus"`
		} `json:"builds"`
	}
	parseJSON(t, out, &getBuilds)
	require.Len(t, getBuilds.Builds, 1)
	assert.Equal(t, startResult.Build.ID, getBuilds.Builds[0].ID)
	assert.Equal(t, "SUCCEEDED", getBuilds.Builds[0].BuildStatus)

	out = runCLI(t, awsCLI("codebuild", "list-builds-for-project",
		"--project-name", "cb-cli-build-proj"))
	var buildList struct {
		IDs []string `json:"ids"`
	}
	parseJSON(t, out, &buildList)
	require.Contains(t, buildList.IDs, startResult.Build.ID)
}
