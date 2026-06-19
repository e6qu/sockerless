package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// Azure Storage Blob data plane.
//
// ARM `Microsoft.Storage/storageAccounts/{name}` returns an
// advertised endpoint URL like `https://{name}.blob.<host>:<port>/`
// in its `primaryEndpoints.blob` field. Real SDK / azcopy / az CLI
// consumers follow that URL for Put/Get/Head/Delete/List operations
// against blob containers + blobs. Previously the sim emitted
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
//	PUT    /{container}/{blob}?comp=block          StageBlock
//	PUT    /{container}/{blob}?comp=blocklist      CommitBlockList
//	GET    /{container}/{blob}                     GetBlob
//	GET    /{container}/{blob}?comp=blocklist      GetBlockList
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
	CopyID       string
	CopyStatus   string
	CopySource   string
}

type BlobContainerData struct {
	Account  string
	Name     string
	Created  string
	ETag     string
	Metadata map[string]string
}

type BlobBlockData struct {
	Account         string
	Container       string
	Blob            string
	BlockID         string
	UncommittedData []byte
	CommittedData   []byte
	HasUncommitted  bool
	HasCommitted    bool
	CommitOrdinal   int
}

type blockRef struct {
	id   string
	data []byte
}

var (
	blobObjects        sim.Store[BlobObject]
	blobContainersData sim.Store[BlobContainerData]
	blobBlocks         sim.Store[BlobBlockData]
)

// Secondary indexes that let a per-container / per-blob operation enumerate
// only the relevant keys instead of scanning every blob in every container
// (or every block in every blob) — the store exposes no prefix scan. Both
// index the FULL store key:
//   - blobIndex:  containerKey ("account/container") → set of blobObjectKeys
//   - blockIndex: blobKey ("account/container/blob") → set of blobBlockKeys
//
// All blobObjects / blobBlocks mutations route through the put*/delete*
// helpers below so the indexes stay consistent. blobIndexMu guards both.
//   - blocksByContainer: containerKey ("account/container") → set of
//     blobBlockKeys, so a container delete can reach staged-but-uncommitted
//     blocks that have no committed blob object.
var (
	blobIndexMu       sync.Mutex
	blobIndex         = map[string]map[string]struct{}{}
	blockIndex        = map[string]map[string]struct{}{}
	blocksByContainer = map[string]map[string]struct{}{}
)

func indexAdd(idx map[string]map[string]struct{}, group, key string) {
	set := idx[group]
	if set == nil {
		set = map[string]struct{}{}
		idx[group] = set
	}
	set[key] = struct{}{}
}

func indexRemove(idx map[string]map[string]struct{}, group, key string) {
	set := idx[group]
	if set == nil {
		return
	}
	delete(set, key)
	if len(set) == 0 {
		delete(idx, group)
	}
}

func putBlobObject(account, container, name string, b BlobObject) {
	key := blobObjectKey(account, container, name)
	blobIndexMu.Lock()
	indexAdd(blobIndex, blobContainerKey(account, container), key)
	blobIndexMu.Unlock()
	blobObjects.Put(key, b)
}

func deleteBlobObject(account, container, name string) {
	key := blobObjectKey(account, container, name)
	blobIndexMu.Lock()
	indexRemove(blobIndex, blobContainerKey(account, container), key)
	blobIndexMu.Unlock()
	blobObjects.Delete(key)
}

func putBlobBlock(account, container, blob, blockID string, b BlobBlockData) {
	key := blobBlockKey(account, container, blob, blockID)
	blobIndexMu.Lock()
	indexAdd(blockIndex, blobObjectKey(account, container, blob), key)
	indexAdd(blocksByContainer, blobContainerKey(account, container), key)
	blobIndexMu.Unlock()
	blobBlocks.Put(key, b)
}

func deleteBlobBlock(account, container, blob, blockID string) {
	key := blobBlockKey(account, container, blob, blockID)
	blobIndexMu.Lock()
	indexRemove(blockIndex, blobObjectKey(account, container, blob), key)
	indexRemove(blocksByContainer, blobContainerKey(account, container), key)
	blobIndexMu.Unlock()
	blobBlocks.Delete(key)
}

// blobKeysInContainer returns the store keys of every blob in the container.
func blobKeysInContainer(account, container string) []string {
	blobIndexMu.Lock()
	defer blobIndexMu.Unlock()
	set := blobIndex[blobContainerKey(account, container)]
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// blockKeysForBlob returns the store keys of every block staged/committed
// under the given blob.
func blockKeysForBlob(account, container, blob string) []string {
	blobIndexMu.Lock()
	defer blobIndexMu.Unlock()
	set := blockIndex[blobObjectKey(account, container, blob)]
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// blockKeysInContainer returns the store keys of every block staged/committed
// in the container, including blocks with no committed blob object.
func blockKeysInContainer(account, container string) []string {
	blobIndexMu.Lock()
	defer blobIndexMu.Unlock()
	set := blocksByContainer[blobContainerKey(account, container)]
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

func registerBlobDataPlane(srv *sim.Server) {
	blobObjects = sim.MakeStore[BlobObject](srv.DB(), "blob_objects")
	blobContainersData = sim.MakeStore[BlobContainerData](srv.DB(), "blob_containers_data")
	blobBlocks = sim.MakeStore[BlobBlockData](srv.DB(), "blob_blocks")

	// Rebuild the secondary indexes from any persisted store contents so a
	// restart with a SQLite-backed store starts consistent.
	blobIndexMu.Lock()
	blobIndex = map[string]map[string]struct{}{}
	blockIndex = map[string]map[string]struct{}{}
	blocksByContainer = map[string]map[string]struct{}{}
	for _, b := range blobObjects.List() {
		indexAdd(blobIndex, blobContainerKey(b.Account, b.Container), blobObjectKey(b.Account, b.Container, b.Name))
	}
	for _, bl := range blobBlocks.List() {
		indexAdd(blockIndex, blobObjectKey(bl.Account, bl.Container, bl.Blob), blobBlockKey(bl.Account, bl.Container, bl.Blob, bl.BlockID))
		indexAdd(blocksByContainer, blobContainerKey(bl.Account, bl.Container), blobBlockKey(bl.Account, bl.Container, bl.Blob, bl.BlockID))
	}
	blobIndexMu.Unlock()

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
			// Path-style fallback (Azurite-compatible). When the
			// host carries NO service-specific Azure subdomain
			// AND the URL path starts with `/{account}/...` AND
			// the request carries an Azure-Storage protocol signal,
			// dispatch as a blob data-plane request. Matches the
			// Azure SDK / azurerm provider default for non-
			// `*.core.windows.net` endpoints and the Azurite
			// connection-string contract. Account names are
			// accepted as-is (real Azurite is permissive — no
			// prior ARM registration required); the storage-signal
			// requirement keeps non-storage co-tenants (IMDS at
			// `/metadata/...`, Monitor ingest at
			// `/dataCollectionRules/...`, MSI at `/metadata/...`)
			// from being misrouted on the shared sim port.
			if hasNonStorageAzureSubdomain(hostname) {
				next.ServeHTTP(w, r)
				return
			}
			if !hasAzureStorageSignal(r) {
				next.ServeHTTP(w, r)
				return
			}
			if account, rest, ok := splitPathStyleAccount(r.URL.Path); ok {
				r.URL.Path = "/" + rest
				handleBlobDataPlane(w, r, account)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
}

// hasAzureStorageSignal reports whether the request carries a
// protocol marker that real Azure Storage SDKs always emit:
//   - `x-ms-version`     — REST API version selector (every SDK call)
//   - `x-ms-date`        — request-signing timestamp
//   - `x-ms-blob-type`   — PutBlob blob-type selector
//   - `x-ms-type`        — Files PutFile file-type selector
//   - `Authorization: SharedKey ...` — account-key signed request
//   - query `restype=`   — container/share/directory operation discriminator
//   - query `comp=`      — sub-resource (list, properties, metadata, …)
//   - query `sv=`        — Shared Access Signature (SAS) version
//
// Co-tenants on the shared sim port (IMDS `/metadata/...`, Monitor
// ingest `/dataCollectionRules/...`, MSI token endpoint) never carry
// any of these markers, so the signal cleanly partitions storage
// path-style requests from everything else.
func hasAzureStorageSignal(r *http.Request) bool {
	if r.Header.Get("x-ms-version") != "" ||
		r.Header.Get("x-ms-date") != "" ||
		r.Header.Get("x-ms-blob-type") != "" ||
		r.Header.Get("x-ms-type") != "" {
		return true
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "SharedKey ") || strings.HasPrefix(auth, "SharedKeyLite ") {
		return true
	}
	q := r.URL.Query()
	if q.Get("restype") != "" || q.Get("comp") != "" || q.Get("sv") != "" {
		return true
	}
	return false
}

// hasNonStorageAzureSubdomain reports whether the host carries an
// Azure-service subdomain other than the four storage services
// (blob/file/queue/table). A host like `myvault.vault.localhost`,
// `myns.servicebus.localhost`, `myfn.azurewebsites.net`, or
// `mycr.azurecr.io` MUST NOT be dispatched as a path-style storage
// request — the subdomain identifies a different data plane. New
// service subdomains added to the sim go here too.
func hasNonStorageAzureSubdomain(hostname string) bool {
	for _, marker := range []string{
		".vault.",
		".servicebus.",
		".redis.cache.",
		".postgres.database.",
		".web.",
		".dfs.",
		".azurewebsites.",
		".azurecr.",
		".azure-api.",
		".azurecontainerapps.",
		".applicationinsights.",
		".cognitiveservices.",
	} {
		if strings.Contains(hostname, marker) {
			return true
		}
	}
	return false
}

// splitPathStyleAccount returns (account, restOfPath, true) when the
// path looks like Azurite-style storage: `/{account}/{container}/{blob}`.
// Real Azurite accepts any account name without prior registration;
// the discriminator from non-storage routes is the prefix exclusion
// — ARM (`/subscriptions/...`, `/providers/...`), Docker SDK
// (`/v1.NN/...`), and GCP-shaped paths (`/v1/...`, `/storage/v1/...`)
// are NOT path-style storage. Anything else with `/{segment}/{rest}`
// is dispatched to the data plane with `{segment}` as the account.
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
	if isNonStorageFirstSegment(first) {
		return "", "", false
	}
	return first, p, true
}

// isNonStorageFirstSegment reports whether the first path segment
// belongs to a non-storage route (ARM, Docker SDK, GCP-shaped, or
// other registered sim surfaces). Anything not in this set is
// treated as a storage account name for path-style dispatch.
func isNonStorageFirstSegment(s string) bool {
	switch s {
	case "subscriptions", "providers", "tenants", "locations",
		"storage", "v1", "$metadata":
		return true
	}
	// Docker SDK paths: /v1.44/, /v1.41/, etc.
	if strings.HasPrefix(s, "v1.") {
		return true
	}
	// internal/v1/ surface (sockerless control plane).
	if s == "internal" {
		return true
	}
	return false
}

// blobStoragePage applies Azure Storage list pagination to an already-sorted
// slice. It reads ?maxresults=N (page size, default unlimited) and
// ?marker=NAME (continuation token = name of first item to include) from the
// request query, slices the items, and returns the page plus a NextMarker
// value (empty when the page is the last one). The name func extracts the
// sortable name from each item.
func blobStoragePage[T any](r *http.Request, items []T, name func(T) string) ([]T, string) {
	// Apply marker: skip items whose name is <= marker.
	marker := r.URL.Query().Get("marker")
	start := 0
	if marker != "" {
		for start < len(items) && name(items[start]) <= marker {
			start++
		}
	}
	items = items[start:]

	// Apply maxresults.
	if raw := r.URL.Query().Get("maxresults"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n < len(items) {
			// NextMarker is the name of the last returned item. The next
			// request passes it as ?marker=NAME and the skip loop above
			// advances past all items whose name is <= marker, landing on
			// the first item of the next page.
			next := name(items[n-1])
			return items[:n], next
		}
	}
	return items, ""
}

func blobObjectKey(account, container, name string) string {
	return account + "/" + container + "/" + name
}
func blobContainerKey(account, container string) string {
	return account + "/" + container
}
func blobBlockKey(account, container, blob, blockID string) string {
	return account + "/" + container + "/" + blob + "/" + blockID
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
		writeStorageError(w, "MissingRequiredQueryParameter",
			"Container ops require restype=container", http.StatusBadRequest)
		return
	}
	// Blob-level op: /{container}/{blob}.
	blob := segs[1]
	switch r.Method {
	case http.MethodPut:
		switch q.Get("comp") {
		case "block":
			handleStageBlock(w, r, account, container, blob)
			return
		case "blocklist":
			handleCommitBlockList(w, r, account, container, blob)
			return
		}
		if r.Header.Get("x-ms-copy-source") != "" {
			handleCopyBlob(w, r, account, container, blob)
			return
		}
		handlePutBlob(w, r, account, container, blob)
	case http.MethodGet:
		if q.Get("comp") == "blocklist" {
			handleGetBlockList(w, r, account, container, blob)
			return
		}
		handleGetBlob(w, r, account, container, blob)
	case http.MethodHead:
		handleHeadBlob(w, r, account, container, blob)
	case http.MethodDelete:
		handleDeleteBlob(w, r, account, container, blob)
	default:
		writeStorageError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
	}
}

func handleCreateContainer(w http.ResponseWriter, r *http.Request, account, container string) {
	key := blobContainerKey(account, container)
	if _, ok := blobContainersData.Get(key); ok {
		writeStorageError(w, "ContainerAlreadyExists",
			"The specified container already exists.", http.StatusConflict)
		return
	}
	c := BlobContainerData{
		Account:  account,
		Name:     container,
		Created:  time.Now().UTC().Format(http.TimeFormat),
		ETag:     `"` + generateUUID() + `"`,
		Metadata: collectMetadata(r),
	}
	blobContainersData.Put(key, c)
	w.Header().Set("ETag", c.ETag)
	w.Header().Set("Last-Modified", c.Created)
	w.WriteHeader(http.StatusCreated)
}

func handleDeleteContainer(w http.ResponseWriter, r *http.Request, account, container string) {
	if !blobContainersData.Delete(blobContainerKey(account, container)) {
		writeStorageError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	// Cascade-delete this container's blobs and their staged/committed blocks
	// via the secondary indexes, touching only the container's own keys.
	for _, key := range blobKeysInContainer(account, container) {
		if b, ok := blobObjects.Get(key); ok {
			deleteBlobObject(b.Account, b.Container, b.Name)
		}
	}
	for _, key := range blockKeysInContainer(account, container) {
		if bl, ok := blobBlocks.Get(key); ok {
			deleteBlobBlock(bl.Account, bl.Container, bl.Blob, bl.BlockID)
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleGetContainer(w http.ResponseWriter, r *http.Request, account, container string) {
	c, ok := blobContainersData.Get(blobContainerKey(account, container))
	if !ok {
		writeStorageError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	w.Header().Set("Last-Modified", c.Created)
	w.Header().Set("ETag", c.ETag)
	for k, v := range c.Metadata {
		w.Header().Set("x-ms-meta-"+k, v)
	}
	w.WriteHeader(http.StatusOK)
}

func handleListContainers(w http.ResponseWriter, r *http.Request, account string) {
	type containerProperties struct {
		LastModified string `xml:"Last-Modified"`
		ETag         string `xml:"Etag"`
	}
	type containerEntry struct {
		Name       string              `xml:"Name"`
		Properties containerProperties `xml:"Properties"`
	}
	type enum struct {
		XMLName         xml.Name         `xml:"EnumerationResults"`
		ServiceEndpoint string           `xml:"ServiceEndpoint,attr"`
		Containers      []containerEntry `xml:"Containers>Container"`
		NextMarker      string           `xml:"NextMarker"`
	}
	prefix := account + "/"
	var all []containerEntry
	for _, c := range blobContainersData.List() {
		if strings.HasPrefix(blobContainerKey(c.Account, c.Name), prefix) {
			all = append(all, containerEntry{
				Name: c.Name,
				Properties: containerProperties{
					LastModified: c.Created,
					ETag:         c.ETag,
				},
			})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	page, marker := blobStoragePage(r, all, func(e containerEntry) string { return e.Name })
	out := enum{
		ServiceEndpoint: azureStorageEndpointURL(r, account, "blob"),
		Containers:      page,
		NextMarker:      marker,
	}
	writeStorageXML(w, http.StatusOK, out)
}

func handleListBlobs(w http.ResponseWriter, r *http.Request, account, container string) {
	if _, ok := blobContainersData.Get(blobContainerKey(account, container)); !ok {
		writeStorageError(w, "ContainerNotFound",
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
	type blobPrefixEntry struct {
		Name string `xml:"Name"`
	}
	type enum struct {
		XMLName         xml.Name          `xml:"EnumerationResults"`
		ServiceEndpoint string            `xml:"ServiceEndpoint,attr"`
		ContainerName   string            `xml:"ContainerName,attr"`
		Prefix          string            `xml:"Prefix,omitempty"`
		Delimiter       string            `xml:"Delimiter,omitempty"`
		Blobs           []blobEntry       `xml:"Blobs>Blob"`
		BlobPrefixes    []blobPrefixEntry `xml:"Blobs>BlobPrefix"`
		NextMarker      string            `xml:"NextMarker"`
	}

	reqPrefix := r.URL.Query().Get("prefix")
	delimiter := r.URL.Query().Get("delimiter")

	var all []blobEntry
	for _, key := range blobKeysInContainer(account, container) {
		b, ok := blobObjects.Get(key)
		if !ok {
			continue
		}
		if reqPrefix != "" && !strings.HasPrefix(b.Name, reqPrefix) {
			continue
		}
		be := blobEntry{Name: b.Name}
		be.Properties.ContentLength = len(b.Data)
		be.Properties.ETag = b.ETag
		be.Properties.LastModified = b.LastModified
		all = append(all, be)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	// With a delimiter, roll names that contain it (past the request prefix)
	// into virtual directories surfaced as <BlobPrefix> entries; only names
	// without a further delimiter are listed as blobs.
	var prefixEntries []blobPrefixEntry
	if delimiter != "" {
		seenPrefix := map[string]bool{}
		var flat []blobEntry
		for _, be := range all {
			rest := strings.TrimPrefix(be.Name, reqPrefix)
			if idx := strings.Index(rest, delimiter); idx >= 0 {
				virtual := reqPrefix + rest[:idx+len(delimiter)]
				if !seenPrefix[virtual] {
					seenPrefix[virtual] = true
					prefixEntries = append(prefixEntries, blobPrefixEntry{Name: virtual})
				}
				continue
			}
			flat = append(flat, be)
		}
		all = flat
		sort.Slice(prefixEntries, func(i, j int) bool { return prefixEntries[i].Name < prefixEntries[j].Name })
	}

	page, marker := blobStoragePage(r, all, func(e blobEntry) string { return e.Name })
	out := enum{
		ServiceEndpoint: azureStorageEndpointURL(r, account, "blob"),
		ContainerName:   container,
		Prefix:          reqPrefix,
		Delimiter:       delimiter,
		Blobs:           page,
		BlobPrefixes:    prefixEntries,
		NextMarker:      marker,
	}
	writeStorageXML(w, http.StatusOK, out)
}

// azureBlobPreconditionOK validates the conditional headers against the current
// blob ETag (exists=false → no current blob). Writes a 412 ConditionNotMet and
// returns false on a failed precondition; an absent header always passes.
func azureBlobPreconditionOK(w http.ResponseWriter, r *http.Request, currentETag string, exists bool) bool {
	if inm := r.Header.Get("If-None-Match"); inm == "*" && exists {
		writeStorageError(w, "ConditionNotMet",
			"The condition specified using HTTP conditional header(s) is not met.", http.StatusPreconditionFailed)
		return false
	}
	if im := r.Header.Get("If-Match"); im != "" && im != "*" {
		if !exists || strings.Trim(im, `"`) != strings.Trim(currentETag, `"`) {
			writeStorageError(w, "ConditionNotMet",
				"The condition specified using HTTP conditional header(s) is not met.", http.StatusPreconditionFailed)
			return false
		}
	}
	return true
}

func handlePutBlob(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	if _, ok := blobContainersData.Get(blobContainerKey(account, container)); !ok {
		writeStorageError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	existing, exists := blobObjects.Get(blobObjectKey(account, container, blob))
	if !azureBlobPreconditionOK(w, r, existing.ETag, exists) {
		return
	}
	body, err := openStreamingBody(r)
	if err != nil {
		writeStorageError(w, "UnsupportedHttpVerb", err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		writeStorageError(w, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}
	hash := md5.Sum(data)
	etag := `"` + hex.EncodeToString(hash[:]) + `"`
	lastMod := time.Now().UTC().Format(http.TimeFormat)
	contentType := r.Header.Get("x-ms-blob-content-type")
	if contentType == "" {
		contentType = r.Header.Get("Content-Type")
	}
	b := BlobObject{
		Account:      account,
		Container:    container,
		Name:         blob,
		Data:         data,
		ContentType:  contentType,
		BlobType:     r.Header.Get("x-ms-blob-type"),
		ETag:         etag,
		LastModified: lastMod,
		Metadata:     collectMetadata(r),
	}
	putBlobObject(account, container, blob, b)
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastMod)
	w.Header().Set("Content-MD5", hex.EncodeToString(hash[:]))
	w.WriteHeader(http.StatusCreated)
}

func handleCopyBlob(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	if _, ok := blobContainersData.Get(blobContainerKey(account, container)); !ok {
		writeStorageError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	sourceURL := r.Header.Get("x-ms-copy-source")
	srcAccount, srcContainer, srcBlob, ok := parseBlobCopySource(sourceURL)
	if !ok {
		writeStorageError(w, "CannotVerifyCopySource",
			"The specified copy source is invalid.", http.StatusNotFound)
		return
	}
	source, ok := blobObjects.Get(blobObjectKey(srcAccount, srcContainer, srcBlob))
	if !ok {
		writeStorageError(w, "CannotVerifyCopySource",
			"The specified copy source does not exist.", http.StatusNotFound)
		return
	}

	data := append([]byte(nil), source.Data...)
	hash := md5.Sum(data)
	etag := `"` + hex.EncodeToString(hash[:]) + `"`
	lastMod := time.Now().UTC().Format(http.TimeFormat)
	metadata := collectMetadata(r)
	if len(metadata) == 0 {
		metadata = cloneBlobMetadata(source.Metadata)
	}
	copyID := generateUUID()
	dst := BlobObject{
		Account:      account,
		Container:    container,
		Name:         blob,
		Data:         data,
		ContentType:  source.ContentType,
		BlobType:     source.BlobType,
		ETag:         etag,
		LastModified: lastMod,
		Metadata:     metadata,
		CopyID:       copyID,
		CopyStatus:   "success",
		CopySource:   sourceURL,
	}
	putBlobObject(account, container, blob, dst)
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastMod)
	w.Header().Set("x-ms-copy-id", copyID)
	w.Header().Set("x-ms-copy-status", "success")
	w.WriteHeader(http.StatusAccepted)
}

func parseBlobCopySource(raw string) (account, container, blob string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", "", false
	}
	host := u.Hostname()
	escapedPath := strings.TrimPrefix(u.EscapedPath(), "/")
	if escapedPath == "" {
		return "", "", "", false
	}
	if parts := strings.SplitN(host, ".blob.", 2); len(parts) == 2 {
		account, ok = pathUnescape(parts[0])
		if !ok {
			return "", "", "", false
		}
		container, blob, ok = splitContainerBlobPath(escapedPath)
		return account, container, blob, ok
	}
	accountEsc, rest, found := strings.Cut(escapedPath, "/")
	if !found {
		return "", "", "", false
	}
	account, ok = pathUnescape(accountEsc)
	if !ok {
		return "", "", "", false
	}
	container, blob, ok = splitContainerBlobPath(rest)
	return account, container, blob, ok
}

func splitContainerBlobPath(escapedPath string) (container, blob string, ok bool) {
	containerEsc, blobEsc, found := strings.Cut(escapedPath, "/")
	if !found || blobEsc == "" {
		return "", "", false
	}
	container, ok = pathUnescape(containerEsc)
	if !ok {
		return "", "", false
	}
	blob, ok = pathUnescape(blobEsc)
	if !ok {
		return "", "", false
	}
	return container, blob, true
}

func pathUnescape(s string) (string, bool) {
	out, err := url.PathUnescape(s)
	if err != nil {
		return "", false
	}
	return out, true
}

func cloneBlobMetadata(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func handleStageBlock(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	if _, ok := blobContainersData.Get(blobContainerKey(account, container)); !ok {
		writeStorageError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	blockID := r.URL.Query().Get("blockid")
	if blockID == "" {
		writeStorageError(w, "MissingRequiredQueryParameter",
			"StageBlock requires blockid.", http.StatusBadRequest)
		return
	}
	body, err := openStreamingBody(r)
	if err != nil {
		writeStorageError(w, "UnsupportedHttpVerb", err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		writeStorageError(w, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}
	key := blobBlockKey(account, container, blob, blockID)
	block, _ := blobBlocks.Get(key)
	block.Account = account
	block.Container = container
	block.Blob = blob
	block.BlockID = blockID
	block.UncommittedData = data
	block.HasUncommitted = true
	putBlobBlock(account, container, blob, blockID, block)
	w.WriteHeader(http.StatusCreated)
}

type blockListRequest struct {
	XMLName     xml.Name `xml:"BlockList"`
	Committed   []string `xml:"Committed"`
	Latest      []string `xml:"Latest"`
	Uncommitted []string `xml:"Uncommitted"`
}

func handleCommitBlockList(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	if _, ok := blobContainersData.Get(blobContainerKey(account, container)); !ok {
		writeStorageError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	var req blockListRequest
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageError(w, "InvalidXmlDocument", err.Error(), http.StatusBadRequest)
		return
	}
	refs := make([]blockRef, 0, len(req.Committed)+len(req.Latest)+len(req.Uncommitted))
	for _, id := range req.Committed {
		block, ok := blobBlocks.Get(blobBlockKey(account, container, blob, id))
		if !ok || !block.HasCommitted {
			writeStorageError(w, "InvalidBlockList",
				"The specified block list is invalid.", http.StatusBadRequest)
			return
		}
		refs = append(refs, blockRef{id: id, data: block.CommittedData})
	}
	for _, id := range req.Latest {
		block, ok := blobBlocks.Get(blobBlockKey(account, container, blob, id))
		if !ok || (!block.HasUncommitted && !block.HasCommitted) {
			writeStorageError(w, "InvalidBlockList",
				"The specified block list is invalid.", http.StatusBadRequest)
			return
		}
		data := block.CommittedData
		if block.HasUncommitted {
			data = block.UncommittedData
		}
		refs = append(refs, blockRef{id: id, data: data})
	}
	for _, id := range req.Uncommitted {
		block, ok := blobBlocks.Get(blobBlockKey(account, container, blob, id))
		if !ok || !block.HasUncommitted {
			writeStorageError(w, "InvalidBlockList",
				"The specified block list is invalid.", http.StatusBadRequest)
			return
		}
		refs = append(refs, blockRef{id: id, data: block.UncommittedData})
	}

	var data []byte
	committed := map[string]blockRef{}
	for _, ref := range refs {
		data = append(data, ref.data...)
		committed[ref.id] = ref
	}
	for _, key := range blockKeysForBlob(account, container, blob) {
		block, ok := blobBlocks.Get(key)
		if !ok {
			continue
		}
		ref, keepCommitted := committed[block.BlockID]
		block.HasCommitted = keepCommitted
		if keepCommitted {
			block.CommittedData = ref.data
			block.CommitOrdinal = indexBlockRef(refs, block.BlockID)
			block.HasUncommitted = false
			block.UncommittedData = nil
		}
		if !block.HasCommitted && !block.HasUncommitted {
			deleteBlobBlock(block.Account, block.Container, block.Blob, block.BlockID)
			continue
		}
		putBlobBlock(block.Account, block.Container, block.Blob, block.BlockID, block)
	}
	for idx, ref := range refs {
		key := blobBlockKey(account, container, blob, ref.id)
		block, _ := blobBlocks.Get(key)
		block.Account = account
		block.Container = container
		block.Blob = blob
		block.BlockID = ref.id
		block.CommittedData = ref.data
		block.HasCommitted = true
		block.HasUncommitted = false
		block.UncommittedData = nil
		block.CommitOrdinal = idx
		putBlobBlock(account, container, blob, ref.id, block)
	}

	hash := md5.Sum(data)
	etag := `"` + hex.EncodeToString(hash[:]) + `"`
	lastMod := time.Now().UTC().Format(http.TimeFormat)
	putBlobObject(account, container, blob, BlobObject{
		Account:      account,
		Container:    container,
		Name:         blob,
		Data:         data,
		ContentType:  r.Header.Get("Content-Type"),
		BlobType:     "BlockBlob",
		ETag:         etag,
		LastModified: lastMod,
		Metadata:     collectMetadata(r),
	})
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastMod)
	w.WriteHeader(http.StatusCreated)
}

func indexBlockRef(refs []blockRef, id string) int {
	for i, ref := range refs {
		if ref.id == id {
			return i
		}
	}
	return 0
}

func handleGetBlockList(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	if _, ok := blobContainersData.Get(blobContainerKey(account, container)); !ok {
		writeStorageError(w, "ContainerNotFound",
			"The specified container does not exist.", http.StatusNotFound)
		return
	}
	listType := r.URL.Query().Get("blocklisttype")
	if listType == "" {
		listType = "committed"
	}
	type blockEntry struct {
		Name string `xml:"Name"`
		Size int64  `xml:"Size"`
	}
	type blockList struct {
		XMLName           xml.Name     `xml:"BlockList"`
		CommittedBlocks   []blockEntry `xml:"CommittedBlocks>Block"`
		UncommittedBlocks []blockEntry `xml:"UncommittedBlocks>Block"`
	}
	out := blockList{}
	for _, key := range blockKeysForBlob(account, container, blob) {
		block, ok := blobBlocks.Get(key)
		if !ok {
			continue
		}
		if (listType == "committed" || listType == "all") && block.HasCommitted {
			out.CommittedBlocks = append(out.CommittedBlocks, blockEntry{
				Name: block.BlockID,
				Size: int64(len(block.CommittedData)),
			})
		}
		if (listType == "uncommitted" || listType == "all") && block.HasUncommitted {
			out.UncommittedBlocks = append(out.UncommittedBlocks, blockEntry{
				Name: block.BlockID,
				Size: int64(len(block.UncommittedData)),
			})
		}
	}
	sort.Slice(out.CommittedBlocks, func(i, j int) bool {
		left, _ := blobBlocks.Get(blobBlockKey(account, container, blob, out.CommittedBlocks[i].Name))
		right, _ := blobBlocks.Get(blobBlockKey(account, container, blob, out.CommittedBlocks[j].Name))
		return left.CommitOrdinal < right.CommitOrdinal
	})
	sort.Slice(out.UncommittedBlocks, func(i, j int) bool {
		return out.UncommittedBlocks[i].Name < out.UncommittedBlocks[j].Name
	})
	writeStorageXML(w, http.StatusOK, out)
}

func handleGetBlob(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	b, ok := blobObjects.Get(blobObjectKey(account, container, blob))
	if !ok {
		writeStorageError(w, "BlobNotFound",
			"The specified blob does not exist.", http.StatusNotFound)
		return
	}
	writeBlobHeaders(w, b)
	_, _ = w.Write(b.Data)
}

func handleHeadBlob(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	b, ok := blobObjects.Get(blobObjectKey(account, container, blob))
	if !ok {
		writeStorageError(w, "BlobNotFound",
			"The specified blob does not exist.", http.StatusNotFound)
		return
	}
	writeBlobHeaders(w, b)
}

func handleDeleteBlob(w http.ResponseWriter, r *http.Request, account, container, blob string) {
	existing, exists := blobObjects.Get(blobObjectKey(account, container, blob))
	if !exists {
		writeStorageError(w, "BlobNotFound",
			"The specified blob does not exist.", http.StatusNotFound)
		return
	}
	if !azureBlobPreconditionOK(w, r, existing.ETag, true) {
		return
	}
	deleteBlobObject(account, container, blob)
	for _, key := range blockKeysForBlob(account, container, blob) {
		if block, ok := blobBlocks.Get(key); ok {
			deleteBlobBlock(block.Account, block.Container, block.Blob, block.BlockID)
		}
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
	if b.CopyID != "" {
		w.Header().Set("x-ms-copy-id", b.CopyID)
	}
	if b.CopyStatus != "" {
		w.Header().Set("x-ms-copy-status", b.CopyStatus)
	}
	if b.CopySource != "" {
		w.Header().Set("x-ms-copy-source", b.CopySource)
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
