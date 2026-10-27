package resources

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"terraform-provider-exasol/internal/exasolclient"
)

// collidingDriver fails the first `fail` statements with a transaction
// collision, like Exasol under parallel GRANT, REVOKE and DROP statements,
// or with `err` when set.
type collidingDriver struct {
	fail, calls int
	err         error
}

func (d *collidingDriver) Open(string) (driver.Conn, error) { return collidingConn{d}, nil }

type collidingConn struct{ d *collidingDriver }

func (c collidingConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (c collidingConn) Close() error                        { return nil }
func (c collidingConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }

func (c collidingConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.d.calls++
	if c.d.err != nil {
		return nil, c.d.err
	}
	if c.d.calls <= c.d.fail {
		return nil, errors.New("E-EGOD-11: execution failed with SQL error code '40001' and message 'GlobalTransactionRollback msg: Transaction collision: automatic transaction rollback.'")
	}
	return driver.RowsAffected(0), nil
}

func TestExecWriteRetriesCollision(t *testing.T) {
	defer func(w func(int) time.Duration) { collisionWait = w }(collisionWait)
	collisionWait = func(int) time.Duration { return 0 }
	for _, tc := range []struct {
		name      string
		fail      int
		wantErr   bool
		wantCalls int
	}{
		{"succeeds after collisions", 2, false, 3},
		{"gives up after retries", collisionRetries + 5, true, collisionRetries + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &collidingDriver{fail: tc.fail}
			db := sql.OpenDB(collidingConnector{d})
			defer func() { _ = db.Close() }()
			_, err := execWrite(context.Background(), db, exasolclient.NewReadCache(), `GRANT IMPERSONATION ON "A" TO "B"`)
			if (err != nil) != tc.wantErr || d.calls != tc.wantCalls {
				t.Fatalf("err = %v, calls = %d; want error %v, calls %d", err, d.calls, tc.wantErr, tc.wantCalls)
			}
		})
	}
}

type collidingConnector struct{ d *collidingDriver }

func (c collidingConnector) Connect(context.Context) (driver.Conn, error) {
	return collidingConn(c), nil
}
func (c collidingConnector) Driver() driver.Driver { return c.d }

func TestLockGrantees(t *testing.T) {
	done := make(chan struct{})
	go func() {
		// The same grantee in two spellings must not lock itself.
		unlock := lockGrantees("hub", "HUB", "Other")
		// A different grantee is not blocked by it.
		lockGrantees("THIRD")()
		unlock()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("lockGrantees deadlocked")
	}

	unlock := lockGrantees("HUB")
	acquired := make(chan struct{})
	go func() {
		lockGrantees("hub")()
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("second lock on the same grantee did not wait")
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	<-acquired
}

func TestExecWriteDoesNotRetryOtherErrors(t *testing.T) {
	d := &collidingDriver{err: errors.New("E-EGOD-11: execution failed with SQL error code '42500' and message 'user or role X does not exist'")}
	db := sql.OpenDB(collidingConnector{d})
	defer func() { _ = db.Close() }()
	if _, err := execWrite(context.Background(), db, nil, `GRANT "A" TO "X"`); err == nil || d.calls != 1 {
		t.Fatalf("err = %v, calls = %d; want the error after 1 call", err, d.calls)
	}
}

func TestExecWriteStopsOnCancel(t *testing.T) {
	defer func(w func(int) time.Duration) { collisionWait = w }(collisionWait)
	collisionWait = func(int) time.Duration { return time.Hour }
	d := &collidingDriver{fail: 1}
	db := sql.OpenDB(collidingConnector{d})
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := execWrite(ctx, db, nil, `GRANT "A" TO "X"`)
	if err == nil || !strings.Contains(err.Error(), "retry stopped") || !isTransactionCollision(err) {
		t.Fatalf("err = %v; want the collision error with the stop reason", err)
	}
}

// fakeViewDriver serves fixed rows for any query.
type fakeViewDriver struct {
	cols    []string
	rows    [][]driver.Value
	queries int
}

func (d *fakeViewDriver) Connect(context.Context) (driver.Conn, error) { return fakeViewConn{d}, nil }
func (d *fakeViewDriver) Driver() driver.Driver                        { return nil }

type fakeViewConn struct{ d *fakeViewDriver }

func (c fakeViewConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (c fakeViewConn) Close() error                        { return nil }
func (c fakeViewConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }
func (c fakeViewConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}
func (c fakeViewConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	c.d.queries++
	return &fakeViewRows{d: c.d}, nil
}

type fakeViewRows struct {
	d *fakeViewDriver
	i int
}

func (r *fakeViewRows) Columns() []string { return r.d.cols }
func (r *fakeViewRows) Close() error      { return nil }
func (r *fakeViewRows) Next(dest []driver.Value) error {
	if r.i == len(r.d.rows) {
		return io.EOF
	}
	copy(dest, r.d.rows[r.i])
	r.i++
	return nil
}

func TestExecWriteClearsReadCache(t *testing.T) {
	d := &fakeViewDriver{cols: []string{"ROLE_NAME"}, rows: [][]driver.Value{{"R"}}}
	db := sql.OpenDB(d)
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	reads := exasolclient.NewReadCache()
	for i := 0; i < 2; i++ {
		if _, err := reads.Rows(ctx, db, `SELECT ROLE_NAME FROM EXA_DBA_ROLES`, "R"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := execWrite(ctx, db, reads, `DROP ROLE "R"`); err != nil {
		t.Fatal(err)
	}
	if _, err := reads.Rows(ctx, db, `SELECT ROLE_NAME FROM EXA_DBA_ROLES`, "R"); err != nil {
		t.Fatal(err)
	}
	if d.queries != 2 {
		t.Fatalf("view queries = %d, want 2 (one load, one after the write)", d.queries)
	}
}

// cachedObjectPrivilegeExists must match the rows the old WHERE lookup in
// checkObjectPrivilegeExists matched.
func TestCachedObjectPrivilegeExists(t *testing.T) {
	d := &fakeViewDriver{
		cols: []string{"GRANTEE", "OBJECT_TYPE", "OBJECT_NAME", "OBJECT_SCHEMA", "PRIVILEGE"},
		rows: [][]driver.Value{
			{"R", "SCHEMA", "S1", nil, "USAGE"},
			{"R", "TABLE", "T", "S1", "SELECT"},
			{"R", "TABLE", "T", "S1", "INSERT"}, // ALL expanded by Exasol
			{"R", "TABLE", "T", nil, "DELETE"},  // no schema: never matches a qualified name
		},
	}
	db := sql.OpenDB(d)
	defer func() { _ = db.Close() }()
	reads := exasolclient.NewReadCache()
	for _, tc := range []struct {
		priv, typ, name string
		want            bool
	}{
		{"USAGE", "SCHEMA", "S1", true},
		{"USAGE", "SCHEMA", `"S1"`, true},
		{"SELECT", "SCHEMA", "S1", false},
		{"SELECT", "TABLE", "S1.T", true},
		{"SELECT", "TABLE", "S2.T", false},
		{"ALL", "TABLE", "S1.T", true},
		{"ALL", "TABLE", "S2.T", false},
		{"DELETE", "TABLE", "S1.T", false},
		{"DELETE", "TABLE", "T", true},
		{"SELECT", "TABLE", "s1.t", false},
		{"USAGE", "SCHEMA", "S1", true},
	} {
		got, err := cachedObjectPrivilegeExists(context.Background(), db, reads, "R", tc.priv, tc.typ, tc.name)
		if err != nil || got != tc.want {
			t.Errorf("%s %s %s: got %v, %v; want %v", tc.priv, tc.typ, tc.name, got, err, tc.want)
		}
	}
	if d.queries != 1 {
		t.Errorf("view queries = %d, want 1", d.queries)
	}
}
