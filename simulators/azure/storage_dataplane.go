package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Azure Storage Files / Queues / Tables data planes.
//
// The Microsoft.Storage ARM PUT response advertises four data-plane
// endpoint URLs on every storage-account create:
//
//	https://{account}.blob.<host>/   (BUG-1103 part 1 fixed in 173.10 → blob.go)
//	https://{account}.file.<host>/   ← this file
//	https://{account}.queue.<host>/  ← this file
//	https://{account}.table.<host>/  ← this file
//
// Real Azure SDK / azcopy / az CLI consumers follow these URLs;
// before this commit the latter three 404'd (the sim emitted the
// URLs but had no handler servicing them — exact BUG-1103 shape).
// Closes BUG-1109. Each data plane is scope-tight to the canonical
// CRUD that terraform-provider-azurerm + the Go SDK exercise; full
// REST surfaces (ranges, leases, SAS, multipart copy, full OData
// query) are out of scope for the first cut.
//
// Wire spec references:
//   Files:  https://learn.microsoft.com/rest/api/storageservices/file-service-rest-api
//   Queues: https://learn.microsoft.com/rest/api/storageservices/queue-service-rest-api
//   Tables: https://learn.microsoft.com/rest/api/storageservices/table-service-rest-api

// ── Files data plane ────────────────────────────────────────────────

// FileShareData is a share's data-plane projection. Distinct from
// the ARM FileShare type (in files.go) which stores ARM-control-plane
// metadata. The data plane stores per-share metadata + the actual
// per-file blob contents under the data-plane host.
type FileShareData struct {
	Account  string
	Name     string
	Quota    int // GiB
	Metadata map[string]string
	Created  string
}

type FileObject struct {
	Account      string
	Share        string
	Path         string // forward-slash separated; "" = share root
	Data         []byte
	ContentType  string
	ETag         string
	LastModified string
	Metadata     map[string]string
}

var (
	fileShareData sim.Store[FileShareData]
	fileObjects   sim.Store[FileObject]
)

// ── Queues data plane ───────────────────────────────────────────────

type QueueData struct {
	Account  string
	Name     string
	Created  string
	Metadata map[string]string
	Messages []QueueMessage
}

type QueueMessage struct {
	MessageID      string
	MessageText    string // base64 (per real Azure spec) or raw
	InsertionTime  string
	ExpirationTime string
	PopReceipt     string
	VisibleAt      int64 // Unix seconds; >now → in-flight
	DequeueCount   int
}

var queueData sim.Store[QueueData]

// ── Tables data plane ───────────────────────────────────────────────

type TableData struct {
	Account string
	Name    string
	Created string
}

// TableEntity stores arbitrary OData properties keyed by name. Real
// Azure Tables types each property; the sim treats every value as
// json.RawMessage so round-trip is byte-exact.
type TableEntity struct {
	Account      string
	Table        string
	PartitionKey string
	RowKey       string
	Properties   map[string]json.RawMessage
	ETag         string
	Timestamp    string
}

var (
	tableData     sim.Store[TableData]
	tableEntities sim.Store[TableEntity]
)

// ── Dispatcher ──────────────────────────────────────────────────────

// registerStorageDataPlane wraps the server handler so requests
// arriving with a `<account>.{file,queue,table}.<host>` Host header
// reach the matching per-service handler. The blob data plane (also
// host-dispatched) is wired separately in blob.go::registerBlobDataPlane;
// the two WrapHandlers stack but only the one whose suffix matches
// dispatches.
func registerStorageDataPlane(srv *sim.Server) {
	fileShareData = sim.MakeStore[FileShareData](srv.DB(), "file_share_data")
	fileObjects = sim.MakeStore[FileObject](srv.DB(), "file_objects")
	queueData = sim.MakeStore[QueueData](srv.DB(), "queue_data")
	tableData = sim.MakeStore[TableData](srv.DB(), "table_data")
	tableEntities = sim.MakeStore[TableEntity](srv.DB(), "table_entities")

	srv.WrapHandler(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if i := strings.LastIndex(host, ":"); i >= 0 {
				host = host[:i]
			}
			if parts := strings.SplitN(host, ".file.", 2); len(parts) == 2 {
				handleFilesDataPlane(w, r, parts[0])
				return
			}
			if parts := strings.SplitN(host, ".queue.", 2); len(parts) == 2 {
				handleQueuesDataPlane(w, r, parts[0])
				return
			}
			if parts := strings.SplitN(host, ".table.", 2); len(parts) == 2 {
				handleTablesDataPlane(w, r, parts[0])
				return
			}
			// Path-style fallback for SDKs configured with a non-
			// `*.core.windows.net` endpoint (Azurite-compatible).
			// Sockerless runs on a single port, so the service is
			// discriminated by a path prefix instead of a per-service
			// port:
			//   /file/{account}/...   → Files data plane
			//   /queue/{account}/...  → Queues data plane
			//   /table/{account}/...  → Tables data plane
			// Connection-string contract: callers configure
			// `FileEndpoint=http://localhost:14568/file/<account>`.
			// Bare `/{account}/...` (blob default) is matched in
			// blob.go's WrapHandler.
			if account, rest, ok := splitServicePrefix(r.URL.Path, "file"); ok {
				r.URL.Path = "/" + rest
				handleFilesDataPlane(w, r, account)
				return
			}
			if account, rest, ok := splitServicePrefix(r.URL.Path, "queue"); ok {
				r.URL.Path = "/" + rest
				handleQueuesDataPlane(w, r, account)
				return
			}
			if account, rest, ok := splitServicePrefix(r.URL.Path, "table"); ok {
				r.URL.Path = "/" + rest
				handleTablesDataPlane(w, r, account)
				return
			}
			// `.web.` (static website) and `.dfs.` (Data Lake Gen2)
			// advertise canonical endpoints in StoragePrimaryEndpoints
			// for wire-fidelity with real Azure, but the sim does not
			// implement either data plane today. Respond with a clear
			// 501 NotImplemented so SDK callers see a typed error
			// rather than 404 — the runner path doesn't reach these
			// surfaces. File a BUG and add real handlers when a runner
			// scenario lands.
			if strings.Contains(host, ".web.") || strings.Contains(host, ".dfs.") {
				surface := "static-website"
				if strings.Contains(host, ".dfs.") {
					surface = "data-lake-gen2"
				}
				sim.AzureErrorf(w, "NotImplemented", http.StatusNotImplemented,
					"Storage %s data plane (host %q) is not implemented by the simulator", surface, host)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
}

// splitServicePrefix matches `/<service>/<account>/<rest...>` where
// `<service>` is the literal `service` argument (file / queue / table)
// and `<account>` is a known storage account. Returns (account,
// rest-of-path, true) on match. Path-style sibling of the bare
// `/{account}/...` blob form in blob.go.
func splitServicePrefix(path, service string) (account, rest string, ok bool) {
	p := strings.TrimPrefix(path, "/")
	prefix := service + "/"
	if !strings.HasPrefix(p, prefix) {
		return "", "", false
	}
	p = p[len(prefix):]
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

// ── Files dispatch ──────────────────────────────────────────────────

func fileShareKey(account, share string) string { return account + "/" + share }
func fileObjectKey(account, share, p string) string {
	return account + "/" + share + "/" + p
}

func handleFilesDataPlane(w http.ResponseWriter, r *http.Request, account string) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	q := r.URL.Query()
	restype := q.Get("restype")

	// Share-level ops: /{share}?restype=share[&comp=...]
	if !strings.Contains(path, "/") && path != "" && restype == "share" {
		switch r.Method {
		case http.MethodPut:
			handleFilesCreateShare(w, r, account, path)
		case http.MethodDelete:
			handleFilesDeleteShare(w, r, account, path)
		case http.MethodGet, http.MethodHead:
			handleFilesGetShareProperties(w, r, account, path)
		default:
			sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
		}
		return
	}
	// Directory-list: /{share}?restype=directory&comp=list
	if !strings.Contains(path, "/") && path != "" && restype == "directory" && q.Get("comp") == "list" {
		handleFilesListFiles(w, r, account, path)
		return
	}
	// File-level ops: /{share}/{file...}
	if i := strings.Index(path, "/"); i > 0 {
		share := path[:i]
		filePath := path[i+1:]
		switch r.Method {
		case http.MethodPut:
			handleFilesPutFile(w, r, account, share, filePath)
		case http.MethodGet:
			handleFilesGetFile(w, r, account, share, filePath)
		case http.MethodHead:
			handleFilesHeadFile(w, r, account, share, filePath)
		case http.MethodDelete:
			handleFilesDeleteFile(w, r, account, share, filePath)
		default:
			sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
		}
		return
	}
	sim.AzureError(w, "InvalidUri", "Unrecognized Files data-plane path", http.StatusBadRequest)
}

func handleFilesCreateShare(w http.ResponseWriter, r *http.Request, account, share string) {
	key := fileShareKey(account, share)
	if _, ok := fileShareData.Get(key); ok {
		sim.AzureError(w, "ShareAlreadyExists", "The specified share already exists.", http.StatusConflict)
		return
	}
	s := FileShareData{
		Account: account, Name: share, Quota: 5120,
		Metadata: collectMetadata(r),
		Created:  time.Now().UTC().Format(time.RFC1123),
	}
	fileShareData.Put(key, s)
	w.Header().Set("Last-Modified", s.Created)
	w.WriteHeader(http.StatusCreated)
}

func handleFilesDeleteShare(w http.ResponseWriter, r *http.Request, account, share string) {
	key := fileShareKey(account, share)
	if !fileShareData.Delete(key) {
		sim.AzureError(w, "ShareNotFound", "The specified share does not exist.", http.StatusNotFound)
		return
	}
	prefix := account + "/" + share + "/"
	for _, f := range fileObjects.List() {
		if strings.HasPrefix(fileObjectKey(f.Account, f.Share, f.Path), prefix) {
			fileObjects.Delete(fileObjectKey(f.Account, f.Share, f.Path))
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleFilesGetShareProperties(w http.ResponseWriter, r *http.Request, account, share string) {
	s, ok := fileShareData.Get(fileShareKey(account, share))
	if !ok {
		sim.AzureError(w, "ShareNotFound", "The specified share does not exist.", http.StatusNotFound)
		return
	}
	w.Header().Set("Last-Modified", s.Created)
	w.Header().Set("x-ms-share-quota", fmt.Sprintf("%d", s.Quota))
	for k, v := range s.Metadata {
		w.Header().Set("x-ms-meta-"+k, v)
	}
	w.WriteHeader(http.StatusOK)
}

func handleFilesListFiles(w http.ResponseWriter, r *http.Request, account, share string) {
	if _, ok := fileShareData.Get(fileShareKey(account, share)); !ok {
		sim.AzureError(w, "ShareNotFound", "The specified share does not exist.", http.StatusNotFound)
		return
	}
	type fileEntry struct {
		Name       string `xml:"Name"`
		Properties struct {
			ContentLength int `xml:"Content-Length"`
		} `xml:"Properties"`
	}
	type enum struct {
		XMLName xml.Name    `xml:"EnumerationResults"`
		Files   []fileEntry `xml:"Entries>File"`
	}
	out := enum{}
	prefix := account + "/" + share + "/"
	for _, f := range fileObjects.List() {
		if strings.HasPrefix(fileObjectKey(f.Account, f.Share, f.Path), prefix) {
			fe := fileEntry{Name: f.Path}
			fe.Properties.ContentLength = len(f.Data)
			out.Files = append(out.Files, fe)
		}
	}
	w.Header().Set("Content-Type", "application/xml")
	body, _ := xml.Marshal(out)
	_, _ = w.Write(body)
}

func handleFilesPutFile(w http.ResponseWriter, r *http.Request, account, share, filePath string) {
	if _, ok := fileShareData.Get(fileShareKey(account, share)); !ok {
		sim.AzureError(w, "ShareNotFound", "The specified share does not exist.", http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	body, err := openStreamingBody(r)
	if err != nil {
		sim.AzureError(w, "UnsupportedHeader", err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	data, err := io.ReadAll(body)
	if err != nil {
		sim.AzureError(w, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC().Format(time.RFC1123)
	etag := `"` + generateUUID() + `"`
	f := FileObject{
		Account: account, Share: share, Path: filePath,
		Data:         data,
		ContentType:  r.Header.Get("Content-Type"),
		ETag:         etag,
		LastModified: now,
		Metadata:     collectMetadata(r),
	}
	fileObjects.Put(fileObjectKey(account, share, filePath), f)
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", now)
	w.WriteHeader(http.StatusCreated)
}

func handleFilesGetFile(w http.ResponseWriter, r *http.Request, account, share, filePath string) {
	f, ok := fileObjects.Get(fileObjectKey(account, share, filePath))
	if !ok {
		sim.AzureError(w, "ResourceNotFound", "The specified file does not exist.", http.StatusNotFound)
		return
	}
	writeFileHeaders(w, f)
	_, _ = w.Write(f.Data)
}

func handleFilesHeadFile(w http.ResponseWriter, r *http.Request, account, share, filePath string) {
	f, ok := fileObjects.Get(fileObjectKey(account, share, filePath))
	if !ok {
		sim.AzureError(w, "ResourceNotFound", "The specified file does not exist.", http.StatusNotFound)
		return
	}
	writeFileHeaders(w, f)
}

func handleFilesDeleteFile(w http.ResponseWriter, r *http.Request, account, share, filePath string) {
	if !fileObjects.Delete(fileObjectKey(account, share, filePath)) {
		sim.AzureError(w, "ResourceNotFound", "The specified file does not exist.", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func writeFileHeaders(w http.ResponseWriter, f FileObject) {
	if f.ContentType != "" {
		w.Header().Set("Content-Type", f.ContentType)
	}
	w.Header().Set("ETag", f.ETag)
	w.Header().Set("Last-Modified", f.LastModified)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(f.Data)))
	for k, v := range f.Metadata {
		w.Header().Set("x-ms-meta-"+k, v)
	}
}

// ── Queues dispatch ─────────────────────────────────────────────────

func queueKey(account, queue string) string { return account + "/" + queue }

func handleQueuesDataPlane(w http.ResponseWriter, r *http.Request, account string) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	q := r.URL.Query()

	// List Queues: GET /?comp=list
	if path == "" && q.Get("comp") == "list" {
		handleQueuesList(w, r, account)
		return
	}

	segs := strings.Split(path, "/")
	if len(segs) == 0 || segs[0] == "" {
		sim.AzureError(w, "InvalidUri", "Unrecognized Queues data-plane path", http.StatusBadRequest)
		return
	}
	queue := segs[0]

	if len(segs) == 1 {
		switch r.Method {
		case http.MethodPut:
			handleQueueCreate(w, r, account, queue)
		case http.MethodDelete:
			handleQueueDelete(w, r, account, queue)
		case http.MethodGet, http.MethodHead:
			handleQueueGetMetadata(w, r, account, queue)
		default:
			sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
		}
		return
	}

	// Messages: /{queue}/messages or /{queue}/messages/{messageid}
	if segs[1] != "messages" {
		sim.AzureError(w, "InvalidUri", "Unrecognized Queues data-plane path", http.StatusBadRequest)
		return
	}
	if len(segs) == 2 {
		switch r.Method {
		case http.MethodPost:
			handleQueuePutMessage(w, r, account, queue)
		case http.MethodGet:
			if q.Get("peekonly") == "true" {
				handleQueuePeekMessages(w, r, account, queue)
			} else {
				handleQueueGetMessages(w, r, account, queue)
			}
		case http.MethodDelete:
			handleQueueClearMessages(w, r, account, queue)
		default:
			sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
		}
		return
	}
	if len(segs) == 3 {
		messageID := segs[2]
		switch r.Method {
		case http.MethodDelete:
			handleQueueDeleteMessage(w, r, account, queue, messageID)
		default:
			sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
		}
		return
	}
	sim.AzureError(w, "InvalidUri", "Unrecognized Queues data-plane path", http.StatusBadRequest)
}

func handleQueueCreate(w http.ResponseWriter, r *http.Request, account, queue string) {
	key := queueKey(account, queue)
	if _, ok := queueData.Get(key); ok {
		// Real Azure: 204 No Content if queue exists with same metadata.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	q := QueueData{
		Account: account, Name: queue,
		Created:  time.Now().UTC().Format(time.RFC1123),
		Metadata: collectMetadata(r),
	}
	queueData.Put(key, q)
	w.WriteHeader(http.StatusCreated)
}

func handleQueueDelete(w http.ResponseWriter, r *http.Request, account, queue string) {
	if !queueData.Delete(queueKey(account, queue)) {
		sim.AzureError(w, "QueueNotFound", "The specified queue does not exist.", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleQueueGetMetadata(w http.ResponseWriter, r *http.Request, account, queue string) {
	q, ok := queueData.Get(queueKey(account, queue))
	if !ok {
		sim.AzureError(w, "QueueNotFound", "The specified queue does not exist.", http.StatusNotFound)
		return
	}
	for k, v := range q.Metadata {
		w.Header().Set("x-ms-meta-"+k, v)
	}
	visible := 0
	now := time.Now().Unix()
	for _, m := range q.Messages {
		if m.VisibleAt <= now {
			visible++
		}
	}
	w.Header().Set("x-ms-approximate-messages-count", fmt.Sprintf("%d", visible))
	w.WriteHeader(http.StatusOK)
}

func handleQueuesList(w http.ResponseWriter, r *http.Request, account string) {
	type qEntry struct {
		Name string `xml:"Name"`
	}
	type enum struct {
		XMLName xml.Name `xml:"EnumerationResults"`
		Queues  []qEntry `xml:"Queues>Queue"`
	}
	out := enum{}
	prefix := account + "/"
	for _, q := range queueData.List() {
		if strings.HasPrefix(queueKey(q.Account, q.Name), prefix) {
			out.Queues = append(out.Queues, qEntry{Name: q.Name})
		}
	}
	w.Header().Set("Content-Type", "application/xml")
	body, _ := xml.Marshal(out)
	_, _ = w.Write(body)
}

// QueueMessageRequest is the XML request body for Put Message.
type QueueMessageRequest struct {
	XMLName     xml.Name `xml:"QueueMessage"`
	MessageText string   `xml:"MessageText"`
}

// QueueMessageResponse is the XML response shape for Get / Peek.
type QueueMessageResponse struct {
	XMLName         xml.Name `xml:"QueueMessage"`
	MessageID       string   `xml:"MessageId,omitempty"`
	InsertionTime   string   `xml:"InsertionTime,omitempty"`
	ExpirationTime  string   `xml:"ExpirationTime,omitempty"`
	PopReceipt      string   `xml:"PopReceipt,omitempty"`
	TimeNextVisible string   `xml:"TimeNextVisible,omitempty"`
	DequeueCount    int      `xml:"DequeueCount,omitempty"`
	MessageText     string   `xml:"MessageText"`
}

func handleQueuePutMessage(w http.ResponseWriter, r *http.Request, account, queue string) {
	key := queueKey(account, queue)
	if _, ok := queueData.Get(key); !ok {
		sim.AzureError(w, "QueueNotFound", "The specified queue does not exist.", http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		sim.AzureError(w, "RequestBodyInvalid",
			"Failed to read request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	var req QueueMessageRequest
	if err := xml.Unmarshal(data, &req); err != nil {
		sim.AzureError(w, "InvalidXmlDocument",
			"The specified XML is not syntactically valid: "+err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now()
	msg := QueueMessage{
		MessageID:      generateUUID(),
		MessageText:    req.MessageText,
		InsertionTime:  now.UTC().Format(time.RFC1123),
		ExpirationTime: now.Add(7 * 24 * time.Hour).UTC().Format(time.RFC1123),
	}
	queueData.Update(key, func(q *QueueData) {
		q.Messages = append(q.Messages, msg)
	})
	resp := QueueMessageResponse{
		MessageID:       msg.MessageID,
		InsertionTime:   msg.InsertionTime,
		ExpirationTime:  msg.ExpirationTime,
		PopReceipt:      "",
		TimeNextVisible: msg.InsertionTime,
	}
	type wrap struct {
		XMLName  xml.Name               `xml:"QueueMessagesList"`
		Messages []QueueMessageResponse `xml:"QueueMessage"`
	}
	body, _ := xml.Marshal(wrap{Messages: []QueueMessageResponse{resp}})
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(body)
}

func handleQueueGetMessages(w http.ResponseWriter, r *http.Request, account, queue string) {
	key := queueKey(account, queue)
	q, ok := queueData.Get(key)
	if !ok {
		sim.AzureError(w, "QueueNotFound", "The specified queue does not exist.", http.StatusNotFound)
		return
	}
	now := time.Now().Unix()
	visTimeout := int64(30)
	if v := r.URL.Query().Get("visibilitytimeout"); v != "" {
		var n int64
		_, _ = fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			visTimeout = n
		}
	}
	numMessages := 1
	if v := r.URL.Query().Get("numofmessages"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &numMessages)
	}
	if numMessages <= 0 || numMessages > 32 {
		numMessages = 1
	}
	var picked []QueueMessage
	queueData.Update(key, func(qq *QueueData) {
		for i := range qq.Messages {
			if len(picked) >= numMessages {
				break
			}
			if qq.Messages[i].VisibleAt > now {
				continue
			}
			qq.Messages[i].PopReceipt = generateUUID()
			qq.Messages[i].VisibleAt = now + visTimeout
			qq.Messages[i].DequeueCount++
			picked = append(picked, qq.Messages[i])
		}
	})
	type wrap struct {
		XMLName  xml.Name               `xml:"QueueMessagesList"`
		Messages []QueueMessageResponse `xml:"QueueMessage"`
	}
	out := wrap{}
	_ = q
	for _, m := range picked {
		out.Messages = append(out.Messages, QueueMessageResponse{
			MessageID:       m.MessageID,
			InsertionTime:   m.InsertionTime,
			ExpirationTime:  m.ExpirationTime,
			PopReceipt:      m.PopReceipt,
			TimeNextVisible: time.Unix(m.VisibleAt, 0).UTC().Format(time.RFC1123),
			DequeueCount:    m.DequeueCount,
			MessageText:     m.MessageText,
		})
	}
	body, _ := xml.Marshal(out)
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(body)
}

func handleQueuePeekMessages(w http.ResponseWriter, r *http.Request, account, queue string) {
	q, ok := queueData.Get(queueKey(account, queue))
	if !ok {
		sim.AzureError(w, "QueueNotFound", "The specified queue does not exist.", http.StatusNotFound)
		return
	}
	type wrap struct {
		XMLName  xml.Name               `xml:"QueueMessagesList"`
		Messages []QueueMessageResponse `xml:"QueueMessage"`
	}
	out := wrap{}
	now := time.Now().Unix()
	for _, m := range q.Messages {
		if m.VisibleAt > now {
			continue
		}
		out.Messages = append(out.Messages, QueueMessageResponse{
			MessageID:      m.MessageID,
			InsertionTime:  m.InsertionTime,
			ExpirationTime: m.ExpirationTime,
			DequeueCount:   m.DequeueCount,
			MessageText:    m.MessageText,
		})
	}
	body, _ := xml.Marshal(out)
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(body)
}

func handleQueueDeleteMessage(w http.ResponseWriter, r *http.Request, account, queue, messageID string) {
	key := queueKey(account, queue)
	if _, ok := queueData.Get(key); !ok {
		sim.AzureError(w, "QueueNotFound", "The specified queue does not exist.", http.StatusNotFound)
		return
	}
	popReceipt := r.URL.Query().Get("popreceipt")
	queueData.Update(key, func(qq *QueueData) {
		out := qq.Messages[:0]
		for _, m := range qq.Messages {
			if m.MessageID == messageID && m.PopReceipt == popReceipt {
				continue
			}
			out = append(out, m)
		}
		qq.Messages = out
	})
	w.WriteHeader(http.StatusNoContent)
}

func handleQueueClearMessages(w http.ResponseWriter, r *http.Request, account, queue string) {
	key := queueKey(account, queue)
	if _, ok := queueData.Get(key); !ok {
		sim.AzureError(w, "QueueNotFound", "The specified queue does not exist.", http.StatusNotFound)
		return
	}
	queueData.Update(key, func(qq *QueueData) {
		qq.Messages = nil
	})
	w.WriteHeader(http.StatusNoContent)
}

// ── Tables dispatch ─────────────────────────────────────────────────

func tableKey(account, table string) string { return account + "/" + table }
func tableEntityKey(account, table, pk, rk string) string {
	return account + "/" + table + "/" + pk + "/" + rk
}

func handleTablesDataPlane(w http.ResponseWriter, r *http.Request, account string) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	// Tables CRUD path: POST /Tables, DELETE /Tables('name')
	if path == "Tables" && r.Method == http.MethodPost {
		handleTableCreate(w, r, account)
		return
	}
	if strings.HasPrefix(path, "Tables('") && strings.HasSuffix(path, "')") {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "Tables('"), "')")
		if r.Method == http.MethodDelete {
			handleTableDelete(w, r, account, name)
			return
		}
		if r.Method == http.MethodGet {
			handleTableGet(w, r, account, name)
			return
		}
	}
	if path == "Tables" && r.Method == http.MethodGet {
		handleTablesList(w, r, account)
		return
	}

	// Entity ops on /{table}
	// PartitionKey/RowKey-addressed: /{table}(PartitionKey='X',RowKey='Y')
	if i := strings.Index(path, "(PartitionKey="); i > 0 {
		table := path[:i]
		rest := path[i+1:]
		rest = strings.TrimSuffix(rest, ")")
		// rest is now PartitionKey='X',RowKey='Y'
		pk, rk := parsePKRK(rest)
		switch r.Method {
		case http.MethodGet:
			handleEntityGet(w, r, account, table, pk, rk)
		case http.MethodPut, http.MethodPatch, "MERGE":
			// MERGE is non-standard but real Azure Tables accepts it
			// for partial-update semantics. Handle the same as PATCH.
			handleEntityUpsert(w, r, account, table, pk, rk)
		case http.MethodDelete:
			handleEntityDelete(w, r, account, table, pk, rk)
		default:
			sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
		}
		return
	}

	// Plain /{table} — POST for insert, GET for query.
	if !strings.Contains(path, "/") && path != "" {
		switch r.Method {
		case http.MethodPost:
			handleEntityInsert(w, r, account, path)
		case http.MethodGet:
			handleEntityQuery(w, r, account, path)
		default:
			sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
		}
		return
	}
	sim.AzureError(w, "InvalidUri", "Unrecognized Tables data-plane path", http.StatusBadRequest)
}

func parsePKRK(s string) (pk, rk string) {
	// Input: `PartitionKey='X',RowKey='Y'`
	for _, kv := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(kv), "=", 2)
		if len(parts) != 2 {
			continue
		}
		val := strings.Trim(parts[1], "'")
		switch parts[0] {
		case "PartitionKey":
			pk = val
		case "RowKey":
			rk = val
		}
	}
	return pk, rk
}

func handleTableCreate(w http.ResponseWriter, r *http.Request, account string) {
	var body struct {
		TableName string `json:"TableName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sim.AzureError(w, "InvalidInput", err.Error(), http.StatusBadRequest)
		return
	}
	key := tableKey(account, body.TableName)
	if _, ok := tableData.Get(key); ok {
		sim.AzureError(w, "TableAlreadyExists", "The specified table already exists.", http.StatusConflict)
		return
	}
	t := TableData{Account: account, Name: body.TableName, Created: time.Now().UTC().Format(time.RFC3339)}
	tableData.Put(key, t)
	sim.WriteJSON(w, http.StatusCreated, map[string]string{"TableName": body.TableName})
}

func handleTableDelete(w http.ResponseWriter, r *http.Request, account, table string) {
	if !tableData.Delete(tableKey(account, table)) {
		sim.AzureError(w, "ResourceNotFound", "The specified table does not exist.", http.StatusNotFound)
		return
	}
	prefix := account + "/" + table + "/"
	for _, e := range tableEntities.List() {
		if strings.HasPrefix(tableEntityKey(e.Account, e.Table, e.PartitionKey, e.RowKey), prefix) {
			tableEntities.Delete(tableEntityKey(e.Account, e.Table, e.PartitionKey, e.RowKey))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleTableGet(w http.ResponseWriter, r *http.Request, account, table string) {
	t, ok := tableData.Get(tableKey(account, table))
	if !ok {
		sim.AzureError(w, "ResourceNotFound", "The specified table does not exist.", http.StatusNotFound)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]string{"TableName": t.Name})
}

func handleTablesList(w http.ResponseWriter, r *http.Request, account string) {
	prefix := account + "/"
	var names []map[string]string
	for _, t := range tableData.List() {
		if strings.HasPrefix(tableKey(t.Account, t.Name), prefix) {
			names = append(names, map[string]string{"TableName": t.Name})
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": names})
}

func handleEntityInsert(w http.ResponseWriter, r *http.Request, account, table string) {
	if _, ok := tableData.Get(tableKey(account, table)); !ok {
		sim.AzureError(w, "ResourceNotFound", "The specified table does not exist.", http.StatusNotFound)
		return
	}
	var props map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&props); err != nil {
		sim.AzureError(w, "InvalidInput", err.Error(), http.StatusBadRequest)
		return
	}
	pk := jsonString(props["PartitionKey"])
	rk := jsonString(props["RowKey"])
	if pk == "" || rk == "" {
		sim.AzureError(w, "InvalidInput", "PartitionKey and RowKey are required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	entity := TableEntity{
		Account: account, Table: table,
		PartitionKey: pk, RowKey: rk,
		Properties: props,
		ETag:       `W/"datetime'` + now + `'"`,
		Timestamp:  now,
	}
	tableEntities.Put(tableEntityKey(account, table, pk, rk), entity)
	w.Header().Set("ETag", entity.ETag)
	w.Header().Set("Preference-Applied", "return-no-content")
	if r.Header.Get("Prefer") == "return-content" {
		props["Timestamp"], _ = json.Marshal(now)
		sim.WriteJSON(w, http.StatusCreated, props)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleEntityGet(w http.ResponseWriter, r *http.Request, account, table, pk, rk string) {
	e, ok := tableEntities.Get(tableEntityKey(account, table, pk, rk))
	if !ok {
		sim.AzureError(w, "ResourceNotFound", "The specified entity does not exist.", http.StatusNotFound)
		return
	}
	out := map[string]json.RawMessage{}
	for k, v := range e.Properties {
		out[k] = v
	}
	out["Timestamp"], _ = json.Marshal(e.Timestamp)
	w.Header().Set("ETag", e.ETag)
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleEntityUpsert(w http.ResponseWriter, r *http.Request, account, table, pk, rk string) {
	if _, ok := tableData.Get(tableKey(account, table)); !ok {
		sim.AzureError(w, "ResourceNotFound", "The specified table does not exist.", http.StatusNotFound)
		return
	}
	var props map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&props); err != nil {
		sim.AzureError(w, "InvalidInput", err.Error(), http.StatusBadRequest)
		return
	}
	props["PartitionKey"], _ = json.Marshal(pk)
	props["RowKey"], _ = json.Marshal(rk)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	entity := TableEntity{
		Account: account, Table: table,
		PartitionKey: pk, RowKey: rk,
		Properties: props,
		ETag:       `W/"datetime'` + now + `'"`,
		Timestamp:  now,
	}
	tableEntities.Put(tableEntityKey(account, table, pk, rk), entity)
	w.Header().Set("ETag", entity.ETag)
	w.WriteHeader(http.StatusNoContent)
}

func handleEntityDelete(w http.ResponseWriter, r *http.Request, account, table, pk, rk string) {
	if !tableEntities.Delete(tableEntityKey(account, table, pk, rk)) {
		sim.AzureError(w, "ResourceNotFound", "The specified entity does not exist.", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleEntityQuery(w http.ResponseWriter, r *http.Request, account, table string) {
	if _, ok := tableData.Get(tableKey(account, table)); !ok {
		sim.AzureError(w, "ResourceNotFound", "The specified table does not exist.", http.StatusNotFound)
		return
	}
	prefix := account + "/" + table + "/"
	var entries []map[string]json.RawMessage
	for _, e := range tableEntities.List() {
		if strings.HasPrefix(tableEntityKey(e.Account, e.Table, e.PartitionKey, e.RowKey), prefix) {
			out := map[string]json.RawMessage{}
			for k, v := range e.Properties {
				out[k] = v
			}
			out["Timestamp"], _ = json.Marshal(e.Timestamp)
			entries = append(entries, out)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": entries})
}

func jsonString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}
