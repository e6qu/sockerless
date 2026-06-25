package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// This file implements the S3 surfaces that sit outside the generic
// bucket/object-subresource families: S3 Express One Zone directory
// buckets (CreateSession / ListDirectoryBuckets), S3 Metadata
// configurations (the V2 ?metadataConfiguration feature and the legacy
// V1 ?metadataTable feature), attribute-based access control
// (?abac), object rename (?renameObject), and per-object server-side
// encryption updates (?encryption).

// s3MetadataConfig holds the S3 Metadata configuration (V2 feature,
// ?metadataConfiguration) for a single general-purpose bucket. The raw
// MetadataConfiguration the client POSTed is kept so the synthesized
// GET result echoes the destination/journal/inventory the client asked
// for; the table statuses progress to their steady-state ACTIVE value
// (the sim has no real S3 Tables backend, so the table is reported as
// created and active, matching what a settled real config returns).
type s3MetadataConfig struct {
	// JournalEnabled is always true (the journal table is required).
	JournalRecordExpirationDays int
	// InventoryEnabled mirrors the InventoryTableConfiguration's
	// ConfigurationState (ENABLED/DISABLED), empty when no inventory
	// table was requested.
	InventoryState string
}

// s3MetadataTableConfig holds the legacy V1 S3 Metadata table
// configuration (?metadataTable). It records the customer-managed
// destination table bucket the client named.
type s3MetadataTableConfig struct {
	TableBucketArn string
	TableName      string
}

var (
	s3MetadataConfigsMu sync.Mutex
	s3MetadataConfigs   = map[string]s3MetadataConfig{} // bucket → config

	s3MetadataTableConfigsMu sync.Mutex
	s3MetadataTableConfigs   = map[string]s3MetadataTableConfig{} // bucket → config

	s3AbacMu     sync.Mutex
	s3AbacStatus = map[string]string{} // bucket → "Enabled"/"Disabled"
)

// ── S3 Metadata configuration (V2, ?metadataConfiguration) ──────────────

// CreateBucketMetadataConfigurationRequest body shape (the
// MetadataConfiguration httpPayload). Only the fields the sim echoes
// back are modeled.
type s3MetadataConfigurationBody struct {
	XMLName                   xml.Name `xml:"MetadataConfiguration"`
	JournalTableConfiguration struct {
		RecordExpiration struct {
			Expiration string `xml:"Expiration"`
			Days       int    `xml:"Days"`
		} `xml:"RecordExpiration"`
	} `xml:"JournalTableConfiguration"`
	InventoryTableConfiguration struct {
		ConfigurationState string `xml:"ConfigurationState"`
	} `xml:"InventoryTableConfiguration"`
}

func handleS3CreateBucketMetadataConfiguration(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3MetadataConfigsMu.Lock()
	if _, exists := s3MetadataConfigs[bucket]; exists {
		s3MetadataConfigsMu.Unlock()
		sim.S3ErrorXML(w, "MetadataTableAlreadyExistsError",
			"A metadata configuration already exists for this bucket. Delete it before creating a new one.",
			bucket, sim.RequestID(r.Context()), http.StatusConflict)
		return
	}
	s3MetadataConfigsMu.Unlock()

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "IncompleteBody", "Failed to read request body: "+err.Error(),
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	var doc s3MetadataConfigurationBody
	if len(body) > 0 {
		if err := xml.Unmarshal(body, &doc); err != nil {
			sim.S3ErrorXML(w, "MalformedXML", "The XML you provided was not well-formed: "+err.Error(),
				bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
			return
		}
	}
	cfg := s3MetadataConfig{
		JournalRecordExpirationDays: doc.JournalTableConfiguration.RecordExpiration.Days,
		InventoryState:              doc.InventoryTableConfiguration.ConfigurationState,
	}
	s3MetadataConfigsMu.Lock()
	s3MetadataConfigs[bucket] = cfg
	s3MetadataConfigsMu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func handleS3GetBucketMetadataConfiguration(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3MetadataConfigsMu.Lock()
	cfg, ok := s3MetadataConfigs[bucket]
	s3MetadataConfigsMu.Unlock()
	if !ok {
		sim.S3ErrorXML(w, "MetadataTableConfigNotFoundError",
			"The metadata configuration does not exist for this bucket.",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}

	tableBucketArn := fmt.Sprintf("arn:aws:s3tables:%s:%s:bucket/aws-s3", awsRegion(), awsAccountID())
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<GetBucketMetadataConfigurationResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	sb.WriteString(`<MetadataConfigurationResult>`)
	sb.WriteString(`<DestinationResult>`)
	sb.WriteString(`<TableBucketType>aws</TableBucketType>`)
	fmt.Fprintf(&sb, `<TableBucketArn>%s</TableBucketArn>`, tableBucketArn)
	sb.WriteString(`<TableNamespace>aws_s3_metadata</TableNamespace>`)
	sb.WriteString(`</DestinationResult>`)
	sb.WriteString(`<JournalTableConfigurationResult>`)
	sb.WriteString(`<TableStatus>ACTIVE</TableStatus>`)
	fmt.Fprintf(&sb, `<TableName>%s-journal</TableName>`, bucket)
	fmt.Fprintf(&sb, `<TableArn>%s/table/%s-journal</TableArn>`, tableBucketArn, bucket)
	sb.WriteString(`<RecordExpiration>`)
	if cfg.JournalRecordExpirationDays > 0 {
		sb.WriteString(`<Expiration>ENABLED</Expiration>`)
		fmt.Fprintf(&sb, `<Days>%d</Days>`, cfg.JournalRecordExpirationDays)
	} else {
		sb.WriteString(`<Expiration>DISABLED</Expiration>`)
	}
	sb.WriteString(`</RecordExpiration>`)
	sb.WriteString(`</JournalTableConfigurationResult>`)
	if cfg.InventoryState != "" {
		sb.WriteString(`<InventoryTableConfigurationResult>`)
		fmt.Fprintf(&sb, `<ConfigurationState>%s</ConfigurationState>`, xmlEscape(cfg.InventoryState))
		if strings.EqualFold(cfg.InventoryState, "ENABLED") {
			sb.WriteString(`<TableStatus>ACTIVE</TableStatus>`)
			fmt.Fprintf(&sb, `<TableName>%s-inventory</TableName>`, bucket)
			fmt.Fprintf(&sb, `<TableArn>%s/table/%s-inventory</TableArn>`, tableBucketArn, bucket)
		}
		sb.WriteString(`</InventoryTableConfigurationResult>`)
	}
	sb.WriteString(`</MetadataConfigurationResult>`)
	sb.WriteString(`</GetBucketMetadataConfigurationResult>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, sb.String())
}

func handleS3DeleteBucketMetadataConfiguration(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3MetadataConfigsMu.Lock()
	delete(s3MetadataConfigs, bucket)
	s3MetadataConfigsMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// UpdateBucketMetadataInventoryTableConfiguration /
// UpdateBucketMetadataJournalTableConfiguration mutate sub-configs of
// an existing V2 metadata configuration. Both return 200 with no body.

type s3InventoryTableUpdateBody struct {
	XMLName            xml.Name `xml:"InventoryTableConfiguration"`
	ConfigurationState string   `xml:"ConfigurationState"`
}

func handleS3UpdateBucketMetadataInventoryTable(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3MetadataConfigsMu.Lock()
	cfg, ok := s3MetadataConfigs[bucket]
	s3MetadataConfigsMu.Unlock()
	if !ok {
		sim.S3ErrorXML(w, "MetadataTableConfigNotFoundError",
			"The metadata configuration does not exist for this bucket.",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var doc s3InventoryTableUpdateBody
	if len(body) > 0 {
		if err := xml.Unmarshal(body, &doc); err != nil {
			sim.S3ErrorXML(w, "MalformedXML", "The XML you provided was not well-formed: "+err.Error(),
				bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
			return
		}
	}
	cfg.InventoryState = doc.ConfigurationState
	s3MetadataConfigsMu.Lock()
	s3MetadataConfigs[bucket] = cfg
	s3MetadataConfigsMu.Unlock()
	w.WriteHeader(http.StatusOK)
}

type s3JournalTableUpdateBody struct {
	XMLName          xml.Name `xml:"JournalTableConfiguration"`
	RecordExpiration struct {
		Expiration string `xml:"Expiration"`
		Days       int    `xml:"Days"`
	} `xml:"RecordExpiration"`
}

func handleS3UpdateBucketMetadataJournalTable(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3MetadataConfigsMu.Lock()
	cfg, ok := s3MetadataConfigs[bucket]
	s3MetadataConfigsMu.Unlock()
	if !ok {
		sim.S3ErrorXML(w, "MetadataTableConfigNotFoundError",
			"The metadata configuration does not exist for this bucket.",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var doc s3JournalTableUpdateBody
	if len(body) > 0 {
		if err := xml.Unmarshal(body, &doc); err != nil {
			sim.S3ErrorXML(w, "MalformedXML", "The XML you provided was not well-formed: "+err.Error(),
				bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
			return
		}
	}
	cfg.JournalRecordExpirationDays = doc.RecordExpiration.Days
	s3MetadataConfigsMu.Lock()
	s3MetadataConfigs[bucket] = cfg
	s3MetadataConfigsMu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// ── S3 Metadata table configuration (V1, ?metadataTable) ────────────────

type s3MetadataTableBody struct {
	XMLName             xml.Name `xml:"MetadataTableConfiguration"`
	S3TablesDestination struct {
		TableBucketArn string `xml:"TableBucketArn"`
		TableName      string `xml:"TableName"`
	} `xml:"S3TablesDestination"`
}

func handleS3CreateBucketMetadataTableConfiguration(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3MetadataTableConfigsMu.Lock()
	if _, exists := s3MetadataTableConfigs[bucket]; exists {
		s3MetadataTableConfigsMu.Unlock()
		sim.S3ErrorXML(w, "MetadataTableAlreadyExistsError",
			"A metadata table configuration already exists for this bucket. Delete it before creating a new one.",
			bucket, sim.RequestID(r.Context()), http.StatusConflict)
		return
	}
	s3MetadataTableConfigsMu.Unlock()

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "IncompleteBody", "Failed to read request body: "+err.Error(),
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	var doc s3MetadataTableBody
	if len(body) > 0 {
		if err := xml.Unmarshal(body, &doc); err != nil {
			sim.S3ErrorXML(w, "MalformedXML", "The XML you provided was not well-formed: "+err.Error(),
				bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
			return
		}
	}
	s3MetadataTableConfigsMu.Lock()
	s3MetadataTableConfigs[bucket] = s3MetadataTableConfig{
		TableBucketArn: doc.S3TablesDestination.TableBucketArn,
		TableName:      doc.S3TablesDestination.TableName,
	}
	s3MetadataTableConfigsMu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func handleS3GetBucketMetadataTableConfiguration(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3MetadataTableConfigsMu.Lock()
	cfg, ok := s3MetadataTableConfigs[bucket]
	s3MetadataTableConfigsMu.Unlock()
	if !ok {
		sim.S3ErrorXML(w, "MetadataTableConfigNotFoundError",
			"The metadata table configuration does not exist for this bucket.",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	tableArn := cfg.TableBucketArn + "/table/" + cfg.TableName
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<GetBucketMetadataTableConfigurationResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	sb.WriteString(`<MetadataTableConfigurationResult>`)
	sb.WriteString(`<S3TablesDestinationResult>`)
	fmt.Fprintf(&sb, `<TableBucketArn>%s</TableBucketArn>`, xmlEscape(cfg.TableBucketArn))
	fmt.Fprintf(&sb, `<TableName>%s</TableName>`, xmlEscape(cfg.TableName))
	fmt.Fprintf(&sb, `<TableArn>%s</TableArn>`, xmlEscape(tableArn))
	sb.WriteString(`<TableNamespace>aws_s3_metadata</TableNamespace>`)
	sb.WriteString(`</S3TablesDestinationResult>`)
	sb.WriteString(`</MetadataTableConfigurationResult>`)
	sb.WriteString(`<Status>ACTIVE</Status>`)
	sb.WriteString(`</GetBucketMetadataTableConfigurationResult>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, sb.String())
}

func handleS3DeleteBucketMetadataTableConfiguration(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3MetadataTableConfigsMu.Lock()
	delete(s3MetadataTableConfigs, bucket)
	s3MetadataTableConfigsMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// ── ABAC (?abac) ────────────────────────────────────────────────────────

type s3AbacStatusBody struct {
	XMLName xml.Name `xml:"AbacStatus"`
	Status  string   `xml:"Status"`
}

func handleS3PutBucketAbac(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.S3ErrorXML(w, "IncompleteBody", "Failed to read request body: "+err.Error(),
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	var doc s3AbacStatusBody
	if len(body) > 0 {
		if err := xml.Unmarshal(body, &doc); err != nil {
			sim.S3ErrorXML(w, "MalformedXML", "The XML you provided was not well-formed: "+err.Error(),
				bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
			return
		}
	}
	s3AbacMu.Lock()
	s3AbacStatus[bucket] = doc.Status
	s3AbacMu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func handleS3GetBucketAbac(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	s3AbacMu.Lock()
	status := s3AbacStatus[bucket]
	s3AbacMu.Unlock()
	if status == "" {
		// Real S3 reports ABAC as Disabled until it is explicitly enabled.
		status = "Disabled"
	}
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<AbacStatus xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	fmt.Fprintf(&sb, `<Status>%s</Status>`, xmlEscape(status))
	sb.WriteString(`</AbacStatus>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, sb.String())
}

// ── Directory buckets: CreateSession / ListDirectoryBuckets ─────────────

func handleS3CreateSession(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	b, ok := s3Buckets_.Get(bucket)
	if !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	if !b.DirectoryBucket {
		// CreateSession is only valid against S3 Express directory buckets.
		sim.S3ErrorXML(w, "InvalidRequest",
			"S3 Express session credentials can only be created for directory buckets.",
			bucket, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	expiry := time.Now().UTC().Add(5 * time.Minute)
	accessKeyID := "ASIA" + strings.ToUpper(strings.ReplaceAll(generateUUID(), "-", ""))[:16]
	secretKey := strings.ReplaceAll(generateUUID(), "-", "") + strings.ReplaceAll(generateUUID(), "-", "")[:8]
	sessionToken := strings.ReplaceAll(generateUUID(), "-", "") + strings.ReplaceAll(generateUUID(), "-", "")

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<CreateSessionResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	sb.WriteString(`<Credentials>`)
	fmt.Fprintf(&sb, `<AccessKeyId>%s</AccessKeyId>`, accessKeyID)
	fmt.Fprintf(&sb, `<SecretAccessKey>%s</SecretAccessKey>`, secretKey)
	fmt.Fprintf(&sb, `<SessionToken>%s</SessionToken>`, sessionToken)
	fmt.Fprintf(&sb, `<Expiration>%s</Expiration>`, expiry.Format(time.RFC3339))
	sb.WriteString(`</Credentials>`)
	sb.WriteString(`</CreateSessionResult>`)

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("x-amz-server-side-encryption", "AES256")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, sb.String())
}

// s3IsListDirectoryBuckets distinguishes ListDirectoryBuckets from
// ListBuckets — both are GET on the service root `/`. On the wire the
// real S3 clients mark ListDirectoryBuckets two ways: the aws-sdk-go-v2
// adds ?x-id=ListDirectoryBuckets, and both the SDK and the aws CLI sign
// the request under the `s3express` SigV4 signing name (ListBuckets signs
// under `s3`). Either signal selects the directory-bucket listing.
func s3IsListDirectoryBuckets(r *http.Request) bool {
	if r.URL.Query().Get("x-id") == "ListDirectoryBuckets" {
		return true
	}
	// SigV4 credential scope: ".../<date>/<region>/<service>/aws4_request".
	auth := r.Header.Get("Authorization")
	return strings.Contains(auth, "/s3express/aws4_request")
}

func handleS3ListDirectoryBuckets(w http.ResponseWriter, r *http.Request) {
	all := s3Buckets_.List()
	dirs := make([]S3Bucket, 0)
	for _, b := range all {
		if b.DirectoryBucket {
			dirs = append(dirs, b)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<ListAllMyDirectoryBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	sb.WriteString(`<Buckets>`)
	for _, b := range dirs {
		region := b.Region
		if region == "" {
			region = awsRegion()
		}
		sb.WriteString(`<Bucket>`)
		fmt.Fprintf(&sb, `<Name>%s</Name>`, xmlEscape(b.Name))
		fmt.Fprintf(&sb, `<CreationDate>%s</CreationDate>`, xmlEscape(b.CreationDate))
		fmt.Fprintf(&sb, `<BucketRegion>%s</BucketRegion>`, xmlEscape(region))
		fmt.Fprintf(&sb, `<BucketArn>arn:aws:s3express:%s:%s:bucket/%s</BucketArn>`,
			region, awsAccountID(), xmlEscape(b.Name))
		sb.WriteString(`</Bucket>`)
	}
	sb.WriteString(`</Buckets>`)
	sb.WriteString(`</ListAllMyDirectoryBucketsResult>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, sb.String())
}

// ── RenameObject (?renameObject) ────────────────────────────────────────

func handleS3RenameObject(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	destKey := sim.PathParam(r, "key")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	rename := r.Header.Get("x-amz-rename-source")
	if rename == "" {
		sim.S3ErrorXML(w, "InvalidRequest",
			"The x-amz-rename-source header is required for RenameObject.",
			destKey, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	// x-amz-rename-source is URL-encoded "/<bucket>/<key>" (or "<bucket>/<key>").
	srcKey := s3RenameSourceKey(rename)
	if srcKey == "" {
		sim.S3ErrorXML(w, "InvalidArgument",
			"The x-amz-rename-source header value is malformed.",
			destKey, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	srcStoreKey := s3ObjectKey(bucket, srcKey)
	obj, ok := s3Objects.Get(srcStoreKey)
	if !ok {
		sim.S3ErrorXML(w, "NoSuchKey", "The specified key does not exist.",
			srcKey, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
	// Destination conditional headers (optimistic concurrency on the target).
	existing, exists := s3Objects.Get(s3ObjectKey(bucket, destKey))
	if inm := r.Header.Get("If-None-Match"); inm == "*" && exists {
		sim.S3ErrorXML(w, "PreconditionFailed", "At least one of the pre-conditions you specified did not hold",
			destKey, sim.RequestID(r.Context()), http.StatusPreconditionFailed)
		return
	}
	if im := r.Header.Get("If-Match"); im != "" {
		if !exists || strings.Trim(im, `"`) != strings.Trim(existing.ETag, `"`) {
			sim.S3ErrorXML(w, "PreconditionFailed", "At least one of the pre-conditions you specified did not hold",
				destKey, sim.RequestID(r.Context()), http.StatusPreconditionFailed)
			return
		}
	}
	// Move: write under the new key, delete the old.
	obj.Key = s3ObjectKey(bucket, destKey)
	obj.LastModified = time.Now()
	s3Objects.Put(obj.Key, obj)
	s3Objects.Delete(srcStoreKey)

	w.WriteHeader(http.StatusOK)
}

// s3RenameSourceKey extracts the object key from an x-amz-rename-source
// header value ("/bucket/key" or "bucket/key", URL-encoded).
func s3RenameSourceKey(header string) string {
	decoded, err := url.QueryUnescape(header)
	if err != nil {
		decoded = header
	}
	decoded = strings.TrimPrefix(decoded, "/")
	idx := strings.Index(decoded, "/")
	if idx < 0 {
		return ""
	}
	return decoded[idx+1:]
}

// ── UpdateObjectEncryption (object ?encryption) ─────────────────────────

type s3ObjectEncryptionBody struct {
	XMLName xml.Name `xml:"ObjectEncryption"`
	SSEKMS  *struct {
		KMSKeyArn        string `xml:"KMSKeyArn"`
		BucketKeyEnabled *bool  `xml:"BucketKeyEnabled"`
	} `xml:"SSE-KMS"`
	SSES3 *struct{} `xml:"SSE-S3"`
}

func handleS3UpdateObjectEncryption(w http.ResponseWriter, r *http.Request) {
	bucket := sim.PathParam(r, "bucket")
	key := sim.PathParam(r, "key")
	if _, ok := s3Buckets_.Get(bucket); !ok {
		sim.S3ErrorXML(w, "NoSuchBucket", "The specified bucket does not exist",
			bucket, sim.RequestID(r.Context()), http.StatusNotFound)
		return
	}
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
		sim.S3ErrorXML(w, "IncompleteBody", "Failed to read request body: "+err.Error(),
			key, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	var doc s3ObjectEncryptionBody
	if len(body) > 0 {
		if err := xml.Unmarshal(body, &doc); err != nil {
			sim.S3ErrorXML(w, "MalformedXML", "The XML you provided was not well-formed: "+err.Error(),
				key, sim.RequestID(r.Context()), http.StatusBadRequest)
			return
		}
	}
	switch {
	case doc.SSEKMS != nil:
		obj.SSEAlgorithm = "aws:kms"
		obj.SSEKMSKeyID = doc.SSEKMS.KMSKeyArn
	case doc.SSES3 != nil:
		obj.SSEAlgorithm = "AES256"
		obj.SSEKMSKeyID = ""
	default:
		sim.S3ErrorXML(w, "InvalidRequest",
			"The ObjectEncryption body must specify SSE-KMS or SSE-S3.",
			key, sim.RequestID(r.Context()), http.StatusBadRequest)
		return
	}
	s3Objects.Put(storeKey, obj)

	w.Header().Set("x-amz-server-side-encryption", obj.SSEAlgorithm)
	if obj.SSEKMSKeyID != "" {
		w.Header().Set("x-amz-server-side-encryption-aws-kms-key-id", obj.SSEKMSKeyID)
	}
	w.WriteHeader(http.StatusOK)
}
