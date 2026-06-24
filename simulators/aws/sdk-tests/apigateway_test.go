package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func apigwClient() *apigateway.Client {
	return apigateway.NewFromConfig(sdkConfig(), func(o *apigateway.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func apigwv2Client() *apigatewayv2.Client {
	return apigatewayv2.NewFromConfig(sdkConfig(), func(o *apigatewayv2.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

// TestAPIGatewayV2_ApiLifecycle exercises the HTTP-API minimal flow:
// CreateApi → CreateIntegration → CreateRoute → CreateStage →
// GetApi → DeleteApi.
func TestAPIGatewayV2_ApiLifecycle(t *testing.T) {
	c := apigwv2Client()
	create, err := c.CreateApi(ctx, &apigatewayv2.CreateApiInput{
		Name:         aws.String("hello-api"),
		ProtocolType: "HTTP",
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(create.ApiId))
	apiId := aws.ToString(create.ApiId)
	t.Cleanup(func() {
		_, _ = c.DeleteApi(ctx, &apigatewayv2.DeleteApiInput{ApiId: aws.String(apiId)})
	})

	get, err := c.GetApi(ctx, &apigatewayv2.GetApiInput{ApiId: aws.String(apiId)})
	require.NoError(t, err)
	assert.Equal(t, "hello-api", aws.ToString(get.Name))

	intg, err := c.CreateIntegration(ctx, &apigatewayv2.CreateIntegrationInput{
		ApiId:           aws.String(apiId),
		IntegrationType: "HTTP_PROXY",
		IntegrationUri:  aws.String("https://example.com"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(intg.IntegrationId))

	rt, err := c.CreateRoute(ctx, &apigatewayv2.CreateRouteInput{
		ApiId:    aws.String(apiId),
		RouteKey: aws.String("GET /hello"),
		Target:   aws.String("integrations/" + aws.ToString(intg.IntegrationId)),
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(rt.RouteId))

	stage, err := c.CreateStage(ctx, &apigatewayv2.CreateStageInput{
		ApiId:      aws.String(apiId),
		StageName:  aws.String("$default"),
		AutoDeploy: aws.Bool(true),
	})
	require.NoError(t, err)
	assert.Equal(t, "$default", aws.ToString(stage.StageName))

	list, err := c.GetApis(ctx, &apigatewayv2.GetApisInput{})
	require.NoError(t, err)
	found := false
	for _, item := range list.Items {
		if aws.ToString(item.ApiId) == apiId {
			found = true
			break
		}
	}
	assert.True(t, found)
}

// TestAPIGateway_RestApiLifecycle exercises the REST-API minimal flow:
// CreateRestApi → GetResources → PutMethod → PutIntegration →
// CreateDeployment → CreateStage → DeleteRestApi.
func TestAPIGateway_RestApiLifecycle(t *testing.T) {
	c := apigwClient()
	create, err := c.CreateRestApi(ctx, &apigateway.CreateRestApiInput{
		Name:        aws.String("hello-rest"),
		Description: aws.String("integration test"),
	})
	require.NoError(t, err)
	apiId := aws.ToString(create.Id)
	require.NotEmpty(t, apiId)
	t.Cleanup(func() {
		_, _ = c.DeleteRestApi(ctx, &apigateway.DeleteRestApiInput{RestApiId: aws.String(apiId)})
	})

	// CreateRestApi auto-creates the root "/" resource.
	res, err := c.GetResources(ctx, &apigateway.GetResourcesInput{RestApiId: aws.String(apiId)})
	require.NoError(t, err)
	require.Len(t, res.Items, 1, "expected one root resource on a fresh REST API")
	rootId := aws.ToString(res.Items[0].Id)

	// CreateResource as a child of the root.
	child, err := c.CreateResource(ctx, &apigateway.CreateResourceInput{
		RestApiId: aws.String(apiId),
		ParentId:  aws.String(rootId),
		PathPart:  aws.String("hello"),
	})
	require.NoError(t, err)
	childId := aws.ToString(child.Id)
	require.NotEmpty(t, childId)

	// PutMethod on the child.
	_, err = c.PutMethod(ctx, &apigateway.PutMethodInput{
		RestApiId:         aws.String(apiId),
		ResourceId:        aws.String(childId),
		HttpMethod:        aws.String("GET"),
		AuthorizationType: aws.String("NONE"),
	})
	require.NoError(t, err)

	// PutIntegration.
	_, err = c.PutIntegration(ctx, &apigateway.PutIntegrationInput{
		RestApiId:  aws.String(apiId),
		ResourceId: aws.String(childId),
		HttpMethod: aws.String("GET"),
		Type:       "HTTP_PROXY",
		Uri:        aws.String("https://example.com"),
	})
	require.NoError(t, err)

	// CreateDeployment + CreateStage.
	dep, err := c.CreateDeployment(ctx, &apigateway.CreateDeploymentInput{
		RestApiId: aws.String(apiId),
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(dep.Id))

	stage, err := c.CreateStage(ctx, &apigateway.CreateStageInput{
		RestApiId:    aws.String(apiId),
		StageName:    aws.String("prod"),
		DeploymentId: dep.Id,
	})
	require.NoError(t, err)
	assert.Equal(t, "prod", aws.ToString(stage.StageName))

	// GetDeployments + GetStages list the resources just created.
	deps, err := c.GetDeployments(ctx, &apigateway.GetDeploymentsInput{RestApiId: aws.String(apiId)})
	require.NoError(t, err)
	foundDep := false
	for _, d := range deps.Items {
		if aws.ToString(d.Id) == aws.ToString(dep.Id) {
			foundDep = true
			break
		}
	}
	assert.True(t, foundDep, "created deployment should appear in GetDeployments")

	stages, err := c.GetStages(ctx, &apigateway.GetStagesInput{RestApiId: aws.String(apiId)})
	require.NoError(t, err)
	foundStage := false
	for _, s := range stages.Item {
		if aws.ToString(s.StageName) == "prod" {
			foundStage = true
			break
		}
	}
	assert.True(t, foundStage, "created stage should appear in GetStages")
}

// TestAPIGatewayV2_UpdateRoute exercises the PATCH route update path:
// CreateRoute → UpdateRoute (change RouteKey) → GetRoute reflects it.
func TestAPIGatewayV2_UpdateRoute(t *testing.T) {
	c := apigwv2Client()
	create, err := c.CreateApi(ctx, &apigatewayv2.CreateApiInput{
		Name:         aws.String("update-route-api"),
		ProtocolType: "HTTP",
	})
	require.NoError(t, err)
	apiId := aws.ToString(create.ApiId)
	require.NotEmpty(t, apiId)
	t.Cleanup(func() {
		_, _ = c.DeleteApi(ctx, &apigatewayv2.DeleteApiInput{ApiId: aws.String(apiId)})
	})

	rt, err := c.CreateRoute(ctx, &apigatewayv2.CreateRouteInput{
		ApiId:    aws.String(apiId),
		RouteKey: aws.String("GET /before"),
	})
	require.NoError(t, err)
	routeId := aws.ToString(rt.RouteId)
	require.NotEmpty(t, routeId)

	upd, err := c.UpdateRoute(ctx, &apigatewayv2.UpdateRouteInput{
		ApiId:    aws.String(apiId),
		RouteId:  aws.String(routeId),
		RouteKey: aws.String("POST /after"),
	})
	require.NoError(t, err)
	assert.Equal(t, "POST /after", aws.ToString(upd.RouteKey))

	got, err := c.GetRoute(ctx, &apigatewayv2.GetRouteInput{
		ApiId:   aws.String(apiId),
		RouteId: aws.String(routeId),
	})
	require.NoError(t, err)
	assert.Equal(t, "POST /after", aws.ToString(got.RouteKey))
}
