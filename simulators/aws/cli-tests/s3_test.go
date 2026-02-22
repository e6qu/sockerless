package aws_cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3_MakeBucketAndList(t *testing.T) {
	runCLI(t, awsS3CLI("s3", "mb", "s3://cli-test-bucket"))

	out := runCLI(t, awsS3CLI("s3", "ls"))
	assert.Contains(t, out, "cli-test-bucket")

	// Cleanup
	runCLI(t, awsS3CLI("s3", "rb", "s3://cli-test-bucket"))
}

func TestS3_CopyUpload(t *testing.T) {
	runCLI(t, awsS3CLI("s3", "mb", "s3://upload-test-bucket"))

	// Create a local file
	localFile := filepath.Join(tmpDir, "upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("hello from cli test"), 0644))

	runCLI(t, awsS3CLI("s3", "cp", localFile, "s3://upload-test-bucket/upload.txt"))

	// Verify via listing objects
	out := runCLI(t, awsS3CLI("s3", "ls", "s3://upload-test-bucket/"))
	assert.Contains(t, out, "upload.txt")

	// Cleanup
	runCLI(t, awsS3CLI("s3", "rm", "s3://upload-test-bucket/upload.txt"))
	runCLI(t, awsS3CLI("s3", "rb", "s3://upload-test-bucket"))
}

func TestS3_CopyDownload(t *testing.T) {
	runCLI(t, awsS3CLI("s3", "mb", "s3://download-test-bucket"))

	content := "download test content"
	localFile := filepath.Join(tmpDir, "to-upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte(content), 0644))

	runCLI(t, awsS3CLI("s3", "cp", localFile, "s3://download-test-bucket/file.txt"))

	// Download
	downloadFile := filepath.Join(tmpDir, "downloaded.txt")
	runCLI(t, awsS3CLI("s3", "cp", "s3://download-test-bucket/file.txt", downloadFile))

	data, err := os.ReadFile(downloadFile)
	require.NoError(t, err)
	assert.Equal(t, content, strings.TrimSpace(string(data)))

	// Cleanup
	runCLI(t, awsS3CLI("s3", "rm", "s3://download-test-bucket/file.txt"))
	runCLI(t, awsS3CLI("s3", "rb", "s3://download-test-bucket"))
}

func TestS3_RemoveBucket(t *testing.T) {
	runCLI(t, awsS3CLI("s3", "mb", "s3://remove-test-bucket"))
	runCLI(t, awsS3CLI("s3", "rb", "s3://remove-test-bucket"))

	// Verify it's gone
	out := runCLI(t, awsS3CLI("s3", "ls"))
	assert.NotContains(t, out, "remove-test-bucket")
}
