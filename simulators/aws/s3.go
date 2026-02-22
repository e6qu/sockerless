package main

import (
	"bytes"
	"crypto/md5"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// S3 types

type S3Bucket struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type S3Object struct {
	Key          string
	Data         []byte
	ContentType  string
	ETag         string
	LastModified time.Time
	Size         int64
	Metadata     map[string]string
}

// XML response types for S3

type s3ListAllMyBucketsResult struct {
	XMLName xml.Name   `xml:"ListAllMyBucketsResult"`
	Xmlns   string     `xml:"xmlns,attr"`
	Owner   s3Owner    `xml:"Owner"`
	Buckets s3Buckets  `xml:"Buckets"`
}

type s3Owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type s3Buckets struct {
	Bucket []S3Bucket `xml:"Bucket"`
}

type s3ListBucketResult struct {
	XMLName               xml.Name       `xml:"ListBucketResult"`
	Xmlns                 string         `xml:"xmlns,attr"`
	Name                  string         `xml:"Name"`
	Prefix                string         `xml:"Prefix,omitempty"`
	MaxKeys               int            `xml:"MaxKeys"`
	KeyCount              int            `xml:"KeyCount"`
	IsTruncated           bool           `xml:"IsTruncated"`
	Contents              []s3ObjectInfo `xml:"Contents"`
	ContinuationToken     string         `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string         `xml:"NextContinuationToken,omitempty"`
}

type s3ObjectInfo struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type s3CopyObjectResult struct {
	XMLName      xml.Name `xml:"CopyObjectResult"`
	ETag         string   `xml:"ETag"`
	LastModified string   `xml:"LastModified"`
}

// State stores
var (
	s3Buckets_ *sim.StateStore[S3Bucket]
	s3Objects  *sim.StateStore[S3Object]
)

func s3ObjectKey(bucket, key string) string {
	return bucket + "/" + key
}

func registerS3(srv *sim.Server) {
	s3Buckets_ = sim.NewStateStore[S3Bucket]()
	s3Objects = sim.NewStateStore[S3Object]()

	mux := srv.Mux()

	// S3 uses path-style URLs: /{bucket} and /{bucket}/{key...}
	// We need a catch-all handler that dispatches based on path structure.
	mux.HandleFunc("GET /s3", handleS3ListBuckets)
	mux.HandleFunc("PUT /s3/{bucket}", handleS3CreateBucket)
	mux.HandleFunc("HEAD /s3/{bucket}", handleS3HeadBucket)
	mux.HandleFunc("DELETE /s3/{bucket}", handleS3DeleteBucket)
	mux.HandleFunc("GET /s3/{bucket}", handleS3GetBucket)
	mux.HandleFunc("PUT /s3/{bucket}/{key...}", handleS3PutObject)
	mux.HandleFunc("GET /s3/{bucket}/{key...}", handleS3GetObject)
	mux.HandleFunc("HEAD /s3/{bucket}/{key...}", handleS3HeadObject)
	mux.HandleFunc("DELETE /s3/{bucket}/{key...}", handleS3DeleteObject)
}

func handleS3ListBuckets(w http.ResponseWriter, r *http.Request) {
	buckets := s3Buckets_.List()
	if buckets == nil {
		buckets = []S3Bucket{}
	}

	result := s3ListAllMyBucketsResult{
		Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/",
		Owner: s3Owner{
			ID:          "123456789012",
			DisplayName: "simulator",
		},
		Buckets: s3Buckets{
			Bucket: buckets,
		},
	}

	sim.WriteXML(w, http.StatusOK, result)
}

func handleS3CreateBucket(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if bucket == "" {
		sim.S3ErrorXML(w, "InvalidBucketName", "Bucket name is required", "/", sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}

	if _, exists := s3Buckets_.Get(bucket); exists {
		sim.S3ErrorXML(w, "BucketAlreadyOwnedByYou",
			"Your previous request to create the named bucket succeeded and you already own it.",
			bucket, sim.RequestID(r.Context()), http.StatusConflict)
		return
	}

	b := S3Bucket{
		Name:         bucket,
		CreationDate: time.Now().UTC().Format(time.RFC3339),
	}
	s3Buckets_.Put(bucket, b)

	w.Header().Set("Location", "/" + bucket)
	w.WriteHeader(http.StatusOK)
}

func handleS3HeadBucket(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")

	if _, ok := s3Buckets_.Get(bucket); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleS3DeleteBucket(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")

	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}

	// Check if bucket is empty
	objects := s3Objects.Filter(func(obj S3Object) bool {
		return strings.HasPrefix(obj.Key, bucket+"/") || obj.Key == bucket+"/"
	})
	if len(objects) > 0 {
		sim.S3ErrorXML(w, "BucketNotEmpty", "The bucket you tried to delete is not empty",
			bucket, sim.RequestID(r.Context()), http.StatusConflict)
		return
	}

	s3Buckets_.Delete(bucket)
	w.WriteHeader(http.StatusNoContent)
}

func handleS3GetBucket(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")

	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}

	prefix := r.URL.Query().Get("prefix")
	maxKeysStr := r.URL.Query().Get("max-keys")
	maxKeys := 1000
	if maxKeysStr != "" {
		fmt.Sscanf(maxKeysStr, "%d", &maxKeys)
	}

	// Collect objects for this bucket
	bucketPrefix := bucket + "/"
	objects := s3Objects.Filter(func(obj S3Object) bool {
		objKey := obj.Key
		if !strings.HasPrefix(objKey, bucketPrefix) {
			return false
		}
		// Get the key relative to bucket
		relKey := objKey[len(bucketPrefix):]
		if prefix != "" && !strings.HasPrefix(relKey, prefix) {
			return false
		}
		return true
	})

	var contents []s3ObjectInfo
	for _, obj := range objects {
		relKey := obj.Key[len(bucketPrefix):]
		contents = append(contents, s3ObjectInfo{
			Key:          relKey,
			LastModified: obj.LastModified.UTC().Format(time.RFC3339),
			ETag:         obj.ETag,
			Size:         obj.Size,
			StorageClass: "STANDARD",
		})
	}
	if contents == nil {
		contents = []s3ObjectInfo{}
	}

	isTruncated := false
	if len(contents) > maxKeys {
		contents = contents[:maxKeys]
		isTruncated = true
	}

	result := s3ListBucketResult{
		Xmlns:       "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:        bucket,
		Prefix:      prefix,
		MaxKeys:     maxKeys,
		KeyCount:    len(contents),
		IsTruncated: isTruncated,
		Contents:    contents,
	}

	sim.WriteXML(w, http.StatusOK, result)
}

func handleS3PutObject(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")

	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "InternalError", "Failed to read request body",
			key, sim.RequestID(r.Context()), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	hash := md5.Sum(body)
	etag := fmt.Sprintf("\"%x\"", hash)

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Collect user metadata from x-amz-meta-* headers
	metadata := make(map[string]string)
	for k, v := range r.Header {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-amz-meta-") && len(v) > 0 {
			metaKey := strings.TrimPrefix(lower, "x-amz-meta-")
			metadata[metaKey] = v[0]
		}
	}

	obj := S3Object{
		Key:          s3ObjectKey(bucket, key),
		Data:         body,
		ContentType:  contentType,
		ETag:         etag,
		LastModified: time.Now(),
		Size:         int64(len(body)),
		Metadata:     metadata,
	}
	storeKey := s3ObjectKey(bucket, key)
	s3Objects.Put(storeKey, obj)

	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
}

func handleS3GetObject(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")

	storeKey := s3ObjectKey(bucket, key)
	obj, ok := s3Objects.Get(storeKey)
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			key, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", obj.LastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))

	for k, v := range obj.Metadata {
		w.Header().Set("x-amz-meta-"+k, v)
	}

	http.ServeContent(w, r, key, obj.LastModified, bytes.NewReader(obj.Data))
}

func handleS3HeadObject(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")

	storeKey := s3ObjectKey(bucket, key)
	obj, ok := s3Objects.Get(storeKey)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", obj.LastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))

	for k, v := range obj.Metadata {
		w.Header().Set("x-amz-meta-"+k, v)
	}

	w.WriteHeader(http.StatusOK)
}

func handleS3DeleteObject(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")

	storeKey := s3ObjectKey(bucket, key)
	s3Objects.Delete(storeKey)

	// S3 returns 204 even if the object didn't exist
	w.WriteHeader(http.StatusNoContent)
}
