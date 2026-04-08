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
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM EXA_DBA_OBJ_PRIVS WHERE GRANTEE = ? AND OBJECT_TYPE = ? AND OBJECT_NAME = ?`, parts[0], parts[2], parts[3]).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("object privilege %s still exists after destroy", rs.Primary.ID)
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
		panic(fmt.Sprintf("CheckDestroy: failed to open database: %v", err))
	}
	return db
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
