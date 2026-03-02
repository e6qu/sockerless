package main

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	sim "github.com/sockerless/simulator"
)

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

// Package-level store for dashboard access.
var gcsBuckets *sim.StateStore[Bucket]

func registerGCS(srv *sim.Server) {
	buckets := sim.NewStateStore[Bucket]()
	gcsBuckets = buckets
	objects := sim.NewStateStore[GCSObject]()

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
		data["selfLink"] = fmt.Sprintf("https://www.googleapis.com/storage/v1/b/%s", name)
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

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"name":        obj.Name,
			"bucket":      obj.Bucket,
			"size":        obj.Size,
			"contentType": obj.ContentType,
			"timeCreated": obj.TimeCreated,
			"updated":     obj.Updated,
			"md5Hash":     obj.Md5Hash,
			"etag":        obj.Etag,
		})
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

	// Upload object
	srv.HandleFunc("POST /upload/storage/v1/b/{bucket}/o", func(w http.ResponseWriter, r *http.Request) {
		bucketName := sim.PathParam(r, "bucket")
		objectName := r.URL.Query().Get("name")

		if objectName == "" {
			sim.GCPError(w, http.StatusBadRequest, "name query parameter is required", "INVALID_ARGUMENT")
			return
		}

		if _, ok := buckets.Get(bucketName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "bucket %q not found", bucketName)
			return
		}

		var data []byte
		var objContentType string

		ct := r.Header.Get("Content-Type")
		mediaType, params, _ := mime.ParseMediaType(ct)

		defer r.Body.Close()

		if mediaType == "multipart/related" {
			// Multipart upload: first part is metadata JSON, second part is data
			mr := multipart.NewReader(r.Body, params["boundary"])
			// Skip metadata part
			metaPart, err := mr.NextPart()
			if err != nil {
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "failed to read metadata part: %v", err)
				return
			}
			_ = metaPart.Close()
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
			// Simple upload
			var err error
			data, err = io.ReadAll(r.Body)
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

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"name":        obj.Name,
			"bucket":      obj.Bucket,
			"size":        obj.Size,
			"contentType": obj.ContentType,
			"timeCreated": obj.TimeCreated,
			"updated":     obj.Updated,
			"md5Hash":     obj.Md5Hash,
			"etag":        obj.Etag,
		})
	})

	// XML API style object access (used by cloud.google.com/go/storage for reads)
	// Registered without method prefix to avoid conflict with "/v2/" (both match all methods,
	// resolved by path specificity - more specific literal paths always win).
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
		key := bucketName + "/" + objectName

		obj, ok := objects.Get(key)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "object %q not found in bucket %q", objectName, bucketName)
			return
		}

		w.Header().Set("Content-Type", obj.ContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(obj.data)))
		w.WriteHeader(http.StatusOK)
		w.Write(obj.data)
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

		w.Header().Set("Content-Type", obj.ContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(obj.data)))
		w.WriteHeader(http.StatusOK)
		w.Write(obj.data)
	})
}
