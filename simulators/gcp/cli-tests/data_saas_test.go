package gcp_cli_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestBigQueryAndFirestore_RESTCLIFlows(t *testing.T) {
	dsBody := `{"datasetReference":{"projectId":"test-project","datasetId":"cli_dataset"},"location":"US"}`
	httpDoJSON(t, "POST", baseURL+"/bigquery/v2/projects/test-project/datasets", dsBody)

	tableBody := `{"tableReference":{"projectId":"test-project","datasetId":"cli_dataset","tableId":"events"},"schema":{"fields":[{"name":"id","type":"STRING"},{"name":"kind","type":"STRING"}]}}`
	httpDoJSON(t, "POST", baseURL+"/bigquery/v2/projects/test-project/datasets/cli_dataset/tables", tableBody)

	insertBody := `{"rows":[{"json":{"id":"evt-1","kind":"build"}},{"json":{"id":"evt-2","kind":"deploy"}}]}`
	httpDoJSON(t, "POST", baseURL+"/bigquery/v2/projects/test-project/datasets/cli_dataset/tables/events/insertAll", insertBody)

	query := `{"query":"SELECT id, kind FROM ` + "`test-project.cli_dataset.events`" + ` WHERE kind = 'deploy'"}`
	out := httpDoJSON(t, "POST", baseURL+"/bigquery/v2/projects/test-project/queries", query)
	if !strings.Contains(out, `"totalRows":"1"`) {
		t.Fatalf("BigQuery query did not return one row: %s", out)
	}

	docPath := "/v1/projects/test-project/databases/(default)/documents/cli-users/alice"
	docBody := `{"fields":{"team":{"stringValue":"platform"},"role":{"stringValue":"admin"}}}`
	httpDoJSON(t, "PATCH", baseURL+docPath, docBody)

	got := httpDoJSON(t, "GET", baseURL+docPath, "")
	if !strings.Contains(got, `"platform"`) {
		t.Fatalf("Firestore get did not return stored document: %s", got)
	}

	runQuery := `{"structuredQuery":{"from":[{"collectionId":"cli-users"}],"where":{"fieldFilter":{"field":{"fieldPath":"team"},"op":"EQUAL","value":{"stringValue":"platform"}}}}}`
	q := httpDoJSON(t, "POST", fmt.Sprintf("%s/v1/projects/test-project/databases/(default)/documents:runQuery", baseURL), runQuery)
	if !strings.Contains(q, "cli-users/alice") {
		t.Fatalf("Firestore runQuery did not return alice: %s", q)
	}
}
