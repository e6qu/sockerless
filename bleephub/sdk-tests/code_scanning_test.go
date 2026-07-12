package sdktests

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func codeQLDatabaseBundle(t *testing.T, language string) []byte {
	t.Helper()
	var body bytes.Buffer
	zw := zip.NewWriter(&body)
	root := language + "-database"
	manifest, err := zw.Create(root + "/codeql-database.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Write([]byte("primaryLanguage: " + language + "\n")); err != nil {
		t.Fatal(err)
	}
	dataset, err := zw.Create(root + "/db-" + language + "/default/cache/pages/0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataset.Write([]byte("real SDK database dataset")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func TestCodeQLDatabaseOfficialClientLifecycle(t *testing.T) {
	repoName := uniqueName("codeql-database")
	createRepo(t, repoName)
	commit := createSDKDefaultBranch(t, repoName)
	bundle := codeQLDatabaseBundle(t, "go")

	uploadURL := fmt.Sprintf(
		"%s/repos/admin/%s/code-scanning/codeql/databases/go?name=go-database&commit_oid=%s",
		baseURL,
		repoName,
		commit.GetSHA(),
	)
	request, err := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(bundle))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "token "+adminToken)
	request.Header.Set("Content-Type", "application/zip")
	request.ContentLength = int64(len(bundle))
	response, err := rawHTTP.Do(request)
	if err != nil {
		t.Fatalf("CodeQL Action database upload: %v", err)
	}
	uploadBody, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("CodeQL Action database upload = %d body=%s", response.StatusCode, uploadBody)
	}

	databases, _, err := client.CodeScanning.ListCodeQLDatabases(ctx(), "admin", repoName)
	if err != nil {
		t.Fatalf("CodeScanning.ListCodeQLDatabases: %v", err)
	}
	if len(databases) != 1 {
		t.Fatalf("CodeScanning.ListCodeQLDatabases returned %d databases, want 1", len(databases))
	}
	if databases[0].GetName() != "go-database" || databases[0].GetLanguage() != "go" || databases[0].GetSize() != int64(len(bundle)) {
		t.Fatalf("CodeScanning.ListCodeQLDatabases returned %+v", databases[0])
	}

	database, _, err := client.CodeScanning.GetCodeQLDatabase(ctx(), "admin", repoName, "go")
	if err != nil {
		t.Fatalf("CodeScanning.GetCodeQLDatabase: %v", err)
	}
	if database.GetID() == 0 || database.GetContentType() != "application/zip" || database.GetUploader().GetLogin() != "admin" {
		t.Fatalf("CodeScanning.GetCodeQLDatabase returned %+v", database)
	}

	deleteRequest, err := http.NewRequest(
		http.MethodDelete,
		baseURL+"/api/v3/repos/admin/"+repoName+"/code-scanning/codeql/databases/go",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	deleteRequest.Header.Set("Authorization", "Bearer "+adminToken)
	deleteResponse, err := rawHTTP.Do(deleteRequest)
	if err != nil {
		t.Fatalf("delete CodeQL database: %v", err)
	}
	deleteResponse.Body.Close()
	if deleteResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("delete CodeQL database = %d, want 204", deleteResponse.StatusCode)
	}
	databases, _, err = client.CodeScanning.ListCodeQLDatabases(ctx(), "admin", repoName)
	if err != nil {
		t.Fatalf("CodeScanning.ListCodeQLDatabases after delete: %v", err)
	}
	if len(databases) != 0 {
		t.Fatalf("CodeScanning.ListCodeQLDatabases after delete returned %d databases", len(databases))
	}
}
