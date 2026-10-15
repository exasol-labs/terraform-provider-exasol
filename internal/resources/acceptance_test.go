//go:build integration

package resources_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"terraform-provider-exasol/internal/provider"

	"github.com/exasol/exasol-driver-go"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// --- Test infrastructure ---

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"exasol": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func providerConfig() string {
	host := envOrDefault("EXASOL_HOST", "localhost")
	port := envOrDefault("EXASOL_PORT", "8563")
	user := envOrDefault("EXASOL_USER", "sys")
	password := envOrDefault("EXASOL_PASSWORD", "exasol")

	return `
provider "exasol" {
  host                        = "` + host + `"
  port                        = ` + port + `
  user                        = "` + user + `"
  password                    = "` + password + `"
  validate_server_certificate = false
}
`
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	db := testAccDB(t)
	defer db.Close()
}

func testAccDB(t *testing.T) *sql.DB {
	t.Helper()
	host := envOrDefault("EXASOL_HOST", "localhost")
	user := envOrDefault("EXASOL_USER", "sys")
	password := envOrDefault("EXASOL_PASSWORD", "exasol")

	dsn := exasol.NewConfig(user, password).
		Host(host).
		Port(8563).
		ValidateServerCertificate(false).
		String()

	db, err := sql.Open("exasol", dsn)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("Exasol not reachable (is Docker container running?): %v", err)
	}
	return db
}

// --- CheckDestroy functions ---

func testAccCheckRoleDestroy(s *terraform.State) error {
	db := mustOpenDB()
	defer db.Close()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "exasol_role" {
			continue
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_ROLES WHERE ROLE_NAME = ?`, rs.Primary.ID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("role %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckUserDestroy(s *terraform.State) error {
	db := mustOpenDB()
	defer db.Close()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "exasol_user" {
			continue
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_USERS WHERE USER_NAME = ?`, rs.Primary.ID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("user %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckSchemaDestroy(s *terraform.State) error {
	db := mustOpenDB()
	defer db.Close()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "exasol_schema" {
			continue
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_SCHEMAS WHERE SCHEMA_NAME = ?`, rs.Primary.ID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("schema %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckConnectionDestroy(s *terraform.State) error {
	db := mustOpenDB()
	defer db.Close()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "exasol_connection" {
			continue
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_CONNECTIONS WHERE CONNECTION_NAME = ?`, rs.Primary.ID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("connection %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckSystemPrivilegeDestroy(s *terraform.State) error {
	db := mustOpenDB()
	defer db.Close()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "exasol_system_privilege" {
			continue
		}
		// ID format: GRANTEE|PRIVILEGE|ADMIN_OPTION - parse and query by fields
		parts := strings.SplitN(rs.Primary.ID, "|", 3)
		if len(parts) < 2 {
			continue
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_SYS_PRIVS WHERE GRANTEE = ? AND PRIVILEGE = ?`, parts[0], parts[1]).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("system privilege %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckRoleGrantDestroy(s *terraform.State) error {
	db := mustOpenDB()
	defer db.Close()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "exasol_role_grant" {
			continue
		}
		// ID format: ROLE|GRANTEE|ADMIN_OPTION - parse and query by fields
		parts := strings.SplitN(rs.Primary.ID, "|", 3)
		if len(parts) < 2 {
			continue
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_ROLE_PRIVS WHERE GRANTED_ROLE = ? AND GRANTEE = ?`, parts[0], parts[1]).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("role grant %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckObjectPrivilegeDestroy(s *terraform.State) error {
	db := mustOpenDB()
	defer db.Close()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "exasol_object_privilege" {
			continue
		}
		// ID format: GRANTEE|PRIVILEGES|OBJECT_TYPE|OBJECT_NAME
		parts := strings.SplitN(rs.Primary.ID, "|", 4)
		if len(parts) < 4 {
			continue
		}
		// SCHEMA.TABLE is stored as OBJECT_SCHEMA + OBJECT_NAME
		schema, name := "", parts[3]
		if i := strings.Index(name, "."); i >= 0 {
			schema, name = name[:i], name[i+1:]
		}
		query := `SELECT COUNT(*) FROM EXA_DBA_OBJ_PRIVS WHERE GRANTEE = ? AND OBJECT_TYPE = ? AND OBJECT_NAME = ?`
		args := []any{parts[0], parts[2], name}
		if schema != "" {
			query += ` AND OBJECT_SCHEMA = ?`
			args = append(args, schema)
		}
		var count int
		if err := db.QueryRow(query, args...).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("object privilege %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckImpersonationGrantDestroy(s *terraform.State) error {
	db := mustOpenDB()
	defer db.Close()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "exasol_user_impersonation_grant" && rs.Type != "exasol_role_impersonation_grant" {
			continue
		}
		parts := strings.SplitN(rs.Primary.ID, "|", 2)
		if len(parts) < 2 {
			continue
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_IMPERSONATION_PRIVS WHERE GRANTEE = ? AND IMPERSONATION_ON = ?`, parts[0], parts[1]).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("impersonation grant %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccCheckConnectionGrantDestroy(s *terraform.State) error {
	db := mustOpenDB()
	defer db.Close()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "exasol_connection_grant" {
			continue
		}
		// ID format: CONNECTION_NAME|GRANTEE - parse and query by fields
		parts := strings.SplitN(rs.Primary.ID, "|", 2)
		if len(parts) < 2 {
			continue
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_CONNECTION_PRIVS WHERE GRANTED_CONNECTION = ? AND GRANTEE = ?`, parts[0], parts[1]).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("connection grant %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func mustOpenDB() *sql.DB {
	return openDBAs(
		envOrDefault("EXASOL_USER", "sys"),
		envOrDefault("EXASOL_PASSWORD", "exasol"),
	)
}

func openDBAs(user, password string) *sql.DB {
	host := envOrDefault("EXASOL_HOST", "localhost")

	dsn := exasol.NewConfig(user, password).
		Host(host).
		Port(8563).
		ValidateServerCertificate(false).
		String()

	db, err := sql.Open("exasol", dsn)
	if err != nil {
		panic(fmt.Sprintf("CheckDestroy: failed to open database: %v", err))
	}
	return db
}

func testAccCheckImpersonationEnforcement(_ *terraform.State) error {
	allowedDB := openDBAs("ACC_IMPERSONATION_LOGIN", "login-password")
	defer allowedDB.Close()
	if _, err := allowedDB.Exec(`IMPERSONATE ACC_IMPERSONATION_ALLOWED`); err != nil {
		return fmt.Errorf("allowed impersonation failed: %w", err)
	}
	return nil
}

func testAccCheckImpersonationEnforcementAfterUpdate(_ *terraform.State) error {
	allowedDB := openDBAs("ACC_IMPERSONATION_LOGIN", "login-password")
	defer allowedDB.Close()
	if _, err := allowedDB.Exec(`IMPERSONATE ACC_IMPERSONATION_DENIED`); err != nil {
		return fmt.Errorf("updated impersonation failed: %w", err)
	}

	deniedDB := openDBAs("ACC_IMPERSONATION_LOGIN", "login-password")
	defer deniedDB.Close()
	if _, err := deniedDB.Exec(`IMPERSONATE ACC_IMPERSONATION_ALLOWED`); err == nil {
		return fmt.Errorf("replaced impersonation grant was not revoked")
	}
	return nil
}

func testAccCheckRoleImpersonationEnforcement(_ *terraform.State) error {
	db := openDBAs("ACC_IMPERSONATION_LOGIN", "login-password")
	defer db.Close()
	if _, err := db.Exec(`IMPERSONATE ACC_IMPERSONATION_DENIED`); err != nil {
		return fmt.Errorf("role-based impersonation failed: %w", err)
	}
	return nil
}

// --- Role: create, rename (update in-place), import ---

func TestAccRole_FullLifecycle(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRoleDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_role" "test" {
  name = "ACC_ROLE_LIFECYCLE_V1"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_role.test", "name", "ACC_ROLE_LIFECYCLE_V1"),
					resource.TestCheckResourceAttr("exasol_role.test", "id", "ACC_ROLE_LIFECYCLE_V1"),
				),
			},
			// Rename: assert update in-place, NOT destroy+create
			{
				Config: providerConfig() + `
resource "exasol_role" "test" {
  name = "ACC_ROLE_LIFECYCLE_V2"
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("exasol_role.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_role.test", "name", "ACC_ROLE_LIFECYCLE_V2"),
					resource.TestCheckResourceAttr("exasol_role.test", "id", "ACC_ROLE_LIFECYCLE_V2"),
				),
			},
			// Import
			{
				ResourceName:      "exasol_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// --- User: create, rename (update in-place), import ---

func TestAccUser_FullLifecycle(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_user" "test" {
  name      = "ACC_USER_LIFECYCLE_V1"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_user.test", "name", "ACC_USER_LIFECYCLE_V1"),
					resource.TestCheckResourceAttr("exasol_user.test", "id", "ACC_USER_LIFECYCLE_V1"),
					resource.TestCheckResourceAttr("exasol_user.test", "auth_type", "PASSWORD"),
				),
			},
			// Rename: assert update in-place
			{
				Config: providerConfig() + `
resource "exasol_user" "test" {
  name      = "ACC_USER_LIFECYCLE_V2"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("exasol_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_user.test", "name", "ACC_USER_LIFECYCLE_V2"),
					resource.TestCheckResourceAttr("exasol_user.test", "id", "ACC_USER_LIFECYCLE_V2"),
				),
			},
			// Import (password is write-only; auth_type is inferred from DB)
			{
				ResourceName:            "exasol_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

// --- User: login only through an explicit CREATE SESSION grant ---

func TestAccUser_NoImplicitCreateSession(t *testing.T) {
	user := `
resource "exasol_user" "test" {
  name      = "ACC_USER_NO_SESSION"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}`
	session := `
resource "exasol_system_privilege" "session" {
  grantee   = exasol_user.test.name
  privilege = "CREATE SESSION"
}`
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy,
		Steps: []resource.TestStep{
			// CREATE USER only: no system privilege, login refused
			{
				Config: providerConfig() + user,
				Check:  testAccCheckUserLogin("ACC_USER_NO_SESSION", "TestPass123", false),
			},
			// Explicit grant: login works
			{
				Config: providerConfig() + user + session,
				Check:  testAccCheckUserLogin("ACC_USER_NO_SESSION", "TestPass123", true),
			},
			// Grant removed: REVOKE, login refused again, user kept
			{
				Config: providerConfig() + user,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_user.test", "id", "ACC_USER_NO_SESSION"),
					testAccCheckUserLogin("ACC_USER_NO_SESSION", "TestPass123", false),
				),
			},
		},
	})
}

// Renaming a user must keep every grant that references it: Exasol moves
// privileges, role grants and impersonation rights to the new name.
func TestAccUser_RenameWithGrants(t *testing.T) {
	config := func(name string) string {
		return providerConfig() + fmt.Sprintf(`
resource "exasol_user" "u" {
  name      = %q
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_system_privilege" "session" {
  grantee   = exasol_user.u.name
  privilege = "CREATE SESSION"
}
resource "exasol_role" "r" {
  name = "ACC_RENAME_ROLE"
}
resource "exasol_role_grant" "r" {
  role    = exasol_role.r.name
  grantee = exasol_user.u.name
}
resource "exasol_schema" "s" {
  name = "ACC_RENAME_SCHEMA"
}
resource "exasol_object_privilege" "s" {
  grantee     = exasol_user.u.name
  privileges  = ["SELECT"]
  object_type = "SCHEMA"
  object_name = exasol_schema.s.name
}
resource "exasol_connection" "c" {
  name = "ACC_RENAME_CONN"
  to   = "localhost:8563"
}
resource "exasol_connection_grant" "c" {
  connection_name = exasol_connection.c.name
  grantee         = exasol_user.u.name
}
resource "exasol_user" "other" {
  name      = "ACC_RENAME_OTHER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_user_impersonation_grant" "as_grantee" {
  grantee = exasol_user.u.name
  target  = exasol_user.other.name
}
resource "exasol_user_impersonation_grant" "as_target" {
  grantee = exasol_role.r.name
  target  = exasol_user.u.name
}`, name)
	}
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("ACC_RENAME_A"),
				Check:  testAccCheckUserLogin("ACC_RENAME_A", "TestPass123", true),
			},
			{
				Config: config("ACC_RENAME_B"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("exasol_user.u", plancheck.ResourceActionUpdate),
					},
				},
				Check: testAccCheckUserLogin("ACC_RENAME_B", "TestPass123", true),
			},
		},
	})
}

// Renaming a role, schema and connection must keep every grant that
// references them, as grantee, granted role, object or impersonation target.
func TestAccRename_RoleSchemaConnection(t *testing.T) {
	db := testAccDB(t)
	t.Cleanup(func() { db.Close() })
	config := func(suffix string) string {
		return providerConfig() + fmt.Sprintf(`
resource "exasol_role" "r" {
  name = "ACC_REN_ROLE_%[1]s"
}
resource "exasol_role" "inner" {
  name = "ACC_REN_INNER_%[1]s"
}
resource "exasol_user" "u" {
  name      = "ACC_REN_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_user" "other" {
  name      = "ACC_REN_OTHER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_schema" "s" {
  name = "ACC_REN_SCHEMA_%[1]s"
}
resource "exasol_connection" "c" {
  name = "ACC_REN_CONN_%[1]s"
  to   = "localhost:8563"
}
resource "exasol_role_grant" "r_to_user" {
  role    = exasol_role.r.name
  grantee = exasol_user.u.name
}
resource "exasol_role_grant" "inner_to_r" {
  role              = exasol_role.inner.name
  grantee           = exasol_role.r.name
  with_admin_option = true
}
resource "exasol_system_privilege" "r_create_table" {
  grantee   = exasol_role.r.name
  privilege = "CREATE TABLE"
}
resource "exasol_object_privilege" "r_on_schema" {
  grantee     = exasol_role.r.name
  privileges  = ["SELECT", "INSERT"]
  object_type = "SCHEMA"
  object_name = exasol_schema.s.name
}
resource "exasol_connection_grant" "c_to_r" {
  connection_name = exasol_connection.c.name
  grantee         = exasol_role.r.name
}
resource "exasol_user_impersonation_grant" "r_as_grantee" {
  grantee = exasol_role.r.name
  target  = exasol_user.other.name
}
resource "exasol_role_impersonation_grant" "r_as_target" {
  grantee = exasol_user.u.name
  target  = exasol_role.inner.name
}`, suffix)
	}
	count := func(query string, want int) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			var n int
			if err := db.QueryRow(query).Scan(&n); err != nil {
				return fmt.Errorf("%s: %w", query, err)
			}
			if n != want {
				return fmt.Errorf("%s: got %d, want %d", query, n, want)
			}
			return nil
		}
	}
	grantsOn := func(suffix string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			count(fmt.Sprintf(`SELECT COUNT(*) FROM EXA_DBA_ROLE_PRIVS WHERE GRANTED_ROLE = 'ACC_REN_ROLE_%s' AND GRANTEE = 'ACC_REN_USER'`, suffix), 1),
			count(fmt.Sprintf(`SELECT COUNT(*) FROM EXA_DBA_ROLE_PRIVS WHERE GRANTED_ROLE = 'ACC_REN_INNER_%[1]s' AND GRANTEE = 'ACC_REN_ROLE_%[1]s' AND ADMIN_OPTION IN ('TRUE', 'true', '1')`, suffix), 1),
			count(fmt.Sprintf(`SELECT COUNT(*) FROM EXA_DBA_SYS_PRIVS WHERE GRANTEE = 'ACC_REN_ROLE_%s' AND PRIVILEGE = 'CREATE TABLE'`, suffix), 1),
			count(fmt.Sprintf(`SELECT COUNT(*) FROM EXA_DBA_OBJ_PRIVS WHERE GRANTEE = 'ACC_REN_ROLE_%[1]s' AND OBJECT_TYPE = 'SCHEMA' AND OBJECT_NAME = 'ACC_REN_SCHEMA_%[1]s'`, suffix), 2),
			count(fmt.Sprintf(`SELECT COUNT(*) FROM EXA_DBA_CONNECTION_PRIVS WHERE GRANTED_CONNECTION = 'ACC_REN_CONN_%[1]s' AND GRANTEE = 'ACC_REN_ROLE_%[1]s'`, suffix), 1),
			count(fmt.Sprintf(`SELECT COUNT(*) FROM EXA_DBA_IMPERSONATION_PRIVS WHERE GRANTEE = 'ACC_REN_ROLE_%s' AND IMPERSONATION_ON = 'ACC_REN_OTHER'`, suffix), 1),
			count(fmt.Sprintf(`SELECT COUNT(*) FROM EXA_DBA_IMPERSONATION_PRIVS WHERE GRANTEE = 'ACC_REN_USER' AND IMPERSONATION_ON = 'ACC_REN_INNER_%s'`, suffix), 1),
		)
	}
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRoleDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("A"),
				Check:  grantsOn("A"),
			},
			{
				Config: config("B"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("exasol_role.r", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("exasol_role.inner", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("exasol_schema.s", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("exasol_connection.c", plancheck.ResourceActionUpdate),
					},
				},
				Check: grantsOn("B"),
			},
		},
	})
}

// testAccCheckUserLogin checks the user's direct CREATE SESSION grant and
// whether a login with the given password succeeds.
func testAccCheckUserLogin(user, password string, wantLogin bool) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		db := mustOpenDB()
		defer db.Close()
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM EXA_DBA_SYS_PRIVS WHERE GRANTEE = ? AND PRIVILEGE = 'CREATE SESSION'`,
			user).Scan(&n); err != nil {
			return fmt.Errorf("query EXA_DBA_SYS_PRIVS: %w", err)
		}
		if wantLogin != (n == 1) {
			return fmt.Errorf("user %s: %d CREATE SESSION grants, want login=%t", user, n, wantLogin)
		}
		userDB := openDBAs(user, password)
		defer userDB.Close()
		err := userDB.Ping()
		if wantLogin && err != nil {
			return fmt.Errorf("user %s: login failed: %w", user, err)
		}
		if !wantLogin && err == nil {
			return fmt.Errorf("user %s: login succeeded without CREATE SESSION", user)
		}
		return nil
	}
}

// --- Schema: create, rename, ownership, import ---

func TestAccSchema_FullLifecycle(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSchemaDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_schema" "test" {
  name = "ACC_SCHEMA_LIFECYCLE_V1"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_schema.test", "name", "ACC_SCHEMA_LIFECYCLE_V1"),
					resource.TestCheckResourceAttrSet("exasol_schema.test", "owner"),
				),
			},
			// Rename: assert update in-place
			{
				Config: providerConfig() + `
resource "exasol_schema" "test" {
  name = "ACC_SCHEMA_LIFECYCLE_V2"
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("exasol_schema.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_schema.test", "name", "ACC_SCHEMA_LIFECYCLE_V2"),
					resource.TestCheckResourceAttrSet("exasol_schema.test", "owner"),
				),
			},
			// Import
			{
				ResourceName:      "exasol_schema.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccSchema_WithOwner(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSchemaDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_role" "owner" {
  name = "ACC_SCHEMA_OWNER_ROLE"
}
resource "exasol_schema" "test" {
  name  = "ACC_OWNED_SCHEMA"
  owner = exasol_role.owner.name
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_schema.test", "owner", "ACC_SCHEMA_OWNER_ROLE"),
				),
			},
		},
	})
}

// --- Connection: create, rename, import ---

func TestAccConnection_FullLifecycle(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckConnectionDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_connection" "test" {
  name = "ACC_CONN_LIFECYCLE_V1"
  to   = "localhost:8563"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_connection.test", "name", "ACC_CONN_LIFECYCLE_V1"),
					resource.TestCheckResourceAttr("exasol_connection.test", "id", "ACC_CONN_LIFECYCLE_V1"),
				),
			},
			// Rename: assert update in-place
			{
				Config: providerConfig() + `
resource "exasol_connection" "test" {
  name = "ACC_CONN_LIFECYCLE_V2"
  to   = "localhost:8563"
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("exasol_connection.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_connection.test", "name", "ACC_CONN_LIFECYCLE_V2"),
				),
			},
			// Import (password is write-only; to and user are read from DB)
			{
				ResourceName:            "exasol_connection.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

// --- Lowercase names: verify no perpetual drift when config uses non-uppercase ---

func TestAccLowercaseNames_NoDrift(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_role" "lc" {
  name = "acc_lowercase_role"
}
resource "exasol_user" "lc" {
  name      = "acc_lowercase_user"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_schema" "lc" {
  name = "acc_lowercase_schema"
}
resource "exasol_connection" "lc" {
  name = "acc_lowercase_conn"
  to   = "localhost:8563"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_role.lc", "name", "acc_lowercase_role"),
					resource.TestCheckResourceAttr("exasol_user.lc", "name", "acc_lowercase_user"),
					resource.TestCheckResourceAttr("exasol_schema.lc", "name", "acc_lowercase_schema"),
					resource.TestCheckResourceAttr("exasol_connection.lc", "name", "acc_lowercase_conn"),
				),
				// Drift detection is automatic: post-apply plan must be empty.
				// If any resource uppercases the name in state, this step fails
				// because plan.Name (lowercase) != state.Name (uppercase).
			},
		},
	})
}

// --- Connection with credentials: verify user round-trips through import ---

func TestAccConnection_WithCredentials(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckConnectionDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_connection" "cred" {
  name     = "ACC_CONN_CRED"
  to       = "https://bucket.s3.amazonaws.com"
  user     = "AKIAIOSFODNN7EXAMPLE"
  password = "wJalrXUtnFEMI"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_connection.cred", "to", "https://bucket.s3.amazonaws.com"),
					resource.TestCheckResourceAttr("exasol_connection.cred", "user", "AKIAIOSFODNN7EXAMPLE"),
				),
			},
			// Import: verify to and user are read back from DB
			{
				ResourceName:            "exasol_connection.cred",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

// --- System Privilege: grant, admin option toggle, import ---

func TestAccSystemPrivilege_FullLifecycle(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSystemPrivilegeDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_user" "test" {
  name      = "ACC_SYSPRIV_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_system_privilege" "test" {
  grantee   = exasol_user.test.name
  privilege = "CREATE TABLE"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_system_privilege.test", "privilege", "CREATE TABLE"),
					resource.TestCheckResourceAttr("exasol_system_privilege.test", "grantee", "ACC_SYSPRIV_USER"),
				),
			},
			// Toggle admin option on
			{
				Config: providerConfig() + `
resource "exasol_user" "test" {
  name      = "ACC_SYSPRIV_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_system_privilege" "test" {
  grantee           = exasol_user.test.name
  privilege         = "CREATE TABLE"
  with_admin_option = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_system_privilege.test", "with_admin_option", "true"),
				),
			},
			// Import
			{
				ResourceName:      "exasol_system_privilege.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// --- Role Grant: grant, admin option, import ---

func TestAccRoleGrant_FullLifecycle(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRoleGrantDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_role" "granted" {
  name = "ACC_ROLEGRANT_ROLE"
}
resource "exasol_user" "grantee" {
  name      = "ACC_ROLEGRANT_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_role_grant" "test" {
  role    = exasol_role.granted.name
  grantee = exasol_user.grantee.name
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_role_grant.test", "role", "ACC_ROLEGRANT_ROLE"),
					resource.TestCheckResourceAttr("exasol_role_grant.test", "grantee", "ACC_ROLEGRANT_USER"),
				),
			},
			// Toggle admin option on
			{
				Config: providerConfig() + `
resource "exasol_role" "granted" {
  name = "ACC_ROLEGRANT_ROLE"
}
resource "exasol_user" "grantee" {
  name      = "ACC_ROLEGRANT_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_role_grant" "test" {
  role              = exasol_role.granted.name
  grantee           = exasol_user.grantee.name
  with_admin_option = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_role_grant.test", "with_admin_option", "true"),
				),
			},
			// Import
			{
				ResourceName:      "exasol_role_grant.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// --- Object Privilege: grant, add privilege ---

// Table grants are stored with OBJECT_SCHEMA and OBJECT_NAME split in
// EXA_DBA_OBJ_PRIVS. Read must find them (no drift) and Delete must revoke them.
func TestAccObjectPrivilege_OnTable(t *testing.T) {
	db := testAccDB(t)
	role := `
resource "exasol_role" "test" {
  name = "ACC_OBJTBL_ROLE"
}`
	grant := `
resource "exasol_object_privilege" "test" {
  grantee     = exasol_role.test.name
  privileges  = ["SELECT"]
  object_type = "TABLE"
  object_name = "ACC_OBJTBL_SCHEMA.T1"
}`
	tablePrivs := func(want int) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_OBJ_PRIVS WHERE GRANTEE = 'ACC_OBJTBL_ROLE' AND OBJECT_SCHEMA = 'ACC_OBJTBL_SCHEMA' AND OBJECT_NAME = 'T1'`).Scan(&n); err != nil {
				return err
			}
			if n != want {
				return fmt.Errorf("table privileges for ACC_OBJTBL_ROLE: got %d, want %d", n, want)
			}
			return nil
		}
	}
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			for _, stmt := range []string{
				`CREATE SCHEMA IF NOT EXISTS ACC_OBJTBL_SCHEMA`,
				`CREATE OR REPLACE TABLE ACC_OBJTBL_SCHEMA.T1 (A INT)`,
				"CREATE OR REPLACE LUA SCALAR SCRIPT ACC_OBJTBL_SCHEMA.S1() RETURNS INT AS\nfunction run(ctx) return 1 end\n/",
			} {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("setup %q: %v", stmt, err)
				}
			}
			t.Cleanup(func() {
				_, _ = db.Exec(`DROP SCHEMA IF EXISTS ACC_OBJTBL_SCHEMA CASCADE`)
				db.Close()
			})
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectPrivilegeDestroy,
		Steps: []resource.TestStep{
			// Grant on a table; the post-apply plan must be empty (no drift)
			{
				Config: providerConfig() + role + grant,
				Check:  tablePrivs(1),
			},
			// Remove the grant only: Delete must revoke it
			{
				Config: providerConfig() + role,
				Check:  tablePrivs(0),
			},
			// ALL on a table, EXECUTE on a script: no drift, and both revoked on removal
			{
				Config: providerConfig() + role + `
resource "exasol_object_privilege" "all" {
  grantee     = exasol_role.test.name
  privileges  = ["ALL"]
  object_type = "TABLE"
  object_name = "ACC_OBJTBL_SCHEMA.T1"
}
resource "exasol_object_privilege" "script" {
  grantee     = exasol_role.test.name
  privileges  = ["EXECUTE"]
  object_type = "SCRIPT"
  object_name = "ACC_OBJTBL_SCHEMA.S1"
}`,
				Check: func(_ *terraform.State) error {
					var n int
					if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_OBJ_PRIVS WHERE GRANTEE = 'ACC_OBJTBL_ROLE' AND OBJECT_SCHEMA = 'ACC_OBJTBL_SCHEMA' AND OBJECT_NAME = 'S1' AND PRIVILEGE = 'EXECUTE'`).Scan(&n); err != nil {
						return err
					}
					if n != 1 {
						return fmt.Errorf("script EXECUTE grants: got %d, want 1", n)
					}
					return nil
				},
			},
			{
				Config: providerConfig() + role,
				Check: func(_ *terraform.State) error {
					var n int
					if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_OBJ_PRIVS WHERE GRANTEE = 'ACC_OBJTBL_ROLE'`).Scan(&n); err != nil {
						return err
					}
					if n != 0 {
						return fmt.Errorf("object privileges left for ACC_OBJTBL_ROLE: %d", n)
					}
					return nil
				},
			},
		},
	})
}

func TestAccObjectPrivilege_OnSchema(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectPrivilegeDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_schema" "test" {
  name = "ACC_OBJPRIV_SCHEMA"
}

resource "exasol_role" "test" {
  name = "ACC_OBJPRIV_ROLE"
}

resource "exasol_object_privilege" "test" {
  grantee     = exasol_role.test.name
  privileges  = ["SELECT"]
  object_type = "SCHEMA"
  object_name = exasol_schema.test.name
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_object_privilege.test", "object_type", "SCHEMA"),
					resource.TestCheckResourceAttr("exasol_object_privilege.test", "privileges.#", "1"),
				),
			},
			// Add a privilege
			{
				Config: providerConfig() + `
resource "exasol_schema" "test" {
  name = "ACC_OBJPRIV_SCHEMA"
}
resource "exasol_role" "test" {
  name = "ACC_OBJPRIV_ROLE"
}
resource "exasol_object_privilege" "test" {
  grantee     = exasol_role.test.name
  privileges  = ["SELECT", "INSERT"]
  object_type = "SCHEMA"
  object_name = exasol_schema.test.name
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("exasol_object_privilege.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_object_privilege.test", "privileges.#", "2"),
				),
			},
		},
	})
}

func TestAccImpersonationGrant(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckImpersonationGrantDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_user" "allowed" {
  name      = "ACC_IMPERSONATION_ALLOWED"
  auth_type = "PASSWORD"
  password  = "allowed-password"
}
resource "exasol_user" "denied" {
  name      = "ACC_IMPERSONATION_DENIED"
  auth_type = "PASSWORD"
  password  = "denied-password"
}
resource "exasol_role" "grantee" {
  name = "ACC_IMPERSONATION_GRANTEE"
}
resource "exasol_role" "target" {
  name = "ACC_IMPERSONATION_TARGET_ROLE"
}
resource "exasol_role_grant" "target_to_denied" {
  role    = exasol_role.target.name
  grantee = exasol_user.denied.name
}
resource "exasol_user" "login" {
  name      = "ACC_IMPERSONATION_LOGIN"
  auth_type = "PASSWORD"
  password  = "login-password"
}
resource "exasol_system_privilege" "login_session" {
  grantee   = exasol_user.login.name
  privilege = "CREATE SESSION"
}
resource "exasol_role_grant" "grantee_to_login" {
  role    = exasol_role.grantee.name
  grantee = exasol_user.login.name
}
resource "exasol_user_impersonation_grant" "allowed" {
  grantee           = exasol_role.grantee.name
  target            = exasol_user.allowed.name
}
resource "exasol_role_impersonation_grant" "target" {
  grantee = exasol_role.grantee.name
  target  = exasol_role.target.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckFunc(testAccCheckImpersonationEnforcement),
					resource.TestCheckFunc(testAccCheckRoleImpersonationEnforcement),
					resource.TestCheckResourceAttr("exasol_user_impersonation_grant.allowed", "grantee", "ACC_IMPERSONATION_GRANTEE"),
					resource.TestCheckResourceAttr("exasol_user_impersonation_grant.allowed", "target", "ACC_IMPERSONATION_ALLOWED"),
					resource.TestCheckResourceAttr("exasol_user_impersonation_grant.allowed", "id", "ACC_IMPERSONATION_GRANTEE|ACC_IMPERSONATION_ALLOWED"),
					resource.TestCheckResourceAttr("exasol_role_impersonation_grant.target", "target", "ACC_IMPERSONATION_TARGET_ROLE"),
					resource.TestCheckResourceAttr("exasol_role_impersonation_grant.target", "id", "ACC_IMPERSONATION_GRANTEE|ACC_IMPERSONATION_TARGET_ROLE"),
				),
			},
			{
				Config: providerConfig() + `
resource "exasol_user" "allowed" {
  name      = "ACC_IMPERSONATION_ALLOWED"
  auth_type = "PASSWORD"
  password  = "allowed-password"
}
resource "exasol_user" "denied" {
  name      = "ACC_IMPERSONATION_DENIED"
  auth_type = "PASSWORD"
  password  = "denied-password"
}
resource "exasol_role" "grantee" {
  name = "ACC_IMPERSONATION_GRANTEE"
}
resource "exasol_role" "target" {
  name = "ACC_IMPERSONATION_TARGET_ROLE"
}
resource "exasol_role_grant" "target_to_denied" {
  role    = exasol_role.target.name
  grantee = exasol_user.denied.name
}
resource "exasol_user" "login" {
  name      = "ACC_IMPERSONATION_LOGIN"
  auth_type = "PASSWORD"
  password  = "login-password"
}
resource "exasol_system_privilege" "login_session" {
  grantee   = exasol_user.login.name
  privilege = "CREATE SESSION"
}
resource "exasol_role_grant" "grantee_to_login" {
  role    = exasol_role.grantee.name
  grantee = exasol_user.login.name
}
resource "exasol_user_impersonation_grant" "allowed" {
  grantee           = exasol_role.grantee.name
  target            = exasol_user.denied.name
}
resource "exasol_role_impersonation_grant" "target" {
  grantee = exasol_role.grantee.name
  target  = exasol_role.target.name
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("exasol_user_impersonation_grant.allowed", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckFunc(testAccCheckImpersonationEnforcementAfterUpdate),
			},
			{
				ResourceName:      "exasol_user_impersonation_grant.allowed",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "ACC_IMPERSONATION_GRANTEE|ACC_IMPERSONATION_DENIED",
			},
			{
				ResourceName:      "exasol_role_impersonation_grant.target",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     "ACC_IMPERSONATION_GRANTEE|ACC_IMPERSONATION_TARGET_ROLE",
			},
		},
	})
}

// --- Connection Grant ---

func TestAccConnectionGrant(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckConnectionGrantDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_connection" "test" {
  name = "ACC_CONNGRANT_CONN"
  to   = "localhost:8563"
}
resource "exasol_role" "test" {
  name = "ACC_CONNGRANT_ROLE"
}
resource "exasol_connection_grant" "test" {
  connection_name = exasol_connection.test.name
  grantee         = exasol_role.test.name
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("exasol_connection_grant.test", "id"),
					resource.TestCheckResourceAttr("exasol_connection_grant.test", "connection_name", "ACC_CONNGRANT_CONN"),
					resource.TestCheckResourceAttr("exasol_connection_grant.test", "grantee", "ACC_CONNGRANT_ROLE"),
				),
			},
			// Import
			{
				ResourceName:      "exasol_connection_grant.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
