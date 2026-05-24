package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	sim "github.com/sockerless/simulator"
)

// gcsHostRoot returns the on-disk backing directory for the whole
// simulated GCS slice. Each bucket becomes a subdirectory so Cloud Run
// tasks the sim launches can bind-mount a real host path and observe
// the same files across invocations.
func gcsHostRoot() string {
	if dir := os.Getenv("SIM_GCS_DATA_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "sockerless-sim-gcs")
}

// GCSBucketHostDir returns the on-disk directory backing a simulated
// GCS bucket. Created lazily; safe for concurrent callers. Exported
// for use by the Cloud Run Jobs/Services + Cloud Functions task
// runners when they honour `Volume{Gcs{Bucket}}`.
func GCSBucketHostDir(bucket string) string {
	dir := filepath.Join(gcsHostRoot(), bucket)
	_ = os.MkdirAll(dir, 0o777)
	return dir
}

// GCS types

// Bucket stores the full JSON object from the API so that terraform read-backs
// return every field the provider expects (id, selfLink, iamConfiguration, etc.).
type Bucket struct {
	Data map[string]any
}

// GCSObject represents a Cloud Storage object (metadata).
type GCSObject struct {
	Name        string `json:"name"`
	Bucket      string `json:"bucket"`
	Size        string `json:"size"`
	ContentType string `json:"contentType,omitempty"`
	TimeCreated string `json:"timeCreated"`
	Updated     string `json:"updated"`
	Md5Hash     string `json:"md5Hash,omitempty"`
	Etag        string `json:"etag,omitempty"`
	data        []byte // unexported: raw object data
}

// Package-level stores. gcsBuckets is for dashboard access; gcsObjects
// is exposed so other slices (e.g. cloudbuild.go) can read uploaded
// build context tarballs without depending on the gcs.go handler
// closure.
var (
	gcsBuckets sim.Store[Bucket]
	gcsObjects sim.Store[GCSObject]
)

// gcsResumableSession holds the in-flight state of a resumable upload
// between session initiation (POST with metadata) and finalization
// (the chunk PUT that delivers the last byte). Keyed by upload_id in
// gcsResumableSessions.
type gcsResumableSession struct {
	mu          sync.Mutex
	Bucket      string
	Object      string
	ContentType string
	Data        []byte
}

var gcsResumableSessions sync.Map // upload_id → *gcsResumableSession

// handleGCSResumableChunk processes a chunk PUT during a resumable
// upload. The client sends `Content-Range: bytes <start>-<end>/<total>`
// (or `bytes <start>-<end>/*` if total unknown). When `end+1 == total`,
// the upload is complete: the session bytes get persisted as a real
// GCS object and the canonical 200 + object-metadata response goes back.
// Otherwise the sim returns 308 Resume Incomplete with a `Range` header
// naming the bytes received so the client knows where to resume.
func handleGCSResumableChunk(w http.ResponseWriter, r *http.Request, uploadID string, buckets sim.Store[Bucket], objects sim.Store[GCSObject]) {
	sv, ok := gcsResumableSessions.Load(uploadID)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
			"resumable upload session %q not found", uploadID)
		return
	}
	sess := sv.(*gcsResumableSession)
	if _, exists := buckets.Get(sess.Bucket); !exists {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
			"bucket %q not found", sess.Bucket)
		return
	}

	defer r.Body.Close()
	// Wrap the chunk body through openStreamingBody so a
	// Content-Encoding: gzip chunk decodes transparently — real GCS
	// resumable uploads can carry gzip-encoded chunks when the SDK
	// sets Object.ContentEncoding = "gzip" alongside the streamed
	// upload.
	chunkReader, err := openStreamingBody(r)
	if err != nil {
		sim.GCPErrorf(w, http.StatusUnsupportedMediaType, "INVALID_ARGUMENT",
			"%s", err.Error())
		return
	}
	chunk, err := io.ReadAll(chunkReader)
	_ = chunkReader.Close()
	if err != nil {
		sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL",
			"failed to read resumable chunk: %v", err)
		return
	}

	contentRange := r.Header.Get("Content-Range")
	start, end, total, rangeErr := parseGCSContentRange(contentRange, int64(len(chunk)))
	if rangeErr != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			"%s", rangeErr.Error())
		return
	}

	sess.mu.Lock()
	// Grow the buffer if this chunk extends past current length.
	needed := int(end + 1)
	if needed > len(sess.Data) {
		grown := make([]byte, needed)
		copy(grown, sess.Data)
		sess.Data = grown
	}
	copy(sess.Data[start:end+1], chunk)
	dataLen := int64(len(sess.Data))
	sess.mu.Unlock()

	if total < 0 || dataLen < total {
		// Resume Incomplete.
		w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", dataLen-1))
		w.WriteHeader(308)
		return
	}

	// Final chunk — finalize the object. Trim accumulated data to
	// the exact total (in case the buffer was over-grown).
	sess.mu.Lock()
	finalData := sess.Data[:total]
	sess.mu.Unlock()
	gcsResumableSessions.Delete(uploadID)

	objContentType := sess.ContentType
	if objContentType == "" {
		objContentType = "application/octet-stream"
	}
	now := nowTimestamp()
	hash := md5.Sum(finalData)
	md5Hash := base64.StdEncoding.EncodeToString(hash[:])
	etag := fmt.Sprintf("%x", hash)

	objPath := filepath.Join(GCSBucketHostDir(sess.Bucket), sess.Object)
	if err := os.MkdirAll(filepath.Dir(objPath), 0o755); err != nil {
		sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL",
			"create object dir: %v", err)
		return
	}
	if err := os.WriteFile(objPath, finalData, 0o644); err != nil {
		sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL",
			"write resumable object: %v", err)
		return
	}

	obj := GCSObject{
		Name:        sess.Object,
		Bucket:      sess.Bucket,
		Size:        strconv.FormatInt(total, 10),
		ContentType: objContentType,
		TimeCreated: now,
		Updated:     now,
		Md5Hash:     md5Hash,
		Etag:        etag,
		data:        finalData,
	}
	objects.Put(sess.Bucket+"/"+sess.Object, obj)
	sim.WriteJSON(w, http.StatusOK, gcsObjectMetadata(r, obj))
}

// parseGCSContentRange parses `Content-Range: bytes <start>-<end>/<total>`
// (total may be `*`). Returns (start, end, total) where total is -1
// when the header carries `/*`. An absent header assumes the chunk is
// the entire object; any other malformed shape returns an error so
// the caller can emit a real 400, matching GCS's Content-Range
// validation.
func parseGCSContentRange(s string, chunkLen int64) (start, end, total int64, err error) {
	if s == "" {
		return 0, chunkLen - 1, chunkLen, nil
	}
	raw := s
	if !strings.HasPrefix(s, "bytes ") {
		return 0, 0, 0, fmt.Errorf("Content-Range %q: missing `bytes ` unit", raw)
	}
	s = strings.TrimPrefix(s, "bytes ")
	slash := strings.IndexByte(s, '/')
	if slash < 0 {
		return 0, 0, 0, fmt.Errorf("Content-Range %q: missing `/` separator", raw)
	}
	rangePart := s[:slash]
	totalPart := s[slash+1:]
	dash := strings.IndexByte(rangePart, '-')
	if dash < 0 {
		return 0, 0, 0, fmt.Errorf("Content-Range %q: missing `-` in range", raw)
	}
	if _, e := fmt.Sscanf(rangePart[:dash], "%d", &start); e != nil {
		return 0, 0, 0, fmt.Errorf("Content-Range %q: bad start: %v", raw, e)
	}
	if _, e := fmt.Sscanf(rangePart[dash+1:], "%d", &end); e != nil {
		return 0, 0, 0, fmt.Errorf("Content-Range %q: bad end: %v", raw, e)
	}
	if totalPart == "*" {
		return start, end, -1, nil
	}
	if _, e := fmt.Sscanf(totalPart, "%d", &total); e != nil {
		return 0, 0, 0, fmt.Errorf("Content-Range %q: bad total: %v", raw, e)
	}
	return start, end, total, nil
}

// gcsObjectMetadata returns the canonical object-metadata response
// shape with hard-coded https URLs (real GCS is HTTPS-only on the
// JSON API surface).
func gcsObjectMetadata(r *http.Request, obj GCSObject) map[string]any {
	escapedObject := url.PathEscape(obj.Name)
	selfLink := fmt.Sprintf("https://%s/storage/v1/b/%s/o/%s", r.Host, obj.Bucket, escapedObject)
	mediaLink := fmt.Sprintf("https://%s/download/storage/v1/b/%s/o/%s?alt=media", r.Host, obj.Bucket, escapedObject)
	return map[string]any{
		"kind":        "storage#object",
		"id":          fmt.Sprintf("%s/%s/1", obj.Bucket, obj.Name),
		"selfLink":    selfLink,
		"mediaLink":   mediaLink,
		"name":        obj.Name,
		"bucket":      obj.Bucket,
		"generation":  "1",
		"size":        obj.Size,
		"contentType": obj.ContentType,
		"timeCreated": obj.TimeCreated,
		"updated":     obj.Updated,
		"md5Hash":     obj.Md5Hash,
		"etag":        obj.Etag,
	}
}

func registerGCS(srv *sim.Server) {
	buckets := sim.MakeStore[Bucket](srv.DB(), "gcs_buckets")
	gcsBuckets = buckets
	gcsObjects = sim.MakeStore[GCSObject](srv.DB(), "gcs_objects")
	objects := gcsObjects

	// Create bucket
	srv.HandleFunc("POST /storage/v1/b", func(w http.ResponseWriter, r *http.Request) {
		var data map[string]any
		if err := sim.ReadJSON(r, &data); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		name, _ := data["name"].(string)
		if name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}

		if _, exists := buckets.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "bucket %q already exists", name)
			return
		}

		now := nowTimestamp()
		data["id"] = name
		data["kind"] = "storage#bucket"
		data["selfLink"] = gcpSelfLink(r, fmt.Sprintf("/storage/v1/b/%s", name))
		data["projectNumber"] = "123456789012"
		data["metageneration"] = "1"
		data["etag"] = "CAE="
		data["timeCreated"] = now
		data["updated"] = now
		if data["location"] == nil {
			data["location"] = "US"
		}
		if data["storageClass"] == nil {
			data["storageClass"] = "STANDARD"
		}

		buckets.Put(name, Bucket{Data: data})
		sim.WriteJSON(w, http.StatusOK, data)
	})

	// Get bucket
	srv.HandleFunc("GET /storage/v1/b/{bucket}", func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")

		// Don't match if the path continues with /o (objects)
		if strings.Contains(r.URL.Path, "/o/") || strings.HasSuffix(r.URL.Path, "/o") {
			return
		}

		bucket, ok := buckets.Get(bucketName)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", bucketName)
			return
		}
		sim.WriteJSON(w, http.StatusOK, bucket.Data)
	})

	// Delete bucket
	srv.HandleFunc("DELETE /storage/v1/b/{bucket}", func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")

		if !buckets.Delete(bucketName) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", bucketName)
			return
		}

		// Delete all objects in the bucket
		objs := objects.Filter(func(o GCSObject) bool {
			return o.Bucket == bucketName
		})
		for _, obj := range objs {
			objects.Delete(bucketName + "/" + obj.Name)
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// List buckets
	srv.HandleFunc("GET /storage/v1/b", func(w http.ResponseWriter, r *http.Request) {
		all := buckets.List()
		var items []map[string]any
		for _, b := range all {
			items = append(items, b.Data)
		}
		if items == nil {
			items = []map[string]any{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":  "storage#buckets",
			"items": items,
		})
	})

	// List objects
	srv.HandleFunc("GET /storage/v1/b/{bucket}/o", func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")
		prefix := r.URL.Query().Get("prefix")
		delimiter := r.URL.Query().Get("delimiter")

		if _, ok := buckets.Get(bucketName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", bucketName)
			return
		}

		allObjects := objects.Filter(func(o GCSObject) bool {
			if o.Bucket != bucketName {
				return false
			}
			if prefix != "" && !strings.HasPrefix(o.Name, prefix) {
				return false
			}
			return true
		})

		// Build response items (without internal data field)
		type objectMeta struct {
			Name        string `json:"name"`
			Bucket      string `json:"bucket"`
			Size        string `json:"size"`
			ContentType string `json:"contentType,omitempty"`
			TimeCreated string `json:"timeCreated"`
			Updated     string `json:"updated"`
			Md5Hash     string `json:"md5Hash,omitempty"`
			Etag        string `json:"etag,omitempty"`
		}

		var items []objectMeta
		var prefixes []string
		seen := make(map[string]bool)

		for _, obj := range allObjects {
			if delimiter != "" && prefix != "" {
				// Check if there's a delimiter after the prefix
				rest := strings.TrimPrefix(obj.Name, prefix)
				if idx := strings.Index(rest, delimiter); idx >= 0 {
					p := prefix + rest[:idx+len(delimiter)]
					if !seen[p] {
						prefixes = append(prefixes, p)
						seen[p] = true
					}
					continue
				}
			} else if delimiter != "" {
				if idx := strings.Index(obj.Name, delimiter); idx >= 0 {
					p := obj.Name[:idx+len(delimiter)]
					if !seen[p] {
						prefixes = append(prefixes, p)
						seen[p] = true
					}
					continue
				}
			}
			items = append(items, objectMeta{
				Name:        obj.Name,
				Bucket:      obj.Bucket,
				Size:        obj.Size,
				ContentType: obj.ContentType,
				TimeCreated: obj.TimeCreated,
				Updated:     obj.Updated,
				Md5Hash:     obj.Md5Hash,
				Etag:        obj.Etag,
			})
		}

		if items == nil {
			items = []objectMeta{}
		}

		resp := map[string]any{
			"kind":  "storage#objects",
			"items": items,
		}
		if len(prefixes) > 0 {
			resp["prefixes"] = prefixes
		}

		sim.WriteJSON(w, http.StatusOK, resp)
	})

	// Get object metadata
	srv.HandleFunc("GET /storage/v1/b/{bucket}/o/{object...}", func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")
		objectName := sim.PathParam(r, "object")
		key := bucketName + "/" + objectName

		obj, ok := objects.Get(key)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "object %q not found in bucket %q", objectName, bucketName)
			return
		}
		sim.WriteJSON(w, http.StatusOK, gcsObjectMetadata(r, obj))
	})

	// Delete object
	srv.HandleFunc("DELETE /storage/v1/b/{bucket}/o/{object...}", func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")
		objectName := sim.PathParam(r, "object")
		key := bucketName + "/" + objectName

		if !objects.Delete(key) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "object %q not found in bucket %q", objectName, bucketName)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// Resumable chunk uploads come back as PUT on the same path with
	// `upload_id` in the query — share the same dispatch by treating
	// PUT identically to POST for the upload route.
	srv.HandleFunc("PUT /upload/storage/v1/b/{bucket}/o", func(w http.ResponseWriter, r *http.Request) {
		uploadID := r.URL.Query().Get("upload_id")
		if uploadID == "" {
			sim.GCPError(w, http.StatusBadRequest,
				"PUT /upload/... requires upload_id (resumable chunk only)",
				"INVALID_ARGUMENT")
			return
		}
		handleGCSResumableChunk(w, r, uploadID, buckets, objects)
	})

	// Upload object
	srv.HandleFunc("POST /upload/storage/v1/b/{bucket}/o", func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")
		objectName := r.URL.Query().Get("name")
		uploadType := r.URL.Query().Get("uploadType")

		if _, ok := buckets.Get(bucketName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", bucketName)
			return
		}

		var data []byte
		var objContentType string

		ct := r.Header.Get("Content-Type")
		mediaType, params, _ := mime.ParseMediaType(ct)

		defer r.Body.Close()

		// Resumable upload session initiation. Real GCS accepts the
		// metadata (including `name`) as JSON in the body and returns
		// 200 + a Location header containing an opaque session ID;
		// the SDK then PUTs chunks at that session URL with
		// Content-Range headers. Each PUT either returns 308 Resume
		// Incomplete (with a Range header naming the bytes received)
		// or 200 with the finalized object metadata.
		if uploadType == "resumable" {
			var meta struct {
				Name        string `json:"name"`
				ContentType string `json:"contentType"`
			}
			// Resumable-init body is a small fixed-shape JSON metadata
			// blob (`{"name":"...","contentType":"..."}`). The Go SDK
			// does not gzip-encode this — chunk uploads at the
			// session URL carry the streaming envelope (see
			// handleGCSResumableChunk which wraps openStreamingBody).
			body, err := io.ReadAll(r.Body)
			if err != nil {
				sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL",
					"failed to read resumable metadata: %v", err)
				return
			}
			if len(body) > 0 {
				if err := json.Unmarshal(body, &meta); err != nil {
					sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT",
						"failed to parse resumable metadata: %v", err)
					return
				}
			}
			if objectName == "" {
				objectName = meta.Name
			}
			if objectName == "" {
				sim.GCPError(w, http.StatusBadRequest,
					"name is required (in query or body)", "INVALID_ARGUMENT")
				return
			}
			sessionID := generateUUID()
			gcsResumableSessions.Store(sessionID, &gcsResumableSession{
				Bucket:      bucketName,
				Object:      objectName,
				ContentType: meta.ContentType,
				Data:        nil,
			})
			location := fmt.Sprintf("https://%s/upload/storage/v1/b/%s/o?uploadType=resumable&upload_id=%s",
				r.Host, bucketName, sessionID)
			w.Header().Set("Location", location)
			w.WriteHeader(http.StatusOK)
			return
		}

		// Resumable chunk upload — PUT (or POST per some SDKs) at the
		// session URL. Looks up the session by upload_id, parses the
		// Content-Range header, appends bytes, and returns 308 if the
		// upload isn't complete yet or the canonical object metadata
		// when the total size is reached.
		uploadID := r.URL.Query().Get("upload_id")
		if uploadID != "" {
			handleGCSResumableChunk(w, r, uploadID, buckets, objects)
			return
		}

		if mediaType == "multipart/related" {
			// Multipart upload: first part is metadata JSON
			// (including `name` when not in query), second part is
			// data. Real GCS multipart/related bodies are not
			// content-encoded — gzip travels on the resumable
			// session chunk PUT instead (handled in
			// handleGCSResumableChunk via openStreamingBody).
			mr := multipart.NewReader(r.Body, params["boundary"])
			metaPart, err := mr.NextPart()
			if err != nil {
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "failed to read metadata part: %v", err)
				return
			}
			metaBytes, err := io.ReadAll(metaPart)
			_ = metaPart.Close()
			if err != nil {
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT",
					"failed to read multipart metadata part: %v", err)
				return
			}
			if len(metaBytes) > 0 {
				var meta struct {
					Name string `json:"name"`
				}
				if jsonErr := json.Unmarshal(metaBytes, &meta); jsonErr != nil {
					sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT",
						"failed to parse multipart metadata: %v", jsonErr)
					return
				}
				if objectName == "" {
					objectName = meta.Name
				}
			}
			if objectName == "" {
				sim.GCPError(w, http.StatusBadRequest,
					"name is required (in query or multipart metadata)", "INVALID_ARGUMENT")
				return
			}
			// Read data part
			dataPart, err := mr.NextPart()
			if err != nil {
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "failed to read data part: %v", err)
				return
			}
			objContentType = dataPart.Header.Get("Content-Type")
			data, err = io.ReadAll(dataPart)
			if err != nil {
				sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "failed to read data: %v", err)
				return
			}
		} else {
			if objectName == "" {
				sim.GCPError(w, http.StatusBadRequest,
					"name query parameter is required", "INVALID_ARGUMENT")
				return
			}
			// Simple upload (streaming-aware: handles gzip).
			rc, err := openStreamingBody(r)
			if err != nil {
				sim.GCPErrorf(w, http.StatusUnsupportedMediaType, "INVALID_ARGUMENT", "%s", err.Error())
				return
			}
			data, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "failed to read body: %v", err)
				return
			}
			objContentType = ct
		}

		if objContentType == "" {
			objContentType = "application/octet-stream"
		}

		now := nowTimestamp()
		hash := md5.Sum(data)
		md5Hash := base64.StdEncoding.EncodeToString(hash[:])
		etag := fmt.Sprintf("%x", hash)

		// Persist the object bytes on disk (real GCS-shape storage —
		// objects survive sim process restart). Metadata goes through
		// the SQLite-backed sim.Store; the byte payload doesn't because
		// the unexported `data` field would be stripped by the JSON
		// round-trip (sim.Store uses JSON encoding for SQLite). The
		// on-disk file at `<gcsHostRoot>/<bucket>/<object>` is the
		// source of truth for object data.
		objPath := filepath.Join(GCSBucketHostDir(bucketName), objectName)
		if err := os.MkdirAll(filepath.Dir(objPath), 0o755); err != nil {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "create object dir: %v", err)
			return
		}
		if err := os.WriteFile(objPath, data, 0o644); err != nil {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "write object: %v", err)
			return
		}

		obj := GCSObject{
			Name:        objectName,
			Bucket:      bucketName,
			Size:        strconv.Itoa(len(data)),
			ContentType: objContentType,
			TimeCreated: now,
			Updated:     now,
			Md5Hash:     md5Hash,
			Etag:        etag,
			data:        data,
		}

		key := bucketName + "/" + objectName
		objects.Put(key, obj)

		// Real GCS object responses include `kind` + `id` + `selfLink` +
		// `mediaLink` (https-hard-coded — GCS' JSON API is HTTPS-only).
		// terraform-provider-google's `google_storage_bucket_object`
		// reads `selfLink` into the resource's `self_link` attribute
		// on apply, so missing it means the attribute is empty
		// downstream.
		sim.WriteJSON(w, http.StatusOK, gcsObjectMetadata(r, obj))
	})

	// Objects.compose — concatenate source objects into a destination
	// object. Real GCS reads up to 32 source objects in order and
	// emits a single composed object; the Go SDK's high-level write
	// path uses this for compositing, and any S3-multipart-equivalent
	// against GCS uses it as the joining primitive.
	srv.HandleFunc("POST /storage/v1/b/{bucket}/o/{destObject...}", func(w http.ResponseWriter, r *http.Request) {
		// Only dispatch :compose paths — other POSTs at this prefix
		// should fall through to the upload handler family.
		destObject := sim.PathParam(r, "destObject")
		if !strings.HasSuffix(destObject, "/compose") {
			http.NotFound(w, r)
			return
		}
		destObject = strings.TrimSuffix(destObject, "/compose")
		bucketName := sim.PathParam(r, "bucket")
		if _, ok := buckets.Get(bucketName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", bucketName)
			return
		}
		var req struct {
			SourceObjects []struct {
				Name       string `json:"name"`
				Generation string `json:"generation,omitempty"`
			} `json:"sourceObjects"`
			Destination *struct {
				ContentType string `json:"contentType"`
			} `json:"destination"`
		}
		// Compose request body is a fixed-shape JSON document
		// (sourceObjects + destination); the GCS Go SDK does not
		// content-encode control-plane JSON bodies. No
		// openStreamingBody wrap needed.
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT",
				"failed to parse compose request: %v", err)
			return
		}
		if len(req.SourceObjects) == 0 {
			sim.GCPError(w, http.StatusBadRequest,
				"compose requires at least one sourceObject", "INVALID_ARGUMENT")
			return
		}
		var composed []byte
		for _, src := range req.SourceObjects {
			srcObj, ok := objects.Get(bucketName + "/" + src.Name)
			if !ok {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
					"source object %q not found in bucket %q", src.Name, bucketName)
				return
			}
			composed = append(composed, srcObj.data...)
		}
		contentType := "application/octet-stream"
		if req.Destination != nil && req.Destination.ContentType != "" {
			contentType = req.Destination.ContentType
		}
		now := nowTimestamp()
		hash := md5.Sum(composed)
		md5Hash := base64.StdEncoding.EncodeToString(hash[:])
		etag := fmt.Sprintf("%x", hash)
		objPath := filepath.Join(GCSBucketHostDir(bucketName), destObject)
		if err := os.MkdirAll(filepath.Dir(objPath), 0o755); err != nil {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "create dest dir: %v", err)
			return
		}
		if err := os.WriteFile(objPath, composed, 0o644); err != nil {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "write composed object: %v", err)
			return
		}
		composedObj := GCSObject{
			Name:        destObject,
			Bucket:      bucketName,
			Size:        strconv.Itoa(len(composed)),
			ContentType: contentType,
			TimeCreated: now,
			Updated:     now,
			Md5Hash:     md5Hash,
			Etag:        etag,
			data:        composed,
		}
		objects.Put(bucketName+"/"+destObject, composedObj)
		sim.WriteJSON(w, http.StatusOK, gcsObjectMetadata(r, composedObj))
	})

	// XML API style object access (used by cloud.google.com/go/storage for reads).
	// Registered without method prefix to avoid conflict with "/v2/" (both match all methods,
	// resolved by path specificity — more specific literal paths always win).
	//
	// The first path segment is the BUCKET name. Reject (404) when
	// the store has no matching bucket: this catch-all
	// `/{bucket}/{object...}` route would otherwise shadow unrelated
	// top-level paths (e.g. AIP-151 `/v1/...` operations) and answer
	// them with a GCS-shaped 404 that looks like a real GCS not-found
	// to clients.
	srv.HandleFunc("/{bucket}/{object...}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		bucketName := sim.PathParam(r, "bucket")
		objectName := sim.PathParam(r, "object")
		if objectName == "" {
			http.NotFound(w, r)
			return
		}
		if _, ok := buckets.Get(bucketName); !ok {
			http.NotFound(w, r)
			return
		}
		key := bucketName + "/" + objectName

		obj, ok := objects.Get(key)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "object %q not found in bucket %q", objectName, bucketName)
			return
		}

		body := gcsObjectBytes(obj, bucketName, objectName)
		w.Header().Set("Content-Type", obj.ContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})


	// Download object data (JSON API)
	srv.HandleFunc("GET /download/storage/v1/b/{bucket}/o/{object...}", func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")
		objectName := sim.PathParam(r, "object")
		key := bucketName + "/" + objectName

		obj, ok := objects.Get(key)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "object %q not found in bucket %q", objectName, bucketName)
			return
		}

		body := gcsObjectBytes(obj, bucketName, objectName)
		w.Header().Set("Content-Type", obj.ContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})
}

// gcsObjectBytes returns the object's payload bytes. Prefers the
// in-memory copy when present (uploaded in the same process lifetime);
// falls back to the on-disk file at <gcsHostRoot>/<bucket>/<object>
// (which IS the source of truth — the in-memory `data` field is
// stripped by the SQLite-backed sim.Store's JSON round-trip on every
// Get). Returns nil if the disk read fails (caller writes empty body).
func gcsObjectBytes(obj GCSObject, bucket, object string) []byte {
	if len(obj.data) > 0 {
		return obj.data
	}
	body, err := os.ReadFile(filepath.Join(GCSBucketHostDir(bucket), object))
	if err != nil {
		return nil
	}
	return body
}

// GCSObjectBytes is exported for cross-package callers (e.g.
// cloudbuild.go's executeBuild source-fetch).
func GCSObjectBytes(bucket, object string) []byte {
	obj, ok := gcsObjects.Get(bucket + "/" + object)
	if !ok {
		return nil
	}
	return gcsObjectBytes(obj, bucket, object)
}
