package main

import (
	"crypto/md5"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Object-level S3 subresources that ride on the stored S3Object:
// ACL, Object Lock legal-hold / retention, GetObjectAttributes,
// GetObjectTorrent, RestoreObject, and the multipart UploadPartCopy.
// Each is faithful to the real S3 REST wire shape (the Smithy output
// shapes in specs/cloud-api/aws/s3.smithy.json.gz) so aws-sdk-go-v2's
// deserializers and the `aws s3api` CLI parse the responses.

// ── Object ACL ───────────────────────────────────────────────────────

// handleS3PutObjectAcl stores the raw <AccessControlPolicy> body (or the
// canned-ACL header) on the object. PutObjectAcl's modeled output is
// empty except for the RequestCharged header, so the success body is
// empty (200 OK).
func handleS3PutObjectAcl(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	storeKey := s3ObjectKey(bucket, key)
	obj, ok := s3Objects.Get(storeKey)
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "IncompleteBody", "Failed to read ACL body: "+err.Error(),
			key, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	obj.ACL = body
	s3Objects.Put(storeKey, obj)
	w.WriteHeader(http.StatusOK)
}

// handleS3GetObjectAcl returns the object ACL. With no ACL ever set the
// canonical owner-FULL_CONTROL policy is synthesized (real S3 returns
// that default for objects created without an explicit ACL).
func handleS3GetObjectAcl(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	obj, ok := s3Objects.Get(s3ObjectKey(bucket, key))
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	if len(obj.ACL) > 0 {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(obj.ACL)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><AccessControlPolicy xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>` + awsAccountID() + `</ID><DisplayName>simulator</DisplayName></Owner><AccessControlList><Grant><Grantee xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="CanonicalUser"><ID>` + awsAccountID() + `</ID><DisplayName>simulator</DisplayName></Grantee><Permission>FULL_CONTROL</Permission></Grant></AccessControlList></AccessControlPolicy>`))
}

// ── Object Lock legal-hold ───────────────────────────────────────────

func handleS3PutObjectLegalHold(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	storeKey := s3ObjectKey(bucket, key)
	obj, ok := s3Objects.Get(storeKey)
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "IncompleteBody", "Failed to read LegalHold body: "+err.Error(),
			key, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	var req struct {
		XMLName xml.Name `xml:"LegalHold"`
		Status  string   `xml:"Status"`
	}
	if err := xml.Unmarshal(body, &req); err != nil {
		sim.S3ErrorXML(w, "MalformedXML", "Failed to parse LegalHold body: "+err.Error(),
			key, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	obj.LegalHoldStatus = req.Status
	s3Objects.Put(storeKey, obj)
	w.WriteHeader(http.StatusOK)
}

func handleS3GetObjectLegalHold(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	obj, ok := s3Objects.Get(s3ObjectKey(bucket, key))
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	status := obj.LegalHoldStatus
	if status == "" {
		// No legal hold ever set: real S3 reports OFF.
		status = "OFF"
	}
	// httpPayload member LegalHold serializes as <LegalHold> (its xmlName).
	out := struct {
		XMLName xml.Name `xml:"LegalHold"`
		Xmlns   string   `xml:"xmlns,attr"`
		Status  string   `xml:"Status"`
	}{
		Xmlns:  "http://s3.amazonaws.com/doc/2006-03-01/",
		Status: status,
	}
	sim.WriteXML(w, http.StatusOK, out)
}

// ── Object Lock retention ────────────────────────────────────────────

func handleS3PutObjectRetention(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	storeKey := s3ObjectKey(bucket, key)
	obj, ok := s3Objects.Get(storeKey)
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "IncompleteBody", "Failed to read Retention body: "+err.Error(),
			key, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	var req struct {
		XMLName         xml.Name `xml:"Retention"`
		Mode            string   `xml:"Mode"`
		RetainUntilDate string   `xml:"RetainUntilDate"`
	}
	if err := xml.Unmarshal(body, &req); err != nil {
		sim.S3ErrorXML(w, "MalformedXML", "Failed to parse Retention body: "+err.Error(),
			key, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	obj.RetentionMode = req.Mode
	obj.RetainUntilDate = req.RetainUntilDate
	s3Objects.Put(storeKey, obj)
	w.WriteHeader(http.StatusOK)
}

func handleS3GetObjectRetention(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	obj, ok := s3Objects.Get(s3ObjectKey(bucket, key))
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	if obj.RetentionMode == "" {
		// No retention configured: real S3 returns 404
		// NoSuchObjectLockConfiguration.
		sim.S3ErrorXML(w, "NoSuchObjectLockConfiguration",
			"The specified object does not have an ObjectLock configuration",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	// httpPayload member Retention serializes as <Retention> (its xmlName);
	// its RetainUntilDate member serializes as <RetainUntilDate>.
	out := struct {
		XMLName         xml.Name `xml:"Retention"`
		Xmlns           string   `xml:"xmlns,attr"`
		Mode            string   `xml:"Mode"`
		RetainUntilDate string   `xml:"RetainUntilDate"`
	}{
		Xmlns:           "http://s3.amazonaws.com/doc/2006-03-01/",
		Mode:            obj.RetentionMode,
		RetainUntilDate: obj.RetainUntilDate,
	}
	sim.WriteXML(w, http.StatusOK, out)
}

// ── GetObjectAttributes ──────────────────────────────────────────────

// handleS3GetObjectAttributes returns the object metadata selected by the
// x-amz-object-attributes header. The modeled output's body members are
// ETag / Checksum / ObjectParts / StorageClass / ObjectSize (the rest are
// header-bound). Real S3 only emits the elements named in the header; the
// sim mirrors that so the response stays a valid subset of the shape.
func handleS3GetObjectAttributes(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	obj, ok := s3Objects.Get(s3ObjectKey(bucket, key))
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	// The x-amz-object-attributes header carries the requested attribute
	// names. aws-sdk-go-v2 serializes the list as repeated header lines
	// (one value each); the CLI / botocore comma-joins them into one. Honor
	// both by splitting every header value on commas.
	want := map[string]bool{}
	for _, hv := range r.Header.Values("x-amz-object-attributes") {
		for _, a := range strings.Split(hv, ",") {
			if a = strings.TrimSpace(a); a != "" {
				want[a] = true
			}
		}
	}

	w.Header().Set("Last-Modified", obj.LastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Content-Type", "application/xml")

	// The output shape's root xmlName is GetObjectAttributesResponse.
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<GetObjectAttributesResponse xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	if want["ETag"] {
		// The ETag attribute is the bare hex (no surrounding quotes),
		// unlike the ETag response header.
		fmt.Fprintf(&b, "<ETag>%s</ETag>", strings.Trim(obj.ETag, `"`))
	}
	if want["StorageClass"] {
		b.WriteString("<StorageClass>STANDARD</StorageClass>")
	}
	if want["ObjectSize"] {
		fmt.Fprintf(&b, "<ObjectSize>%d</ObjectSize>", obj.Size)
	}
	b.WriteString(`</GetObjectAttributesResponse>`)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

// ── GetObjectTorrent ─────────────────────────────────────────────────

// handleS3GetObjectTorrent returns the BitTorrent metainfo for the object.
// The output's sole body member is the streaming Body blob (httpPayload),
// so the response is raw bytes — the sim returns a minimal, well-formed
// bencoded torrent dictionary referencing the object so SDK/CLI parse a
// non-empty body without error.
func handleS3GetObjectTorrent(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	obj, ok := s3Objects.Get(s3ObjectKey(bucket, key))
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	// Bencoded metainfo: a dict with the object length and name. This is
	// a faithful (minimal) BitTorrent metainfo document, not XML — the
	// httpPayload Body member carries opaque bytes.
	torrent := fmt.Sprintf("d4:infod6:lengthi%de4:name%d:%see",
		obj.Size, len(key), key)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(torrent)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(torrent))
}

// ── RestoreObject ────────────────────────────────────────────────────

// handleS3RestoreObject initiates a restore of an archived object. Real
// S3 returns 202 Accepted for a newly-initiated restore and 200 OK when a
// restore is already in progress; RestoreObject's modeled output carries
// only header-bound members, so the body is empty.
func handleS3RestoreObject(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	storeKey := s3ObjectKey(bucket, key)
	obj, ok := s3Objects.Get(storeKey)
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	// Drain the RestoreRequest body (Days / GlacierJobParameters / Tier);
	// the sim doesn't model storage tiers, so the request is accepted and
	// the object marked restored.
	_, _ = io.ReadAll(r.Body)

	status := http.StatusAccepted
	if obj.RestoreRequested {
		// A restore was already requested for this object.
		status = http.StatusOK
	}
	obj.RestoreRequested = true
	obj.RestoreInProgress = false
	obj.RestoreExpiryDate = time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	s3Objects.Put(storeKey, obj)
	w.WriteHeader(status)
}

// ── UploadPartCopy ───────────────────────────────────────────────────

// handleS3UploadPartCopy copies a byte range from the x-amz-copy-source
// object into a multipart upload part — UploadPart + CopyObject combined.
// The output's CopyPartResult httpPayload carries the part ETag and last
// modified time.
func handleS3UploadPartCopy(w http.ResponseWriter, r *http.Request) {
	uploadID := r.URL.Query().Get("uploadId")
	partNumStr := r.URL.Query().Get("partNumber")
	var partNum int
	_, _ = fmt.Sscanf(partNumStr, "%d", &partNum)
	if partNum < 1 || partNum > 10000 {
		sim.S3ErrorXML(w, "InvalidArgument", "Part number must be between 1 and 10000",
			sim.PathParam(r, "bucket"), sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}

	srcRaw := strings.TrimPrefix(r.Header.Get("x-amz-copy-source"), "/")
	srcParts := strings.SplitN(srcRaw, "/", 2)
	if len(srcParts) != 2 {
		sim.S3ErrorXML(w, "InvalidArgument",
			"x-amz-copy-source must be of the form /<bucket>/<key>",
			"", sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	srcBucket, srcKey := srcParts[0], srcParts[1]
	src, ok := s3Objects.Get(srcBucket + "/" + srcKey)
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified source object does not exist",
			srcBucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}

	s3MultipartUploadsMu.Lock()
	mp, ok := s3MultipartUploads[uploadID]
	s3MultipartUploadsMu.Unlock()
	if !ok {
		sim.S3ErrorXML(w, "NoSuchUpload", "The specified multipart upload does not exist",
			sim.PathParam(r, "bucket"), sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}

	// Optional x-amz-copy-source-range: bytes=<start>-<end> (inclusive).
	data := src.Data
	if rng := r.Header.Get("x-amz-copy-source-range"); rng != "" {
		start, end, ok := parseCopySourceRange(rng, int64(len(src.Data)))
		if !ok {
			sim.S3ErrorXML(w, "InvalidArgument",
				"The x-amz-copy-source-range value must be of the form bytes=first-last",
				"", sim.RequestID(r.Context()), http.StatusBadRequest)
			return
		}
		data = src.Data[start : end+1]
	}

	part := append([]byte(nil), data...)
	hash := md5.Sum(part)
	etag := fmt.Sprintf(`"%x"`, hash)
	now := time.Now().UTC()

	s3MultipartUploadsMu.Lock()
	mp.Parts[partNum] = s3MultipartPart{Data: part, ETag: etag}
	s3MultipartUploadsMu.Unlock()

	// httpPayload member CopyPartResult serializes as <CopyPartResult>.
	out := struct {
		XMLName      xml.Name `xml:"CopyPartResult"`
		Xmlns        string   `xml:"xmlns,attr"`
		ETag         string   `xml:"ETag"`
		LastModified string   `xml:"LastModified"`
	}{
		Xmlns:        "http://s3.amazonaws.com/doc/2006-03-01/",
		ETag:         etag,
		LastModified: now.Format(time.RFC3339),
	}
	sim.WriteXML(w, http.StatusOK, out)
}

// parseCopySourceRange parses a `bytes=first-last` range header against the
// object length, returning the inclusive [start, end] byte offsets.
func parseCopySourceRange(rng string, length int64) (int64, int64, bool) {
	spec, ok := strings.CutPrefix(rng, "bytes=")
	if !ok {
		return 0, 0, false
	}
	firstStr, lastStr, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, 0, false
	}
	var first, last int64
	if _, err := fmt.Sscanf(firstStr, "%d", &first); err != nil {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(lastStr, "%d", &last); err != nil {
		return 0, 0, false
	}
	if first < 0 || last < first || last >= length {
		return 0, 0, false
	}
	return first, last, true
}

// ── ListObjects (V1) ─────────────────────────────────────────────────

type s3ListBucketResultV1 struct {
	XMLName        xml.Name         `xml:"ListBucketResult"`
	Xmlns          string           `xml:"xmlns,attr"`
	Name           string           `xml:"Name"`
	Prefix         string           `xml:"Prefix"`
	Marker         string           `xml:"Marker"`
	NextMarker     string           `xml:"NextMarker,omitempty"`
	MaxKeys        int              `xml:"MaxKeys"`
	Delimiter      string           `xml:"Delimiter,omitempty"`
	IsTruncated    bool             `xml:"IsTruncated"`
	Contents       []s3ObjectInfo   `xml:"Contents"`
	CommonPrefixes []s3CommonPrefix `xml:"CommonPrefixes,omitempty"`
}

// handleS3ListObjectsV1 implements the legacy ListObjects (V1) operation.
// It mirrors ListObjectsV2's filtering/delimiter/pagination logic but uses
// the V1 response shape: Marker (request cursor) and NextMarker (the
// continuation cursor when truncated) replace ContinuationToken, and there
// is no KeyCount.
func handleS3ListObjectsV1(w http.ResponseWriter, r *http.Request, bucket string) {
	q := r.URL.Query()
	prefix := q.Get("prefix")
	delimiter := q.Get("delimiter")
	marker := q.Get("marker")
	maxKeys := 1000
	if v := q.Get("max-keys"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &maxKeys); err != nil {
			maxKeys = 1000
		}
	}
	if maxKeys < 0 {
		maxKeys = 0
	}

	bucketPrefix := bucket + "/"
	objects := s3Objects.Filter(func(obj S3Object) bool {
		if !strings.HasPrefix(obj.Key, bucketPrefix) {
			return false
		}
		relKey := obj.Key[len(bucketPrefix):]
		return prefix == "" || strings.HasPrefix(relKey, prefix)
	})

	var contents []s3ObjectInfo
	for _, obj := range objects {
		contents = append(contents, s3ObjectInfo{
			Key:          obj.Key[len(bucketPrefix):],
			LastModified: obj.LastModified.UTC().Format(time.RFC3339),
			ETag:         obj.ETag,
			Size:         obj.Size,
			StorageClass: "STANDARD",
		})
	}
	sort.Slice(contents, func(i, j int) bool {
		return contents[i].Key < contents[j].Key
	})

	if marker != "" {
		next := contents[:0]
		for _, obj := range contents {
			if obj.Key > marker {
				next = append(next, obj)
			}
		}
		contents = next
	}

	type listEntry struct {
		key          string
		object       s3ObjectInfo
		commonPrefix string
		isPrefix     bool
	}
	entries := make([]listEntry, 0, len(contents))
	if delimiter != "" {
		prefixes := map[string]bool{}
		for _, obj := range contents {
			rest := strings.TrimPrefix(obj.Key, prefix)
			if idx := strings.Index(rest, delimiter); idx >= 0 {
				cp := prefix + rest[:idx+len(delimiter)]
				if !prefixes[cp] {
					prefixes[cp] = true
					entries = append(entries, listEntry{key: cp, commonPrefix: cp, isPrefix: true})
				}
				continue
			}
			entries = append(entries, listEntry{key: obj.Key, object: obj})
		}
	} else {
		for _, obj := range contents {
			entries = append(entries, listEntry{key: obj.Key, object: obj})
		}
	}

	isTruncated := false
	nextMarker := ""
	if len(entries) > maxKeys {
		if maxKeys > 0 {
			nextMarker = entries[maxKeys-1].key
		}
		entries = entries[:maxKeys]
		isTruncated = true
	}

	var outContents []s3ObjectInfo
	var commonPrefixes []s3CommonPrefix
	for _, entry := range entries {
		if entry.isPrefix {
			commonPrefixes = append(commonPrefixes, s3CommonPrefix{Prefix: entry.commonPrefix})
			continue
		}
		outContents = append(outContents, entry.object)
	}
	if outContents == nil {
		outContents = []s3ObjectInfo{}
	}

	result := s3ListBucketResultV1{
		Xmlns:          "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:           bucket,
		Prefix:         prefix,
		Marker:         marker,
		MaxKeys:        maxKeys,
		Delimiter:      delimiter,
		IsTruncated:    isTruncated,
		Contents:       outContents,
		CommonPrefixes: commonPrefixes,
	}
	// NextMarker is only meaningful when a delimiter is in play, but real
	// S3 also returns it for a truncated non-delimited listing as a
	// convenience; emit it whenever truncated.
	if isTruncated {
		result.NextMarker = nextMarker
	}
	sim.WriteXML(w, http.StatusOK, result)
}
