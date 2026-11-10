package testutil

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
)

func TestFakeSQL_QueryReturnsScriptedRowsAndRecordsCalls(t *testing.T) {
	f := &FakeSQL{Respond: func(string) FakeSQLResponse {
		return FakeSQLResponse{Columns: []string{"id", "name"}, Rows: [][]driver.Value{{int64(1), "a"}, {int64(2), "b"}}}
	}}
	db := f.Open()
	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.QueryContext(context.Background(), "SELECT id, name FROM t WHERE x = $1", "v")
	if err != nil {
		t.Fatalf("QueryContext() error = %v", err)
	}
	t.Cleanup(func() { _ = rows.Close() })

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("Columns() error = %v", err)
	}
	if !reflect.DeepEqual(cols, []string{"id", "name"}) {
		t.Errorf("Columns() = %v", cols)
	}

	var got []string
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err() = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("names = %v, want [a b]", got)
	}

	want := FakeSQLCall{Query: "SELECT id, name FROM t WHERE x = $1", Args: []driver.Value{"v"}}
	if calls := f.Calls(); len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
		t.Errorf("Calls() = %+v, want [%+v]", calls, want)
	}
	if last := f.LastCall(); !reflect.DeepEqual(last, want) {
		t.Errorf("LastCall() = %+v, want %+v", last, want)
	}
}

func TestFakeSQL_LastCallIsZeroWithoutCalls(t *testing.T) {
	f := &FakeSQL{}
	if last := f.LastCall(); !reflect.DeepEqual(last, FakeSQLCall{}) {
		t.Errorf("LastCall() = %+v, want the zero value", last)
	}
}

func TestFakeSQL_ScriptedErrors(t *testing.T) {
	queryErr := errors.New("query failed")
	iterErr := errors.New("iteration failed")
	affectedErr := errors.New("affected failed")
	ctx := context.Background()

	t.Run("query error", func(t *testing.T) {
		f := &FakeSQL{Respond: func(string) FakeSQLResponse { return FakeSQLResponse{Err: queryErr} }}
		db := f.Open()
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.QueryContext(ctx, "SELECT 1"); !errors.Is(err, queryErr) {
			t.Errorf("QueryContext() error = %v, want %v", err, queryErr)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM t"); !errors.Is(err, queryErr) {
			t.Errorf("ExecContext() error = %v, want %v", err, queryErr)
		}
	})

	t.Run("iteration error", func(t *testing.T) {
		f := &FakeSQL{Respond: func(string) FakeSQLResponse {
			return FakeSQLResponse{Columns: []string{"id"}, IterErr: iterErr}
		}}
		db := f.Open()
		t.Cleanup(func() { _ = db.Close() })
		rows, err := db.QueryContext(ctx, "SELECT id FROM t")
		if err != nil {
			t.Fatalf("QueryContext() error = %v", err)
		}
		t.Cleanup(func() { _ = rows.Close() })
		for rows.Next() {
			t.Fatal("Next() = true, want no rows")
		}
		if err := rows.Err(); !errors.Is(err, iterErr) {
			t.Errorf("rows.Err() = %v, want %v", err, iterErr)
		}
	})

	t.Run("exec result", func(t *testing.T) {
		f := &FakeSQL{Respond: func(string) FakeSQLResponse {
			return FakeSQLResponse{RowsAffected: 3, AffectedErr: affectedErr}
		}}
		db := f.Open()
		t.Cleanup(func() { _ = db.Close() })
		res, err := db.ExecContext(ctx, "UPDATE t SET x = 1")
		if err != nil {
			t.Fatalf("ExecContext() error = %v", err)
		}
		n, err := res.RowsAffected()
		if n != 3 || !errors.Is(err, affectedErr) {
			t.Errorf("RowsAffected() = %d, %v, want 3, %v", n, err, affectedErr)
		}
		if _, err := res.LastInsertId(); err == nil {
			t.Error("LastInsertId() error = nil, want an error")
		}
	})

	t.Run("nil respond answers empty", func(t *testing.T) {
		f := &FakeSQL{}
		db := f.Open()
		t.Cleanup(func() { _ = db.Close() })
		res, err := db.ExecContext(ctx, "UPDATE t SET x = 1")
		if err != nil {
			t.Fatalf("ExecContext() error = %v", err)
		}
		if n, _ := res.RowsAffected(); n != 0 {
			t.Errorf("RowsAffected() = %d, want 0", n)
		}
	})
}

func TestFakeSQL_PingAndBeginErrors(t *testing.T) {
	pingErr := errors.New("ping failed")
	beginErr := errors.New("begin failed")
	f := &FakeSQL{PingErr: pingErr, BeginErr: beginErr}
	db := f.Open()
	t.Cleanup(func() { _ = db.Close() })

	if err := db.PingContext(context.Background()); !errors.Is(err, pingErr) {
		t.Errorf("PingContext() error = %v, want %v", err, pingErr)
	}
	if _, err := db.BeginTx(context.Background(), nil); !errors.Is(err, beginErr) {
		t.Errorf("BeginTx() error = %v, want %v", err, beginErr)
	}
}

func TestFakeSQL_CountsCommitsAndRollbacks(t *testing.T) {
	commitErr := errors.New("commit failed")
	rollbackErr := errors.New("rollback failed")
	f := &FakeSQL{CommitErr: commitErr, RollbackErr: rollbackErr}
	db := f.Open()
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := tx.Commit(); !errors.Is(err, commitErr) {
		t.Errorf("Commit() error = %v, want %v", err, commitErr)
	}

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := tx.Rollback(); !errors.Is(err, rollbackErr) {
		t.Errorf("Rollback() error = %v, want %v", err, rollbackErr)
	}

	if f.Committed() != 1 || f.RolledBack() != 1 {
		t.Errorf("Committed() = %d, RolledBack() = %d, want 1 and 1", f.Committed(), f.RolledBack())
	}
}

func TestFakeSQL_DriverInternals(t *testing.T) {
	f := &FakeSQL{}
	connector := fakeSQLConnector{f}

	if _, err := connector.Driver().Open("dsn"); err == nil {
		t.Error("Driver().Open() error = nil, want an error")
	}

	conn, err := connector.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if _, err := conn.Prepare("SELECT 1"); err == nil {
		t.Error("Prepare() error = nil, want an error")
	}
	tx, err := conn.Begin() //nolint:staticcheck // Begin is part of driver.Conn.
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Errorf("Commit() error = %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
	if f.Committed() != 1 {
		t.Errorf("Committed() = %d, want 1", f.Committed())
	}
}
