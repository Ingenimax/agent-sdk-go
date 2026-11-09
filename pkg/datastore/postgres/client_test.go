package postgres_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/datastore/postgres"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

var (
	insertRe = regexp.MustCompile(`^INSERT INTO "([^"]+)" \((.*)\) VALUES \((.*)\) RETURNING id$`)
	updateRe = regexp.MustCompile(`^UPDATE "([^"]+)" SET (.*) WHERE id = \$(\d+) AND org_id = \$(\d+)$`)
	assignRe = regexp.MustCompile(`^"([^"]+)" = \$(\d+)$`)
)

func orgCtx() context.Context {
	return multitenancy.WithOrgID(context.Background(), "org-1")
}

// argAt returns the argument bound to a "$n" placeholder.
func argAt(t *testing.T, args []driver.Value, placeholder string) driver.Value {
	t.Helper()
	n, err := strconv.Atoi(strings.TrimPrefix(placeholder, "$"))
	require.NoError(t, err)
	require.LessOrEqual(t, n, len(args))
	return args[n-1]
}

// parseInsert maps each inserted column to its bound value.
func parseInsert(t *testing.T, call testutil.FakeSQLCall) (string, map[string]driver.Value) {
	t.Helper()
	m := insertRe.FindStringSubmatch(call.Query)
	require.NotNil(t, m, "unexpected insert query: %s", call.Query)
	cols := strings.Split(m[2], ", ")
	phs := strings.Split(m[3], ", ")
	require.Len(t, phs, len(cols))
	require.Len(t, call.Args, len(cols))
	out := make(map[string]driver.Value, len(cols))
	for i, c := range cols {
		out[strings.Trim(c, `"`)] = argAt(t, call.Args, phs[i])
	}
	return m[1], out
}

// parseAssignments maps each `"col" = $n` term to its bound value.
func parseAssignments(t *testing.T, terms []string, args []driver.Value) map[string]driver.Value {
	t.Helper()
	out := make(map[string]driver.Value, len(terms))
	for _, term := range terms {
		m := assignRe.FindStringSubmatch(term)
		require.NotNil(t, m, "unexpected term: %s", term)
		out[m[1]] = argAt(t, args, "$"+m[2])
	}
	return out
}

// eachMode runs fn once against a plain collection and once against a
// collection inside a committed transaction.
func eachMode(t *testing.T, db *testutil.FakeSQL, fn func(t *testing.T, coll interfaces.CollectionRef)) {
	t.Helper()
	client, err := postgres.NewWithDB(db.Open())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	t.Run("collection", func(t *testing.T) {
		fn(t, client.Collection("docs"))
	})
	t.Run("transaction", func(t *testing.T) {
		err := client.Transaction(orgCtx(), func(tx interfaces.Transaction) error {
			fn(t, tx.Collection("docs"))
			return nil
		})
		require.NoError(t, err)
	})
}

func TestNewInvalidConnectionString(t *testing.T) {
	// A malformed URL fails while parsing, before any network access.
	_, err := postgres.New("postgres://%zz")
	require.ErrorContains(t, err, "failed to ping PostgreSQL")
}

func TestNewWithDBPingError(t *testing.T) {
	db := &testutil.FakeSQL{PingErr: errors.New("down")}
	_, err := postgres.NewWithDB(db.Open())
	require.ErrorContains(t, err, "failed to ping PostgreSQL: down")
}

func TestMissingOrgID(t *testing.T) {
	db := &testutil.FakeSQL{}
	client, err := postgres.NewWithDB(db.Open())
	require.NoError(t, err)

	check := func(t *testing.T, coll interfaces.CollectionRef) {
		ctx := context.Background()
		_, err := coll.Insert(ctx, map[string]interface{}{})
		assert.ErrorIs(t, err, multitenancy.ErrNoOrgID)
		_, err = coll.Get(ctx, "1")
		assert.ErrorIs(t, err, multitenancy.ErrNoOrgID)
		assert.ErrorIs(t, coll.Update(ctx, "1", map[string]interface{}{}), multitenancy.ErrNoOrgID)
		assert.ErrorIs(t, coll.Delete(ctx, "1"), multitenancy.ErrNoOrgID)
		_, err = coll.Query(ctx, nil)
		assert.ErrorIs(t, err, multitenancy.ErrNoOrgID)
	}

	check(t, client.Collection("docs"))
	require.NoError(t, client.Transaction(context.Background(), func(tx interfaces.Transaction) error {
		check(t, tx.Collection("docs"))
		return nil
	}))
	// No statement reaches the database without an org ID.
	assert.Empty(t, db.Calls())
}

func TestInsert(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse {
		return testutil.FakeSQLResponse{Columns: []string{"id"}, Rows: [][]driver.Value{{"returned-id"}}}
	}}
	eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
		data := map[string]interface{}{"id": "my-id", "name": "doc"}
		id, err := coll.Insert(orgCtx(), data)
		require.NoError(t, err)
		assert.Equal(t, "returned-id", id)

		table, cols := parseInsert(t, db.LastCall())
		assert.Equal(t, "docs", table)
		assert.Equal(t, "my-id", cols["id"])
		assert.Equal(t, "doc", cols["name"])
		assert.Equal(t, "org-1", cols["org_id"])
		assert.IsType(t, time.Time{}, cols["created_at"])
		assert.Len(t, cols, 4)
	})
}

func TestInsertGeneratesID(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse {
		return testutil.FakeSQLResponse{Columns: []string{"id"}, Rows: [][]driver.Value{{"x"}}}
	}}
	eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
		data := map[string]interface{}{"id": "", "name": "doc"}
		_, err := coll.Insert(orgCtx(), data)
		require.NoError(t, err)

		_, cols := parseInsert(t, db.LastCall())
		_, err = uuid.Parse(cols["id"].(string))
		assert.NoError(t, err, "generated id must be a UUID")
		// The caller's map is updated in place.
		assert.Equal(t, cols["id"], data["id"])
	})
}

func TestInsertQuotesIdentifiers(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse {
		return testutil.FakeSQLResponse{Columns: []string{"id"}, Rows: [][]driver.Value{{"x"}}}
	}}
	client, err := postgres.NewWithDB(db.Open())
	require.NoError(t, err)

	_, err = client.Collection(`bad"table`).Insert(orgCtx(), map[string]interface{}{`a"b`: 1})
	require.NoError(t, err)
	q := db.LastCall().Query
	assert.True(t, strings.HasPrefix(q, `INSERT INTO "bad""table" (`), q)
	assert.Contains(t, q, `"a""b"`)
}

func TestInsertErrors(t *testing.T) {
	tests := []struct {
		name string
		resp testutil.FakeSQLResponse
	}{
		{"query error", testutil.FakeSQLResponse{Err: errors.New("boom")}},
		{"no rows", testutil.FakeSQLResponse{Columns: []string{"id"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return tt.resp }}
			eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
				_, err := coll.Insert(orgCtx(), map[string]interface{}{"name": "x"})
				require.ErrorContains(t, err, "failed to insert document")
			})
		})
	}
}

func TestGet(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse {
		return testutil.FakeSQLResponse{
			Columns: []string{"id", "name", "count"},
			Rows:    [][]driver.Value{{"1", "doc", int64(3)}},
		}
	}}
	eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
		doc, err := coll.Get(orgCtx(), "1")
		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"id": "1", "name": "doc", "count": int64(3)}, doc)

		call := db.LastCall()
		assert.Equal(t, `SELECT * FROM "docs" WHERE id = $1 AND org_id = $2`, call.Query)
		assert.Equal(t, []driver.Value{"1", "org-1"}, call.Args)
	})
}

func TestGetErrors(t *testing.T) {
	tests := []struct {
		name    string
		resp    testutil.FakeSQLResponse
		wantErr string
	}{
		{"query error", testutil.FakeSQLResponse{Err: errors.New("boom")}, "failed to query document"},
		{"not found", testutil.FakeSQLResponse{Columns: []string{"id"}}, "document not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return tt.resp }}
			eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
				_, err := coll.Get(orgCtx(), "1")
				require.ErrorContains(t, err, tt.wantErr)
			})
		})
	}
}

func TestUpdateBuildsSQL(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return testutil.FakeSQLResponse{RowsAffected: 1} }}
	eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
		require.NoError(t, coll.Update(orgCtx(), "doc-1", map[string]interface{}{"name": "new", "count": 2}))

		call := db.LastCall()
		m := updateRe.FindStringSubmatch(call.Query)
		require.NotNil(t, m, call.Query)
		assert.Equal(t, "docs", m[1])
		assert.Equal(t, "doc-1", argAt(t, call.Args, "$"+m[3]))
		assert.Equal(t, "org-1", argAt(t, call.Args, "$"+m[4]))

		set := parseAssignments(t, strings.Split(m[2], ", "), call.Args)
		assert.Equal(t, "new", set["name"])
		assert.Equal(t, int64(2), set["count"])
		assert.IsType(t, time.Time{}, set["updated_at"])
		assert.Len(t, set, 3)
	})
}

func TestUpdateAndDeleteErrors(t *testing.T) {
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
			eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
				require.ErrorContains(t, coll.Update(orgCtx(), "1", map[string]interface{}{"a": 1}), tt.updateErr)
				require.ErrorContains(t, coll.Delete(orgCtx(), "1"), tt.deleteErr)
			})
		})
	}
}

func TestDelete(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return testutil.FakeSQLResponse{RowsAffected: 1} }}
	eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
		require.NoError(t, coll.Delete(orgCtx(), "doc-1"))
		call := db.LastCall()
		assert.Equal(t, `DELETE FROM "docs" WHERE id = $1 AND org_id = $2`, call.Query)
		assert.Equal(t, []driver.Value{"doc-1", "org-1"}, call.Args)
	})
}

func TestQueryBuildsSQL(t *testing.T) {
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
			opts:     []interfaces.QueryOption{interfaces.QueryWithOrderBy("name", "DESC"), interfaces.QueryWithLimit(10), interfaces.QueryWithOffset(5)},
			wantSQL:  `SELECT * FROM "docs" WHERE org_id = $1 AND "type" = $2 ORDER BY "name" DESC LIMIT 10 OFFSET 5`,
			wantArgs: []driver.Value{"org-1", "a"},
		},
		{
			name:     "order defaults to ascending",
			opts:     []interfaces.QueryOption{interfaces.QueryWithOrderBy("name", "sideways")},
			wantSQL:  `SELECT * FROM "docs" WHERE org_id = $1 ORDER BY "name" ASC`,
			wantArgs: []driver.Value{"org-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return testutil.FakeSQLResponse{Columns: []string{"id"}} }}
			eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
				got, err := coll.Query(orgCtx(), tt.filter, tt.opts...)
				require.NoError(t, err)
				assert.Empty(t, got)
				call := db.LastCall()
				assert.Equal(t, tt.wantSQL, call.Query)
				assert.Equal(t, tt.wantArgs, call.Args)
			})
		})
	}
}

func TestQueryMultipleFiltersBindCorrectly(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return testutil.FakeSQLResponse{Columns: []string{"id"}} }}
	eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
		_, err := coll.Query(orgCtx(), map[string]interface{}{"a": "x", "b": int64(2), "c": true})
		require.NoError(t, err)

		call := db.LastCall()
		where := strings.TrimPrefix(call.Query, `SELECT * FROM "docs" WHERE org_id = $1 AND `)
		got := parseAssignments(t, strings.Split(where, " AND "), call.Args)
		assert.Equal(t, map[string]driver.Value{"a": "x", "b": int64(2), "c": true}, got)
		assert.Equal(t, "org-1", call.Args[0])
	})
}

func TestQueryReturnsRows(t *testing.T) {
	db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse {
		return testutil.FakeSQLResponse{
			Columns: []string{"id", "name"},
			Rows:    [][]driver.Value{{"1", "a"}, {"2", "b"}},
		}
	}}
	eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
		got, err := coll.Query(orgCtx(), nil)
		require.NoError(t, err)
		assert.Equal(t, []map[string]interface{}{
			{"id": "1", "name": "a"},
			{"id": "2", "name": "b"},
		}, got)
	})
}

func TestQueryErrors(t *testing.T) {
	tests := []struct {
		name    string
		resp    testutil.FakeSQLResponse
		wantErr string
	}{
		{"query error", testutil.FakeSQLResponse{Err: errors.New("boom")}, "failed to query documents"},
		{"iteration error", testutil.FakeSQLResponse{Columns: []string{"id"}, Rows: [][]driver.Value{{"1"}}, IterErr: errors.New("broken")}, "error iterating rows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &testutil.FakeSQL{Respond: func(string) testutil.FakeSQLResponse { return tt.resp }}
			eachMode(t, db, func(t *testing.T, coll interfaces.CollectionRef) {
				_, err := coll.Query(orgCtx(), nil)
				require.ErrorContains(t, err, tt.wantErr)
			})
		})
	}
}

func TestTransactionCommits(t *testing.T) {
	db := &testutil.FakeSQL{}
	client, err := postgres.NewWithDB(db.Open())
	require.NoError(t, err)

	require.NoError(t, client.Transaction(orgCtx(), func(interfaces.Transaction) error { return nil }))
	assert.Equal(t, 1, db.Committed())
	assert.Equal(t, 0, db.RolledBack())
}

func TestTransactionErrors(t *testing.T) {
	fnErr := errors.New("fn failed")
	tests := []struct {
		name         string
		db           *testutil.FakeSQL
		fnErr        error
		wantErr      string
		wantIs       error
		wantRollback int
	}{
		{"begin fails", &testutil.FakeSQL{BeginErr: errors.New("no tx")}, nil, "failed to start transaction", nil, 0},
		{"fn fails and rolls back", &testutil.FakeSQL{}, fnErr, "fn failed", fnErr, 1},
		{"rollback fails", &testutil.FakeSQL{RollbackErr: errors.New("rb")}, fnErr, "rollback failed with error: rb", fnErr, 1},
		{"commit fails", &testutil.FakeSQL{CommitErr: errors.New("cm")}, nil, "failed to commit transaction", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := postgres.NewWithDB(tt.db.Open())
			require.NoError(t, err)

			err = client.Transaction(orgCtx(), func(interfaces.Transaction) error { return tt.fnErr })
			require.ErrorContains(t, err, tt.wantErr)
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
			assert.Equal(t, tt.wantRollback, tt.db.RolledBack())
		})
	}
}

func TestTransactionExplicitCommitAndRollback(t *testing.T) {
	db := &testutil.FakeSQL{}
	client, err := postgres.NewWithDB(db.Open())
	require.NoError(t, err)

	// An explicit commit or rollback inside fn reaches the driver exactly once.
	_ = client.Transaction(orgCtx(), func(tx interfaces.Transaction) error {
		return tx.Commit()
	})
	assert.Equal(t, 1, db.Committed())

	err = client.Transaction(orgCtx(), func(tx interfaces.Transaction) error {
		require.NoError(t, tx.Rollback())
		return errors.New("stop")
	})
	require.ErrorContains(t, err, "stop")
	assert.Equal(t, 1, db.RolledBack())
}
