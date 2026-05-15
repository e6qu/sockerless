package aws_sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
	amplifytypes "github.com/aws/aws-sdk-go-v2/service/amplify/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func amplifyClient() *amplify.Client {
	cfg := sdkConfig()
	return amplify.NewFromConfig(cfg, func(o *amplify.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestAmplifyAppLifecycle(t *testing.T) {
	c := amplifyClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	name := "sdk-app-" + time.Now().Format("150405.000000")
	createOut, err := c.CreateApp(ctx, &amplify.CreateAppInput{
		Name:                  aws.String(name),
		Description:           aws.String("sdk test"),
		Platform:              amplifytypes.PlatformWeb,
		EnableBranchAutoBuild: aws.Bool(true),
		Tags:                  map[string]string{"env": "test"},
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.App)
	appID := aws.ToString(createOut.App.AppId)
	require.NotEmpty(t, appID)
	assert.Equal(t, name, aws.ToString(createOut.App.Name))
	assert.Equal(t, appID+".amplifyapp.com", aws.ToString(createOut.App.DefaultDomain))

	getOut, err := c.GetApp(ctx, &amplify.GetAppInput{AppId: aws.String(appID)})
	require.NoError(t, err)
	assert.Equal(t, name, aws.ToString(getOut.App.Name))

	updOut, err := c.UpdateApp(ctx, &amplify.UpdateAppInput{
		AppId:       aws.String(appID),
		Description: aws.String("updated"),
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", aws.ToString(updOut.App.Description))

	listOut, err := c.ListApps(ctx, &amplify.ListAppsInput{})
	require.NoError(t, err)
	found := false
	for _, a := range listOut.Apps {
		if aws.ToString(a.AppId) == appID {
			found = true
		}
	}
	assert.True(t, found)

	_, err = c.DeleteApp(ctx, &amplify.DeleteAppInput{AppId: aws.String(appID)})
	require.NoError(t, err)

	_, err = c.GetApp(ctx, &amplify.GetAppInput{AppId: aws.String(appID)})
	require.Error(t, err)
}

func TestAmplifyBranchLifecycle(t *testing.T) {
	c := amplifyClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	app, err := c.CreateApp(ctx, &amplify.CreateAppInput{
		Name:     aws.String("br-app-" + time.Now().Format("150405.000000")),
		Platform: amplifytypes.PlatformWeb,
	})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	defer func() {
		_, _ = c.DeleteApp(ctx, &amplify.DeleteAppInput{AppId: aws.String(appID)})
	}()

	br, err := c.CreateBranch(ctx, &amplify.CreateBranchInput{
		AppId:           aws.String(appID),
		BranchName:      aws.String("main"),
		Stage:           amplifytypes.StagePullRequest, // any valid enum
		EnableAutoBuild: aws.Bool(true),
	})
	require.NoError(t, err)
	assert.Equal(t, "main", aws.ToString(br.Branch.BranchName))

	// Duplicate create
	_, err = c.CreateBranch(ctx, &amplify.CreateBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})
	require.Error(t, err)

	getBr, err := c.GetBranch(ctx, &amplify.GetBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})
	require.NoError(t, err)
	assert.Equal(t, "main", aws.ToString(getBr.Branch.BranchName))

	updBr, err := c.UpdateBranch(ctx, &amplify.UpdateBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
		Description: aws.String("updated branch"),
	})
	require.NoError(t, err)
	assert.Equal(t, "updated branch", aws.ToString(updBr.Branch.Description))

	listBr, err := c.ListBranches(ctx, &amplify.ListBranchesInput{AppId: aws.String(appID)})
	require.NoError(t, err)
	require.Len(t, listBr.Branches, 1)

	_, err = c.DeleteBranch(ctx, &amplify.DeleteBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})
	require.NoError(t, err)
}

func TestAmplifyWebhookLifecycle(t *testing.T) {
	c := amplifyClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	app, err := c.CreateApp(ctx, &amplify.CreateAppInput{
		Name: aws.String("wh-app-" + time.Now().Format("150405.000000")),
	})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	defer func() {
		_, _ = c.DeleteApp(ctx, &amplify.DeleteAppInput{AppId: aws.String(appID)})
	}()
	_, _ = c.CreateBranch(ctx, &amplify.CreateBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})

	wh, err := c.CreateWebhook(ctx, &amplify.CreateWebhookInput{
		AppId:       aws.String(appID),
		BranchName:  aws.String("main"),
		Description: aws.String("sdk webhook"),
	})
	require.NoError(t, err)
	whID := aws.ToString(wh.Webhook.WebhookId)
	require.NotEmpty(t, whID)
	require.NotEmpty(t, aws.ToString(wh.Webhook.WebhookUrl))

	getWh, err := c.GetWebhook(ctx, &amplify.GetWebhookInput{WebhookId: aws.String(whID)})
	require.NoError(t, err)
	assert.Equal(t, "main", aws.ToString(getWh.Webhook.BranchName))

	updWh, err := c.UpdateWebhook(ctx, &amplify.UpdateWebhookInput{
		WebhookId:   aws.String(whID),
		Description: aws.String("updated"),
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", aws.ToString(updWh.Webhook.Description))

	listWh, err := c.ListWebhooks(ctx, &amplify.ListWebhooksInput{AppId: aws.String(appID)})
	require.NoError(t, err)
	require.Len(t, listWh.Webhooks, 1)

	_, err = c.DeleteWebhook(ctx, &amplify.DeleteWebhookInput{WebhookId: aws.String(whID)})
	require.NoError(t, err)
}

func TestAmplifyJobLifecycle(t *testing.T) {
	c := amplifyClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	app, err := c.CreateApp(ctx, &amplify.CreateAppInput{
		Name: aws.String("job-app-" + time.Now().Format("150405.000000")),
	})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	defer func() {
		_, _ = c.DeleteApp(ctx, &amplify.DeleteAppInput{AppId: aws.String(appID)})
	}()
	_, _ = c.CreateBranch(ctx, &amplify.CreateBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})

	startOut, err := c.StartJob(ctx, &amplify.StartJobInput{
		AppId:         aws.String(appID),
		BranchName:    aws.String("main"),
		JobType:       amplifytypes.JobTypeRelease,
		CommitId:      aws.String("HEAD"),
		CommitMessage: aws.String("sdk test commit"),
		CommitTime:    aws.Time(time.Now()),
	})
	require.NoError(t, err)
	jobID := aws.ToString(startOut.JobSummary.JobId)
	require.NotEmpty(t, jobID)
	// Sim synthesises SUCCEEDED eagerly
	assert.Contains(t, []string{"SUCCEED", "SUCCEEDED", "RUNNING"}, string(startOut.JobSummary.Status))

	getJob, err := c.GetJob(ctx, &amplify.GetJobInput{
		AppId: aws.String(appID), BranchName: aws.String("main"), JobId: aws.String(jobID),
	})
	require.NoError(t, err)
	require.NotNil(t, getJob.Job)
	require.NotEmpty(t, getJob.Job.Steps)

	listJobs, err := c.ListJobs(ctx, &amplify.ListJobsInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})
	require.NoError(t, err)
	require.Len(t, listJobs.JobSummaries, 1)
}

func TestAmplifyCreateDeployment(t *testing.T) {
	c := amplifyClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	app, err := c.CreateApp(ctx, &amplify.CreateAppInput{
		Name: aws.String("dep-app-" + time.Now().Format("150405.000000")),
	})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	defer func() {
		_, _ = c.DeleteApp(ctx, &amplify.DeleteAppInput{AppId: aws.String(appID)})
	}()
	_, _ = c.CreateBranch(ctx, &amplify.CreateBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})

	createDep, err := c.CreateDeployment(ctx, &amplify.CreateDeploymentInput{
		AppId:      aws.String(appID),
		BranchName: aws.String("main"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(createDep.JobId))
	require.NotEmpty(t, aws.ToString(createDep.ZipUploadUrl))

	startDep, err := c.StartDeployment(ctx, &amplify.StartDeploymentInput{
		AppId:      aws.String(appID),
		BranchName: aws.String("main"),
		JobId:      createDep.JobId,
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(startDep.JobSummary.JobId))
}
