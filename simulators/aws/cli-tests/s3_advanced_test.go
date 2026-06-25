package aws_cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3CLI_BucketMetadataTableConfiguration round-trips the legacy V1
// S3 Metadata table configuration (?metadataTable) via the aws CLI.
//
// The V2 ?metadataConfiguration ops, ?abac, ?renameObject, object
// ?encryption, and the inventory/journal table-config updates are not
// present in the local aws CLI 2.26.6 — those ops are covered by the
// SDK round-trips in sdk-tests/s3_advanced_test.go (which exercise the
// same simulator handlers and the conformance hook).
func TestS3CLI_BucketMetadataTableConfiguration(t *testing.T) {
	bucket := "cli-metadata-table-bucket"
	_ = awsCLI("s3api", "create-bucket", "--bucket", bucket).Run()
	t.Cleanup(func() {
		_ = awsCLI("s3api", "delete-bucket-metadata-table-configuration", "--bucket", bucket).Run()
		_ = awsCLI("s3api", "delete-bucket", "--bucket", bucket).Run()
	})

	cfg := `{"S3TablesDestination":{"TableBucketArn":"arn:aws:s3tables:us-east-1:000000000000:bucket/cli-table-bucket","TableName":"cli-metadata-table"}}`
	runCLI(t, awsCLI("s3api", "create-bucket-metadata-table-configuration",
		"--bucket", bucket, "--metadata-table-configuration", cfg))

	out := runCLI(t, awsCLI("s3api", "get-bucket-metadata-table-configuration", "--bucket", bucket))
	var resp struct {
		GetBucketMetadataTableConfigurationResult struct {
			MetadataTableConfigurationResult struct {
				S3TablesDestinationResult struct {
					TableBucketArn string `json:"TableBucketArn"`
					TableName      string `json:"TableName"`
					TableNamespace string `json:"TableNamespace"`
				} `json:"S3TablesDestinationResult"`
			} `json:"MetadataTableConfigurationResult"`
			Status string `json:"Status"`
		} `json:"GetBucketMetadataTableConfigurationResult"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &resp))
	dest := resp.GetBucketMetadataTableConfigurationResult.MetadataTableConfigurationResult.S3TablesDestinationResult
	assert.Equal(t, "cli-metadata-table", dest.TableName)
	assert.Equal(t, "aws_s3_metadata", dest.TableNamespace)

	runCLI(t, awsCLI("s3api", "delete-bucket-metadata-table-configuration", "--bucket", bucket))
}

// TestS3CLI_ListDirectoryBuckets exercises ListDirectoryBuckets via the
// aws CLI. It is a service-level GET on `/` (no bucket subdomain), so it
// routes faithfully against the path-style sim endpoint.
//
// CreateSession and the directory-bucket variant of CreateBucket are NOT
// exercised over the CLI: botocore forces S3 Express virtual-host
// addressing (`{bucket}.s3express-{zone}.{region}.amazonaws.com`) and an
// auto CreateSession pre-fetch for any `*--x-s3` bucket name, neither of
// which a path-style custom endpoint resolves (and `*.localhost`
// subdomains don't resolve on macOS). Both ops are covered faithfully by
// the SDK round-trip in sdk-tests/s3_advanced_test.go (which drives the
// same handlers and the conformance hook) using path-style + the
// DisableS3ExpressSessionAuth canonical posture.
func TestS3CLI_ListDirectoryBuckets(t *testing.T) {
	out := runCLI(t, awsCLI("s3api", "list-directory-buckets"))
	var listResp struct {
		Buckets []struct {
			Name string `json:"Name"`
		} `json:"Buckets"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &listResp))
	// A general-purpose bucket must never appear in the directory listing.
	_ = awsCLI("s3api", "create-bucket", "--bucket", "cli-plain-gp-bucket").Run()
	t.Cleanup(func() { _ = awsCLI("s3api", "delete-bucket", "--bucket", "cli-plain-gp-bucket").Run() })
	out2 := runCLI(t, awsCLI("s3api", "list-directory-buckets"))
	require.NoError(t, json.Unmarshal([]byte(out2), &listResp))
	for _, b := range listResp.Buckets {
		assert.NotEqual(t, "cli-plain-gp-bucket", b.Name)
	}
}
