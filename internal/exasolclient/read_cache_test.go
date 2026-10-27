package exasolclient

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
)

// viewDriver serves one fixed view and counts how often it is queried.
type viewDriver struct {
	queries atomic.Int32
	fail    atomic.Bool
}

var viewRows = [][]driver.Value{
	{"ROLE_A", "USER_X", "true"},
	{"ROLE_A", "USER_Y", "false"},
	{nil, "USER_Z", "false"}, // NULL key: matches nothing
}

func (d *viewDriver) Connect(context.Context) (driver.Conn, error) { return viewConn{d}, nil }
func (d *viewDriver) Driver() driver.Driver                        { return nil }

type viewConn struct{ d *viewDriver }

func (c viewConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (c viewConn) Close() error                        { return nil }
func (c viewConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }

func (c viewConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	c.d.queries.Add(1)
	if c.d.fail.Load() {
		return nil, errors.New("view not reachable")
	}
	return &viewResult{}, nil
}

type viewResult struct{ i int }

func (r *viewResult) Columns() []string { return []string{"GRANTED_ROLE", "GRANTEE", "ADMIN_OPTION"} }
func (r *viewResult) Close() error      { return nil }
func (r *viewResult) Next(dest []driver.Value) error {
	if r.i == len(viewRows) {
		return io.EOF
	}
	copy(dest, viewRows[r.i])
	r.i++
	return nil
}

func TestReadCache(t *testing.T) {
	d := &viewDriver{}
	db := sql.OpenDB(d)
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	const q = `SELECT GRANTED_ROLE, GRANTEE, ADMIN_OPTION FROM EXA_DBA_ROLE_PRIVS`
	c := NewReadCache()

	// Parallel reads share one load.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := c.Rows(ctx, db, q, "ROLE_A", "USER_X")
			if err != nil || len(rows) != 1 || rows[0][2].String != "true" {
				t.Errorf("rows = %v, err = %v; want one row with ADMIN_OPTION true", rows, err)
			}
		}()
	}
	wg.Wait()
	if n := d.queries.Load(); n != 1 {
		t.Fatalf("queries = %d, want 1", n)
	}

	if rows, _ := c.Rows(ctx, db, q, "role_a", "USER_X"); len(rows) != 0 {
		t.Error("lookup is not case-sensitive")
	}
	if rows, _ := c.Rows(ctx, db, q, "", "USER_Z"); len(rows) != 0 {
		t.Error("a NULL key column matched")
	}

	// Clear makes the next read load again.
	c.Clear()
	if _, err := c.Rows(ctx, db, q, "ROLE_A", "USER_Y"); err != nil {
		t.Fatal(err)
	}
	if n := d.queries.Load(); n != 2 {
		t.Fatalf("queries after Clear = %d, want 2", n)
	}

	// A failed load is kept until Clear.
	c.Clear()
	d.fail.Store(true)
	for i := 0; i < 3; i++ {
		if _, err := c.Rows(ctx, db, q, "ROLE_A", "USER_X"); err == nil {
			t.Fatal("want the load error")
		}
	}
	if n := d.queries.Load(); n != 3 {
		t.Fatalf("queries after failed loads = %d, want 3", n)
	}
}
