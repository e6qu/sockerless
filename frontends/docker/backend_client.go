package frontend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sockerless/api"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// BackendClient makes HTTP calls to the backend's internal API.
type BackendClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewBackendClient creates a new backend client.
func NewBackendClient(baseURL string) *BackendClient {
	return &BackendClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 0, // no timeout for long-poll (wait, attach)
			Transport: otelhttp.NewTransport(&http.Transport{
				MaxIdleConns:       100,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: true,
			}),
		},
	}
}

func (c *BackendClient) url(path string) string {
	return c.baseURL + "/internal/v1" + path
}

func (c *BackendClient) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url(path), nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

func (c *BackendClient) post(ctx context.Context, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.url(path), r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

func (c *BackendClient) postRaw(ctx context.Context, path string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.url(path), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.httpClient.Do(req)
}

func (c *BackendClient) postRawWithQuery(ctx context.Context, path string, query url.Values, contentType string, body io.Reader) (*http.Response, error) {
	u := c.url(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.httpClient.Do(req)
}

func (c *BackendClient) delete(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.url(path), nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

func (c *BackendClient) deleteWithQuery(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u := c.url(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "DELETE", u, nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

func (c *BackendClient) getWithQuery(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u := c.url(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

func (c *BackendClient) putWithQuery(ctx context.Context, path string, query url.Values, body io.Reader) (*http.Response, error) {
	u := c.url(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	return c.httpClient.Do(req)
}

func (c *BackendClient) headWithQuery(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u := c.url(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "HEAD", u, nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

func (c *BackendClient) postWithQuery(ctx context.Context, path string, query url.Values, body any) (*http.Response, error) {
	u := c.url(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", u, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

// Info returns backend system information.
func (c *BackendClient) Info(ctx context.Context) (*api.BackendInfo, error) {
	resp, err := c.get(ctx, "/info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var info api.BackendInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

// dialUpgrade sends an HTTP request to the backend and expects a 101 Upgrade response.
// It returns the raw bidirectional connection for streaming (exec/attach).
func (c *BackendClient) dialUpgrade(method, path string, body any) (net.Conn, *bufio.Reader, error) {
	u, err := url.Parse(c.url(path))
	if err != nil {
		return nil, nil, err
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	conn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial backend: %w", err)
	}

	// Build the HTTP request manually
	var bodyBytes []byte
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			conn.Close()
			return nil, nil, err
		}
	}

	reqLine := fmt.Sprintf("%s %s HTTP/1.1\r\n", method, u.RequestURI())
	headers := fmt.Sprintf("Host: %s\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n", u.Host)
	if len(bodyBytes) > 0 {
		headers += fmt.Sprintf("Content-Type: application/json\r\nContent-Length: %d\r\n", len(bodyBytes))
	}
	headers += "\r\n"

	_, _ = conn.Write([]byte(reqLine))
	_, _ = conn.Write([]byte(headers))
	if len(bodyBytes) > 0 {
		_, _ = conn.Write(bodyBytes)
	}

	// Read the HTTP response
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to read backend response: %w", err)
	}

	if resp.StatusCode == http.StatusSwitchingProtocols {
		// Success — return the raw connection
		return conn, br, nil
	}

	// Error response — read the body and return error
	defer resp.Body.Close()
	var errResp api.ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	conn.Close()
	return nil, nil, &httpError{status: resp.StatusCode, message: errResp.Message}
}

type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string {
	if e.message != "" {
		return e.message
	}
	return fmt.Sprintf("backend returned status %d", e.status)
}

func (e *httpError) StatusCode() int {
	return e.status
}
