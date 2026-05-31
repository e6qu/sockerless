package azure_sdk_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/streaming"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/aztables"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azfile/file"
	fileservice "github.com/Azure/azure-sdk-for-go/sdk/storage/azfile/service"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type storageSDKTransport struct {
	inner *http.Client
}

func (t storageSDKTransport) Do(req *http.Request) (*http.Response, error) {
	rewritten := req.Clone(req.Context())
	u := *req.URL
	rewritten.Host = req.URL.Host
	u.Host = strings.TrimPrefix(baseURL, "http://")
	u.Scheme = "http"
	rewritten.URL = &u
	client := t.inner
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(rewritten)
}

func storageSDKURL(t *testing.T, account, service string) string {
	t.Helper()
	hostPort := strings.TrimPrefix(baseURL, "http://")
	_, port, ok := strings.Cut(hostPort, ":")
	require.True(t, ok, "baseURL must include a port: %s", baseURL)
	return "http://" + account + "." + service + ".localhost:" + port + "/"
}

func storageSDKOptions() azcore.ClientOptions {
	return azcore.ClientOptions{Transport: storageSDKTransport{}}
}

func storageAdvertisedEndpoint(t *testing.T, account, service string) string {
	t.Helper()
	hostPort := strings.TrimPrefix(baseURL, "http://")
	_, port, ok := strings.Cut(hostPort, ":")
	require.True(t, ok, "baseURL must include a port: %s", baseURL)
	return fmt.Sprintf("http://%s.%s.shim.localhost:%s/", account, service, port)
}

func storageRawRequest(t *testing.T, method, account, service, target string) *http.Response {
	t.Helper()
	u, err := url.Parse(baseURL)
	require.NoError(t, err)
	_, port, ok := strings.Cut(u.Host, ":")
	require.True(t, ok, "baseURL must include a port: %s", baseURL)
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+target, nil)
	require.NoError(t, err)
	req.Host = fmt.Sprintf("%s.%s.localhost:%s", account, service, port)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestStorageSDK_BlobLifecycleAndPagedLists(t *testing.T) {
	account := "sdkblobacct"
	container := "sdk-blob-container"
	blobName := "one.txt"
	payload := []byte("hello from azblob")

	client, err := azblob.NewClientWithNoCredential(storageSDKURL(t, account, "blob"),
		&azblob.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	_, err = client.CreateContainer(ctx, container, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = client.DeleteContainer(ctx, container, nil) })

	_, err = client.UploadBuffer(ctx, container, blobName, payload, nil)
	require.NoError(t, err)

	download, err := client.DownloadStream(ctx, container, blobName, nil)
	require.NoError(t, err)
	got, err := io.ReadAll(download.Body)
	require.NoError(t, err)
	require.NoError(t, download.Body.Close())
	assert.Equal(t, payload, got)

	blobPager := client.NewListBlobsFlatPager(container, &azblob.ListBlobsFlatOptions{MaxResults: to.Ptr(int32(1))})
	blobPage, err := blobPager.NextPage(ctx)
	require.NoError(t, err)
	require.NotNil(t, blobPage.Segment)
	require.Len(t, blobPage.Segment.BlobItems, 1)
	require.NotNil(t, blobPage.Segment.BlobItems[0].Name)
	assert.Equal(t, blobName, *blobPage.Segment.BlobItems[0].Name)

	rawBlobList := storageRawRequest(t, http.MethodGet, account, "blob", "/"+container+"?restype=container&comp=list&maxresults=1")
	require.Equal(t, http.StatusOK, rawBlobList.StatusCode)
	rawBlobListBody, _ := io.ReadAll(rawBlobList.Body)
	rawBlobList.Body.Close()
	assert.True(t, strings.HasPrefix(string(rawBlobListBody), "<?xml"), string(rawBlobListBody))
	assert.Contains(t, string(rawBlobListBody), `ServiceEndpoint="`+storageAdvertisedEndpoint(t, account, "blob")+`"`)
	assert.Contains(t, string(rawBlobListBody), `ContainerName="`+container+`"`)
	assert.Contains(t, string(rawBlobListBody), "<NextMarker>")

	containerPager := client.NewListContainersPager(&azblob.ListContainersOptions{MaxResults: to.Ptr(int32(1))})
	containerPage, err := containerPager.NextPage(ctx)
	require.NoError(t, err)
	require.Len(t, containerPage.ContainerItems, 1)
	require.NotNil(t, containerPage.ContainerItems[0].Name)
	assert.Equal(t, container, *containerPage.ContainerItems[0].Name)

	rawContainerList := storageRawRequest(t, http.MethodGet, account, "blob", "/?comp=list&maxresults=1")
	require.Equal(t, http.StatusOK, rawContainerList.StatusCode)
	rawContainerListBody, _ := io.ReadAll(rawContainerList.Body)
	rawContainerList.Body.Close()
	assert.True(t, strings.HasPrefix(string(rawContainerListBody), "<?xml"), string(rawContainerListBody))
	assert.Contains(t, string(rawContainerListBody), `ServiceEndpoint="`+storageAdvertisedEndpoint(t, account, "blob")+`"`)
	assert.Contains(t, string(rawContainerListBody), "<NextMarker>")

	_, err = client.DeleteBlob(ctx, container, blobName, nil)
	require.NoError(t, err)
}

func TestStorageDataPlane_XMLErrors(t *testing.T) {
	account := "sdkstorageerrors"

	blobResp := storageRawRequest(t, http.MethodGet, account, "blob", "/missing-container/missing-blob.txt")
	assertStorageXMLError(t, blobResp, http.StatusNotFound, "BlobNotFound")

	fileResp := storageRawRequest(t, http.MethodGet, account, "file", "/missing-share?restype=share")
	assertStorageXMLError(t, fileResp, http.StatusNotFound, "ShareNotFound")

	queueResp := storageRawRequest(t, http.MethodGet, account, "queue", "/missingqueue/messages")
	assertStorageXMLError(t, queueResp, http.StatusNotFound, "QueueNotFound")
}

func assertStorageXMLError(t *testing.T, resp *http.Response, status int, code string) {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, status, resp.StatusCode, string(body))
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/xml")
	assert.Equal(t, code, resp.Header.Get("x-ms-error-code"))
	assert.True(t, strings.HasPrefix(string(body), "<?xml"), string(body))
	assert.Contains(t, string(body), "<Error>")
	assert.Contains(t, string(body), "<Code>"+code+"</Code>")
}

func TestStorageSDK_BlobStartCopyFromURL(t *testing.T) {
	account := "sdkcopyblobacct"
	container := "sdk-copy-container"
	srcName := "nested/source blob.txt"
	dstName := "copied/dest blob.txt"
	payload := []byte("copied through Azure Blob Copy Blob")
	contentType := "text/plain"

	client, err := azblob.NewClientWithNoCredential(storageSDKURL(t, account, "blob"),
		&azblob.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	_, err = client.CreateContainer(ctx, container, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = client.DeleteContainer(ctx, container, nil) })

	_, err = client.UploadBuffer(ctx, container, srcName, payload, &azblob.UploadBufferOptions{
		HTTPHeaders: &blob.HTTPHeaders{BlobContentType: &contentType},
		Metadata: map[string]*string{
			"origin": to.Ptr("source"),
		},
	})
	require.NoError(t, err)

	base := storageSDKURL(t, account, "blob")
	srcURL := base + container + "/" + url.PathEscape(srcName)
	dstClient, err := blob.NewClientWithNoCredential(base+container+"/"+url.PathEscape(dstName),
		&blob.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	copyResp, err := dstClient.StartCopyFromURL(ctx, srcURL, &blob.StartCopyFromURLOptions{
		Metadata: map[string]*string{
			"origin": to.Ptr("dest"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, copyResp.CopyID)
	require.NotNil(t, copyResp.CopyStatus)
	assert.Equal(t, "success", string(*copyResp.CopyStatus))

	download, err := client.DownloadStream(ctx, container, dstName, nil)
	require.NoError(t, err)
	got, err := io.ReadAll(download.Body)
	require.NoError(t, err)
	require.NoError(t, download.Body.Close())
	assert.Equal(t, payload, got)

	props, err := dstClient.GetProperties(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, props.CopyStatus)
	assert.Equal(t, "success", string(*props.CopyStatus))
	require.NotNil(t, props.Metadata["Origin"])
	assert.Equal(t, "dest", *props.Metadata["Origin"])
	require.NotNil(t, props.ContentType)
	assert.Equal(t, contentType, *props.ContentType)

	pathStyleSrc := strings.TrimRight(baseURL, "/") + "/" + account + "/" + container + "/" + url.PathEscape(srcName)
	inheritName := "copied/inherit metadata.txt"
	inheritClient, err := blob.NewClientWithNoCredential(base+container+"/"+url.PathEscape(inheritName),
		&blob.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	_, err = inheritClient.StartCopyFromURL(ctx, pathStyleSrc, nil)
	require.NoError(t, err)

	inheritProps, err := inheritClient.GetProperties(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, inheritProps.Metadata["Origin"])
	assert.Equal(t, "source", *inheritProps.Metadata["Origin"])

	missingClient, err := blob.NewClientWithNoCredential(base+container+"/"+url.PathEscape("copied/missing.txt"),
		&blob.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)
	_, err = missingClient.StartCopyFromURL(ctx, base+container+"/"+url.PathEscape("missing-source.txt"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CannotVerifyCopySource")
}

func TestStorageSDK_BlobBlockStaging(t *testing.T) {
	account := "sdkblockblobacct"
	container := "sdk-block-container"
	blobName := "staged.txt"
	payloadA := []byte("hello ")
	payloadB := []byte("from blocks")

	client, err := azblob.NewClientWithNoCredential(storageSDKURL(t, account, "blob"),
		&azblob.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	_, err = client.CreateContainer(ctx, container, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = client.DeleteContainer(ctx, container, nil) })

	blobClient, err := blockblob.NewClientWithNoCredential(
		storageSDKURL(t, account, "blob")+container+"/"+blobName,
		&blockblob.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	blockA := base64.StdEncoding.EncodeToString([]byte("block-000001"))
	blockB := base64.StdEncoding.EncodeToString([]byte("block-000002"))
	_, err = blobClient.StageBlock(ctx, blockA, streaming.NopCloser(bytes.NewReader(payloadA)), nil)
	require.NoError(t, err)
	_, err = blobClient.StageBlock(ctx, blockB, streaming.NopCloser(bytes.NewReader(payloadB)), nil)
	require.NoError(t, err)

	uncommitted, err := blobClient.GetBlockList(ctx, blockblob.BlockListTypeUncommitted, nil)
	require.NoError(t, err)
	require.Len(t, uncommitted.UncommittedBlocks, 2)
	require.NotNil(t, uncommitted.UncommittedBlocks[0].Name)
	require.NotNil(t, uncommitted.UncommittedBlocks[1].Name)
	assert.ElementsMatch(t,
		[]string{blockA, blockB},
		[]string{*uncommitted.UncommittedBlocks[0].Name, *uncommitted.UncommittedBlocks[1].Name})

	_, err = blobClient.CommitBlockList(ctx, []string{blockA, blockB}, nil)
	require.NoError(t, err)

	allBlocks, err := blobClient.GetBlockList(ctx, blockblob.BlockListTypeAll, nil)
	require.NoError(t, err)
	require.Len(t, allBlocks.CommittedBlocks, 2)
	require.Empty(t, allBlocks.UncommittedBlocks)
	require.NotNil(t, allBlocks.CommittedBlocks[0].Name)
	require.NotNil(t, allBlocks.CommittedBlocks[1].Name)
	assert.Equal(t, blockA, *allBlocks.CommittedBlocks[0].Name)
	assert.Equal(t, blockB, *allBlocks.CommittedBlocks[1].Name)

	download, err := client.DownloadStream(ctx, container, blobName, nil)
	require.NoError(t, err)
	got, err := io.ReadAll(download.Body)
	require.NoError(t, err)
	require.NoError(t, download.Body.Close())
	assert.Equal(t, append(payloadA, payloadB...), got)
}

func TestStorageSDK_FileLifecycleAndPagedLists(t *testing.T) {
	account := "sdkfileacct"
	share := "sdk-file-share"
	fileName := "one.txt"
	payload := []byte("hello from azfile")

	serviceClient, err := fileservice.NewClientWithNoCredential(storageSDKURL(t, account, "file"),
		&fileservice.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	_, err = serviceClient.CreateShare(ctx, share, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = serviceClient.DeleteShare(ctx, share, nil) })

	fileClient, err := file.NewClientWithNoCredential(storageSDKURL(t, account, "file")+share+"/"+fileName,
		&file.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	_, err = fileClient.Create(ctx, int64(len(payload)), nil)
	require.NoError(t, err)
	require.NoError(t, fileClient.UploadBuffer(ctx, payload, nil))

	buf := make([]byte, len(payload))
	n, err := fileClient.DownloadBuffer(ctx, buf, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(len(payload)), n)
	assert.Equal(t, payload, buf)

	sharePager := serviceClient.NewListSharesPager(&fileservice.ListSharesOptions{MaxResults: to.Ptr(int32(1))})
	sharePage, err := sharePager.NextPage(ctx)
	require.NoError(t, err)
	require.Len(t, sharePage.Shares, 1)
	require.NotNil(t, sharePage.Shares[0].Name)
	assert.Equal(t, share, *sharePage.Shares[0].Name)

	rootPager := serviceClient.NewShareClient(share).NewRootDirectoryClient().NewListFilesAndDirectoriesPager(nil)
	rootPage, err := rootPager.NextPage(ctx)
	require.NoError(t, err)
	require.NotNil(t, rootPage.Segment)
	require.Len(t, rootPage.Segment.Files, 1)
	require.NotNil(t, rootPage.Segment.Files[0].Name)
	assert.Equal(t, fileName, *rootPage.Segment.Files[0].Name)

	_, err = fileClient.Delete(ctx, nil)
	require.NoError(t, err)
}

func TestStorageSDK_QueueLifecycleAndPagedLists(t *testing.T) {
	account := "sdkqueueacct"
	queueName := "sdkqueue"
	message := "hello from azqueue"

	serviceClient, err := azqueue.NewServiceClientWithNoCredential(storageSDKURL(t, account, "queue"),
		&azqueue.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	_, err = serviceClient.CreateQueue(ctx, queueName, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = serviceClient.DeleteQueue(ctx, queueName, nil) })

	queueClient := serviceClient.NewQueueClient(queueName)
	_, err = queueClient.EnqueueMessage(ctx, message, nil)
	require.NoError(t, err)

	propsResp := storageRawRequest(t, http.MethodGet, account, "queue", "/?restype=service&comp=properties")
	require.Equal(t, http.StatusOK, propsResp.StatusCode)
	propsBody, err := io.ReadAll(propsResp.Body)
	require.NoError(t, err)
	propsResp.Body.Close()
	assert.Contains(t, propsResp.Header.Get("Content-Type"), "application/xml")
	assert.True(t, strings.HasPrefix(string(propsBody), "<?xml"), string(propsBody))
	assert.Contains(t, string(propsBody), "<StorageServiceProperties>")
	assert.Contains(t, string(propsBody), "<HourMetrics>")

	dequeued, err := queueClient.DequeueMessage(ctx, nil)
	require.NoError(t, err)
	require.Len(t, dequeued.Messages, 1)
	require.NotNil(t, dequeued.Messages[0].MessageText)
	assert.Equal(t, message, *dequeued.Messages[0].MessageText)
	require.NotNil(t, dequeued.Messages[0].MessageID)
	require.NotNil(t, dequeued.Messages[0].PopReceipt)

	_, err = queueClient.DeleteMessage(ctx, *dequeued.Messages[0].MessageID, *dequeued.Messages[0].PopReceipt, nil)
	require.NoError(t, err)

	pager := serviceClient.NewListQueuesPager(&azqueue.ListQueuesOptions{MaxResults: to.Ptr(int32(1))})
	page, err := pager.NextPage(ctx)
	require.NoError(t, err)
	require.Len(t, page.Queues, 1)
	require.NotNil(t, page.Queues[0].Name)
	assert.Equal(t, queueName, *page.Queues[0].Name)
}

func TestStorageSDK_TableLifecycleAndPagedLists(t *testing.T) {
	account := "sdktableacct"
	tableName := "SdkTable"

	serviceClient, err := aztables.NewServiceClientWithNoCredential(storageSDKURL(t, account, "table"),
		&aztables.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)

	_, err = serviceClient.CreateTable(ctx, tableName, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = serviceClient.DeleteTable(ctx, tableName, nil) })

	tableClient := serviceClient.NewClient(tableName)
	entity := map[string]any{
		"PartitionKey": "p1",
		"RowKey":       "r1",
		"Value":        "hello from aztables",
	}
	body, err := json.Marshal(entity)
	require.NoError(t, err)
	_, err = tableClient.AddEntity(ctx, body, nil)
	require.NoError(t, err)

	got, err := tableClient.GetEntity(ctx, "p1", "r1", nil)
	require.NoError(t, err)
	assert.Contains(t, string(got.Value), "hello from aztables")

	entityPager := tableClient.NewListEntitiesPager(&aztables.ListEntitiesOptions{Top: to.Ptr(int32(1))})
	entityPage, err := entityPager.NextPage(ctx)
	require.NoError(t, err)
	require.Len(t, entityPage.Entities, 1)
	assert.Contains(t, string(entityPage.Entities[0]), "hello from aztables")

	tablePager := serviceClient.NewListTablesPager(&aztables.ListTablesOptions{Top: to.Ptr(int32(1))})
	tablePage, err := tablePager.NextPage(ctx)
	require.NoError(t, err)
	require.Len(t, tablePage.Tables, 1)
	require.NotNil(t, tablePage.Tables[0].Name)
	assert.Equal(t, tableName, *tablePage.Tables[0].Name)

	_, err = tableClient.DeleteEntity(ctx, "p1", "r1", nil)
	require.NoError(t, err)
}
