package aws_cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3_MakeBucketAndList(t *testing.T) {
	runCLI(t, awsCLI("s3", "mb", "s3://cli-test-bucket"))

	out := runCLI(t, awsCLI("s3", "ls"))
	assert.Contains(t, out, "cli-test-bucket")

	// Cleanup
	runCLI(t, awsCLI("s3", "rb", "s3://cli-test-bucket"))
}

func TestS3_CopyUpload(t *testing.T) {
	runCLI(t, awsCLI("s3", "mb", "s3://upload-test-bucket"))

	// Create a local file
	localFile := filepath.Join(tmpDir, "upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("hello from cli test"), 0644))

	runCLI(t, awsCLI("s3", "cp", localFile, "s3://upload-test-bucket/upload.txt"))

	// Verify via listing objects
	out := runCLI(t, awsCLI("s3", "ls", "s3://upload-test-bucket/"))
	assert.Contains(t, out, "upload.txt")

	// Cleanup
	runCLI(t, awsCLI("s3", "rm", "s3://upload-test-bucket/upload.txt"))
	runCLI(t, awsCLI("s3", "rb", "s3://upload-test-bucket"))
}

func TestS3_CopyDownload(t *testing.T) {
	runCLI(t, awsCLI("s3", "mb", "s3://download-test-bucket"))

	content := "download test content"
	localFile := filepath.Join(tmpDir, "to-upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte(content), 0644))

	runCLI(t, awsCLI("s3", "cp", localFile, "s3://download-test-bucket/file.txt"))

	// Download
	downloadFile := filepath.Join(tmpDir, "downloaded.txt")
	runCLI(t, awsCLI("s3", "cp", "s3://download-test-bucket/file.txt", downloadFile))

	data, err := os.ReadFile(downloadFile)
	require.NoError(t, err)
	assert.Equal(t, content, strings.TrimSpace(string(data)))

	// Cleanup
	runCLI(t, awsCLI("s3", "rm", "s3://download-test-bucket/file.txt"))
	runCLI(t, awsCLI("s3", "rb", "s3://download-test-bucket"))
}

func TestS3_RemoveBucket(t *testing.T) {
	runCLI(t, awsCLI("s3", "mb", "s3://remove-test-bucket"))
	runCLI(t, awsCLI("s3", "rb", "s3://remove-test-bucket"))

	// Verify it's gone
	out := runCLI(t, awsCLI("s3", "ls"))
	assert.NotContains(t, out, "remove-test-bucket")
}

func TestS3APIMultipartLists(t *testing.T) {
	bucket := "cli-multipart-list-bucket"
	key := "object.txt"
	runCLI(t, awsCLI("s3api", "create-bucket", "--bucket", bucket))

	createOut := runCLI(t, awsCLI("s3api", "create-multipart-upload", "--bucket", bucket, "--key", key))
	var created struct {
		UploadID string `json:"UploadId"`
	}
	require.NoError(t, json.Unmarshal([]byte(createOut), &created))
	require.NotEmpty(t, created.UploadID)

	partFile := filepath.Join(tmpDir, "multipart-part.txt")
	require.NoError(t, os.WriteFile(partFile, []byte("part-one"), 0644))
	runCLI(t, awsCLI("s3api", "upload-part",
		"--bucket", bucket,
		"--key", key,
		"--upload-id", created.UploadID,
		"--part-number", "1",
		"--body", partFile,
	))

	partsOut := runCLI(t, awsCLI("s3api", "list-parts",
		"--bucket", bucket,
		"--key", key,
		"--upload-id", created.UploadID,
	))
	assert.Contains(t, partsOut, `"PartNumber": 1`)

	uploadsOut := runCLI(t, awsCLI("s3api", "list-multipart-uploads", "--bucket", bucket))
	assert.Contains(t, uploadsOut, created.UploadID)
	assert.Contains(t, uploadsOut, key)

	runCLI(t, awsCLI("s3api", "abort-multipart-upload",
		"--bucket", bucket,
		"--key", key,
		"--upload-id", created.UploadID,
	))
	runCLI(t, awsCLI("s3", "rb", "s3://"+bucket))
}
