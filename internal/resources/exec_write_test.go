package resources

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"terraform-provider-exasol/internal/exasolclient"
)

// collidingDriver fails the first `fail` statements with a transaction
// collision, like Exasol under parallel GRANT, REVOKE and DROP statements.
type collidingDriver struct{ fail, calls int }

func (d *collidingDriver) Open(string) (driver.Conn, error) { return collidingConn{d}, nil }

type collidingConn struct{ d *collidingDriver }

func (c collidingConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (c collidingConn) Close() error                        { return nil }
func (c collidingConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }

func (c collidingConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.d.calls++
	if c.d.calls <= c.d.fail {
		return nil, errors.New("E-EGOD-11: execution failed with SQL error code '40001' and message 'GlobalTransactionRollback msg: Transaction collision: automatic transaction rollback.'")
	}
	return driver.RowsAffected(0), nil
}

func TestExecWriteRetriesCollision(t *testing.T) {
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
			db := sql.OpenDB(connector{d})
			defer func() { _ = db.Close() }()
			_, err := execWrite(context.Background(), db, exasolclient.NewReadCache(), `GRANT IMPERSONATION ON "A" TO "B"`)
			if (err != nil) != tc.wantErr || d.calls != tc.wantCalls {
				t.Fatalf("err = %v, calls = %d; want error %v, calls %d", err, d.calls, tc.wantErr, tc.wantCalls)
			}
		})
	}
}

type connector struct{ d *collidingDriver }

func (c connector) Connect(context.Context) (driver.Conn, error) { return collidingConn(c), nil }
func (c connector) Driver() driver.Driver                        { return c.d }
