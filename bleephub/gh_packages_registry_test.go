package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContainerRegistryPublishCreatesPackageVersion(t *testing.T) {
	requireStatus(t, registryRequest(t, http.MethodGet, "/v2/", nil), http.StatusOK)

	configDigest := uploadRegistryBlob(t, "admin/registry-image", []byte(`{"architecture":"amd64","os":"linux"}`))
	layerBytes := []byte("layer bytes")
	layerDigest := uploadRegistryBlob(t, "admin/registry-image", layerBytes)

	manifest := map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]interface{}{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"digest":    configDigest,
			"size":      len(`{"architecture":"amd64","os":"linux"}`),
		},
		"layers": []map[string]interface{}{
			{
				"mediaType": "application/vnd.oci.image.layer.v1.tar",
				"digest":    layerDigest,
				"size":      len(layerBytes),
			},
		},
	}
	manifestBytes := mustRegistryJSON(manifest)
	putManifest := registryRequest(t, http.MethodPut, "/v2/admin/registry-image/manifests/1.0.0", bytes.NewReader(manifestBytes))
	requireStatus(t, putManifest, http.StatusCreated)
	if got, want := putManifest.Header.Get("Docker-Content-Digest"), digestSHA256(manifestBytes); got != want {
		t.Fatalf("manifest digest = %q, want %q", got, want)
	}

	list := decodeJSONArray(t, ghGet(t, "/api/v3/users/admin/packages?package_type=container", defaultToken))
	foundPackage := false
	for _, pkg := range list {
		if pkg["name"] == "registry-image" && pkg["package_type"] == "container" {
			foundPackage = true
			if pkg["version_count"].(float64) != 1 {
				t.Fatalf("version_count = %v, want 1", pkg["version_count"])
			}
		}
	}
	if !foundPackage {
		t.Fatalf("published package not listed: %#v", list)
	}

	versions := decodeJSONArray(t, ghGet(t, "/api/v3/users/admin/packages/container/registry-image/versions", defaultToken))
	if len(versions) != 1 {
		t.Fatalf("versions len = %d, want 1: %#v", len(versions), versions)
	}
	if versions[0]["name"] != "1.0.0" {
		t.Fatalf("version name = %v, want 1.0.0", versions[0]["name"])
	}
	versionID := int(versions[0]["id"].(float64))
	files := decodeJSONArray(t, ghGet(t, "/api/v3/users/admin/packages/container/registry-image/versions/"+itoa(versionID)+"/files", defaultToken))
	if len(files) != 3 {
		t.Fatalf("files len = %d, want manifest + config + layer: %#v", len(files), files)
	}

	getManifest := registryRequest(t, http.MethodGet, "/v2/admin/registry-image/manifests/1.0.0", nil)
	requireStatusNoClose(t, getManifest, http.StatusOK)
	gotManifest, _ := io.ReadAll(getManifest.Body)
	getManifest.Body.Close()
	if !bytes.Equal(gotManifest, manifestBytes) {
		t.Fatalf("registry manifest bytes = %q, want %q", string(gotManifest), string(manifestBytes))
	}

	getBlob := registryRequest(t, http.MethodGet, "/v2/admin/registry-image/blobs/"+layerDigest, nil)
	requireStatusNoClose(t, getBlob, http.StatusOK)
	gotLayer, _ := io.ReadAll(getBlob.Body)
	getBlob.Body.Close()
	if !bytes.Equal(gotLayer, layerBytes) {
		t.Fatalf("registry blob bytes = %q, want %q", string(gotLayer), string(layerBytes))
	}
}

func TestPackageAndRegistryBytesUseObjectStore(t *testing.T) {
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: fs.bucket, prefix: "objects"}
	s := newTestServer()
	s.store.ObjectByteStore = &s3ActionsByteStore{fs: objectFS}
	admin := s.store.UsersByLogin["admin"]
	pkg, _ := s.store.CreatePackage("User", admin.Login, "container", "object-package", "public")
	version, err := s.store.CreatePackageVersion("User", admin.Login, "container", pkg.Name, "1.0.0", "", nil, []PackageFileInput{{
		Name:          "manifest.json",
		ContentType:   "application/vnd.oci.image.manifest.v1+json",
		ContentBase64: "cGFja2FnZSBvYmplY3QgYnl0ZXM=",
	}})
	if err != nil {
		t.Fatalf("create package version: %v", err)
	}
	files := s.store.ListPackageFiles(version.ID)
	if len(files) != 1 {
		t.Fatalf("package files len = %d, want 1", len(files))
	}
	got := readS3TestFile(t, objectFS, packageFileDataKey(files[0].ID))
	if string(got) != "package object bytes" {
		t.Fatalf("package object bytes = %q", string(got))
	}
	data, contentType, ok := s.packageVersionFileData(version.ID, "manifest.json")
	if !ok || string(data) != "package object bytes" || contentType != "application/vnd.oci.image.manifest.v1+json" {
		t.Fatalf("package file data ok=%v contentType=%q data=%q", ok, contentType, string(data))
	}
	s.registerGHPackagesRoutes()
	req := httptest.NewRequest("GET", "/api/v3/users/admin/packages/container/object-package/versions/"+itoa(version.ID)+"/files/"+itoa(files[0].ID), nil)
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	rec := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download package object file status = %d, body = %s", rec.Code, rec.Body.String())
	}
	downloaded := rec.Body.Bytes()
	if string(downloaded) != "package object bytes" {
		t.Fatalf("downloaded package object bytes = %q", string(downloaded))
	}

	digest := digestSHA256([]byte("registry object bytes"))
	if err := s.writeRegistryBlob(digest, []byte("registry object bytes")); err != nil {
		t.Fatalf("write registry blob: %v", err)
	}
	registryGot := readS3TestFile(t, objectFS, packageRegistryBlobDataKey(digest))
	if string(registryGot) != "registry object bytes" {
		t.Fatalf("registry object bytes = %q", string(registryGot))
	}
	readBack, err := s.readRegistryBlob(digest)
	if err != nil {
		t.Fatalf("read registry blob: %v", err)
	}
	if string(readBack) != "registry object bytes" {
		t.Fatalf("registry read back = %q", string(readBack))
	}
}

func TestDeleteRepoPurgesRepositoryPackageObjectBytes(t *testing.T) {
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: fs.bucket, prefix: "objects"}
	s := newTestServer()
	s.store.ObjectByteStore = &s3ActionsByteStore{fs: objectFS}
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "repo-package-objects", "", false)
	pkg, _ := s.store.CreatePackage("Repository", repo.FullName, "container", "image", "private")
	version, err := s.store.CreatePackageVersion("Repository", repo.FullName, "container", pkg.Name, "1.0.0", "", nil, []PackageFileInput{{
		Name:          "manifest.json",
		ContentType:   "application/vnd.oci.image.manifest.v1+json",
		ContentBase64: "cmVwbyBwYWNrYWdlIGJ5dGVz",
	}})
	if err != nil {
		t.Fatalf("create package version: %v", err)
	}
	files := s.store.ListPackageFiles(version.ID)
	if len(files) != 1 {
		t.Fatalf("package files len = %d, want 1", len(files))
	}
	if got := string(readS3TestFile(t, objectFS, files[0].StoragePath)); got != "repo package bytes" {
		t.Fatalf("package object bytes = %q", got)
	}

	deleted, err := s.store.DeleteRepo("admin", repo.Name)
	if err != nil {
		t.Fatalf("delete repo: %v", err)
	}
	if !deleted {
		t.Fatal("delete repo returned false")
	}
	if s.store.GetPackage(repo.FullName, "container", "image") != nil {
		t.Fatal("repository-owned package metadata survived repository deletion")
	}
	f, err := objectFS.Open(files[0].StoragePath)
	if err == nil {
		_ = f.Close()
		t.Fatalf("repository package object %s survived repository deletion", files[0].StoragePath)
	}
}

func TestContainerRegistryRejectsDuplicateVersionName(t *testing.T) {
	name := "admin/registry-duplicate"
	layerDigest := uploadRegistryBlob(t, name, []byte("layer"))
	manifest := mustRegistryJSON(map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"layers": []map[string]interface{}{
			{"mediaType": "application/vnd.oci.image.layer.v1.tar", "digest": layerDigest, "size": 5},
		},
	})
	requireStatus(t, registryRequest(t, http.MethodPut, "/v2/"+name+"/manifests/latest", bytes.NewReader(manifest)), http.StatusCreated)
	requireStatus(t, registryRequest(t, http.MethodPut, "/v2/"+name+"/manifests/latest", bytes.NewReader(manifest)), http.StatusConflict)
	versions := decodeJSONArray(t, ghGet(t, "/api/v3/users/admin/packages/container/registry-duplicate/versions", defaultToken))
	if len(versions) != 1 {
		t.Fatalf("versions len = %d, want 1 after duplicate push", len(versions))
	}
}

func TestContainerRegistryRequiresAuthentication(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, testBaseURL+"/v2/admin/no-auth/manifests/latest", strings.NewReader(`{"schemaVersion":2}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, resp, http.StatusUnauthorized)
}

func uploadRegistryBlob(t *testing.T, name string, data []byte) string {
	t.Helper()
	start := registryRequest(t, http.MethodPost, "/v2/"+name+"/blobs/uploads/", nil)
	requireStatus(t, start, http.StatusAccepted)
	location := start.Header.Get("Location")
	if location == "" {
		t.Fatalf("start upload missing Location")
	}
	patch := registryRequest(t, http.MethodPatch, location, bytes.NewReader(data))
	requireStatus(t, patch, http.StatusAccepted)
	digest := digestSHA256(data)
	sep := "?"
	if strings.Contains(location, "?") {
		sep = "&"
	}
	put := registryRequest(t, http.MethodPut, location+sep+"digest="+digest, nil)
	requireStatus(t, put, http.StatusCreated)
	if got := put.Header.Get("Docker-Content-Digest"); got != digest {
		t.Fatalf("blob digest = %q, want %q", got, digest)
	}
	return digest
}

func registryRequest(t *testing.T, method, path string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, testBaseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	if method == http.MethodPut && strings.Contains(path, "/manifests/") {
		req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func requireStatusNoClose(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, want, body)
	}
}

func mustRegistryJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
