package gcp_sdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apigateway "google.golang.org/api/apigateway/v1"
	"google.golang.org/api/option"
	redis "google.golang.org/api/redis/v1"
)

func redisService(t *testing.T) *redis.Service {
	t.Helper()
	svc, err := redis.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

func apigatewayService(t *testing.T) *apigateway.Service {
	t.Helper()
	svc, err := apigateway.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

// TestMemorystoreRedis_InstanceLifecycle exercises the Memorystore
// Redis instance lifecycle: create (LRO) → get → list → patch (LRO)
// → delete (LRO).
func TestMemorystoreRedis_InstanceLifecycle(t *testing.T) {
	svc := redisService(t)
	parent := "projects/test-project/locations/us-central1"
	id := "test-cache"

	op, err := svc.Projects.Locations.Instances.Create(parent, &redis.Instance{
		Tier:         "BASIC",
		MemorySizeGb: 1,
	}).InstanceId(id).Do()
	require.NoError(t, err)
	assert.True(t, op.Done, "sim emits synchronous done=true LROs")

	inst, err := svc.Projects.Locations.Instances.Get(parent + "/instances/" + id).Do()
	require.NoError(t, err)
	assert.Equal(t, parent+"/instances/"+id, inst.Name)
	assert.Equal(t, "READY", inst.State)
	assert.Equal(t, int64(6379), inst.Port, "redis default port")

	list, err := svc.Projects.Locations.Instances.List(parent).Do()
	require.NoError(t, err)
	found := false
	for _, x := range list.Instances {
		if x.Name == inst.Name {
			found = true
			break
		}
	}
	assert.True(t, found)

	// Patch.
	_, err = svc.Projects.Locations.Instances.Patch(inst.Name, &redis.Instance{
		MemorySizeGb: 5,
	}).UpdateMask("memorySizeGb").Do()
	require.NoError(t, err)

	got, err := svc.Projects.Locations.Instances.Get(inst.Name).Do()
	require.NoError(t, err)
	assert.Equal(t, int64(5), got.MemorySizeGb)

	delOp, err := svc.Projects.Locations.Instances.Delete(inst.Name).Do()
	require.NoError(t, err)
	assert.True(t, delOp.Done)
}

// TestAPIGateway_Lifecycle exercises Api + ApiConfig + Gateway CRUD.
func TestAPIGateway_Lifecycle(t *testing.T) {
	svc := apigatewayService(t)
	project := "test-project"
	parentGlobal := "projects/" + project + "/locations/global"
	apiId := "test-api"

	// Create Api.
	apiOp, err := svc.Projects.Locations.Apis.Create(parentGlobal, &apigateway.ApigatewayApi{
		DisplayName: "Test API",
	}).ApiId(apiId).Do()
	require.NoError(t, err)
	assert.True(t, apiOp.Done)

	// Get Api.
	api, err := svc.Projects.Locations.Apis.Get(parentGlobal + "/apis/" + apiId).Do()
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", api.State)

	// Create ApiConfig.
	cfgOp, err := svc.Projects.Locations.Apis.Configs.Create(api.Name, &apigateway.ApigatewayApiConfig{
		DisplayName: "v1",
	}).ApiConfigId("v1").Do()
	require.NoError(t, err)
	assert.True(t, cfgOp.Done)

	// List ApiConfigs.
	cfgList, err := svc.Projects.Locations.Apis.Configs.List(api.Name).Do()
	require.NoError(t, err)
	require.Len(t, cfgList.ApiConfigs, 1)

	// Create Gateway in a regional location.
	parentRegion := "projects/" + project + "/locations/us-central1"
	gwOp, err := svc.Projects.Locations.Gateways.Create(parentRegion, &apigateway.ApigatewayGateway{
		ApiConfig: cfgList.ApiConfigs[0].Name,
	}).GatewayId("test-gw").Do()
	require.NoError(t, err)
	assert.True(t, gwOp.Done)

	gw, err := svc.Projects.Locations.Gateways.Get(parentRegion + "/gateways/test-gw").Do()
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", gw.State)
	assert.NotEmpty(t, gw.DefaultHostname)

	// Cleanup: delete in dependency order.
	_, _ = svc.Projects.Locations.Gateways.Delete(gw.Name).Do()
	_, _ = svc.Projects.Locations.Apis.Configs.Delete(cfgList.ApiConfigs[0].Name).Do()
	_, _ = svc.Projects.Locations.Apis.Delete(api.Name).Do()
}
