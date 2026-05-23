package azure_sdk_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAzureServiceBus_ARMLifecycle exercises namespace + queue +
// topic + subscription CRUD on the Microsoft.ServiceBus surface.
func TestAzureServiceBus_ARMLifecycle(t *testing.T) {
	sub := "00000000-0000-0000-0000-000000000000"
	rg := "test-rg"
	ns := "lifecycle-sb"
	nsPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ServiceBus/namespaces/%s", sub, rg, ns)

	resp := armReq(t, "PUT", nsPath, `{"location":"eastus","sku":{"name":"Standard","tier":"Standard"}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(body), `"provisioningState":"Succeeded"`)
	assert.Contains(t, string(body), "lifecycle-sb.servicebus.windows.net")

	// Create a queue.
	qPath := nsPath + "/queues/myqueue"
	resp = armReq(t, "PUT", qPath, `{"properties":{"maxSizeInMegabytes":2048}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Create a topic + subscription.
	tPath := nsPath + "/topics/mytopic"
	resp = armReq(t, "PUT", tPath, `{"properties":{}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	subPath := tPath + "/subscriptions/mysub"
	resp = armReq(t, "PUT", subPath, `{"properties":{}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// List subscriptions.
	resp = armReq(t, "GET", tPath+"/subscriptions", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	listBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(listBody), "mysub")

	// Delete namespace (cascade).
	resp = armReq(t, "DELETE", nsPath, "")
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	resp = armReq(t, "GET", qPath, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"queue should be cascade-deleted when namespace is deleted")
	resp.Body.Close()
}

// TestAzureAPIM_ARMLifecycle exercises service + api + product +
// subscription CRUD on the Microsoft.ApiManagement surface.
func TestAzureAPIM_ARMLifecycle(t *testing.T) {
	sub := "00000000-0000-0000-0000-000000000000"
	rg := "test-rg"
	name := "lifecycle-apim"
	svcPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ApiManagement/service/%s", sub, rg, name)

	resp := armReq(t, "PUT", svcPath, `{"location":"eastus","sku":{"name":"Developer","capacity":1},"properties":{"publisherEmail":"ops@example.com","publisherName":"Ops"}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(body), `"provisioningState":"Succeeded"`)
	assert.Contains(t, string(body), "lifecycle-apim.azure-api.net")

	apiPath := svcPath + "/apis/myapi"
	resp = armReq(t, "PUT", apiPath, `{"properties":{"displayName":"My API","path":"v1"}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	prodPath := svcPath + "/products/myproduct"
	resp = armReq(t, "PUT", prodPath, `{"properties":{"displayName":"My Product","state":"published"}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	apimSubPath := svcPath + "/subscriptions/sub1"
	resp = armReq(t, "PUT", apimSubPath, `{"properties":{"displayName":"Sub 1"}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = armReq(t, "GET", svcPath+"/apis", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	listBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(listBody), "myapi")

	resp = armReq(t, "DELETE", svcPath, "")
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	resp = armReq(t, "GET", apiPath, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"api should be cascade-deleted when service is deleted")
	resp.Body.Close()
}
