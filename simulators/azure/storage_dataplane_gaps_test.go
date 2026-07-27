package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sim "github.com/sockerless/simulator"
)

// The Blob / Files / Queues data planes select an operation from the
// `restype` + `comp` query pair. For a value a dispatcher does not serve it
// must say so — never run whichever sibling handler happens to sit under the
// same method. The difference is not cosmetic: falling through means
// `PUT /{container}/{blob}?comp=tier` (Set Blob Tier) CREATES A BLOB and
// answers 201, so the caller is told a tier change succeeded that never
// happened, and a coverage measurement of the plane counts an operation the
// simulator has never implemented.
//
// Every gap case below therefore asserts two things: the declared gap in the
// response, and — the part that actually catches the bug — that the store is
// untouched. A test that only checked the status code would still pass if the
// handler answered 501 after creating the blob.

func buildStorageTestSim(t *testing.T) *sim.Server {
	t.Helper()
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := buildSimulator(sim.Config{Provider: "azure", ListenAddr: ":0", LogLevel: "error"})
	if err != nil {
		t.Fatalf("build simulator: %v", err)
	}
	return srv
}

// storagePlaneReq issues one request at a storage data-plane coordinate: the
// `<account>.<service>.<host>` hostname Azure publishes the plane on, which is
// how the simulator's host-addressed middleware reaches it.
func storagePlaneReq(t *testing.T, srv *sim.Server, method, account, service, target string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, target, reader)
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Host = account + "." + service + ".localhost"
	req.Header.Set("x-ms-version", "2025-01-05")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// assertStorageGap asserts the response is a declared gap: 501 in the storage
// XML error envelope, with the machine-readable code in x-ms-error-code where
// every Azure Storage SDK looks for it.
func assertStorageGap(t *testing.T, rec *httptest.ResponseRecorder, what string) {
	t.Helper()
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("%s: status = %d, want 501 — the dispatcher answered instead of declaring the gap: %s",
			what, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("x-ms-error-code"); got != "NotImplemented" {
		t.Fatalf("%s: x-ms-error-code = %q, want NotImplemented", what, got)
	}
	var body struct {
		XMLName xml.Name `xml:"Error"`
		Code    string   `xml:"Code"`
		Message string   `xml:"Message"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: response is not a storage XML error: %v (%s)", what, err, rec.Body.String())
	}
	if body.Code != "NotImplemented" || !strings.Contains(body.Message, "not implemented by the simulator") {
		t.Fatalf("%s: error body = %+v, want a NotImplemented gap declaration", what, body)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int, what string) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("%s: status = %d, want %d: %s", what, rec.Code, want, rec.Body.String())
	}
}

// ── Blob ────────────────────────────────────────────────────────────

func TestBlobDataPlaneServedOperations(t *testing.T) {
	srv := buildStorageTestSim(t)
	const account, container, blob = "gapblobacct", "served-container", "served.txt"

	// Get Blob Service Properties. The azurerm provider polls this while waiting
	// for a storage account's data plane to come up, so a gap here fails an
	// apply rather than merely leaving an operation unimplemented.
	assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/?restype=service&comp=properties", nil, nil),
		http.StatusOK, "GetBlobServiceProperties")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodHead, account, "blob", "/?restype=service&comp=properties", nil, nil),
		http.StatusOK, "GetBlobServiceProperties (HEAD)")

	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob", "/"+container+"?restype=container", nil, nil),
		http.StatusCreated, "CreateContainer")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"?restype=container", nil, nil),
		http.StatusOK, "GetContainerProperties")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodHead, account, "blob", "/"+container+"?restype=container", nil, nil),
		http.StatusOK, "GetContainerProperties (HEAD)")

	rec := storagePlaneReq(t, srv, http.MethodPut, account, "blob", "/"+container+"/"+blob, []byte("v1"),
		map[string]string{"x-ms-blob-type": "BlockBlob"})
	assertStatus(t, rec, http.StatusCreated, "PutBlob")

	rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"/"+blob, nil, nil)
	assertStatus(t, rec, http.StatusOK, "GetBlob")
	if got := rec.Body.String(); got != "v1" {
		t.Fatalf("GetBlob body = %q, want %q", got, "v1")
	}
	assertStatus(t, storagePlaneReq(t, srv, http.MethodHead, account, "blob", "/"+container+"/"+blob, nil, nil),
		http.StatusOK, "GetBlobProperties")

	// A ranged Get Blob answers 206 with Content-Range. Clients that read a
	// blob in chunks — the az CLI's `storage blob download` among them — fail
	// outright when a ranged request is answered 200 with the whole blob.
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"/"+blob, nil,
		map[string]string{"x-ms-range": "bytes=1-1"})
	assertStatus(t, rec, http.StatusPartialContent, "GetBlob (ranged)")
	if got := rec.Body.String(); got != "1" {
		t.Fatalf("ranged GetBlob body = %q, want %q", got, "1")
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 1-1/2" {
		t.Fatalf("Content-Range = %q, want %q", got, "bytes 1-1/2")
	}
	// A range running past the blob is clamped, exactly as Azure clamps it —
	// the az CLI asks for the whole 4 MiB first chunk of a two-byte blob.
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"/"+blob, nil,
		map[string]string{"Range": "bytes=0-4194303"})
	assertStatus(t, rec, http.StatusPartialContent, "GetBlob (clamped range)")
	if got := rec.Header().Get("Content-Range"); got != "bytes 0-1/2" {
		t.Fatalf("clamped Content-Range = %q, want %q", got, "bytes 0-1/2")
	}
	if got := rec.Body.String(); got != "v1" {
		t.Fatalf("clamped ranged GetBlob body = %q, want %q", got, "v1")
	}
	// A start past the end is unsatisfiable.
	assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"/"+blob, nil,
		map[string]string{"x-ms-range": "bytes=9-9"}), http.StatusRequestedRangeNotSatisfiable, "GetBlob (out of range)")

	// Copy Blob: the bare PUT plus x-ms-copy-source, exactly as Azure spells it.
	rec = storagePlaneReq(t, srv, http.MethodPut, account, "blob", "/"+container+"/copy.txt", nil,
		map[string]string{"x-ms-copy-source": "http://" + account + ".blob.localhost/" + container + "/" + blob})
	assertStatus(t, rec, http.StatusAccepted, "CopyBlob")
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"/copy.txt", nil, nil)
	if got := rec.Body.String(); got != "v1" {
		t.Fatalf("copied blob body = %q, want %q", got, "v1")
	}

	// Staged blocks then a commit, and the block list read back.
	blockID := "YmxvY2sx"
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob",
		"/"+container+"/blocks.txt?comp=block&blockid="+blockID, []byte("chunk"), nil),
		http.StatusCreated, "StageBlock")
	commit := []byte(`<?xml version="1.0" encoding="utf-8"?><BlockList><Latest>` + blockID + `</Latest></BlockList>`)
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob",
		"/"+container+"/blocks.txt?comp=blocklist", commit, nil),
		http.StatusCreated, "CommitBlockList")
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob",
		"/"+container+"/blocks.txt?comp=blocklist&blocklisttype=all", nil, nil)
	assertStatus(t, rec, http.StatusOK, "GetBlockList")
	if !strings.Contains(rec.Body.String(), blockID) {
		t.Fatalf("GetBlockList body = %s, want it to name block %s", rec.Body.String(), blockID)
	}

	// Listing at both levels.
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"?restype=container&comp=list", nil, nil)
	assertStatus(t, rec, http.StatusOK, "ListBlobs")
	if !strings.Contains(rec.Body.String(), blob) {
		t.Fatalf("ListBlobs body = %s, want it to name %s", rec.Body.String(), blob)
	}
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/?comp=list", nil, nil)
	assertStatus(t, rec, http.StatusOK, "ListContainers")
	if !strings.Contains(rec.Body.String(), container) {
		t.Fatalf("ListContainers body = %s, want it to name %s", rec.Body.String(), container)
	}

	assertStatus(t, storagePlaneReq(t, srv, http.MethodDelete, account, "blob", "/"+container+"/"+blob, nil, nil),
		http.StatusAccepted, "DeleteBlob")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodDelete, account, "blob", "/"+container+"?restype=container", nil, nil),
		http.StatusAccepted, "DeleteContainer")
}

func TestBlobDataPlaneUnservedCompDeclaresGap(t *testing.T) {
	srv := buildStorageTestSim(t)
	const account, container, blob = "gapblobacct2", "gap-container", "gap.txt"

	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob", "/"+container+"?restype=container", nil, nil),
		http.StatusCreated, "CreateContainer")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob", "/"+container+"/"+blob, []byte("v1"),
		map[string]string{"x-ms-blob-type": "BlockBlob"}), http.StatusCreated, "PutBlob")

	blobBody := func() string {
		rec := storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"/"+blob, nil, nil)
		assertStatus(t, rec, http.StatusOK, "GetBlob")
		return rec.Body.String()
	}

	// Set Blob Tier on an EXISTING blob: the fall-through wrote the (empty)
	// request body over the blob's contents and answered 201 Created.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob",
		"/"+container+"/"+blob+"?comp=tier", nil, map[string]string{"x-ms-access-tier": "Cool"}),
		"PUT blob ?comp=tier")
	if got := blobBody(); got != "v1" {
		t.Fatalf("blob contents = %q after a refused Set Blob Tier, want %q — the gap performed a sibling write", got, "v1")
	}

	// Set Blob Tier addressed at a name that does not exist: the fall-through
	// CREATED the blob.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob",
		"/"+container+"/never-created.txt?comp=tier", nil, map[string]string{"x-ms-access-tier": "Cool"}),
		"PUT new blob ?comp=tier")
	rec := storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"/never-created.txt", nil, nil)
	assertStatus(t, rec, http.StatusNotFound, "GetBlob after refused Set Blob Tier")

	// The rest of the blob-level PUT verbs, each of which used to land on
	// Put Blob and overwrite the blob.
	for _, comp := range []string{"metadata", "snapshot", "expiry", "tags", "undelete", "seal", "legalhold",
		"appendblock", "incrementalcopy", "immutabilityPolicies", "copy", "lease", "page", "properties"} {
		assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob",
			"/"+container+"/"+blob+"?comp="+comp, []byte("clobber"), nil), "PUT blob ?comp="+comp)
		if got := blobBody(); got != "v1" {
			t.Fatalf("blob contents = %q after a refused ?comp=%s, want %q", got, comp, "v1")
		}
	}

	// Stage Block From URL is a Stage Block sibling discriminated by
	// x-ms-copy-source; staging the empty body for it would commit nothing.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob",
		"/"+container+"/"+blob+"?comp=block&blockid=YmxvY2sx", nil,
		map[string]string{"x-ms-copy-source": "http://" + account + ".blob.localhost/" + container + "/" + blob}),
		"PUT blob ?comp=block with x-ms-copy-source")
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob",
		"/"+container+"/"+blob+"?comp=blocklist&blocklisttype=all", nil, nil)
	assertStatus(t, rec, http.StatusOK, "GetBlockList")
	if strings.Contains(rec.Body.String(), "<Name>") {
		t.Fatalf("block list = %s after a refused Stage Block From URL, want no staged block", rec.Body.String())
	}

	// GET/DELETE blob-level gaps must not read or delete the blob.
	for _, comp := range []string{"tags", "pagelist"} {
		rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"/"+blob+"?comp="+comp, nil, nil)
		assertStorageGap(t, rec, "GET blob ?comp="+comp)
		if strings.Contains(rec.Body.String(), "v1") {
			t.Fatalf("GET ?comp=%s returned the blob contents: %s", comp, rec.Body.String())
		}
	}
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodDelete, account, "blob",
		"/"+container+"/"+blob+"?comp=immutabilityPolicies", nil, nil), "DELETE blob ?comp=immutabilityPolicies")
	if got := blobBody(); got != "v1" {
		t.Fatalf("blob contents = %q after a refused Delete Immutability Policy, want %q", got, "v1")
	}

	// Container level: every unserved comp used to reach Create/Delete/Get
	// Container.
	for _, comp := range []string{"metadata", "rename", "undelete", "acl", "lease"} {
		assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob",
			"/never-a-container?restype=container&comp="+comp, nil, nil), "PUT container ?comp="+comp)
		rec = storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/never-a-container?restype=container", nil, nil)
		assertStatus(t, rec, http.StatusNotFound, "GetContainerProperties after refused ?comp="+comp)
	}
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodDelete, account, "blob",
		"/"+container+"?restype=container&comp=lease", nil, nil), "DELETE container ?comp=lease")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "blob", "/"+container+"?restype=container", nil, nil),
		http.StatusOK, "container survives a refused DELETE ?comp=lease")

	// Get Account Information is addressed with restype=account, never
	// restype=container — it must not read back container properties.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodGet, account, "blob",
		"/"+container+"?restype=account&comp=properties", nil, nil), "GET container ?restype=account")
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodGet, account, "blob",
		"/"+container+"/"+blob+"?restype=account&comp=properties", nil, nil), "GET blob ?restype=account")

	// Service level.
	for _, target := range []string{"/?comp=batch", "/?comp=blobs",
		"/?restype=service&comp=stats", "/?restype=account&comp=properties", "/"} {
		assertStorageGap(t, storagePlaneReq(t, srv, http.MethodGet, account, "blob", target, nil, nil),
			"GET service "+target)
	}
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "blob",
		"/?restype=service&comp=properties", []byte("<StorageServiceProperties/>"), nil),
		"PUT service ?restype=service&comp=properties")
}

// ── Files ───────────────────────────────────────────────────────────

func TestFilesDataPlaneServedOperations(t *testing.T) {
	srv := buildStorageTestSim(t)
	const account, share, file = "gapfileacct", "served-share", "served.txt"

	// Get File Service Properties, the Files sibling of the Blob operation the
	// azurerm provider polls.
	assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "file", "/?restype=service&comp=properties", nil, nil),
		http.StatusOK, "GetFileServiceProperties")
	payload := []byte("hello files")

	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "file", "/"+share+"?restype=share", nil, nil),
		http.StatusCreated, "CreateShare")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"?restype=share", nil, nil),
		http.StatusOK, "GetShareProperties")

	// Create File allocates the declared size; Upload Range fills it.
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "file", "/"+share+"/"+file, nil,
		map[string]string{"x-ms-type": "file", "x-ms-content-length": fmt.Sprintf("%d", len(payload))}),
		http.StatusCreated, "CreateFile")
	rec := storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"/"+file, nil, nil)
	assertStatus(t, rec, http.StatusOK, "DownloadFile (allocated)")
	if got := rec.Body.Bytes(); len(got) != len(payload) || strings.Trim(string(got), "\x00") != "" {
		t.Fatalf("freshly created file = %q, want %d zero bytes", got, len(payload))
	}
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "file", "/"+share+"/"+file+"?comp=range", payload,
		map[string]string{"x-ms-write": "update", "x-ms-range": fmt.Sprintf("bytes=0-%d", len(payload)-1)}),
		http.StatusCreated, "UploadRange")
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"/"+file, nil, nil)
	assertStatus(t, rec, http.StatusOK, "DownloadFile")
	if got := rec.Body.String(); got != string(payload) {
		t.Fatalf("DownloadFile body = %q, want %q", got, payload)
	}

	// A partial range write lands at its offset and leaves the rest intact.
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "file", "/"+share+"/"+file+"?comp=range", []byte("HELLO"),
		map[string]string{"x-ms-range": "bytes=0-4"}), http.StatusCreated, "UploadRange (partial)")
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"/"+file, nil, nil)
	if got, want := rec.Body.String(), "HELLO"+string(payload[5:]); got != want {
		t.Fatalf("DownloadFile body = %q, want %q", got, want)
	}
	// x-ms-write: clear zeroes the range instead of writing a body.
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "file", "/"+share+"/"+file+"?comp=range", nil,
		map[string]string{"x-ms-write": "clear", "x-ms-range": "bytes=0-4"}), http.StatusCreated, "UploadRange (clear)")
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"/"+file, nil, nil)
	if got, want := rec.Body.String(), "\x00\x00\x00\x00\x00"+string(payload[5:]); got != want {
		t.Fatalf("DownloadFile body = %q after clear, want %q", got, want)
	}
	// A range past the allocated size is rejected — Upload Range never grows a
	// file on Azure Files.
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "file", "/"+share+"/"+file+"?comp=range",
		[]byte("xxx"), map[string]string{"x-ms-range": fmt.Sprintf("bytes=%d-%d", len(payload), len(payload)+2)}),
		http.StatusRequestedRangeNotSatisfiable, "UploadRange (out of range)")

	assertStatus(t, storagePlaneReq(t, srv, http.MethodHead, account, "file", "/"+share+"/"+file, nil, nil),
		http.StatusOK, "GetFileProperties")

	// A ranged Download File answers 206 with Content-Range, like Get Blob.
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"/"+file, nil,
		map[string]string{"x-ms-range": "bytes=5-9"})
	assertStatus(t, rec, http.StatusPartialContent, "DownloadFile (ranged)")
	if got := rec.Header().Get("Content-Range"); got != fmt.Sprintf("bytes 5-9/%d", len(payload)) {
		t.Fatalf("Content-Range = %q, want bytes 5-9/%d", got, len(payload))
	}
	if got, want := rec.Body.String(), string(payload[5:10]); got != want {
		t.Fatalf("ranged DownloadFile body = %q, want %q", got, want)
	}
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"?restype=directory&comp=list", nil, nil)
	assertStatus(t, rec, http.StatusOK, "ListFilesAndDirectories")
	if !strings.Contains(rec.Body.String(), file) {
		t.Fatalf("directory listing = %s, want it to name %s", rec.Body.String(), file)
	}
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "file", "/?comp=list", nil, nil)
	assertStatus(t, rec, http.StatusOK, "ListShares")
	if !strings.Contains(rec.Body.String(), share) {
		t.Fatalf("share listing = %s, want it to name %s", rec.Body.String(), share)
	}

	assertStatus(t, storagePlaneReq(t, srv, http.MethodDelete, account, "file", "/"+share+"/"+file, nil, nil),
		http.StatusAccepted, "DeleteFile")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodDelete, account, "file", "/"+share+"?restype=share", nil, nil),
		http.StatusAccepted, "DeleteShare")
}

func TestFilesDataPlaneUnservedCompDeclaresGap(t *testing.T) {
	srv := buildStorageTestSim(t)
	const account, share, file = "gapfileacct2", "gap-share", "gap.txt"
	payload := []byte("hello files")

	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "file", "/"+share+"?restype=share", nil, nil),
		http.StatusCreated, "CreateShare")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "file", "/"+share+"/"+file, nil,
		map[string]string{"x-ms-content-length": fmt.Sprintf("%d", len(payload))}), http.StatusCreated, "CreateFile")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "file", "/"+share+"/"+file+"?comp=range", payload,
		map[string]string{"x-ms-range": fmt.Sprintf("bytes=0-%d", len(payload)-1)}), http.StatusCreated, "UploadRange")

	fileBody := func() string {
		rec := storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"/"+file, nil, nil)
		assertStatus(t, rec, http.StatusOK, "DownloadFile")
		return rec.Body.String()
	}

	// Create Directory used to land on Create File and answer 201 for a
	// directory that was never created.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "file",
		"/"+share+"/subdir?restype=directory", nil, nil), "PUT ?restype=directory")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"/subdir", nil, nil),
		http.StatusNotFound, "the refused Create Directory created nothing")

	// File-level comps that used to overwrite the file through Create File.
	for _, comp := range []string{"metadata", "properties", "lease", "rename", "copy", "forceclosehandles"} {
		assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "file",
			"/"+share+"/"+file+"?comp="+comp, []byte("clobber"), nil), "PUT file ?comp="+comp)
		if got := fileBody(); got != string(payload) {
			t.Fatalf("file contents = %q after a refused ?comp=%s, want %q", got, comp, payload)
		}
	}
	// Upload Range From URL is discriminated by x-ms-copy-source.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "file",
		"/"+share+"/"+file+"?comp=range", nil, map[string]string{
			"x-ms-range":       "bytes=0-4",
			"x-ms-copy-source": "http://" + account + ".file.localhost/" + share + "/" + file,
		}), "PUT file ?comp=range with x-ms-copy-source")
	if got := fileBody(); got != string(payload) {
		t.Fatalf("file contents = %q after a refused Upload Range From URL, want %q", got, payload)
	}
	// Hard and symbolic links address a file path with restype.
	for _, restype := range []string{"hardlink", "symboliclink"} {
		assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "file",
			"/"+share+"/"+file+"?restype="+restype, []byte("clobber"), nil), "PUT file ?restype="+restype)
		if got := fileBody(); got != string(payload) {
			t.Fatalf("file contents = %q after a refused ?restype=%s, want %q", got, restype, payload)
		}
	}
	// GET/DELETE gaps must not read or delete the file.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodGet, account, "file",
		"/"+share+"/"+file+"?comp=rangelist", nil, nil), "GET file ?comp=rangelist")
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodDelete, account, "file",
		"/"+share+"/"+file+"?comp=lease", nil, nil), "DELETE file ?comp=lease")
	if got := fileBody(); got != string(payload) {
		t.Fatalf("file contents = %q after a refused DELETE ?comp=lease, want %q", got, payload)
	}

	// Share level: an unserved comp must not create, read back or delete the
	// share.
	for _, comp := range []string{"acl", "metadata", "properties", "snapshot", "lease", "filepermission", "undelete"} {
		assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "file",
			"/never-a-share?restype=share&comp="+comp, nil, nil), "PUT share ?comp="+comp)
		assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "file", "/never-a-share?restype=share", nil, nil),
			http.StatusNotFound, "the refused ?comp="+comp+" created no share")
	}
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodGet, account, "file",
		"/"+share+"?restype=share&comp=stats", nil, nil), "GET share ?comp=stats")
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodDelete, account, "file",
		"/"+share+"?restype=share&comp=lease", nil, nil), "DELETE share ?comp=lease")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "file", "/"+share+"?restype=share", nil, nil),
		http.StatusOK, "share survives a refused DELETE ?comp=lease")

	// Directory operations below the share root, and the service level.
	for _, target := range []string{"/" + share + "/subdir?restype=directory&comp=list",
		"/" + share + "/subdir?comp=listhandles"} {
		assertStorageGap(t, storagePlaneReq(t, srv, http.MethodGet, account, "file", target, nil, nil), "GET "+target)
	}
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "file",
		"/?restype=service&comp=properties", []byte("<StorageServiceProperties/>"), nil), "PUT service properties")
}

// ── Queues ──────────────────────────────────────────────────────────

func TestQueuesDataPlaneServedOperations(t *testing.T) {
	srv := buildStorageTestSim(t)
	const account, queue = "gapqueueacct", "served-queue"

	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "queue", "/"+queue, nil, nil),
		http.StatusCreated, "CreateQueue")

	// Set Queue Metadata replaces the metadata wholesale; Get Queue Metadata
	// reads back exactly what was set.
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "queue", "/"+queue+"?comp=metadata", nil,
		map[string]string{"x-ms-meta-owner": "sockerless"}), http.StatusNoContent, "SetQueueMetadata")
	rec := storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/"+queue+"?comp=metadata", nil, nil)
	assertStatus(t, rec, http.StatusOK, "GetQueueMetadata")
	if got := rec.Header().Get("x-ms-meta-owner"); got != "sockerless" {
		t.Fatalf("x-ms-meta-owner = %q after Set Queue Metadata, want %q", got, "sockerless")
	}

	assertStatus(t, storagePlaneReq(t, srv, http.MethodPost, account, "queue", "/"+queue+"/messages",
		[]byte(`<QueueMessage><MessageText>aGVsbG8=</MessageText></QueueMessage>`), nil),
		http.StatusCreated, "PutMessage")
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/"+queue+"/messages?peekonly=true", nil, nil)
	assertStatus(t, rec, http.StatusOK, "PeekMessages")
	if !strings.Contains(rec.Body.String(), "aGVsbG8=") {
		t.Fatalf("PeekMessages body = %s, want the enqueued message", rec.Body.String())
	}
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/"+queue+"/messages", nil, nil)
	assertStatus(t, rec, http.StatusOK, "GetMessages")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodDelete, account, "queue", "/"+queue+"/messages", nil, nil),
		http.StatusNoContent, "ClearMessages")

	rec = storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/?comp=list", nil, nil)
	assertStatus(t, rec, http.StatusOK, "ListQueues")
	if !strings.Contains(rec.Body.String(), queue) {
		t.Fatalf("ListQueues body = %s, want it to name %s", rec.Body.String(), queue)
	}
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/?restype=service&comp=properties", nil, nil)
	assertStatus(t, rec, http.StatusOK, "GetServiceProperties")
	if !strings.Contains(rec.Body.String(), "<StorageServiceProperties>") {
		t.Fatalf("GetServiceProperties body = %s, want a StorageServiceProperties document", rec.Body.String())
	}

	assertStatus(t, storagePlaneReq(t, srv, http.MethodDelete, account, "queue", "/"+queue, nil, nil),
		http.StatusNoContent, "DeleteQueue")
}

func TestQueuesDataPlaneUnservedCompDeclaresGap(t *testing.T) {
	srv := buildStorageTestSim(t)
	const account, queue = "gapqueueacct2", "gap-queue"

	assertStatus(t, storagePlaneReq(t, srv, http.MethodPut, account, "queue", "/"+queue, nil,
		map[string]string{"x-ms-meta-owner": "sockerless"}), http.StatusCreated, "CreateQueue")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodPost, account, "queue", "/"+queue+"/messages",
		[]byte(`<QueueMessage><MessageText>aGVsbG8=</MessageText></QueueMessage>`), nil),
		http.StatusCreated, "PutMessage")

	// Set Queue ACL used to land on Create Queue: for a name that did not
	// exist it created the queue and answered 201.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "queue", "/never-a-queue?comp=acl", nil, nil),
		"PUT queue ?comp=acl")
	assertStatus(t, storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/never-a-queue?comp=metadata", nil, nil),
		http.StatusNotFound, "the refused Set Queue ACL created no queue")

	// Get Queue ACL used to land on Get Queue Metadata and answer 200.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/"+queue+"?comp=acl", nil, nil),
		"GET queue ?comp=acl")
	// A bare GET on the queue is not an operation Azure documents.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/"+queue, nil, nil),
		"GET queue (no comp)")

	// Delete Queue is the bare DELETE; with a comp it must not delete.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodDelete, account, "queue", "/"+queue+"?comp=acl", nil, nil),
		"DELETE queue ?comp=acl")
	rec := storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/"+queue+"?comp=metadata", nil, nil)
	assertStatus(t, rec, http.StatusOK, "queue survives a refused DELETE ?comp=acl")
	if got := rec.Header().Get("x-ms-meta-owner"); got != "sockerless" {
		t.Fatalf("queue metadata = %q after refused operations, want it unchanged", got)
	}

	// Update Message.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodPut, account, "queue",
		"/"+queue+"/messages/some-id?popreceipt=x&visibilitytimeout=0",
		[]byte(`<QueueMessage><MessageText>b3RoZXI=</MessageText></QueueMessage>`), nil), "PUT message")
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/"+queue+"/messages?peekonly=true", nil, nil)
	assertStatus(t, rec, http.StatusOK, "PeekMessages")
	if !strings.Contains(rec.Body.String(), "aGVsbG8=") {
		t.Fatalf("PeekMessages body = %s, want the original message intact", rec.Body.String())
	}

	// A comp on the messages collection names no documented operation, so it
	// must not enqueue, dequeue or clear.
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodDelete, account, "queue",
		"/"+queue+"/messages?comp=acl", nil, nil), "DELETE messages ?comp=acl")
	rec = storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/"+queue+"/messages?peekonly=true", nil, nil)
	if !strings.Contains(rec.Body.String(), "aGVsbG8=") {
		t.Fatalf("PeekMessages body = %s, want the message to survive a refused clear", rec.Body.String())
	}

	// Service level: Set Service Properties used to be answered by the GET
	// handler, so a write was reported as applied.
	rec = storagePlaneReq(t, srv, http.MethodPut, account, "queue", "/?restype=service&comp=properties",
		[]byte("<StorageServiceProperties/>"), nil)
	assertStorageGap(t, rec, "PUT service properties")
	assertStorageGap(t, storagePlaneReq(t, srv, http.MethodGet, account, "queue", "/?restype=service&comp=stats", nil, nil),
		"GET service statistics")
}
