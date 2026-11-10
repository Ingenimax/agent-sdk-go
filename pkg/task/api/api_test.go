package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorded captures what the test server received.
type recorded struct {
	method  string
	path    string
	query   string
	headers http.Header
	body    string
}

// newServer returns a server that records each request and answers with the
// given status and body.
func newServer(t *testing.T, status int, body string) (*httptest.Server, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		*rec = recorded{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, headers: r.Header.Clone(), body: string(b)}
		w.Header().Set("X-Reply", "yes")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestClientMethods(t *testing.T) {
	ctx := context.Background()
	payload := map[string]string{"a": "b"}

	tests := []struct {
		name       string
		call       func(c *Client) (*Response, error)
		wantMethod string
		wantBody   string
		wantQuery  string
		wantCType  string
	}{
		{
			name:       "get with query",
			call:       func(c *Client) (*Response, error) { return c.Get(ctx, "/items", map[string]string{"q": "x y"}, nil) },
			wantMethod: http.MethodGet,
			wantQuery:  "q=x+y",
		},
		{
			name:       "post sets json content type",
			call:       func(c *Client) (*Response, error) { return c.Post(ctx, "/items", payload, nil) },
			wantMethod: http.MethodPost,
			wantBody:   `{"a":"b"}`,
			wantCType:  "application/json",
		},
		{
			name: "put keeps explicit content type",
			call: func(c *Client) (*Response, error) {
				return c.Put(ctx, "/items", payload, map[string]string{"Content-Type": "text/plain"})
			},
			wantMethod: http.MethodPut,
			wantBody:   `{"a":"b"}`,
			wantCType:  "text/plain",
		},
		{
			name:       "delete",
			call:       func(c *Client) (*Response, error) { return c.Delete(ctx, "/items", nil) },
			wantMethod: http.MethodDelete,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, rec := newServer(t, http.StatusCreated, "reply")
			c := NewClient(srv.URL, time.Second)

			resp, err := tt.call(c)
			require.NoError(t, err)
			assert.Equal(t, http.StatusCreated, resp.StatusCode)
			assert.Equal(t, "reply", string(resp.Body))
			assert.Equal(t, "yes", resp.Headers.Get("X-Reply"))

			assert.Equal(t, tt.wantMethod, rec.method)
			assert.Equal(t, "/items", rec.path)
			assert.Equal(t, tt.wantQuery, rec.query)
			assert.Equal(t, tt.wantBody, rec.body)
			assert.Equal(t, tt.wantCType, rec.headers.Get("Content-Type"))
		})
	}
}

func TestClientHeaders(t *testing.T) {
	srv, rec := newServer(t, http.StatusOK, "")
	c := NewClient(srv.URL, time.Second)
	c.SetHeader("Authorization", "Bearer token")
	c.SetHeaders(map[string]string{"X-One": "1", "X-Two": "2"})

	_, err := c.Get(context.Background(), "/", nil, map[string]string{"X-Two": "override"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer token", rec.headers.Get("Authorization"))
	assert.Equal(t, "1", rec.headers.Get("X-One"))
	assert.Equal(t, "override", rec.headers.Get("X-Two"), "per-request headers win over client headers")
}

func TestClientErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("body cannot be marshaled", func(t *testing.T) {
		c := NewClient("http://127.0.0.1", time.Second)
		_, err := c.Post(ctx, "/", make(chan int), nil)
		assert.ErrorContains(t, err, "failed to marshal request body")
	})

	t.Run("invalid method", func(t *testing.T) {
		c := NewClient("http://127.0.0.1", time.Second)
		_, err := c.Do(ctx, Request{Method: "BAD METHOD", Path: "/"})
		assert.ErrorContains(t, err, "failed to create HTTP request")
	})

	t.Run("server unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		_, err := NewClient(url, time.Second).Get(ctx, "/", nil, nil)
		assert.ErrorContains(t, err, "failed to make HTTP request")
	})

	t.Run("body read fails", func(t *testing.T) {
		// Declare a longer body than is sent so the read ends early.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, "short")
		}))
		defer srv.Close()
		_, err := NewClient(srv.URL, time.Second).Get(ctx, "/", nil, nil)
		assert.ErrorContains(t, err, "failed to read response body")
	})
}

func TestTaskFunc(t *testing.T) {
	ctx := context.Background()

	t.Run("success uses params as body", func(t *testing.T) {
		srv, rec := newServer(t, http.StatusOK, `{"ok":true}`)
		api := NewTaskAPI(NewClient(srv.URL, time.Second))
		fn := api.Task(Request{Method: http.MethodPost, Path: "/run", Body: map[string]int{"old": 1}})

		out, err := fn(ctx, map[string]int{"new": 2})
		require.NoError(t, err)
		assert.Equal(t, []byte(`{"ok":true}`), out)
		assert.Equal(t, `{"new":2}`, rec.body)
	})

	t.Run("nil params keep the original body", func(t *testing.T) {
		srv, rec := newServer(t, http.StatusOK, "")
		api := NewTaskAPI(NewClient(srv.URL, time.Second))
		fn := api.Task(Request{Method: http.MethodPost, Path: "/run", Body: map[string]int{"old": 1}})

		_, err := fn(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, `{"old":1}`, rec.body)
	})

	t.Run("non 2xx status", func(t *testing.T) {
		srv, _ := newServer(t, http.StatusBadGateway, "upstream down")
		fn := NewTaskAPI(NewClient(srv.URL, time.Second)).Task(Request{Method: http.MethodGet, Path: "/"})
		_, err := fn(ctx, nil)
		assert.EqualError(t, err, "API task failed with status 502: upstream down")
	})

	t.Run("transport error", func(t *testing.T) {
		fn := NewTaskAPI(NewClient("http://127.0.0.1", time.Second)).Task(Request{Method: "BAD METHOD"})
		_, err := fn(ctx, nil)
		assert.ErrorContains(t, err, "failed to execute API task")
	})
}

func TestRegisterWithExecutor(t *testing.T) {
	srv, rec := newServer(t, http.StatusOK, "done")
	api := NewTaskAPI(NewClient(srv.URL, time.Second))
	exec := executor.NewTaskExecutor()
	api.RegisterWithExecutor(exec, "remote", Request{Method: http.MethodPost, Path: "/remote"})

	res, err := exec.ExecuteSync(context.Background(), "remote", map[string]string{"x": "y"}, nil)
	require.NoError(t, err)
	require.NoError(t, res.Error)
	assert.Equal(t, []byte("done"), res.Data)
	assert.Equal(t, "/remote", rec.path)
	assert.Equal(t, `{"x":"y"}`, rec.body)
}

func TestExecuteTask(t *testing.T) {
	ctx := context.Background()

	srv, rec := newServer(t, http.StatusAccepted, "")
	require.NoError(t, NewTaskAPI(NewClient(srv.URL, time.Second)).ExecuteTask(ctx, "t1"))
	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/tasks/t1/execute", rec.path)

	failing, _ := newServer(t, http.StatusNotFound, "nope")
	err := NewTaskAPI(NewClient(failing.URL, time.Second)).ExecuteTask(ctx, "t1")
	assert.EqualError(t, err, "task execution failed with status 404: nope")

	err = NewTaskAPI(NewClient("http://127.0.0.1", time.Second)).ExecuteTask(ctx, "bad id\x7f")
	assert.ErrorContains(t, err, "failed to execute task via API")
}

func TestGetTaskStatus(t *testing.T) {
	ctx := context.Background()
	body, err := json.Marshal(map[string]string{"status": string(core.StatusCompleted)})
	require.NoError(t, err)

	srv, rec := newServer(t, http.StatusOK, string(body))
	status, err := NewTaskAPI(NewClient(srv.URL, time.Second)).GetTaskStatus(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, core.StatusCompleted, status)
	assert.Equal(t, http.MethodGet, rec.method)
	assert.Equal(t, "/tasks/t1", rec.path)

	bad, _ := newServer(t, http.StatusOK, "not json")
	_, err = NewTaskAPI(NewClient(bad.URL, time.Second)).GetTaskStatus(ctx, "t1")
	assert.ErrorContains(t, err, "failed to parse task status response")

	failing, _ := newServer(t, http.StatusInternalServerError, "oops")
	_, err = NewTaskAPI(NewClient(failing.URL, time.Second)).GetTaskStatus(ctx, "t1")
	assert.EqualError(t, err, "get task status failed with status 500: oops")

	_, err = NewTaskAPI(NewClient("http://127.0.0.1", time.Second)).GetTaskStatus(ctx, "bad id\x7f")
	assert.ErrorContains(t, err, "failed to get task status via API")
}
