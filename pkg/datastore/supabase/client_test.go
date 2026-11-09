package supabase_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/datastore/supabase"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func orgCtx() context.Context {
	return multitenancy.WithOrgID(context.Background(), "org-1")
}

// restRequest is one request received by the fake PostgREST server.
type restRequest struct {
	method string
	path   string
	query  url.Values
	header http.Header
	body   []byte
}

// restServer is a fake PostgREST endpoint that records requests and
// replies with a fixed status and body.
type restServer struct {
	*httptest.Server
	mu     sync.Mutex
	last   restRequest
	status int
	reply  string
}

func newRestServer(t *testing.T, status int, reply string) *restServer {
	t.Helper()
	rs := &restServer{status: status, reply: reply}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rs.mu.Lock()
		rs.last = restRequest{method: r.Method, path: r.URL.Path, query: r.URL.Query(), header: r.Header.Clone(), body: body}
		rs.mu.Unlock()
		w.WriteHeader(rs.status)
		_, _ = w.Write([]byte(rs.reply))
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *restServer) request() restRequest {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.last
}

func newRestClient(t *testing.T, rs *restServer) *supabase.Client {
	t.Helper()
	client, err := supabase.New(rs.URL, "test-key")
	require.NoError(t, err)
	return client
}

func TestNewValidation(t *testing.T) {
	_, err := supabase.New("", "key")
	require.ErrorContains(t, err, "failed to create Supabase client")
	_, err = supabase.New("http://localhost", "")
	require.ErrorContains(t, err, "failed to create Supabase client")
}

func TestCloseWithoutDB(t *testing.T) {
	client, err := supabase.New("http://localhost", "key")
	require.NoError(t, err)
	assert.NoError(t, client.Close())
}

func TestCloseWithDB(t *testing.T) {
	client, err := supabase.New("http://localhost", "key", supabase.WithDB((&testutil.FakeSQL{}).Open()))
	require.NoError(t, err)
	assert.NoError(t, client.Close())
}

func TestRestMissingOrgID(t *testing.T) {
	rs := newRestServer(t, http.StatusOK, `[]`)
	coll := newRestClient(t, rs).Collection("docs")
	ctx := context.Background()

	_, err := coll.Insert(ctx, map[string]interface{}{})
	assert.ErrorIs(t, err, multitenancy.ErrNoOrgID)
	_, err = coll.Get(ctx, "1")
	assert.ErrorIs(t, err, multitenancy.ErrNoOrgID)
	assert.ErrorIs(t, coll.Update(ctx, "1", map[string]interface{}{}), multitenancy.ErrNoOrgID)
	assert.ErrorIs(t, coll.Delete(ctx, "1"), multitenancy.ErrNoOrgID)
	_, err = coll.Query(ctx, nil)
	assert.ErrorIs(t, err, multitenancy.ErrNoOrgID)

	assert.Empty(t, rs.request().method, "no request must reach the server")
}

func TestRestInsert(t *testing.T) {
	rs := newRestServer(t, http.StatusCreated, `[{"id":"doc-1"}]`)
	coll := newRestClient(t, rs).Collection("docs")

	id, err := coll.Insert(orgCtx(), map[string]interface{}{"id": "doc-1", "name": "a"})
	require.NoError(t, err)
	assert.Equal(t, "doc-1", id)

	req := rs.request()
	assert.Equal(t, http.MethodPost, req.method)
	assert.Equal(t, "/rest/v1/docs", req.path)
	assert.Equal(t, "test-key", req.header.Get("apikey"))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(req.body, &body))
	assert.Equal(t, "doc-1", body["id"])
	assert.Equal(t, "a", body["name"])
	assert.Equal(t, "org-1", body["org_id"])
	assert.Contains(t, body, "created_at")
}

func TestRestInsertGeneratesID(t *testing.T) {
	rs := newRestServer(t, http.StatusCreated, `[{}]`)
	coll := newRestClient(t, rs).Collection("docs")

	id, err := coll.Insert(orgCtx(), map[string]interface{}{"name": "a"})
	require.NoError(t, err)
	_, err = uuid.Parse(id)
	require.NoError(t, err)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rs.request().body, &body))
	assert.Equal(t, id, body["id"])
}

func TestRestGet(t *testing.T) {
	rs := newRestServer(t, http.StatusOK, `[{"id":"doc-1","name":"a"}]`)
	coll := newRestClient(t, rs).Collection("docs")

	doc, err := coll.Get(orgCtx(), "doc-1")
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"id": "doc-1", "name": "a"}, doc)

	req := rs.request()
	assert.Equal(t, http.MethodGet, req.method)
	assert.Equal(t, "*", req.query.Get("select"))
	assert.Equal(t, "eq.doc-1", req.query.Get("id"))
	assert.Equal(t, "eq.org-1", req.query.Get("org_id"))
}

func TestRestUpdate(t *testing.T) {
	rs := newRestServer(t, http.StatusOK, `[{"id":"doc-1"}]`)
	coll := newRestClient(t, rs).Collection("docs")

	require.NoError(t, coll.Update(orgCtx(), "doc-1", map[string]interface{}{"name": "b"}))

	req := rs.request()
	assert.Equal(t, http.MethodPatch, req.method)
	assert.Equal(t, "eq.doc-1", req.query.Get("id"))
	assert.Equal(t, "eq.org-1", req.query.Get("org_id"))
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(req.body, &body))
	assert.Equal(t, "b", body["name"])
	assert.Contains(t, body, "updated_at")
}

func TestRestDelete(t *testing.T) {
	rs := newRestServer(t, http.StatusOK, `[{"id":"doc-1"}]`)
	coll := newRestClient(t, rs).Collection("docs")

	require.NoError(t, coll.Delete(orgCtx(), "doc-1"))
	req := rs.request()
	assert.Equal(t, http.MethodDelete, req.method)
	assert.Equal(t, "eq.doc-1", req.query.Get("id"))
	assert.Equal(t, "eq.org-1", req.query.Get("org_id"))
}

func TestRestEmptyAndErrorResponses(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		reply     string
		insertErr string
		getErr    string
		updateErr string
		deleteErr string
		queryErr  string
	}{
		{
			name: "empty result", status: http.StatusOK, reply: `[]`,
			insertErr: "no document was inserted", getErr: "document not found",
			updateErr: "no document was updated", deleteErr: "no document was deleted",
		},
		{
			name: "invalid json", status: http.StatusOK, reply: `{"not":"a list"}`,
			insertErr: "failed to unmarshal response", getErr: "failed to unmarshal response",
			updateErr: "failed to unmarshal response", deleteErr: "failed to unmarshal response",
			queryErr: "failed to unmarshal response",
		},
		{
			name: "server error", status: http.StatusBadRequest, reply: `{"code":"42P01","message":"missing table"}`,
			insertErr: "failed to insert document", getErr: "failed to get document",
			updateErr: "failed to update document", deleteErr: "failed to delete document",
			queryErr: "failed to query documents",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coll := newRestClient(t, newRestServer(t, tt.status, tt.reply)).Collection("docs")
			ctx := orgCtx()

			_, err := coll.Insert(ctx, map[string]interface{}{"name": "a"})
			assert.ErrorContains(t, err, tt.insertErr)
			_, err = coll.Get(ctx, "1")
			assert.ErrorContains(t, err, tt.getErr)
			assert.ErrorContains(t, coll.Update(ctx, "1", map[string]interface{}{"a": "b"}), tt.updateErr)
			assert.ErrorContains(t, coll.Delete(ctx, "1"), tt.deleteErr)
			_, err = coll.Query(ctx, nil)
			if tt.queryErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.queryErr)
			}
		})
	}
}

func TestRestQuery(t *testing.T) {
	tests := []struct {
		name      string
		filter    map[string]interface{}
		opts      []interfaces.QueryOption
		wantQuery url.Values
	}{
		{
			name:      "org filter only",
			wantQuery: url.Values{"select": {"*"}, "org_id": {"eq.org-1"}},
		},
		{
			name:   "filter, limit and descending order",
			filter: map[string]interface{}{"type": "a"},
			opts: []interfaces.QueryOption{
				interfaces.QueryWithLimit(5),
				interfaces.QueryWithOffset(10),
				interfaces.QueryWithOrderBy("name", "DESC"),
			},
			wantQuery: url.Values{
				"select": {"*"}, "org_id": {"eq.org-1"}, "type": {"eq.a"},
				"limit": {"5"}, "order": {"name.desc.nullslast"},
			},
		},
		{
			name:      "ascending order",
			opts:      []interfaces.QueryOption{interfaces.QueryWithOrderBy("name", "asc")},
			wantQuery: url.Values{"select": {"*"}, "org_id": {"eq.org-1"}, "order": {"name.asc.nullslast"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := newRestServer(t, http.StatusOK, `[{"id":"1"},{"id":"2"}]`)
			got, err := newRestClient(t, rs).Collection("docs").Query(orgCtx(), tt.filter, tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, []map[string]interface{}{{"id": "1"}, {"id": "2"}}, got)

			req := rs.request()
			assert.Equal(t, http.MethodGet, req.method)
			// The REST path drops the offset today; ignore it either way.
			query := req.query
			query.Del("offset")
			assert.Equal(t, tt.wantQuery, query)
		})
	}
}

func TestTransactionRequiresDB(t *testing.T) {
	client, err := supabase.New("http://localhost", "key")
	require.NoError(t, err)
	err = client.Transaction(orgCtx(), func(interfaces.Transaction) error { return nil })
	require.ErrorContains(t, err, "database connection is required")
}

func TestTransactionLifecycle(t *testing.T) {
	fnErr := errors.New("fn failed")
	tests := []struct {
		name          string
		db            *testutil.FakeSQL
		fnErr         error
		wantErr       string
		wantCommits   int
		wantRollbacks int
	}{
		{"commit", &testutil.FakeSQL{}, nil, "", 1, 0},
		{"begin fails", &testutil.FakeSQL{BeginErr: errors.New("no tx")}, nil, "failed to start transaction", 0, 0},
		{"fn fails", &testutil.FakeSQL{}, fnErr, "fn failed", 0, 1},
		{"rollback fails", &testutil.FakeSQL{RollbackErr: errors.New("rb")}, fnErr, "rollback failed with error: rb", 0, 1},
		{"commit fails", &testutil.FakeSQL{CommitErr: errors.New("cm")}, nil, "failed to commit transaction", 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := supabase.New("http://localhost", "key", supabase.WithDB(tt.db.Open()))
			require.NoError(t, err)

			err = client.Transaction(orgCtx(), func(interfaces.Transaction) error { return tt.fnErr })
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantCommits, tt.db.Committed())
			assert.Equal(t, tt.wantRollbacks, tt.db.RolledBack())
		})
	}
}

func TestTransactionExplicitRollback(t *testing.T) {
	db := &testutil.FakeSQL{}
	client, err := supabase.New("http://localhost", "key", supabase.WithDB(db.Open()))
	require.NoError(t, err)

	// An explicit rollback inside fn reaches the driver once and never commits.
	_ = client.Transaction(orgCtx(), func(tx interfaces.Transaction) error {
		require.NoError(t, tx.Rollback())
		return nil
	})
	assert.Equal(t, 1, db.RolledBack())
	assert.Equal(t, 0, db.Committed())
}

// inTx runs fn with a collection bound to a committed transaction.
func inTx(t *testing.T, db *testutil.FakeSQL, fn func(coll interfaces.CollectionRef)) {
	t.Helper()
	client, err := supabase.New("http://localhost", "key", supabase.WithDB(db.Open()))
	require.NoError(t, err)
	require.NoError(t, client.Transaction(orgCtx(), func(tx interfaces.Transaction) error {
		fn(tx.Collection("docs"))
		return nil
	}))
}

func TestTxMissingOrgID(t *testing.T) {
	db := &testutil.FakeSQL{}
	inTx(t, db, func(coll interfaces.CollectionRef) {
		ctx := context.Background()
		_, err := coll.Insert(ctx, map[string]interface{}{})
		assert.ErrorIs(t, err, multitenancy.ErrNoOrgID)
		_, err = coll.Get(ctx, "1")
		assert.ErrorIs(t, err, multitenancy.ErrNoOrgID)
		assert.ErrorIs(t, coll.Update(ctx, "1", map[string]interface{}{}), multitenancy.ErrNoOrgID)
		assert.ErrorIs(t, coll.Delete(ctx, "1"), multitenancy.ErrNoOrgID)
		_, err = coll.Query(ctx, nil)
		assert.ErrorIs(t, err, multitenancy.ErrNoOrgID)
	})
	assert.Empty(t, db.Calls())
}

func TestTxInsert(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse {
		return testutil.FakeSQLResponse{Columns: []string{"id"}, Rows: [][]driver.Value{{"returned"}}}
	}}
	inTx(t, db, func(coll interfaces.CollectionRef) {
		data := map[string]interface{}{"name": "a"}
		id, err := coll.Insert(orgCtx(), data)
		require.NoError(t, err)
		assert.Equal(t, "returned", id)

		call := db.LastCall()
		assert.True(t, strings.HasPrefix(call.Query, `INSERT INTO "docs" (`), call.Query)
		assert.True(t, strings.HasSuffix(call.Query, ") RETURNING id"), call.Query)
		assert.Len(t, call.Args, 4)
		assert.Contains(t, call.Args, driver.Value("org-1"))
		assert.Contains(t, call.Args, driver.Value("a"))
		_, err = uuid.Parse(data["id"].(string))
		assert.NoError(t, err)
	})
}

func TestTxInsertError(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return testutil.FakeSQLResponse{Err: errors.New("boom")} }}
	inTx(t, db, func(coll interfaces.CollectionRef) {
		_, err := coll.Insert(orgCtx(), map[string]interface{}{"id": "x"})
		require.ErrorContains(t, err, "failed to insert document")
	})
}

func TestTxGetErrors(t *testing.T) {
	tests := []struct {
		name    string
		resp    testutil.FakeSQLResponse
		wantErr string
	}{
		{"not found", testutil.FakeSQLResponse{Columns: []string{"id"}}, "document not found"},
		{"query error", testutil.FakeSQLResponse{Err: errors.New("boom")}, "failed to scan row"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return tt.resp }}
			inTx(t, db, func(coll interfaces.CollectionRef) {
				_, err := coll.Get(orgCtx(), "1")
				require.ErrorContains(t, err, tt.wantErr)
			})
			call := db.LastCall()
			assert.Equal(t, `SELECT * FROM "docs" WHERE id = $1 AND org_id = $2`, call.Query)
			assert.Equal(t, []driver.Value{"1", "org-1"}, call.Args)
		})
	}
}

func TestTxUpdateAndDelete(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return testutil.FakeSQLResponse{RowsAffected: 1} }}
	inTx(t, db, func(coll interfaces.CollectionRef) {
		require.NoError(t, coll.Update(orgCtx(), "doc-1", map[string]interface{}{"name": "b"}))
		call := db.LastCall()
		assert.True(t, strings.HasPrefix(call.Query, `UPDATE "docs" SET `), call.Query)
		assert.True(t, strings.HasSuffix(call.Query, " WHERE id = $3 AND org_id = $4"), call.Query)
		assert.Equal(t, []driver.Value{"doc-1", "org-1"}, call.Args[2:])

		require.NoError(t, coll.Delete(orgCtx(), "doc-1"))
		call = db.LastCall()
		assert.Equal(t, `DELETE FROM "docs" WHERE id = $1 AND org_id = $2`, call.Query)
		assert.Equal(t, []driver.Value{"doc-1", "org-1"}, call.Args)
	})
}

func TestTxUpdateAndDeleteErrors(t *testing.T) {
	tests := []struct {
		name      string
		resp      testutil.FakeSQLResponse
		updateErr string
		deleteErr string
	}{
		{"exec error", testutil.FakeSQLResponse{Err: errors.New("boom")}, "failed to update document", "failed to delete document"},
		{"rows affected error", testutil.FakeSQLResponse{AffectedErr: errors.New("x")}, "failed to get rows affected", "failed to get rows affected"},
		{"no rows", testutil.FakeSQLResponse{}, "document not found or not owned", "document not found or not owned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return tt.resp }}
			inTx(t, db, func(coll interfaces.CollectionRef) {
				require.ErrorContains(t, coll.Update(orgCtx(), "1", map[string]interface{}{"a": 1}), tt.updateErr)
				require.ErrorContains(t, coll.Delete(orgCtx(), "1"), tt.deleteErr)
			})
		})
	}
}

func TestTxQuery(t *testing.T) {
	tests := []struct {
		name     string
		filter   map[string]interface{}
		opts     []interfaces.QueryOption
		wantSQL  string
		wantArgs []driver.Value
	}{
		{
			name:     "no filter",
			wantSQL:  `SELECT * FROM "docs" WHERE org_id = $1`,
			wantArgs: []driver.Value{"org-1"},
		},
		{
			name:     "filter and all options",
			filter:   map[string]interface{}{"type": "a"},
			opts:     []interfaces.QueryOption{interfaces.QueryWithOrderBy("name", "desc"), interfaces.QueryWithLimit(3), interfaces.QueryWithOffset(6)},
			wantSQL:  `SELECT * FROM "docs" WHERE org_id = $1 AND "type" = $2 ORDER BY name DESC LIMIT 3 OFFSET 6`,
			wantArgs: []driver.Value{"org-1", "a"},
		},
		{
			name:     "ascending order",
			opts:     []interfaces.QueryOption{interfaces.QueryWithOrderBy("name", "")},
			wantSQL:  `SELECT * FROM "docs" WHERE org_id = $1 ORDER BY name ASC`,
			wantArgs: []driver.Value{"org-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse {
				return testutil.FakeSQLResponse{Columns: []string{"id", "n"}, Rows: [][]driver.Value{{"1", int64(2)}}}
			}}
			inTx(t, db, func(coll interfaces.CollectionRef) {
				got, err := coll.Query(orgCtx(), tt.filter, tt.opts...)
				require.NoError(t, err)
				assert.Equal(t, []map[string]interface{}{{"id": "1", "n": int64(2)}}, got)
			})
			call := db.LastCall()
			// The order column is not quoted today. Accept both forms, so
			// quoting it later does not break this test.
			query := strings.Replace(call.Query, `ORDER BY "name"`, "ORDER BY name", 1)
			assert.Equal(t, tt.wantSQL, query)
			assert.Equal(t, tt.wantArgs, call.Args)
		})
	}
}

func TestTxQueryErrors(t *testing.T) {
	tests := []struct {
		name    string
		resp    testutil.FakeSQLResponse
		wantErr string
	}{
		{"query error", testutil.FakeSQLResponse{Err: errors.New("boom")}, "failed to query documents"},
		{"iteration error", testutil.FakeSQLResponse{Columns: []string{"id"}, IterErr: errors.New("broken")}, "error iterating rows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return tt.resp }}
			inTx(t, db, func(coll interfaces.CollectionRef) {
				_, err := coll.Query(orgCtx(), nil)
				require.ErrorContains(t, err, tt.wantErr)
			})
		})
	}
}
