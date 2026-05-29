package azure_cli_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAzureCosmosDB_ARMAndDataPlaneRESTCLIFlows(t *testing.T) {
	account := "clicosmos"
	armBase := armURL("Microsoft.DocumentDB", "databaseAccounts/"+account, "2024-05-15")
	runCLI(t, azRest("PUT", armBase, `{"location":"eastus","kind":"GlobalDocumentDB","properties":{"databaseAccountOfferType":"Standard"}}`))
	runCLI(t, azRest("POST", armBase+"/listKeys?api-version=2024-05-15", ""))

	dbURL := armURL("Microsoft.DocumentDB", "databaseAccounts/"+account+"/sqlDatabases/appdb", "2024-05-15")
	runCLI(t, azRest("PUT", dbURL, `{"properties":{"resource":{"id":"appdb"}}}`))

	collURL := armURL("Microsoft.DocumentDB", "databaseAccounts/"+account+"/sqlDatabases/appdb/containers/users", "2024-05-15")
	runCLI(t, azRest("PUT", collURL, `{"properties":{"resource":{"id":"users","partitionKey":{"paths":["/id"],"kind":"Hash"}}}}`))

	req, err := http.NewRequest("POST", baseURL+"/dbs/appdb/colls/users/docs", strings.NewReader(`{"id":"alice","team":"platform"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-ms-cosmos-account", account)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create document status = %d", resp.StatusCode)
	}

	req, err = http.NewRequest("POST", baseURL+"/dbs/appdb/colls/users/docs", strings.NewReader(`{"query":"SELECT * FROM c WHERE c.team = 'platform'"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-ms-cosmos-account", account)
	req.Header.Set("Content-Type", "application/query+json")
	req.Header.Set("x-ms-documentdb-isquery", "True")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "alice") {
		t.Fatalf("query response did not include alice: %s", string(body))
	}
}
