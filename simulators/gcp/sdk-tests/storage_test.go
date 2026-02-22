package gcp_sdk_test

import (
	"io"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storageClient(t *testing.T) *storage.Client {
	t.Helper()
	// Use STORAGE_EMULATOR_HOST for proper download URL construction
	host := strings.TrimPrefix(baseURL, "http://")
	t.Setenv("STORAGE_EMULATOR_HOST", host)
	client, err := storage.NewClient(ctx)
	require.NoError(t, err)
	return client
}

func TestGCS_CreateBucket(t *testing.T) {
	client := storageClient(t)
	defer client.Close()

	err := client.Bucket("sdk-test-bucket").Create(ctx, "test-project", nil)
	require.NoError(t, err)

	attrs, err := client.Bucket("sdk-test-bucket").Attrs(ctx)
	require.NoError(t, err)
	assert.Equal(t, "sdk-test-bucket", attrs.Name)
}

func TestGCS_UploadAndDownload(t *testing.T) {
	client := storageClient(t)
	defer client.Close()

	err := client.Bucket("upload-bucket").Create(ctx, "test-project", nil)
	require.NoError(t, err)

	// Upload
	w := client.Bucket("upload-bucket").Object("hello.txt").NewWriter(ctx)
	_, err = w.Write([]byte("hello world"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Download
	r, err := client.Bucket("upload-bucket").Object("hello.txt").NewReader(ctx)
	require.NoError(t, err)
	defer r.Close()

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestGCS_ListObjects(t *testing.T) {
	client := storageClient(t)
	defer client.Close()

	err := client.Bucket("list-obj-bucket").Create(ctx, "test-project", nil)
	require.NoError(t, err)

	for _, name := range []string{"a.txt", "b.txt"} {
		w := client.Bucket("list-obj-bucket").Object(name).NewWriter(ctx)
		w.Write([]byte("data"))
		w.Close()
	}

	var names []string
	it := client.Bucket("list-obj-bucket").Objects(ctx, nil)
	for {
		attrs, err := it.Next()
		if err != nil {
			break
		}
		names = append(names, attrs.Name)
	}
	assert.GreaterOrEqual(t, len(names), 2)
}

func TestGCS_DeleteObject(t *testing.T) {
	client := storageClient(t)
	defer client.Close()

	err := client.Bucket("del-obj-bucket").Create(ctx, "test-project", nil)
	require.NoError(t, err)

	w := client.Bucket("del-obj-bucket").Object("temp.txt").NewWriter(ctx)
	w.Write([]byte("temp"))
	w.Close()

	err = client.Bucket("del-obj-bucket").Object("temp.txt").Delete(ctx)
	require.NoError(t, err)
}
