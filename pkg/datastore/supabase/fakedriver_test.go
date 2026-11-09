package supabase_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
)

// fakeResponse is what the fake driver returns for one statement.
type fakeResponse struct {
	columns      []string
	rows         [][]driver.Value
	rowsAffected int64
	err          error // returned by the query or exec call itself
	affectedErr  error // returned by Result.RowsAffected
	iterErr      error // returned by Rows.Next after the last row
}

// fakeCall records one statement sent to the fake driver.
type fakeCall struct {
	query string
	args  []driver.Value
}

// fakeDB is a minimal database/sql driver that records statements and
// replies with scripted responses. It lets the tests check the SQL the
// client builds without a real PostgreSQL server.
type fakeDB struct {
	mu          sync.Mutex
	calls       []fakeCall
	respond     func(query string) fakeResponse
	pingErr     error
	beginErr    error
	commitErr   error
	rollbackErr error
	committed   int
	rolledBack  int
}

func (f *fakeDB) open() *sql.DB {
	return sql.OpenDB(fakeConnector{f})
}

func (f *fakeDB) record(query string, args []driver.NamedValue) fakeResponse {
	values := make([]driver.Value, len(args))
	for i, a := range args {
		values[i] = a.Value
	}
	f.mu.Lock()
	f.calls = append(f.calls, fakeCall{query: query, args: values})
	respond := f.respond
	f.mu.Unlock()
	if respond == nil {
		return fakeResponse{}
	}
	return respond(query)
}

func (f *fakeDB) lastCall() fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return fakeCall{}
	}
	return f.calls[len(f.calls)-1]
}

type fakeConnector struct{ db *fakeDB }

func (c fakeConnector) Connect(context.Context) (driver.Conn, error) { return &fakeConn{db: c.db}, nil }
func (c fakeConnector) Driver() driver.Driver                        { return fakeDriver{} }

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use the connector") }

type fakeConn struct{ db *fakeDB }

func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *fakeConn) Close() error { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *fakeConn) Ping(context.Context) error { return c.db.pingErr }

func (c *fakeConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	if c.db.beginErr != nil {
		return nil, c.db.beginErr
	}
	return &fakeTx{db: c.db}, nil
}

func (c *fakeConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	resp := c.db.record(query, args)
	if resp.err != nil {
		return nil, resp.err
	}
	return &fakeRows{resp: resp}, nil
}

func (c *fakeConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	resp := c.db.record(query, args)
	if resp.err != nil {
		return nil, resp.err
	}
	return fakeResult{resp: resp}, nil
}

type fakeTx struct{ db *fakeDB }

func (t *fakeTx) Commit() error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	t.db.committed++
	return t.db.commitErr
}

func (t *fakeTx) Rollback() error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	t.db.rolledBack++
	return t.db.rollbackErr
}

type fakeRows struct {
	resp fakeResponse
	pos  int
}

func (r *fakeRows) Columns() []string { return r.resp.columns }
func (r *fakeRows) Close() error      { return nil }

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.resp.rows) {
		if r.resp.iterErr != nil {
			return r.resp.iterErr
		}
		return io.EOF
	}
	copy(dest, r.resp.rows[r.pos])
	r.pos++
	return nil
}

type fakeResult struct{ resp fakeResponse }

func (r fakeResult) LastInsertId() (int64, error) { return 0, errors.New("not supported") }
func (r fakeResult) RowsAffected() (int64, error) { return r.resp.rowsAffected, r.resp.affectedErr }
