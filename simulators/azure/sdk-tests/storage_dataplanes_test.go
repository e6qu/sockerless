package azure_sdk_test

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storageDataplaneReq points at one of the four storage data-plane
// subdomains (`<account>.{blob,file,queue,table}.<host>`). Tests use
// raw HTTP rather than the Azure Storage SDK because the SDK's
// strict Shared Key signature verifier rejects sim auth-passthrough
// config; raw HTTP exercises the same wire shape.
func storageDataplaneReq(t *testing.T, method, account, service, path string, body []byte, headers map[string]string) *http.Response {
	t.Helper()
	var br io.Reader
	if body != nil {
		br = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, baseURL+path, br)
	require.NoError(t, err)
	req.Host = account + "." + service + ".localhost"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestFilesDataPlane exercises Files data-plane share + file CRUD.
// Regression guard for BUG-1109 (Azure storage data planes that
// weren't serviced — the ARM response had advertised these URLs
// since Phase 173.10 fixed Blob; this commit fixes the remaining 3).
func TestFilesDataPlane(t *testing.T) {
	account := "testacct"
	share := "myshare"

	resp := storageDataplaneReq(t, "PUT", account, "file", "/"+share+"?restype=share", nil, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "PUT share")
	resp.Body.Close()

	payload := []byte("hello files")
	resp = storageDataplaneReq(t, "PUT", account, "file", "/"+share+"/myfile.txt", payload, map[string]string{
		"Content-Type": "text/plain",
		"x-ms-type":    "file",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "PUT file")
	resp.Body.Close()

	resp = storageDataplaneReq(t, "HEAD", account, "file", "/"+share+"/myfile.txt", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, fmt.Sprintf("%d", len(payload)), resp.Header.Get("Content-Length"))
	resp.Body.Close()

	resp = storageDataplaneReq(t, "GET", account, "file", "/"+share+"/myfile.txt", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, payload, got, "GET file payload round-trip")

	// List files in share.
	resp = storageDataplaneReq(t, "GET", account, "file", "/"+share+"?restype=directory&comp=list", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	listBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(listBody), "myfile.txt")

	// Cleanup.
	resp = storageDataplaneReq(t, "DELETE", account, "file", "/"+share+"/myfile.txt", nil, nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()
	resp = storageDataplaneReq(t, "DELETE", account, "file", "/"+share+"?restype=share", nil, nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()
}

// TestQueuesDataPlane exercises queue + message enqueue/dequeue/ack.
func TestQueuesDataPlane(t *testing.T) {
	account := "testacct"
	queue := "myqueue"

	resp := storageDataplaneReq(t, "PUT", account, "queue", "/"+queue, nil, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "PUT queue")
	resp.Body.Close()

	// Enqueue a message — body is XML <QueueMessage><MessageText>...</MessageText></QueueMessage>.
	xmlBody := []byte(`<QueueMessage><MessageText>aGVsbG8=</MessageText></QueueMessage>`)
	resp = storageDataplaneReq(t, "POST", account, "queue", "/"+queue+"/messages", xmlBody, map[string]string{
		"Content-Type": "application/xml",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "POST message")
	rb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	type qmsgResp struct {
		XMLName  xml.Name `xml:"QueueMessagesList"`
		Messages []struct {
			MessageID string `xml:"MessageId"`
		} `xml:"QueueMessage"`
	}
	var msgs qmsgResp
	require.NoError(t, xml.Unmarshal(rb, &msgs))
	require.Len(t, msgs.Messages, 1)
	assert.NotEmpty(t, msgs.Messages[0].MessageID)

	// Peek without dequeue.
	resp = storageDataplaneReq(t, "GET", account, "queue", "/"+queue+"/messages?peekonly=true", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	peekBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(peekBody), "aGVsbG8=")

	// Dequeue (Get Messages).
	resp = storageDataplaneReq(t, "GET", account, "queue", "/"+queue+"/messages?visibilitytimeout=30", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	deqBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var deq qmsgResp
	type fullMsg struct {
		MessageID  string `xml:"MessageId"`
		PopReceipt string `xml:"PopReceipt"`
	}
	type fullList struct {
		XMLName  xml.Name  `xml:"QueueMessagesList"`
		Messages []fullMsg `xml:"QueueMessage"`
	}
	var full fullList
	require.NoError(t, xml.Unmarshal(deqBody, &full))
	require.Len(t, full.Messages, 1)
	assert.NotEmpty(t, full.Messages[0].PopReceipt)
	_ = deq

	// Delete via popreceipt.
	resp = storageDataplaneReq(t, "DELETE", account, "queue",
		fmt.Sprintf("/%s/messages/%s?popreceipt=%s", queue, full.Messages[0].MessageID, full.Messages[0].PopReceipt),
		nil, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Cleanup queue.
	resp = storageDataplaneReq(t, "DELETE", account, "queue", "/"+queue, nil, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()
}

// TestTablesDataPlane exercises Tables + entity CRUD.
func TestTablesDataPlane(t *testing.T) {
	account := "testacct"
	table := "MyTable"

	// Create table.
	resp := storageDataplaneReq(t, "POST", account, "table", "/Tables",
		[]byte(`{"TableName":"`+table+`"}`),
		map[string]string{"Content-Type": "application/json"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Insert entity.
	resp = storageDataplaneReq(t, "POST", account, "table", "/"+table,
		[]byte(`{"PartitionKey":"p1","RowKey":"r1","Foo":"bar","Num":42}`),
		map[string]string{"Content-Type": "application/json"})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "POST entity (no-content prefer)")
	resp.Body.Close()

	// Get entity.
	resp = storageDataplaneReq(t, "GET", account, "table",
		"/"+table+"(PartitionKey='p1',RowKey='r1')", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	getBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(getBody), `"Foo":"bar"`)
	assert.Contains(t, string(getBody), `"Num":42`)

	// Query (full-table scan).
	resp = storageDataplaneReq(t, "GET", account, "table", "/"+table, nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	queryBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(queryBody), `"value"`)
	assert.Contains(t, string(queryBody), `"Foo":"bar"`)

	// Delete entity.
	resp = storageDataplaneReq(t, "DELETE", account, "table",
		"/"+table+"(PartitionKey='p1',RowKey='r1')", nil, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Delete table.
	resp = storageDataplaneReq(t, "DELETE", account, "table", "/Tables('"+table+"')", nil, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Final get should 404.
	resp = storageDataplaneReq(t, "GET", account, "table", "/Tables('"+table+"')", nil, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.True(t,
		strings.Contains(string(body), "ResourceNotFound") || strings.Contains(string(body), "does not exist"),
		"expected ResourceNotFound; got %s", string(body))
}
