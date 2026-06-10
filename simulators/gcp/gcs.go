package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
	Name               string            `json:"name"`
	Bucket             string            `json:"bucket"`
	Size               string            `json:"size"`
	ContentType        string            `json:"contentType,omitempty"`
	ContentEncoding    string            `json:"contentEncoding,omitempty"`
	ContentLanguage    string            `json:"contentLanguage,omitempty"`
	ContentDisposition string            `json:"contentDisposition,omitempty"`
	CacheControl       string            `json:"cacheControl,omitempty"`
	StorageClass       string            `json:"storageClass,omitempty"`
	CustomTime         string            `json:"customTime,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	TimeCreated        string            `json:"timeCreated"`
	Updated            string            `json:"updated"`
	Generation         string            `json:"generation,omitempty"`
	Metageneration     string            `json:"metageneration,omitempty"`
	Md5Hash            string            `json:"md5Hash,omitempty"`
	Crc32c             string            `json:"crc32c,omitempty"`
	ComponentCount     int64             `json:"componentCount,omitempty"`
	Etag               string            `json:"etag,omitempty"`
	data               []byte            // unexported: raw object data
	metadataCloned     bool
}

type gcsObjectResource struct {
	Name               string             `json:"name,omitempty"`
	ContentType        *string            `json:"contentType,omitempty"`
	ContentEncoding    *string            `json:"contentEncoding,omitempty"`
	ContentLanguage    *string            `json:"contentLanguage,omitempty"`
	ContentDisposition *string            `json:"contentDisposition,omitempty"`
	CacheControl       *string            `json:"cacheControl,omitempty"`
	StorageClass       *string            `json:"storageClass,omitempty"`
	CustomTime         *string            `json:"customTime,omitempty"`
	Metadata           *map[string]string `json:"metadata,omitempty"`
}

// Package-level stores. gcsBuckets is for dashboard access; gcsObjects
// is exposed so other slices (e.g. cloudbuild.go) can read uploaded
// build context tarballs without depending on the gcs.go handler
// closure.
var (
	gcsBuckets sim.Store[Bucket]
	gcsObjects sim.Store[GCSObject]
)

func (res gcsObjectResource) applyTo(obj GCSObject) GCSObject {
	if res.ContentType != nil {
		obj.ContentType = *res.ContentType
	}
	if res.ContentEncoding != nil {
		obj.ContentEncoding = *res.ContentEncoding
	}
	if res.ContentLanguage != nil {
		obj.ContentLanguage = *res.ContentLanguage
	}
	if res.ContentDisposition != nil {
		obj.ContentDisposition = *res.ContentDisposition
	}
	if res.CacheControl != nil {
		obj.CacheControl = *res.CacheControl
	}
	if res.StorageClass != nil {
		obj.StorageClass = *res.StorageClass
	}
	if res.CustomTime != nil {
		obj.CustomTime = *res.CustomTime
	}
	if res.Metadata != nil {
		obj.Metadata = cloneStringMap(*res.Metadata)
		obj.metadataCloned = true
	}
	return obj
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type invalidGCSObjectMetadataError struct {
	field  string
	reason string
}

func (e *invalidGCSObjectMetadataError) Error() string {
	return fmt.Sprintf("%s: %s", e.field, e.reason)
}

func validateGCSObjectAttrs(attrs GCSObject) error {
	if attrs.CustomTime != "" {
		if _, err := time.Parse(time.RFC3339Nano, attrs.CustomTime); err != nil {
			return &invalidGCSObjectMetadataError{
				field:  "customTime",
				reason: "must be an RFC 3339 timestamp",
			}
		}
	}
	if utf8.RuneCountInString(attrs.ContentLanguage) > 100 {
		return &invalidGCSObjectMetadataError{
			field:  "contentLanguage",
			reason: "must be at most 100 characters",
		}
	}
	return nil
}

func gcsMultipartBoundary(contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err == nil && params["boundary"] != "" {
		return params["boundary"]
	}
	for _, part := range strings.Split(contentType, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "boundary=") {
			return strings.Trim(strings.TrimSpace(part[strings.Index(part, "=")+1:]), `"'`)
		}
	}
	return ""
}

func gcsLocationType(location string) string {
	switch strings.ToUpper(location) {
	case "US", "EU", "ASIA":
		return "multi-region"
	}
	if strings.Contains(location, "+") {
		return "dual-region"
	}
	return "region"
}

// applyBucketPatch merges a PATCH/PUT body into the stored bucket JSON.
// Top-level fields replace; `labels` follows GCS null-value-deletes
// semantics (a null value removes the label, others merge in). Immutable
// server-set fields (id/name/kind/selfLink/timeCreated/projectNumber) are
// not overwritten from the patch body.
func applyBucketPatch(data, patch map[string]any) {
	for k, v := range patch {
		switch k {
		case "id", "name", "kind", "selfLink", "timeCreated", "projectNumber", "metageneration", "etag":
			continue
		case "labels":
			incoming, ok := v.(map[string]any)
			if !ok {
				data["labels"] = v
				continue
			}
			labels, _ := data["labels"].(map[string]any)
			if labels == nil {
				labels = map[string]any{}
			}
			for lk, lv := range incoming {
				if lv == nil {
					delete(labels, lk)
				} else {
					labels[lk] = lv
				}
			}
			data["labels"] = labels
		default:
			data[k] = v
		}
	}
}

func gcsTimestamp() string {
	return time.Now().UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
}

func gcsCRC32C(data []byte) string {
	sum := crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli))
	b := []byte{byte(sum >> 24), byte(sum >> 16), byte(sum >> 8), byte(sum)}
	return base64.StdEncoding.EncodeToString(b)
}

func persistGCSObject(objects sim.Store[GCSObject], bucketName, objectName string, data []byte, attrs GCSObject) (GCSObject, error) {
	if attrs.ContentType == "" {
		attrs.ContentType = "application/octet-stream"
	}
	if err := validateGCSObjectAttrs(attrs); err != nil {
		return GCSObject{}, err
	}
	now := gcsTimestamp()
	hash := md5.Sum(data)
	md5Hash := base64.StdEncoding.EncodeToString(hash[:])
	etag := base64.StdEncoding.EncodeToString(append(hash[:], []byte(now)...))
	generation := int64(1)
	if existing, ok := objects.Get(bucketName + "/" + objectName); ok {
		if n, err := strconv.ParseInt(existing.Generation, 10, 64); err == nil && n >= generation {
			generation = n + 1
		}
	}

	objPath := filepath.Join(GCSBucketHostDir(bucketName), objectName)
	if err := os.MkdirAll(filepath.Dir(objPath), 0o755); err != nil {
		return GCSObject{}, fmt.Errorf("create object dir: %w", err)
	}
	if err := os.WriteFile(objPath, data, 0o644); err != nil {
		return GCSObject{}, fmt.Errorf("write object: %w", err)
	}

	obj := attrs
	obj.Name = objectName
	obj.Bucket = bucketName
	obj.Size = strconv.Itoa(len(data))
	obj.TimeCreated = now
	obj.Updated = now
	obj.Generation = strconv.FormatInt(generation, 10)
	obj.Metageneration = "1"
	obj.Md5Hash = md5Hash
	obj.Crc32c = gcsCRC32C(data)
	obj.Etag = etag
	if !attrs.metadataCloned {
		obj.Metadata = cloneStringMap(attrs.Metadata)
	}
	obj.metadataCloned = true
	obj.data = append([]byte(nil), data...)
	objects.Put(bucketName+"/"+objectName, obj)
	return obj, nil
}

func writeGCSPersistError(w http.ResponseWriter, action string, err error) {
	var invalid *invalidGCSObjectMetadataError
	if errors.As(err, &invalid) {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%s: %v", action, err)
		return
	}
	sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "%s: %v", action, err)
}

// gcsResumableSession holds the in-flight state of a resumable upload
// between session initiation (POST with metadata) and finalization
// (the chunk PUT that delivers the last byte). Keyed by upload_id in
// gcsResumableSessions.
type gcsResumableSession struct {
	mu     sync.Mutex
	Bucket string
	Object string
	Attrs  GCSObject
	Data   []byte
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

	obj, err := persistGCSObject(objects, sess.Bucket, sess.Object, finalData, sess.Attrs)
	if err != nil {
		writeGCSPersistError(w, "write resumable object", err)
		return
	}
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
	meta := map[string]any{
		"kind":           "storage#object",
		"id":             fmt.Sprintf("%s/%s/1", obj.Bucket, obj.Name),
		"selfLink":       selfLink,
		"mediaLink":      mediaLink,
		"name":           obj.Name,
		"bucket":         obj.Bucket,
		"generation":     defaultStr(obj.Generation, "1"),
		"metageneration": defaultStr(obj.Metageneration, "1"),
		"size":           obj.Size,
		"contentType":    obj.ContentType,
		"timeCreated":    obj.TimeCreated,
		"updated":        obj.Updated,
		"crc32c":         obj.Crc32c,
		"etag":           obj.Etag,
	}
	if obj.Md5Hash != "" {
		meta["md5Hash"] = obj.Md5Hash
	}
	if obj.ComponentCount > 0 {
		meta["componentCount"] = obj.ComponentCount
	}
	if obj.ContentEncoding != "" {
		meta["contentEncoding"] = obj.ContentEncoding
	}
	if obj.ContentLanguage != "" {
		meta["contentLanguage"] = obj.ContentLanguage
	}
	if obj.ContentDisposition != "" {
		meta["contentDisposition"] = obj.ContentDisposition
	}
	if obj.CacheControl != "" {
		meta["cacheControl"] = obj.CacheControl
	}
	if obj.StorageClass != "" {
		meta["storageClass"] = obj.StorageClass
	}
	if obj.CustomTime != "" {
		meta["customTime"] = obj.CustomTime
	}
	if len(obj.Metadata) > 0 {
		meta["metadata"] = cloneStringMap(obj.Metadata)
	}
	return meta
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

		now := gcsTimestamp()
		data["id"] = name
		data["kind"] = "storage#bucket"
		data["selfLink"] = gcpSelfLink(r, fmt.Sprintf("/storage/v1/b/%s", name))
		data["projectNumber"] = "123456789012"
		data["metageneration"] = "1"
		data["etag"] = "CAE="
		data["timeCreated"] = now
		data["updated"] = now
		// GCS normalizes bucket locations to upper-case on read; echo that or
		// terraform-provider-google replaces the bucket (location is ForceNew).
		if loc, ok := data["location"].(string); ok && loc != "" {
			data["location"] = strings.ToUpper(loc)
		} else {
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

	// Patch / Update bucket. The Go storage client's BucketHandle.Update
	// and terraform-provider-google send PATCH (PUT for full Update) to
	// mutate versioning/lifecycle/labels/iamConfiguration/retentionPolicy.
	// The bucket stores its JSON verbatim, so apply the patch body as a
	// top-level field merge into that map. `labels` honours GCS null-value-
	// deletes semantics (a null label value removes the key).
	patchBucket := func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")
		if strings.Contains(r.URL.Path, "/o/") || strings.HasSuffix(r.URL.Path, "/o") {
			return
		}
		bucket, ok := buckets.Get(bucketName)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", bucketName)
			return
		}
		var patch map[string]any
		if err := sim.ReadJSON(r, &patch); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		applyBucketPatch(bucket.Data, patch)
		bucket.Data["updated"] = gcsTimestamp()
		buckets.Put(bucketName, bucket)
		sim.WriteJSON(w, http.StatusOK, bucket.Data)
	}
	srv.HandleFunc("PATCH /storage/v1/b/{bucket}", patchBucket)
	srv.HandleFunc("PUT /storage/v1/b/{bucket}", patchBucket)

	// Get bucket storage layout
	srv.HandleFunc("GET /storage/v1/b/{bucket}/storageLayout", func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")
		bucket, ok := buckets.Get(bucketName)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", bucketName)
			return
		}

		location, _ := bucket.Data["location"].(string)
		if location == "" {
			location = "US"
		}
		enabled := false
		if hns, ok := bucket.Data["hierarchicalNamespace"].(map[string]any); ok {
			enabled, _ = hns["enabled"].(bool)
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":         "storage#storageLayout",
			"bucket":       bucketName,
			"location":     location,
			"locationType": gcsLocationType(location),
			"hierarchicalNamespace": map[string]any{
				"enabled": enabled,
			},
		})
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
		sort.Slice(all, func(i, j int) bool {
			ni, _ := all[i].Data["name"].(string)
			nj, _ := all[j].Data["name"].(string)
			return ni < nj
		})
		items := make([]map[string]any, 0, len(all))
		for _, b := range all {
			items = append(items, b.Data)
		}
		page, next, ok := paginateList(w, r, items)
		if !ok {
			return
		}
		resp := map[string]any{"kind": "storage#buckets", "items": page}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
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

		var items []map[string]any
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
			items = append(items, gcsObjectMetadata(r, obj))
		}

		if items == nil {
			items = []map[string]any{}
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i]["name"].(string) < items[j]["name"].(string)
		})
		sort.Strings(prefixes)

		page, next, ok := paginateList(w, r, items)
		if !ok {
			return
		}
		resp := map[string]any{"kind": "storage#objects", "items": page}
		if len(prefixes) > 0 {
			resp["prefixes"] = prefixes
		}
		if next != "" {
			resp["nextPageToken"] = next
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

	// Patch / Update object metadata. ObjectHandle.Update (and the JSON
	// API's objects.patch / objects.update) mutate contentType /
	// cacheControl / metadata etc. in place without re-uploading the
	// payload. Merge the resource fields onto the stored object.
	patchObject := func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")
		objectName := sim.PathParam(r, "object")
		key := bucketName + "/" + objectName
		obj, ok := objects.Get(key)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "object %q not found in bucket %q", objectName, bucketName)
			return
		}
		var res gcsObjectResource
		if err := sim.ReadJSON(r, &res); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		obj = res.applyTo(obj)
		if err := validateGCSObjectAttrs(obj); err != nil {
			writeGCSPersistError(w, "patch object", err)
			return
		}
		obj.Updated = gcsTimestamp()
		if mg, err := strconv.ParseInt(obj.Metageneration, 10, 64); err == nil {
			obj.Metageneration = strconv.FormatInt(mg+1, 10)
		}
		objects.Put(key, obj)
		sim.WriteJSON(w, http.StatusOK, gcsObjectMetadata(r, obj))
	}
	srv.HandleFunc("PATCH /storage/v1/b/{bucket}/o/{object...}", patchObject)
	srv.HandleFunc("PUT /storage/v1/b/{bucket}/o/{object...}", patchObject)

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
		objAttrs := GCSObject{}

		ct := r.Header.Get("Content-Type")
		mediaType, _, _ := mime.ParseMediaType(ct)
		if mediaType == "" && strings.HasPrefix(strings.ToLower(ct), "multipart/related") {
			mediaType = "multipart/related"
		}

		defer r.Body.Close()

		// Resumable upload session initiation. Real GCS accepts the
		// metadata (including `name`) as JSON in the body and returns
		// 200 + a Location header containing an opaque session ID;
		// the SDK then PUTs chunks at that session URL with
		// Content-Range headers. Each PUT either returns 308 Resume
		// Incomplete (with a Range header naming the bytes received)
		// or 200 with the finalized object metadata.
		if uploadType == "resumable" {
			var meta gcsObjectResource
			// Resumable-init body is a JSON object resource. The Go SDK
			// does not gzip-encode this control-plane request; chunk
			// uploads at the session URL carry the streaming envelope.
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
			objAttrs = meta.applyTo(objAttrs)
			if err := validateGCSObjectAttrs(objAttrs); err != nil {
				writeGCSPersistError(w, "init resumable object", err)
				return
			}
			sessionID := generateUUID()
			gcsResumableSessions.Store(sessionID, &gcsResumableSession{
				Bucket: bucketName,
				Object: objectName,
				Attrs:  objAttrs,
				Data:   nil,
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
			boundary := gcsMultipartBoundary(ct)
			// Multipart upload: first part is metadata JSON
			// (including `name` when not in query), second part is
			// data. Real GCS multipart/related bodies are not
			// content-encoded — gzip travels on the resumable
			// session chunk PUT instead (handled in
			// handleGCSResumableChunk via openStreamingBody).
			mr := multipart.NewReader(r.Body, boundary)
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
				var meta gcsObjectResource
				if jsonErr := json.Unmarshal(metaBytes, &meta); jsonErr != nil {
					sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT",
						"failed to parse multipart metadata: %v", jsonErr)
					return
				}
				if objectName == "" {
					objectName = meta.Name
				}
				objAttrs = meta.applyTo(objAttrs)
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
			if objAttrs.ContentType == "" {
				objAttrs.ContentType = dataPart.Header.Get("Content-Type")
			}
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
			objAttrs.ContentType = ct
		}

		obj, err := persistGCSObject(objects, bucketName, objectName, data, objAttrs)
		if err != nil {
			writeGCSPersistError(w, "write object", err)
			return
		}

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
		if handled := handleGCSObjectCopyRequest(w, r, buckets, objects); handled {
			return
		}
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
			Destination *gcsObjectResource `json:"destination"`
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
		var componentCount int64
		for _, src := range req.SourceObjects {
			srcObj, ok := objects.Get(bucketName + "/" + src.Name)
			if !ok {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
					"source object %q not found in bucket %q", src.Name, bucketName)
				return
			}
			if srcObj.ComponentCount > 0 {
				componentCount += srcObj.ComponentCount
			} else {
				componentCount++
			}
			composed = append(composed, gcsObjectBytes(srcObj, bucketName, src.Name)...)
		}
		objAttrs := GCSObject{}
		if req.Destination != nil {
			objAttrs = req.Destination.applyTo(objAttrs)
		}
		composedObj, err := persistGCSObject(objects, bucketName, destObject, composed, objAttrs)
		if err != nil {
			writeGCSPersistError(w, "write composed object", err)
			return
		}
		composedObj.ComponentCount = componentCount
		composedObj.Md5Hash = ""
		objects.Update(bucketName+"/"+destObject, func(o *GCSObject) {
			o.ComponentCount = componentCount
			o.Md5Hash = ""
		})
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
		setGCSObjectResponseHeaders(w.Header(), obj, len(body))
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
		setGCSObjectResponseHeaders(w.Header(), obj, len(body))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})
}

func setGCSObjectResponseHeaders(h http.Header, obj GCSObject, size int) {
	h.Set("Content-Type", obj.ContentType)
	h.Set("Content-Length", strconv.Itoa(size))
	if obj.ContentEncoding != "" {
		h.Set("Content-Encoding", obj.ContentEncoding)
	}
	if obj.ContentLanguage != "" {
		h.Set("Content-Language", obj.ContentLanguage)
	}
	if obj.ContentDisposition != "" {
		h.Set("Content-Disposition", obj.ContentDisposition)
	}
	if obj.CacheControl != "" {
		h.Set("Cache-Control", obj.CacheControl)
	}
	for k, v := range obj.Metadata {
		h.Set("X-Goog-Meta-"+k, v)
	}
}

func handleGCSObjectCopyRequest(w http.ResponseWriter, r *http.Request, buckets sim.Store[Bucket], objects sim.Store[GCSObject]) bool {
	srcBucket, srcObject, dstBucket, dstObject, op, ok := parseGCSCopyPath(r)
	if !ok {
		return false
	}
	if _, ok := buckets.Get(srcBucket); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", srcBucket)
		return true
	}
	if _, ok := buckets.Get(dstBucket); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", dstBucket)
		return true
	}
	copied, ok := copyGCSObject(w, r, srcBucket, srcObject, dstBucket, dstObject, objects)
	if !ok {
		return true
	}
	switch op {
	case "rewriteTo":
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":                "storage#rewriteResponse",
			"totalBytesRewritten": copied.Size,
			"objectSize":          copied.Size,
			"done":                true,
			"resource":            gcsObjectMetadata(r, copied),
		})
	case "copyTo":
		sim.WriteJSON(w, http.StatusOK, gcsObjectMetadata(r, copied))
	}
	return true
}

func parseGCSCopyPath(r *http.Request) (srcBucket, srcObject, dstBucket, dstObject, op string, ok bool) {
	srcBucket = sim.PathParam(r, "bucket")
	if srcBucket == "" {
		return "", "", "", "", "", false
	}
	prefix := "/storage/v1/b/" + url.PathEscape(srcBucket) + "/o/"
	escapedPath := r.URL.EscapedPath()
	if !strings.HasPrefix(escapedPath, prefix) {
		return "", "", "", "", "", false
	}
	rest := strings.TrimPrefix(escapedPath, prefix)
	for _, candidate := range []string{"rewriteTo", "copyTo"} {
		marker := "/" + candidate + "/b/"
		idx := strings.LastIndex(rest, marker)
		if idx < 0 {
			continue
		}
		before := rest[:idx]
		after := rest[idx+len(marker):]
		dstBucketEsc, dstObjectEsc, found := strings.Cut(after, "/o/")
		if !found || before == "" || dstBucketEsc == "" || dstObjectEsc == "" {
			return "", "", "", "", "", false
		}
		srcObject, ok = pathUnescape(before)
		if !ok {
			return "", "", "", "", "", false
		}
		dstBucket, ok = pathUnescape(dstBucketEsc)
		if !ok {
			return "", "", "", "", "", false
		}
		dstObject, ok = pathUnescape(dstObjectEsc)
		if !ok {
			return "", "", "", "", "", false
		}
		return srcBucket, srcObject, dstBucket, dstObject, candidate, true
	}
	return "", "", "", "", "", false
}

func pathUnescape(s string) (string, bool) {
	out, err := url.PathUnescape(s)
	if err != nil {
		return "", false
	}
	return out, true
}

func copyGCSObject(w http.ResponseWriter, r *http.Request, srcBucket, srcObject, dstBucket, dstObject string, objects sim.Store[GCSObject]) (GCSObject, bool) {
	src, ok := objects.Get(srcBucket + "/" + srcObject)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
			"source object %q not found in bucket %q", srcObject, srcBucket)
		return GCSObject{}, false
	}
	dstAttrs := src
	if r.Body != nil {
		defer r.Body.Close()
		var meta gcsObjectResource
		if err := json.NewDecoder(r.Body).Decode(&meta); err != nil && err != io.EOF {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT",
				"failed to parse copy metadata: %v", err)
			return GCSObject{}, false
		}
		dstAttrs = meta.applyTo(dstAttrs)
	}
	data := append([]byte(nil), gcsObjectBytes(src, srcBucket, srcObject)...)
	dst, err := persistGCSObject(objects, dstBucket, dstObject, data, dstAttrs)
	if err != nil {
		writeGCSPersistError(w, "write copied object", err)
		return GCSObject{}, false
	}
	return dst, true
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
