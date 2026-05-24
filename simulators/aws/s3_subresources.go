package main

import (
	"crypto/md5"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// S3MultipartUpload tracks an in-flight multipart upload between
// InitiateMultipartUpload and CompleteMultipartUpload. Each Part is
// kept in-memory and stitched together on Complete.
type S3MultipartUpload struct {
	UploadID    string
	Bucket      string
	Key         string
	ContentType string
	Initiated   time.Time
	Parts       map[int]s3MultipartPart // partNumber → bytes+etag
}

type s3MultipartPart struct {
	Data []byte
	ETag string
}

var (
	s3MultipartUploadsMu sync.Mutex
	s3MultipartUploads   = map[string]*S3MultipartUpload{}

	s3ObjectTagsMu sync.Mutex
	s3ObjectTags   = map[string]map[string]string{} // "bucket/key" → tag map
)

// handleS3PostObjectDispatch routes POST /{bucket}/{key...} based on
// the canonical S3 subresource query strings.
func handleS3PostObjectDispatch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch {
	case q.Has("uploads"):
		handleS3InitiateMultipart(w, r)
	case q.Has("uploadId"):
		handleS3CompleteMultipart(w, r)
	default:
		sim.S3ErrorXML(w, "InvalidRequest",
			"POST on an object requires ?uploads (InitiateMultipartUpload) or ?uploadId (CompleteMultipartUpload)",
			"", sim.RequestID(r.Context()), http.StatusBadRequest)
	}
}

// handleS3PostBucketDispatch handles bucket-level POSTs:
// `?uploads` lists in-flight multipart uploads, `?delete` is the
// multi-object delete (already covered elsewhere; surface a friendly
// 400 if reached without that path).
func handleS3PostBucketDispatch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch {
	case q.Has("delete"):
		// Multi-object delete: parse <Delete><Object><Key>... XML, delete
		// each, return <DeleteResult><Deleted>...</Deleted></DeleteResult>.
		handleS3MultiObjectDelete(w, r)
	default:
		sim.S3ErrorXML(w, "InvalidRequest",
			"POST on a bucket requires ?delete",
			sim.PathParam(r, "bucket"), sim.RequestID(r.Context()),
			http.StatusBadRequest)
	}
}

// handleS3PutObjectDispatch routes PUT /{bucket}/{key...} based on the
// special headers and subresource query strings. CopyObject is
// signaled by `x-amz-copy-source`; UploadPart by `?uploadId` + `?partNumber`;
// PutObjectTagging by `?tagging`; otherwise PutObject.
func handleS3PutObjectDispatch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.Header.Get("x-amz-copy-source") != "" {
		handleS3CopyObject(w, r)
		return
	}
	switch {
	case q.Has("uploadId") && q.Has("partNumber"):
		handleS3UploadPart(w, r)
	case q.Has("tagging"):
		handleS3PutObjectTagging(w, r)
	default:
		handleS3PutObject(w, r)
	}
}

// handleS3GetOrHeadObjectDispatch routes GET / HEAD /{bucket}/{key...}
// based on subresource query strings.
func handleS3GetOrHeadObjectDispatch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch {
	case q.Has("tagging"):
		handleS3GetObjectTagging(w, r)
	default:
		handleS3GetOrHeadObject(w, r)
	}
}

// handleS3DeleteObjectDispatch routes DELETE /{bucket}/{key...} based on
// subresource query strings.
func handleS3DeleteObjectDispatch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch {
	case q.Has("uploadId"):
		handleS3AbortMultipart(w, r)
	case q.Has("tagging"):
		handleS3DeleteObjectTagging(w, r)
	default:
		handleS3DeleteObject(w, r)
	}
}

// ── Multipart upload ─────────────────────────────────────────────────

func handleS3InitiateMultipart(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	uploadID := generateUUID()
	contentType := r.Header.Get("Content-Type")
	s3MultipartUploadsMu.Lock()
	s3MultipartUploads[uploadID] = &S3MultipartUpload{
		UploadID:    uploadID,
		Bucket:      bucket,
		Key:         key,
		ContentType: contentType,
		Initiated:   time.Now().UTC(),
		Parts:       map[int]s3MultipartPart{},
	}
	s3MultipartUploadsMu.Unlock()
	result := struct {
		XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
		Xmlns    string   `xml:"xmlns,attr"`
		Bucket   string   `xml:"Bucket"`
		Key      string   `xml:"Key"`
		UploadID string   `xml:"UploadId"`
	}{
		Xmlns:    "http://s3.amazonaws.com/doc/2006-03-01/",
		Bucket:   bucket,
		Key:      key,
		UploadID: uploadID,
	}
	sim.WriteXML(w, http.StatusOK, result)
}

func handleS3UploadPart(w http.ResponseWriter, r *http.Request) {
	uploadID := r.URL.Query().Get("uploadId")
	partNumStr := r.URL.Query().Get("partNumber")
	var partNum int
	_, _ = fmt.Sscanf(partNumStr, "%d", &partNum)
	if partNum < 1 || partNum > 10000 {
		sim.S3ErrorXML(w, "InvalidArgument",
			"Part number must be between 1 and 10000",
			sim.PathParam(r, "bucket"), sim.RequestID(r.Context()),
			http.StatusBadRequest)
		return
	}

	s3MultipartUploadsMu.Lock()
	mp, ok := s3MultipartUploads[uploadID]
	s3MultipartUploadsMu.Unlock()
	if !ok {
		sim.S3ErrorXML(w, "NoSuchUpload",
			"The specified multipart upload does not exist",
			sim.PathParam(r, "bucket"), sim.RequestID(r.Context()),
			http.StatusNotFound)
		return
	}

	defer r.Body.Close()
	var bodyReader io.Reader = r.Body
	if isAWSChunkedRequest(r.Header) {
		bodyReader = newAWSChunkedReader(r.Body)
	}
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		sim.S3ErrorXML(w, "InternalError", "Failed to read part body: "+err.Error(),
			mp.Bucket, sim.RequestID(r.Context()), http.StatusInternalServerError)
		return
	}
	hash := md5.Sum(body)
	etag := fmt.Sprintf(`"%x"`, hash)

	s3MultipartUploadsMu.Lock()
	mp.Parts[partNum] = s3MultipartPart{Data: body, ETag: etag}
	s3MultipartUploadsMu.Unlock()
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
}

func handleS3CompleteMultipart(w http.ResponseWriter, r *http.Request) {
	uploadID := r.URL.Query().Get("uploadId")
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")

	s3MultipartUploadsMu.Lock()
	mp, ok := s3MultipartUploads[uploadID]
	if ok {
		delete(s3MultipartUploads, uploadID)
	}
	s3MultipartUploadsMu.Unlock()
	if !ok {
		sim.S3ErrorXML(w, "NoSuchUpload",
			"The specified multipart upload does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}

	// Parse the <CompleteMultipartUpload><Part>...</Part></CompleteMultipartUpload>
	// XML to learn the order in which to stitch parts. Real S3 verifies
	// the client-supplied ETag matches the stored one; the sim does too.
	var req struct {
		XMLName xml.Name `xml:"CompleteMultipartUpload"`
		Parts   []struct {
			PartNumber int    `xml:"PartNumber"`
			ETag       string `xml:"ETag"`
		} `xml:"Part"`
	}
	defer r.Body.Close()
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "IncompleteBody",
			"Failed to read request body: "+err.Error(),
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	if err := xml.Unmarshal(rawBody, &req); err != nil {
		sim.S3ErrorXML(w, "MalformedXML",
			"Failed to parse CompleteMultipartUpload body: "+err.Error(),
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	if len(req.Parts) == 0 {
		sim.S3ErrorXML(w, "MalformedXML",
			"CompleteMultipartUpload requires at least one Part",
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}

	var assembled []byte
	for _, p := range req.Parts {
		part, ok := mp.Parts[p.PartNumber]
		if !ok {
			sim.S3ErrorXML(w, "InvalidPart",
				fmt.Sprintf("Part number %d not found", p.PartNumber),
				bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
			return
		}
		if p.ETag != "" && p.ETag != part.ETag {
			sim.S3ErrorXML(w, "InvalidPart",
				fmt.Sprintf("ETag mismatch for part %d: got %s want %s", p.PartNumber, p.ETag, part.ETag),
				bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
			return
		}
		assembled = append(assembled, part.Data...)
	}

	// Final-object ETag uses S3's multipart convention:
	// `"<hex(md5(concat(part_md5_bytes)))>-<numParts>"`. We approximate
	// with hex(md5(assembled))-N for sim-side determinism.
	finalHash := md5.Sum(assembled)
	finalETag := fmt.Sprintf(`"%x-%d"`, finalHash, len(req.Parts))

	obj := S3Object{
		Key:          key,
		Data:         assembled,
		Size:         int64(len(assembled)),
		ETag:         finalETag,
		ContentType:  mp.ContentType,
		LastModified: time.Now().UTC(),
	}
	s3Objects.Put(bucket+"/"+key, obj)

	// The Location field is the real-AWS canonical
	// `https://<bucket>.s3.amazonaws.com/<key>` URL that the SDK
	// surfaces as the completed-upload location. aws-sdk-go-v2's
	// high-level Uploader and terraform-provider-aws treat it as
	// advertised metadata (bucket+key are the load-bearing fields
	// for subsequent operations); the sim emits the canonical shape
	// for fidelity even though the *.s3.amazonaws.com subdomain
	// resolves to real S3, not the sim.
	result := struct {
		XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
		Xmlns    string   `xml:"xmlns,attr"`
		Location string   `xml:"Location"` // external: real-AWS canonical *.s3.amazonaws.com URL
		Bucket   string   `xml:"Bucket"`
		Key      string   `xml:"Key"`
		ETag     string   `xml:"ETag"`
	}{
		Xmlns:    "http://s3.amazonaws.com/doc/2006-03-01/",
		Location: fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucket, key),
		Bucket:   bucket,
		Key:      key,
		ETag:     finalETag,
	}
	sim.WriteXML(w, http.StatusOK, result)
}

func handleS3AbortMultipart(w http.ResponseWriter, r *http.Request) {
	uploadID := r.URL.Query().Get("uploadId")
	s3MultipartUploadsMu.Lock()
	_, ok := s3MultipartUploads[uploadID]
	delete(s3MultipartUploads, uploadID)
	s3MultipartUploadsMu.Unlock()
	if !ok {
		sim.S3ErrorXML(w, "NoSuchUpload",
			"The specified multipart upload does not exist",
			sim.PathParam(r, "bucket"), sim.RequestID(r.Context()),
			http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Object tagging ───────────────────────────────────────────────────

func handleS3PutObjectTagging(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	if _, ok := s3Objects.Get(bucket + "/" + key); !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	var req struct {
		XMLName xml.Name `xml:"Tagging"`
		TagSet  struct {
			Tags []struct {
				Key   string `xml:"Key"`
				Value string `xml:"Value"`
			} `xml:"Tag"`
		} `xml:"TagSet"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "IncompleteBody",
			"Failed to read Tagging body: "+err.Error(),
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	if err := xml.Unmarshal(body, &req); err != nil {
		sim.S3ErrorXML(w, "MalformedXML",
			"Failed to parse Tagging body: "+err.Error(),
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	tags := make(map[string]string, len(req.TagSet.Tags))
	for _, t := range req.TagSet.Tags {
		tags[t.Key] = t.Value
	}
	s3ObjectTagsMu.Lock()
	s3ObjectTags[bucket+"/"+key] = tags
	s3ObjectTagsMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func handleS3GetObjectTagging(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	if _, ok := s3Objects.Get(bucket + "/" + key); !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3ObjectTagsMu.Lock()
	tags := s3ObjectTags[bucket+"/"+key]
	s3ObjectTagsMu.Unlock()

	type tag struct {
		Key   string `xml:"Key"`
		Value string `xml:"Value"`
	}
	out := struct {
		XMLName xml.Name `xml:"Tagging"`
		Xmlns   string   `xml:"xmlns,attr"`
		TagSet  struct {
			Tags []tag `xml:"Tag"`
		} `xml:"TagSet"`
	}{
		Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/",
	}
	for k, v := range tags {
		out.TagSet.Tags = append(out.TagSet.Tags, tag{Key: k, Value: v})
	}
	sim.WriteXML(w, http.StatusOK, out)
}

func handleS3DeleteObjectTagging(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	s3ObjectTagsMu.Lock()
	delete(s3ObjectTags, bucket+"/"+key)
	s3ObjectTagsMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// ── CopyObject ───────────────────────────────────────────────────────

func handleS3CopyObject(w http.ResponseWriter, r *http.Request) {
	srcRaw := r.Header.Get("x-amz-copy-source")
	srcRaw = strings.TrimPrefix(srcRaw, "/")
	parts := strings.SplitN(srcRaw, "/", 2)
	if len(parts) != 2 {
		sim.S3ErrorXML(w, "InvalidArgument",
			"x-amz-copy-source must be of the form /<bucket>/<key>",
			"", sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	srcBucket, srcKey := parts[0], parts[1]
	dstBucket := sim.PathParam(r, "bucket")
	dstKey := sim.PathParam(r, "key")

	src, ok := s3Objects.Get(srcBucket + "/" + srcKey)
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey",
			"The specified source object does not exist",
			srcBucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	if _, ok := s3Buckets_.Get(dstBucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			dstBucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}

	now := time.Now().UTC()
	dst := S3Object{
		Key:          dstKey,
		Data:         append([]byte(nil), src.Data...),
		Size:         src.Size,
		ETag:         src.ETag,
		ContentType:  src.ContentType,
		LastModified: now,
	}
	s3Objects.Put(dstBucket+"/"+dstKey, dst)
	result := struct {
		XMLName      xml.Name `xml:"CopyObjectResult"`
		Xmlns        string   `xml:"xmlns,attr"`
		ETag         string   `xml:"ETag"`
		LastModified string   `xml:"LastModified"`
	}{
		Xmlns:        "http://s3.amazonaws.com/doc/2006-03-01/",
		ETag:         src.ETag,
		LastModified: now.Format(time.RFC3339),
	}
	sim.WriteXML(w, http.StatusOK, result)
}

// ── Multi-object delete ──────────────────────────────────────────────

func handleS3MultiObjectDelete(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	var req struct {
		XMLName xml.Name `xml:"Delete"`
		Quiet   bool     `xml:"Quiet"`
		Objects []struct {
			Key string `xml:"Key"`
		} `xml:"Object"`
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "IncompleteBody",
			"Failed to read Delete body: "+err.Error(),
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	if err := xml.Unmarshal(body, &req); err != nil {
		sim.S3ErrorXML(w, "MalformedXML",
			"Failed to parse Delete body: "+err.Error(),
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	type deleted struct {
		Key string `xml:"Key"`
	}
	out := struct {
		XMLName xml.Name  `xml:"DeleteResult"`
		Xmlns   string    `xml:"xmlns,attr"`
		Deleted []deleted `xml:"Deleted"`
	}{
		Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/",
	}
	for _, o := range req.Objects {
		s3Objects.Delete(bucket + "/" + o.Key)
		if !req.Quiet {
			out.Deleted = append(out.Deleted, deleted{Key: o.Key})
		}
	}
	sim.WriteXML(w, http.StatusOK, out)
}

