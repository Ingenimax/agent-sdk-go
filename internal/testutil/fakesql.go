package testutil

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
)

// FakeSQLResponse is what FakeSQL returns for one statement.
type FakeSQLResponse struct {
	Columns      []string
	Rows         [][]driver.Value
	RowsAffected int64
	// Err is returned by the query or exec call itself.
	Err error
	// AffectedErr is returned by Result.RowsAffected.
	AffectedErr error
	// IterErr is returned by Rows.Next after the last row.
	IterErr error
}

// FakeSQLCall records one statement sent to FakeSQL.
type FakeSQLCall struct {
	Query string
	Args  []driver.Value
}

// FakeSQL is a minimal database/sql driver that records statements and
// replies with scripted responses, so tests can check the SQL a client builds
// without a real database server.
//
// Set the fields before calling Open; they are read without locking.
type FakeSQL struct {
	// Respond answers every query and exec. Nil answers with an empty response.
	Respond func(query string) FakeSQLResponse

	PingErr     error
	BeginErr    error
	CommitErr   error
	RollbackErr error

	mu         sync.Mutex
	calls      []FakeSQLCall
	committed  int
	rolledBack int
}

// Open returns a *sql.DB backed by f.
func (f *FakeSQL) Open() *sql.DB {
	return sql.OpenDB(fakeSQLConnector{f})
}

// Calls returns every statement received, in order.
func (f *FakeSQL) Calls() []FakeSQLCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeSQLCall(nil), f.calls...)
}

// LastCall returns the most recent statement, or the zero value if none.
func (f *FakeSQL) LastCall() FakeSQLCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return FakeSQLCall{}
	}
	return f.calls[len(f.calls)-1]
}

// Committed returns how many transactions reached the driver's Commit.
func (f *FakeSQL) Committed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.committed
}

// RolledBack returns how many transactions reached the driver's Rollback.
func (f *FakeSQL) RolledBack() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rolledBack
}

func (f *FakeSQL) record(query string, args []driver.NamedValue) FakeSQLResponse {
	values := make([]driver.Value, len(args))
	for i, a := range args {
		values[i] = a.Value
	}
	f.mu.Lock()
	f.calls = append(f.calls, FakeSQLCall{Query: query, Args: values})
	f.mu.Unlock()
	if f.Respond == nil {
		return FakeSQLResponse{}
	}
	return f.Respond(query)
}

type fakeSQLConnector struct{ db *FakeSQL }

func (c fakeSQLConnector) Connect(context.Context) (driver.Conn, error) {
	return fakeSQLConn(c), nil
}

func (c fakeSQLConnector) Driver() driver.Driver { return fakeSQLDriver{} }

type fakeSQLDriver struct{}

func (fakeSQLDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use the connector") }

// fakeSQLConn implements QueryerContext and ExecerContext, so database/sql
// never calls Prepare.
type fakeSQLConn struct{ db *FakeSQL }

func (c fakeSQLConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}

func (c fakeSQLConn) Close() error { return nil }

func (c fakeSQLConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c fakeSQLConn) Ping(context.Context) error { return c.db.PingErr }

func (c fakeSQLConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	if c.db.BeginErr != nil {
		return nil, c.db.BeginErr
	}
	return fakeSQLTx(c), nil
}

func (c fakeSQLConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	resp := c.db.record(query, args)
	if resp.Err != nil {
		return nil, resp.Err
	}
	return &fakeSQLRows{resp: resp}, nil
}

func (c fakeSQLConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	resp := c.db.record(query, args)
	if resp.Err != nil {
		return nil, resp.Err
	}
	return fakeSQLResult{resp: resp}, nil
}

type fakeSQLTx struct{ db *FakeSQL }

func (t fakeSQLTx) Commit() error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	t.db.committed++
	return t.db.CommitErr
}

func (t fakeSQLTx) Rollback() error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	t.db.rolledBack++
	return t.db.RollbackErr
}

type fakeSQLRows struct {
	resp FakeSQLResponse
	pos  int
}

func (r *fakeSQLRows) Columns() []string { return r.resp.Columns }
func (r *fakeSQLRows) Close() error      { return nil }

func (r *fakeSQLRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.resp.Rows) {
		if r.resp.IterErr != nil {
			return r.resp.IterErr
		}
		return io.EOF
	}
	copy(dest, r.resp.Rows[r.pos])
	r.pos++
	return nil
}

type fakeSQLResult struct{ resp FakeSQLResponse }

func (r fakeSQLResult) LastInsertId() (int64, error) { return 0, errors.New("not supported") }
func (r fakeSQLResult) RowsAffected() (int64, error) { return r.resp.RowsAffected, r.resp.AffectedErr }
