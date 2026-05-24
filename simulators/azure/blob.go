package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Azure Storage Blob data plane.
//
// ARM `Microsoft.Storage/storageAccounts/{name}` returns an
// advertised endpoint URL like `https://{name}.blob.<host>:<port>/`
// in its `primaryEndpoints.blob` field. Real SDK / azcopy / az CLI
// consumers follow that URL for Put/Get/Head/Delete/List operations
// against blob containers + blobs. Before BUG-1103 the sim emitted
// the URL but had no handler servicing it — operators got 404 on
// the URL the sim itself handed them.
//
// Wire format (per Azure REST docs, https://learn.microsoft.com/rest/api/storageservices/blob-service-rest-api):
//
//	PUT    /{container}?restype=container          CreateContainer
//	DELETE /{container}?restype=container          DeleteContainer
//	GET    /{container}?restype=container          GetContainerProperties
//	GET    /{container}?restype=container&comp=list ListBlobs (XML)
//	GET    /?comp=list                             ListContainers (XML)
//	PUT    /{container}/{blob}                     PutBlob
//	GET    /{container}/{blob}                     GetBlob
//	HEAD   /{container}/{blob}                     GetBlobProperties
//	DELETE /{container}/{blob}                     DeleteBlob
//
// The sim collapses block-blob / page-blob / append-blob into one
// stored byte slice; `x-ms-blob-type` is recorded for round-trip
// fidelity but not enforced. SSE-C headers
// (`x-ms-encryption-key-sha256`) surface via the sentinel-header
// log added in 173.0 but are not enforced by the handler.

type BlobObject struct {
	Account      string
	Container    string
	Name         string
	Data         []byte
	ContentType  string
	BlobType     string
	ETag         string
	LastModified string
	Metadata     map[string]string
}

type BlobContainerData struct {
	Account  string
	Name     string
	Created  string
	Metadata map[string]string
}

var (
	blobObjects        sim.Store[BlobObject]
	blobContainersData sim.Store[BlobContainerData]
)

func registerBlobDataPlane(srv *sim.Server) {
	blobObjects = sim.MakeStore[BlobObject](srv.DB(), "blob_objects")
	blobContainersData = sim.MakeStore[BlobContainerData](srv.DB(), "blob_containers_data")

	srv.WrapHandler(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			hostname := host
			if i := strings.LastIndex(hostname, ":"); i >= 0 {
				hostname = hostname[:i]
			}
			parts := strings.SplitN(hostname, ".blob.", 2)
			if len(parts) == 2 {
				handleBlobDataPlane(w, r, parts[0])
				return
			}
			// Path-style fallback (Azurite-compatible). When the host
			// has no `.blob.` subdomain but the URL path starts with
			// `/{knownAccount}/...`, dispatch as a blob data-plane
			// request. Matches the Azure SDK / azurerm provider
			// default for non-`*.core.windows.net` endpoints. The
			// account-name lookup against azStorageAccounts protects
			// against false matches with ARM routes, which start
			// with `/subscriptions/`.
			if account, rest, ok := splitPathStyleAccount(r.URL.Path); ok {
				r.URL.Path = "/" + rest
				handleBlobDataPlane(w, r, account)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
}

// splitPathStyleAccount returns (account, restOfPath, true) when the
// first path segment matches a known storage account. Used by the
// blob (and storage_dataplane.go) dispatchers to accept Azurite-style
// `/{account}/{container}/{blob}` URLs alongside the host-subdomain
// form. Returns false if the path has no leading segment or the
// segment isn't a registered account.
func splitPathStyleAccount(path string) (account, rest string, ok bool) {
	p := strings.TrimPrefix(path, "/")
	if p == "" {
		return "", "", false
	}
	slash := strings.IndexByte(p, '/')
	var first string
	if slash < 0 {
		first = p
		p = ""
	} else {
		first = p[:slash]
		p = p[slash+1:]
	}
	if !knownStorageAccount(first) {
		return "", "", false
	}
	return first, p, true
}

// knownStorageAccount reports whether name matches a registered
// storage account. The `azStorageAccounts` store is keyed by full
// ARM resource ID; we scan by Name. O(N) but N is bounded by the
// number of operator-created accounts (typically 1).
func knownStorageAccount(name string) bool {
	if name == "" {
		return false
	}
	for _, a := range azStorageAccounts.List() {
		if a.Name == name {
			return true
		}
	}
	return false
}

func blobObjectKey(account, container, name string) string {
	return account + "/" + container + "/" + name
}
func blobContainerKey(account, container string) string {
	return account + "/" + container
}

func handleBlobDataPlane(w http.ResponseWriter, r *http.Request, account string) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	q := r.URL.Query()
	switch {
	case path == "" && q.Get("comp") == "list":
		handleListContainers(w, r, account)
		return
	case path == "":
		w.WriteHeader(http.StatusOK)
		return
	}
	segs := strings.SplitN(path, "/", 2)
	container := segs[0]
	if len(segs) == 1 {
		// Container-level op: depends on query params.
		switch r.Method {
		case http.MethodPut:
			if q.Get("restype") == "container" {
				handleCreateContainer(w, r, account, container)
				return
			}
		case http.MethodDelete:
			if q.Get("restype") == "container" {
				handleDeleteContainer(w, r, account, container)
				return
			}
		case http.MethodGet, http.MethodHead:
			if q.Get("restype") == "container" {
				if q.Get("comp") == "list" {
					handleListBlobs(w, r, account, container)
					return
				}
				handleGetContainer(w, r, account, container)
				return
			}
		}
		sim.AzureError(w, "MissingRequiredQueryParameter",
			"Container ops require restype=container", http.StatusBadRequest)
		return
	}
	// Blob-level op: /{container}/{blob}.
	blob := segs[1]
	switch r.Method {
	case http.MethodPut:
		handlePutBlob(w, r, account, container, blob)
	case http.MethodGet:
		handleGetBlob(w, r, account, container, blob)
	case http.MethodHead:
		handleHeadBlob(w, r, account, container, blob)
	case http.MethodDelete:
		handleDeleteBlob(w, r, account, container, blob)
	default:
		sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
	}
}

func handleCreateContainer(w http.ResponseWriter, r *http.Request, account, container string) {
	key := blobContainerKey(account, container)
	if _, ok := blobContainersData.Get(key); ok {
		sim.AzureError(w, "ContainerAlreadyExists",
			"The specified container already exists.", http.StatusConflict)
		return
	}
	c := BlobContainerData{
		Account:  account,
		Name:     container,
		Created:  time.Now().UTC().Format(time.RFC1123),
		Metadata: collectMetadata(r),
	}
	blobContainersData.Put(key, c)
	w.Header().Set("ETag", `"`+generateUUID()+`"`)
	w.Header().Set("Last-Modified", c.Created)
	w.WriteHeader(http.StatusCreated)
}

func handleDeleteContainer(w http.ResponseWriter, r *http.Request, account, container string) {
	if !blobContainersData.Delete(blobContainerKey(account, container)) {
		sim.AzureError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	// Cascade-delete blobs.
	prefix := account + "/" + container + "/"
	for _, b := range blobObjects.List() {
		if strings.HasPrefix(blobObjectKey(b.Account, b.Container, b.Name), prefix) {
			blobObjects.Delete(blobObjectKey(b.Account, b.Container, b.Name))
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleGetContainer(w http.ResponseWriter, r *http.Request, account, container string) {
	c, ok := blobContainersData.Get(blobContainerKey(account, container))
	if !ok {
		sim.AzureError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	w.Header().Set("Last-Modified", c.Created)
	for k, v := range c.Metadata {
		w.Header().Set("x-ms-meta-"+k, v)
	}
	w.WriteHeader(http.StatusOK)
}

func handleListContainers(w http.ResponseWriter, r *http.Request, account string) {
	type containerEntry struct {
		Name string `xml:"Name"`
	}
	type enum struct {
		XMLName    xml.Name         `xml:"EnumerationResults"`
		Containers []containerEntry `xml:"Containers>Container"`
	}
	out := enum{}
	prefix := account + "/"
	for _, c := range blobContainersData.List() {
		if strings.HasPrefix(blobContainerKey(c.Account, c.Name), prefix) {
			out.Containers = append(out.Containers, containerEntry{Name: c.Name})
		}
	}
	w.Header().Set("Content-Type", "application/xml")
	body, _ := xml.Marshal(out)
	_, _ = w.Write(body)
}

func handleListBlobs(w http.ResponseWriter, r *http.Request, account, container string) {
	if _, ok := blobContainersData.Get(blobContainerKey(account, container)); !ok {
		sim.AzureError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	type blobEntry struct {
		Name       string `xml:"Name"`
		Properties struct {
			ContentLength int    `xml:"Content-Length"`
			ETag          string `xml:"Etag"`
			LastModified  string `xml:"Last-Modified"`
		} `xml:"Properties"`
	}
	type enum struct {
		XMLName xml.Name    `xml:"EnumerationResults"`
		Blobs   []blobEntry `xml:"Blobs>Blob"`
	}
	out := enum{}
	prefix := account + "/" + container + "/"
	for _, b := range blobObjects.List() {
		if strings.HasPrefix(blobObjectKey(b.Account, b.Container, b.Name), prefix) {
			be := blobEntry{Name: b.Name}
			be.Properties.ContentLength = len(b.Data)
			be.Properties.ETag = b.ETag
			be.Properties.LastModified = b.LastModified
			out.Blobs = append(out.Blobs, be)
		}
	}
	w.Header().Set("Content-Type", "application/xml")
	body, _ := xml.Marshal(out)
	_, _ = w.Write(body)
}

func handlePutBlob(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	if _, ok := blobContainersData.Get(blobContainerKey(account, container)); !ok {
		sim.AzureError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	body, err := openStreamingBody(r)
	if err != nil {
		sim.AzureError(w, "UnsupportedHttpVerb", err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		sim.AzureError(w, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}
	hash := md5.Sum(data)
	etag := `"` + hex.EncodeToString(hash[:]) + `"`
	lastMod := time.Now().UTC().Format(time.RFC1123)
	b := BlobObject{
		Account:      account,
		Container:    container,
		Name:         blob,
		Data:         data,
		ContentType:  r.Header.Get("Content-Type"),
		BlobType:     r.Header.Get("x-ms-blob-type"),
		ETag:         etag,
		LastModified: lastMod,
		Metadata:     collectMetadata(r),
	}
	blobObjects.Put(blobObjectKey(account, container, blob), b)
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastMod)
	w.Header().Set("Content-MD5", hex.EncodeToString(hash[:]))
	w.WriteHeader(http.StatusCreated)
}

func handleGetBlob(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	b, ok := blobObjects.Get(blobObjectKey(account, container, blob))
	if !ok {
		sim.AzureError(w, "BlobNotFound",
			"The specified blob does not exist.", http.StatusNotFound)
		return
	}
	writeBlobHeaders(w, b)
	_, _ = w.Write(b.Data)
}

func handleHeadBlob(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	b, ok := blobObjects.Get(blobObjectKey(account, container, blob))
	if !ok {
		sim.AzureError(w, "BlobNotFound",
			"The specified blob does not exist.", http.StatusNotFound)
		return
	}
	writeBlobHeaders(w, b)
}

func handleDeleteBlob(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	if !blobObjects.Delete(blobObjectKey(account, container, blob)) {
		sim.AzureError(w, "BlobNotFound",
			"The specified blob does not exist.", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func writeBlobHeaders(w http.ResponseWriter, b BlobObject) {
	if b.ContentType != "" {
		w.Header().Set("Content-Type", b.ContentType)
	}
	if b.BlobType != "" {
		w.Header().Set("x-ms-blob-type", b.BlobType)
	}
	w.Header().Set("ETag", b.ETag)
	w.Header().Set("Last-Modified", b.LastModified)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(b.Data)))
	for k, v := range b.Metadata {
		w.Header().Set("x-ms-meta-"+k, v)
	}
}

func collectMetadata(r *http.Request) map[string]string {
	out := map[string]string{}
	for k, v := range r.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-ms-meta-") && len(v) > 0 {
			out[strings.TrimPrefix(lk, "x-ms-meta-")] = v[0]
		}
	}
	return out
}
