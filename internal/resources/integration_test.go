//go:build integration

package resources

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/exasol/exasol-driver-go"
)

// testDB returns a database connection for integration tests.
// It reads connection details from environment variables with Docker defaults.
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	host := envOrDefault("EXASOL_HOST", "localhost")
	port := envOrDefault("EXASOL_PORT", "8563")
	user := envOrDefault("EXASOL_USER", "sys")
	password := envOrDefault("EXASOL_PASSWORD", "exasol")

	dsnStr := exasol.NewConfig(user, password).
		Host(host).
		Port(mustAtoi(port)).
		ValidateServerCertificate(false).
		String()

	db, err := sql.Open("exasol", dsnStr)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("Failed to ping database (is Exasol running?): %v", err)
	}

	return db
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustAtoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// execSQL is a test helper that executes SQL and fails the test on error.
func execSQL(t *testing.T, db *sql.DB, sql string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), sql); err != nil {
		t.Fatalf("SQL exec failed: %s\nError: %v", sql, err)
	}
}

// cleanup registers a SQL statement to run during test cleanup.
func cleanup(t *testing.T, db *sql.DB, sql string) {
	t.Helper()
	t.Cleanup(func() {
		db.ExecContext(context.Background(), sql) // best effort, ignore errors
	})
}

// --- RENAME tests ---

func TestIntegration_RenameUser(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	execSQL(t, db, `CREATE USER "TEST_RENAME_USER_OLD" IDENTIFIED BY "pass123"`)
	cleanup(t, db, `DROP USER "TEST_RENAME_USER_NEW"`)
	cleanup(t, db, `DROP USER "TEST_RENAME_USER_OLD"`)

	execSQL(t, db, `RENAME USER "TEST_RENAME_USER_OLD" TO "TEST_RENAME_USER_NEW"`)

	// Verify old name is gone
	var count int
	err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM EXA_DBA_USERS WHERE USER_NAME = 'TEST_RENAME_USER_OLD'`).Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 0 {
		t.Error("Old user name still exists after RENAME")
	}

	// Verify new name exists
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM EXA_DBA_USERS WHERE USER_NAME = 'TEST_RENAME_USER_NEW'`).Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 1 {
		t.Error("New user name not found after RENAME")
	}
}

func TestIntegration_RenameRole(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	execSQL(t, db, `CREATE ROLE "TEST_RENAME_ROLE_OLD"`)
	cleanup(t, db, `DROP ROLE "TEST_RENAME_ROLE_NEW"`)
	cleanup(t, db, `DROP ROLE "TEST_RENAME_ROLE_OLD"`)

	execSQL(t, db, `RENAME ROLE "TEST_RENAME_ROLE_OLD" TO "TEST_RENAME_ROLE_NEW"`)

	var count int
	err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM EXA_DBA_ROLES WHERE ROLE_NAME = 'TEST_RENAME_ROLE_NEW'`).Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 1 {
		t.Error("New role name not found after RENAME")
	}
}

func TestIntegration_RenameSchema(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	execSQL(t, db, `CREATE SCHEMA "TEST_RENAME_SCHEMA_OLD"`)
	cleanup(t, db, `DROP SCHEMA "TEST_RENAME_SCHEMA_NEW" CASCADE`)
	cleanup(t, db, `DROP SCHEMA "TEST_RENAME_SCHEMA_OLD" CASCADE`)

	execSQL(t, db, `RENAME SCHEMA "TEST_RENAME_SCHEMA_OLD" TO "TEST_RENAME_SCHEMA_NEW"`)

	var count int
	err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM EXA_DBA_SCHEMAS WHERE SCHEMA_NAME = 'TEST_RENAME_SCHEMA_NEW'`).Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 1 {
		t.Error("New schema name not found after RENAME")
	}
}

func TestIntegration_RenameConnection(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	execSQL(t, db, `CREATE CONNECTION "TEST_RENAME_CONN_OLD" TO 'localhost:8563'`)
	cleanup(t, db, `DROP CONNECTION "TEST_RENAME_CONN_NEW"`)
	cleanup(t, db, `DROP CONNECTION "TEST_RENAME_CONN_OLD"`)

	execSQL(t, db, `RENAME CONNECTION "TEST_RENAME_CONN_OLD" TO "TEST_RENAME_CONN_NEW"`)

	var count int
	err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM EXA_DBA_CONNECTIONS WHERE CONNECTION_NAME = 'TEST_RENAME_CONN_NEW'`).Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 1 {
		t.Error("New connection name not found after RENAME")
	}
}

// --- CRUD lifecycle tests ---

func TestIntegration_UserLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cleanup(t, db, `DROP USER "TEST_LIFECYCLE_USER"`)

	// Create
	execSQL(t, db, `CREATE USER "TEST_LIFECYCLE_USER" IDENTIFIED BY "testpass123"`)

	// Read
	var userName string
	err := db.QueryRowContext(context.Background(),
		`SELECT USER_NAME FROM EXA_DBA_USERS WHERE USER_NAME = 'TEST_LIFECYCLE_USER'`).Scan(&userName)
	if err != nil {
		t.Fatalf("Read after create failed: %v", err)
	}
	if userName != "TEST_LIFECYCLE_USER" {
		t.Errorf("Expected user name TEST_LIFECYCLE_USER, got %s", userName)
	}

	// Update (change password)
	execSQL(t, db, `ALTER USER "TEST_LIFECYCLE_USER" IDENTIFIED BY "newpass456"`)

	// Delete
	execSQL(t, db, `DROP USER "TEST_LIFECYCLE_USER"`)

	// Verify deletion
	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM EXA_DBA_USERS WHERE USER_NAME = 'TEST_LIFECYCLE_USER'`).Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 0 {
		t.Error("User still exists after DROP")
	}
}

func TestIntegration_RoleLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cleanup(t, db, `DROP ROLE "TEST_LIFECYCLE_ROLE"`)

	execSQL(t, db, `CREATE ROLE "TEST_LIFECYCLE_ROLE"`)

	var roleName string
	err := db.QueryRowContext(context.Background(),
		`SELECT ROLE_NAME FROM EXA_DBA_ROLES WHERE ROLE_NAME = 'TEST_LIFECYCLE_ROLE'`).Scan(&roleName)
	if err != nil {
		t.Fatalf("Read after create failed: %v", err)
	}

	execSQL(t, db, `DROP ROLE "TEST_LIFECYCLE_ROLE"`)

	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM EXA_DBA_ROLES WHERE ROLE_NAME = 'TEST_LIFECYCLE_ROLE'`).Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 0 {
		t.Error("Role still exists after DROP")
	}
}

func TestIntegration_SchemaLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cleanup(t, db, `DROP SCHEMA "TEST_LIFECYCLE_SCHEMA" CASCADE`)

	execSQL(t, db, `CREATE SCHEMA "TEST_LIFECYCLE_SCHEMA"`)

	var owner string
	err := db.QueryRowContext(context.Background(),
		`SELECT SCHEMA_OWNER FROM EXA_DBA_SCHEMAS WHERE SCHEMA_NAME = 'TEST_LIFECYCLE_SCHEMA'`).Scan(&owner)
	if err != nil {
		t.Fatalf("Read after create failed: %v", err)
	}
	if owner != "SYS" {
		t.Errorf("Expected owner SYS, got %s", owner)
	}

	// Change owner
	execSQL(t, db, `CREATE ROLE "TEST_SCHEMA_OWNER"`)
	cleanup(t, db, `DROP ROLE "TEST_SCHEMA_OWNER"`)
	execSQL(t, db, `ALTER SCHEMA "TEST_LIFECYCLE_SCHEMA" CHANGE OWNER "TEST_SCHEMA_OWNER"`)

	err = db.QueryRowContext(context.Background(),
		`SELECT SCHEMA_OWNER FROM EXA_DBA_SCHEMAS WHERE SCHEMA_NAME = 'TEST_LIFECYCLE_SCHEMA'`).Scan(&owner)
	if err != nil {
		t.Fatalf("Read after ownership change failed: %v", err)
	}
	if owner != "TEST_SCHEMA_OWNER" {
		t.Errorf("Expected owner TEST_SCHEMA_OWNER, got %s", owner)
	}

	execSQL(t, db, `DROP SCHEMA "TEST_LIFECYCLE_SCHEMA" CASCADE`)
}

func TestIntegration_ConnectionLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	cleanup(t, db, `DROP CONNECTION "TEST_LIFECYCLE_CONN"`)

	execSQL(t, db, `CREATE CONNECTION "TEST_LIFECYCLE_CONN" TO 'localhost:8563' USER 'sys' IDENTIFIED BY 'exasol'`)

	var dummy int
	err := db.QueryRowContext(context.Background(),
		`SELECT 1 FROM EXA_DBA_CONNECTIONS WHERE CONNECTION_NAME = 'TEST_LIFECYCLE_CONN'`).Scan(&dummy)
	if err != nil {
		t.Fatalf("Read after create failed: %v", err)
	}

	// Update connection string
	execSQL(t, db, `ALTER CONNECTION "TEST_LIFECYCLE_CONN" TO 'localhost:9999'`)

	execSQL(t, db, `DROP CONNECTION "TEST_LIFECYCLE_CONN"`)

	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM EXA_DBA_CONNECTIONS WHERE CONNECTION_NAME = 'TEST_LIFECYCLE_CONN'`).Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 0 {
		t.Error("Connection still exists after DROP")
	}
}

// --- Privilege tests ---

func TestIntegration_SystemPrivilege(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	execSQL(t, db, `CREATE USER "TEST_SYSPRIV_USER" IDENTIFIED BY "pass123"`)
	cleanup(t, db, `DROP USER "TEST_SYSPRIV_USER"`)

	execSQL(t, db, `GRANT CREATE SESSION TO "TEST_SYSPRIV_USER"`)

	var adminOption string
	err := db.QueryRowContext(context.Background(),
		`SELECT ADMIN_OPTION FROM EXA_DBA_SYS_PRIVS WHERE GRANTEE = 'TEST_SYSPRIV_USER' AND PRIVILEGE = 'CREATE SESSION'`).Scan(&adminOption)
	if err != nil {
		t.Fatalf("Read privilege failed: %v", err)
	}

	// Grant with admin option
	execSQL(t, db, `REVOKE CREATE SESSION FROM "TEST_SYSPRIV_USER"`)
	execSQL(t, db, `GRANT CREATE SESSION TO "TEST_SYSPRIV_USER" WITH ADMIN OPTION`)

	err = db.QueryRowContext(context.Background(),
		`SELECT ADMIN_OPTION FROM EXA_DBA_SYS_PRIVS WHERE GRANTEE = 'TEST_SYSPRIV_USER' AND PRIVILEGE = 'CREATE SESSION'`).Scan(&adminOption)
	if err != nil {
		t.Fatalf("Read privilege with admin option failed: %v", err)
	}
	// Docker Exasol returns lowercase "true"
	if adminOption != "TRUE" && adminOption != "true" && adminOption != "1" {
		t.Errorf("Expected admin_option TRUE, got %q", adminOption)
	}

	execSQL(t, db, `REVOKE CREATE SESSION FROM "TEST_SYSPRIV_USER"`)
}

func TestIntegration_RoleGrant(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	execSQL(t, db, `CREATE ROLE "TEST_GRANT_ROLE"`)
	execSQL(t, db, `CREATE USER "TEST_GRANT_USER" IDENTIFIED BY "pass123"`)
	cleanup(t, db, `DROP USER "TEST_GRANT_USER"`)
	cleanup(t, db, `DROP ROLE "TEST_GRANT_ROLE"`)

	execSQL(t, db, `GRANT "TEST_GRANT_ROLE" TO "TEST_GRANT_USER"`)

	var adminOption string
	err := db.QueryRowContext(context.Background(),
		`SELECT ADMIN_OPTION FROM EXA_DBA_ROLE_PRIVS WHERE GRANTED_ROLE = 'TEST_GRANT_ROLE' AND GRANTEE = 'TEST_GRANT_USER'`).Scan(&adminOption)
	if err != nil {
		t.Fatalf("Read role grant failed: %v", err)
	}

	execSQL(t, db, `REVOKE "TEST_GRANT_ROLE" FROM "TEST_GRANT_USER"`)

	// With admin option
	execSQL(t, db, `GRANT "TEST_GRANT_ROLE" TO "TEST_GRANT_USER" WITH ADMIN OPTION`)

	err = db.QueryRowContext(context.Background(),
		`SELECT ADMIN_OPTION FROM EXA_DBA_ROLE_PRIVS WHERE GRANTED_ROLE = 'TEST_GRANT_ROLE' AND GRANTEE = 'TEST_GRANT_USER'`).Scan(&adminOption)
	if err != nil {
		t.Fatalf("Read role grant with admin option failed: %v", err)
	}
	if adminOption != "TRUE" && adminOption != "true" && adminOption != "1" {
		t.Errorf("Expected admin_option TRUE, got %q", adminOption)
	}

	execSQL(t, db, `REVOKE "TEST_GRANT_ROLE" FROM "TEST_GRANT_USER"`)
}

func TestIntegration_ObjectPrivilege(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	execSQL(t, db, `CREATE SCHEMA "TEST_OBJPRIV_SCHEMA"`)
	execSQL(t, db, `CREATE ROLE "TEST_OBJPRIV_ROLE"`)
	cleanup(t, db, `DROP SCHEMA "TEST_OBJPRIV_SCHEMA" CASCADE`)
	cleanup(t, db, `DROP ROLE "TEST_OBJPRIV_ROLE"`)

	execSQL(t, db, `GRANT SELECT ON SCHEMA "TEST_OBJPRIV_SCHEMA" TO "TEST_OBJPRIV_ROLE"`)

	var dummy int
	err := db.QueryRowContext(context.Background(),
		`SELECT 1 FROM EXA_DBA_OBJ_PRIVS WHERE GRANTEE = 'TEST_OBJPRIV_ROLE' AND PRIVILEGE = 'SELECT' AND OBJECT_NAME = 'TEST_OBJPRIV_SCHEMA'`).Scan(&dummy)
	if err != nil {
		t.Fatalf("Read object privilege failed: %v", err)
	}

	execSQL(t, db, `REVOKE SELECT ON SCHEMA "TEST_OBJPRIV_SCHEMA" FROM "TEST_OBJPRIV_ROLE"`)
}

func TestIntegration_ConnectionGrant(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	execSQL(t, db, `CREATE CONNECTION "TEST_CONNPRIV_CONN" TO 'localhost:8563'`)
	execSQL(t, db, `CREATE ROLE "TEST_CONNPRIV_ROLE"`)
	cleanup(t, db, `DROP CONNECTION "TEST_CONNPRIV_CONN"`)
	cleanup(t, db, `DROP ROLE "TEST_CONNPRIV_ROLE"`)

	execSQL(t, db, `GRANT CONNECTION "TEST_CONNPRIV_CONN" TO "TEST_CONNPRIV_ROLE"`)

	var dummy int
	err := db.QueryRowContext(context.Background(),
		`SELECT 1 FROM EXA_DBA_CONNECTION_PRIVS WHERE GRANTED_CONNECTION = 'TEST_CONNPRIV_CONN' AND GRANTEE = 'TEST_CONNPRIV_ROLE'`).Scan(&dummy)
	if err != nil {
		t.Fatalf("Read connection grant failed: %v", err)
	}

	execSQL(t, db, `REVOKE CONNECTION "TEST_CONNPRIV_CONN" FROM "TEST_CONNPRIV_ROLE"`)
}

