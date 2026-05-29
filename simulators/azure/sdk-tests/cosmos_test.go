package azure_sdk_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureCosmosDB_ARMAndDataPlaneLifecycle(t *testing.T) {
	sub := "00000000-0000-0000-0000-000000000000"
	rg := "test-rg"
	account := "sdkcosmos"
	armBase := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DocumentDB/databaseAccounts/%s", sub, rg, account)

	resp := armReq(t, "PUT", armBase, `{"location":"eastus","kind":"GlobalDocumentDB","properties":{"databaseAccountOfferType":"Standard"}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(body), `"provisioningState":"Succeeded"`)
	assert.Contains(t, string(body), `"documentEndpoint"`)

	resp = armReq(t, "POST", armBase+"/listKeys", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(body), "primaryMasterKey")

	dbPath := armBase + "/sqlDatabases/appdb"
	resp = armReq(t, "PUT", dbPath, `{"properties":{"resource":{"id":"appdb"}}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	collPath := dbPath + "/containers/users"
	resp = armReq(t, "PUT", collPath, `{"properties":{"resource":{"id":"users","partitionKey":{"paths":["/id"],"kind":"Hash"}}}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	req, err := http.NewRequest("POST", baseURL+"/dbs/appdb/colls/users/docs", strings.NewReader(`{"id":"alice","team":"platform"}`))
	require.NoError(t, err)
	req.Header.Set("x-ms-cosmos-account", account)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	req, err = http.NewRequest("POST", baseURL+"/dbs/appdb/colls/users/docs", strings.NewReader(`{"query":"SELECT * FROM c WHERE c.team = 'platform'"}`))
	require.NoError(t, err)
	req.Header.Set("x-ms-cosmos-account", account)
	req.Header.Set("Content-Type", "application/query+json")
	req.Header.Set("x-ms-documentdb-isquery", "True")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(body), `"alice"`)
}
