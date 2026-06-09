package aws_sdk_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func codebuildClient() *codebuild.Client {
	return codebuild.NewFromConfig(sdkConfig(), func(o *codebuild.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestCodeBuild_ProjectCRUD_SDK(t *testing.T) {
	c := codebuildClient()

	create, err := c.CreateProject(ctx, &codebuild.CreateProjectInput{
		Name: aws.String("cb-sdk-project"),
		Source: &cbtypes.ProjectSource{
			Type: cbtypes.SourceTypeNoSource,
		},
		Artifacts: &cbtypes.ProjectArtifacts{
			Type: cbtypes.ArtifactsTypeNoArtifacts,
		},
		Environment: &cbtypes.ProjectEnvironment{
			Type:        cbtypes.EnvironmentTypeLinuxContainer,
			Image:       aws.String("aws/codebuild/standard:7.0"),
			ComputeType: cbtypes.ComputeTypeBuildGeneral1Small,
		},
		ServiceRole: aws.String("arn:aws:iam::123456789012:role/cb-role"),
		Tags: []cbtypes.Tag{
			{Key: aws.String("env"), Value: aws.String("sdk")},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, create.Project)
	assert.Equal(t, "cb-sdk-project", aws.ToString(create.Project.Name))
	t.Cleanup(func() {
		_, _ = c.DeleteProject(ctx, &codebuild.DeleteProjectInput{Name: aws.String("cb-sdk-project")})
	})

	get, err := c.BatchGetProjects(ctx, &codebuild.BatchGetProjectsInput{
		Names: []string{"cb-sdk-project"},
	})
	require.NoError(t, err)
	require.Len(t, get.Projects, 1)
	assert.Equal(t, "cb-sdk-project", aws.ToString(get.Projects[0].Name))
	assert.Empty(t, get.ProjectsNotFound)

	list, err := c.ListProjects(ctx, &codebuild.ListProjectsInput{})
	require.NoError(t, err)
	found := false
	for _, name := range list.Projects {
		if name == "cb-sdk-project" {
			found = true
		}
	}
	assert.True(t, found)

	_, err = c.UpdateProject(ctx, &codebuild.UpdateProjectInput{
		Name:        aws.String("cb-sdk-project"),
		Description: aws.String("updated"),
	})
	require.NoError(t, err)

	get, err = c.BatchGetProjects(ctx, &codebuild.BatchGetProjectsInput{
		Names: []string{"cb-sdk-project"},
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", aws.ToString(get.Projects[0].Description))

	// Tags are set on create and accessible via BatchGetProjects.
	get, err = c.BatchGetProjects(ctx, &codebuild.BatchGetProjectsInput{
		Names: []string{"cb-sdk-project"},
	})
	require.NoError(t, err)
	found = false
	for _, tag := range get.Projects[0].Tags {
		if aws.ToString(tag.Key) == "env" && aws.ToString(tag.Value) == "sdk" {
			found = true
		}
	}
	assert.True(t, found, "expected env=sdk tag set at create time")
}

func TestCodeBuild_BuildLifecycle_SDK(t *testing.T) {
	c := codebuildClient()

	_, err := c.CreateProject(ctx, &codebuild.CreateProjectInput{
		Name: aws.String("cb-sdk-build-project"),
		Source: &cbtypes.ProjectSource{
			Type:      cbtypes.SourceTypeNoSource,
			Buildspec: aws.String("version: 0.2\nphases:\n  build:\n    commands:\n      - printf codebuild-sdk-ready\n"),
		},
		Artifacts: &cbtypes.ProjectArtifacts{
			Type: cbtypes.ArtifactsTypeNoArtifacts,
		},
		Environment: &cbtypes.ProjectEnvironment{
			Type:        cbtypes.EnvironmentTypeLinuxContainer,
			Image:       aws.String("aws/codebuild/standard:7.0"),
			ComputeType: cbtypes.ComputeTypeBuildGeneral1Small,
		},
		ServiceRole: aws.String("arn:aws:iam::123456789012:role/cb-role"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteProject(ctx, &codebuild.DeleteProjectInput{Name: aws.String("cb-sdk-build-project")})
	})

	startResp, err := c.StartBuild(ctx, &codebuild.StartBuildInput{
		ProjectName: aws.String("cb-sdk-build-project"),
	})
	require.NoError(t, err)
	require.NotNil(t, startResp.Build)
	buildID := aws.ToString(startResp.Build.Id)
	assert.NotEmpty(t, buildID)

	var build cbtypes.Build
	require.Eventually(t, func() bool {
		builds, err := c.BatchGetBuilds(ctx, &codebuild.BatchGetBuildsInput{
			Ids: []string{buildID},
		})
		require.NoError(t, err)
		require.Len(t, builds.Builds, 1)
		build = builds.Builds[0]
		return build.BuildStatus == cbtypes.StatusTypeSucceeded
	}, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, buildID, aws.ToString(build.Id))
	assert.NotEmpty(t, build.Phases)

	buildList, err := c.ListBuildsForProject(ctx, &codebuild.ListBuildsForProjectInput{
		ProjectName: aws.String("cb-sdk-build-project"),
	})
	require.NoError(t, err)
	require.Contains(t, buildList.Ids, buildID)

	allBuilds, err := c.ListBuilds(ctx, &codebuild.ListBuildsInput{})
	require.NoError(t, err)
	require.Contains(t, allBuilds.Ids, buildID)
}
