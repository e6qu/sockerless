package gcp_sdk_test

import (
	"io"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
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

	for _, name := range []string{"b.txt", "a.txt", "c.txt"} {
		w := client.Bucket("list-obj-bucket").Object(name).NewWriter(ctx)
		_, err := w.Write([]byte("data"))
		require.NoError(t, err)
		require.NoError(t, w.Close())
	}

	var names []string
	it := client.Bucket("list-obj-bucket").Objects(ctx, nil)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		require.NoError(t, err)
		names = append(names, attrs.Name)
	}
	assert.Equal(t, []string{"a.txt", "b.txt", "c.txt"}, names)
}

func TestGCS_CopierFromRewriteTo(t *testing.T) {
	client := storageClient(t)
	defer client.Close()

	bucket := client.Bucket("copy-obj-bucket")
	err := bucket.Create(ctx, "test-project", nil)
	require.NoError(t, err)

	src := bucket.Object("dir/source file.txt")
	w := src.NewWriter(ctx)
	w.ContentType = "text/plain"
	_, err = w.Write([]byte("copied through rewriteTo"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	dst := bucket.Object("copied/dest file.txt")
	attrs, err := dst.CopierFrom(src).Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, "copied/dest file.txt", attrs.Name)
	assert.Equal(t, int64(len("copied through rewriteTo")), attrs.Size)
	assert.Equal(t, "text/plain", attrs.ContentType)

	r, err := dst.NewReader(ctx)
	require.NoError(t, err)
	defer r.Close()
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "copied through rewriteTo", string(got))
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
