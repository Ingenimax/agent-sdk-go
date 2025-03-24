package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a client for making API calls
type Client struct {
	client  *http.Client
	baseURL string
	headers map[string]string
}

// Request represents an API request
type Request struct {
	Method  string
	Path    string
	Body    interface{}
	Headers map[string]string
	Query   map[string]string
}

// Response represents an API response
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// NewClient creates a new API client
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL: baseURL,
		headers: make(map[string]string),
	}
}

// SetHeader sets a header for all requests
func (c *Client) SetHeader(key, value string) {
	c.headers[key] = value
}

// SetHeaders sets multiple headers for all requests
func (c *Client) SetHeaders(headers map[string]string) {
	for k, v := range headers {
		c.headers[k] = v
	}
}

// Do makes an API request
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	// Prepare URL
	url := c.baseURL + req.Path

	// Prepare body
	var bodyReader io.Reader
	if req.Body != nil {
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Set content type if not set and body is not nil
	if req.Body != nil && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Set query parameters
	if req.Query != nil {
		q := httpReq.URL.Query()
		for k, v := range req.Query {
			q.Add(k, v)
		}
		httpReq.URL.RawQuery = q.Encode()
	}

	// Make the request
	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer httpResp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Create response
	resp := &Response{
		StatusCode: httpResp.StatusCode,
		Body:       respBody,
		Headers:    httpResp.Header,
	}

	return resp, nil
}

// Get makes a GET request
func (c *Client) Get(ctx context.Context, path string, query map[string]string, headers map[string]string) (*Response, error) {
	req := Request{
		Method:  http.MethodGet,
		Path:    path,
		Query:   query,
		Headers: headers,
	}
	return c.Do(ctx, req)
}

// Post makes a POST request
func (c *Client) Post(ctx context.Context, path string, body interface{}, headers map[string]string) (*Response, error) {
	req := Request{
		Method:  http.MethodPost,
		Path:    path,
		Body:    body,
		Headers: headers,
	}
	return c.Do(ctx, req)
}

// Put makes a PUT request
func (c *Client) Put(ctx context.Context, path string, body interface{}, headers map[string]string) (*Response, error) {
	req := Request{
		Method:  http.MethodPut,
		Path:    path,
		Body:    body,
		Headers: headers,
	}
	return c.Do(ctx, req)
}

// Delete makes a DELETE request
func (c *Client) Delete(ctx context.Context, path string, headers map[string]string) (*Response, error) {
	req := Request{
		Method:  http.MethodDelete,
		Path:    path,
		Headers: headers,
	}
	return c.Do(ctx, req)
}
