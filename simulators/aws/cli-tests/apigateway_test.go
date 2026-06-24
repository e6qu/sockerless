package aws_cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIGateway_RestApiLifecycle(t *testing.T) {
	createdOut := runCLI(t, awsCLI("apigateway", "create-rest-api",
		"--name", "cli-rest-api",
		"--description", "cli coverage",
	))
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(createdOut), &created))
	require.NotEmpty(t, created.ID)
	t.Cleanup(func() {
		runCLI(t, awsCLI("apigateway", "delete-rest-api", "--rest-api-id", created.ID))
	})

	apisOut := runCLI(t, awsCLI("apigateway", "get-rest-apis"))
	assert.Contains(t, apisOut, created.ID)

	resourcesOut := runCLI(t, awsCLI("apigateway", "get-resources", "--rest-api-id", created.ID))
	var resources struct {
		Items []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(resourcesOut), &resources))
	require.Len(t, resources.Items, 1)
	rootID := resources.Items[0].ID

	resourceOut := runCLI(t, awsCLI("apigateway", "create-resource",
		"--rest-api-id", created.ID,
		"--parent-id", rootID,
		"--path-part", "cli",
	))
	var resource struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	require.NoError(t, json.Unmarshal([]byte(resourceOut), &resource))
	require.NotEmpty(t, resource.ID)
	assert.Equal(t, "/cli", resource.Path)

	runCLI(t, awsCLI("apigateway", "put-method",
		"--rest-api-id", created.ID,
		"--resource-id", resource.ID,
		"--http-method", "GET",
		"--authorization-type", "NONE",
	))
	assert.Contains(t, runCLI(t, awsCLI("apigateway", "get-method",
		"--rest-api-id", created.ID,
		"--resource-id", resource.ID,
		"--http-method", "GET",
	)), `"httpMethod": "GET"`)

	runCLI(t, awsCLI("apigateway", "put-integration",
		"--rest-api-id", created.ID,
		"--resource-id", resource.ID,
		"--http-method", "GET",
		"--type", "MOCK",
		"--request-templates", `{"application/json":"{\"statusCode\":200}"}`,
	))
	assert.Contains(t, runCLI(t, awsCLI("apigateway", "get-integration",
		"--rest-api-id", created.ID,
		"--resource-id", resource.ID,
		"--http-method", "GET",
	)), `"type": "MOCK"`)

	deploymentOut := runCLI(t, awsCLI("apigateway", "create-deployment",
		"--rest-api-id", created.ID,
		"--description", "cli deployment",
	))
	var deployment struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(deploymentOut), &deployment))
	require.NotEmpty(t, deployment.ID)

	runCLI(t, awsCLI("apigateway", "create-stage",
		"--rest-api-id", created.ID,
		"--stage-name", "cli",
		"--deployment-id", deployment.ID,
	))
	assert.Contains(t, runCLI(t, awsCLI("apigateway", "get-stage",
		"--rest-api-id", created.ID,
		"--stage-name", "cli",
	)), `"stageName": "cli"`)

	// List endpoints: the deployment + stage just created appear.
	assert.Contains(t, runCLI(t, awsCLI("apigateway", "get-deployments",
		"--rest-api-id", created.ID,
	)), deployment.ID)
	assert.Contains(t, runCLI(t, awsCLI("apigateway", "get-stages",
		"--rest-api-id", created.ID,
	)), `"stageName": "cli"`)

	runCLI(t, awsCLI("apigateway", "delete-stage",
		"--rest-api-id", created.ID,
		"--stage-name", "cli",
	))
	runCLI(t, awsCLI("apigateway", "delete-deployment",
		"--rest-api-id", created.ID,
		"--deployment-id", deployment.ID,
	))
	runCLI(t, awsCLI("apigateway", "delete-integration",
		"--rest-api-id", created.ID,
		"--resource-id", resource.ID,
		"--http-method", "GET",
	))
	runCLI(t, awsCLI("apigateway", "delete-method",
		"--rest-api-id", created.ID,
		"--resource-id", resource.ID,
		"--http-method", "GET",
	))
	runCLI(t, awsCLI("apigateway", "delete-resource",
		"--rest-api-id", created.ID,
		"--resource-id", resource.ID,
	))
}

func TestAPIGatewayV2_HttpApiLifecycle(t *testing.T) {
	apiOut := runCLI(t, awsCLI("apigatewayv2", "create-api",
		"--name", "cli-http-api",
		"--protocol-type", "HTTP",
	))
	var api struct {
		APIID string `json:"ApiId"`
	}
	require.NoError(t, json.Unmarshal([]byte(apiOut), &api))
	require.NotEmpty(t, api.APIID)
	t.Cleanup(func() {
		runCLI(t, awsCLI("apigatewayv2", "delete-api", "--api-id", api.APIID))
	})

	assert.Contains(t, runCLI(t, awsCLI("apigatewayv2", "get-apis")), api.APIID)

	integrationOut := runCLI(t, awsCLI("apigatewayv2", "create-integration",
		"--api-id", api.APIID,
		"--integration-type", "HTTP_PROXY",
		"--integration-uri", "https://example.com",
		"--integration-method", "GET",
		"--payload-format-version", "1.0",
	))
	var integration struct {
		IntegrationID string `json:"IntegrationId"`
	}
	require.NoError(t, json.Unmarshal([]byte(integrationOut), &integration))
	require.NotEmpty(t, integration.IntegrationID)
	assert.Contains(t, runCLI(t, awsCLI("apigatewayv2", "get-integration",
		"--api-id", api.APIID,
		"--integration-id", integration.IntegrationID,
	)), integration.IntegrationID)

	routeOut := runCLI(t, awsCLI("apigatewayv2", "create-route",
		"--api-id", api.APIID,
		"--route-key", "GET /cli",
		"--target", "integrations/"+integration.IntegrationID,
	))
	var route struct {
		RouteID string `json:"RouteId"`
	}
	require.NoError(t, json.Unmarshal([]byte(routeOut), &route))
	require.NotEmpty(t, route.RouteID)
	assert.Contains(t, runCLI(t, awsCLI("apigatewayv2", "get-route",
		"--api-id", api.APIID,
		"--route-id", route.RouteID,
	)), route.RouteID)

	// UpdateRoute (PATCH) changes the route key; GetRoute reflects it.
	assert.Contains(t, runCLI(t, awsCLI("apigatewayv2", "update-route",
		"--api-id", api.APIID,
		"--route-id", route.RouteID,
		"--route-key", "POST /cli-updated",
	)), `"RouteKey": "POST /cli-updated"`)
	assert.Contains(t, runCLI(t, awsCLI("apigatewayv2", "get-route",
		"--api-id", api.APIID,
		"--route-id", route.RouteID,
	)), `"RouteKey": "POST /cli-updated"`)

	deploymentOut := runCLI(t, awsCLI("apigatewayv2", "create-deployment",
		"--api-id", api.APIID,
		"--description", "cli deployment",
	))
	var deployment struct {
		DeploymentID string `json:"DeploymentId"`
	}
	require.NoError(t, json.Unmarshal([]byte(deploymentOut), &deployment))
	require.NotEmpty(t, deployment.DeploymentID)

	runCLI(t, awsCLI("apigatewayv2", "create-stage",
		"--api-id", api.APIID,
		"--stage-name", "cli",
		"--deployment-id", deployment.DeploymentID,
	))
	assert.Contains(t, runCLI(t, awsCLI("apigatewayv2", "get-stage",
		"--api-id", api.APIID,
		"--stage-name", "cli",
	)), `"StageName": "cli"`)

	runCLI(t, awsCLI("apigatewayv2", "delete-stage",
		"--api-id", api.APIID,
		"--stage-name", "cli",
	))
	runCLI(t, awsCLI("apigatewayv2", "delete-deployment",
		"--api-id", api.APIID,
		"--deployment-id", deployment.DeploymentID,
	))
	runCLI(t, awsCLI("apigatewayv2", "delete-route",
		"--api-id", api.APIID,
		"--route-id", route.RouteID,
	))
	runCLI(t, awsCLI("apigatewayv2", "delete-integration",
		"--api-id", api.APIID,
		"--integration-id", integration.IntegrationID,
	))
}
