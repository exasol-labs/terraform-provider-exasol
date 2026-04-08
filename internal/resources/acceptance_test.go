//go:build integration

package resources_test

import (
	"os"
	"testing"

	"terraform-provider-exasol/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func testProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"exasol": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
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

// --- Role: create, rename, destroy ---

func TestAccRole_CreateAndRename(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_role" "test" {
  name = "ACC_TEST_ROLE_V1"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_role.test", "name", "ACC_TEST_ROLE_V1"),
					resource.TestCheckResourceAttr("exasol_role.test", "id", "ACC_TEST_ROLE_V1"),
				),
			},
			{
				Config: providerConfig() + `
resource "exasol_role" "test" {
  name = "ACC_TEST_ROLE_V2"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_role.test", "name", "ACC_TEST_ROLE_V2"),
					resource.TestCheckResourceAttr("exasol_role.test", "id", "ACC_TEST_ROLE_V2"),
				),
			},
		},
	})
}

// --- User: create, rename, destroy ---

func TestAccUser_CreateAndRename(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_user" "test" {
  name      = "ACC_TEST_USER_V1"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_user.test", "name", "ACC_TEST_USER_V1"),
					resource.TestCheckResourceAttr("exasol_user.test", "id", "ACC_TEST_USER_V1"),
				),
			},
			{
				Config: providerConfig() + `
resource "exasol_user" "test" {
  name      = "ACC_TEST_USER_V2"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_user.test", "name", "ACC_TEST_USER_V2"),
					resource.TestCheckResourceAttr("exasol_user.test", "id", "ACC_TEST_USER_V2"),
				),
			},
		},
	})
}

// --- Schema: create, rename, ownership ---

func TestAccSchema_CreateAndRename(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_schema" "test" {
  name = "ACC_TEST_SCHEMA_V1"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_schema.test", "name", "ACC_TEST_SCHEMA_V1"),
					resource.TestCheckResourceAttrSet("exasol_schema.test", "owner"),
				),
			},
			{
				Config: providerConfig() + `
resource "exasol_schema" "test" {
  name = "ACC_TEST_SCHEMA_V2"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_schema.test", "name", "ACC_TEST_SCHEMA_V2"),
					resource.TestCheckResourceAttrSet("exasol_schema.test", "owner"),
				),
			},
		},
	})
}

func TestAccSchema_WithOwner(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_role" "owner" {
  name = "ACC_TEST_SCHEMA_OWNER"
}
resource "exasol_schema" "test" {
  name  = "ACC_TEST_OWNED_SCHEMA"
  owner = exasol_role.owner.name
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_schema.test", "owner", "ACC_TEST_SCHEMA_OWNER"),
				),
			},
		},
	})
}

// --- Connection: create, rename, update credentials ---

func TestAccConnection_CreateAndRename(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_connection" "test" {
  name = "ACC_TEST_CONN_V1"
  to   = "localhost:8563"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_connection.test", "name", "ACC_TEST_CONN_V1"),
				),
			},
			{
				Config: providerConfig() + `
resource "exasol_connection" "test" {
  name = "ACC_TEST_CONN_V2"
  to   = "localhost:8563"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_connection.test", "name", "ACC_TEST_CONN_V2"),
				),
			},
		},
	})
}

// --- System Privilege: grant, admin option toggle ---

func TestAccSystemPrivilege_WithAdminOption(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_user" "test" {
  name      = "ACC_TEST_SYSPRIV_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_system_privilege" "test" {
  grantee   = exasol_user.test.name
  privilege = "CREATE TABLE"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_system_privilege.test", "privilege", "CREATE TABLE"),
				),
			},
			{
				Config: providerConfig() + `
resource "exasol_user" "test" {
  name      = "ACC_TEST_SYSPRIV_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_system_privilege" "test" {
  grantee           = exasol_user.test.name
  privilege         = "CREATE TABLE"
  with_admin_option = true
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_system_privilege.test", "with_admin_option", "true"),
				),
			},
		},
	})
}

// --- Role Grant: grant, admin option ---

func TestAccRoleGrant_WithAdminOption(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_role" "granted" {
  name = "ACC_TEST_GRANTED_ROLE"
}
resource "exasol_user" "grantee" {
  name      = "ACC_TEST_ROLEGRANT_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_role_grant" "test" {
  role    = exasol_role.granted.name
  grantee = exasol_user.grantee.name
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_role_grant.test", "role", "ACC_TEST_GRANTED_ROLE"),
					resource.TestCheckResourceAttr("exasol_role_grant.test", "grantee", "ACC_TEST_ROLEGRANT_USER"),
				),
			},
			{
				Config: providerConfig() + `
resource "exasol_role" "granted" {
  name = "ACC_TEST_GRANTED_ROLE"
}
resource "exasol_user" "grantee" {
  name      = "ACC_TEST_ROLEGRANT_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123"
}
resource "exasol_role_grant" "test" {
  role              = exasol_role.granted.name
  grantee           = exasol_user.grantee.name
  with_admin_option = true
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_role_grant.test", "with_admin_option", "true"),
				),
			},
		},
	})
}

// --- Object Privilege ---

func TestAccObjectPrivilege_OnSchema(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_schema" "test" {
  name = "ACC_TEST_OBJPRIV_SCHEMA"
}
resource "exasol_role" "test" {
  name = "ACC_TEST_OBJPRIV_ROLE"
}
resource "exasol_object_privilege" "test" {
  grantee     = exasol_role.test.name
  privileges  = ["SELECT"]
  object_type = "SCHEMA"
  object_name = exasol_schema.test.name
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_object_privilege.test", "object_type", "SCHEMA"),
				),
			},
			{
				Config: providerConfig() + `
resource "exasol_schema" "test" {
  name = "ACC_TEST_OBJPRIV_SCHEMA"
}
resource "exasol_role" "test" {
  name = "ACC_TEST_OBJPRIV_ROLE"
}
resource "exasol_object_privilege" "test" {
  grantee     = exasol_role.test.name
  privileges  = ["SELECT", "INSERT"]
  object_type = "SCHEMA"
  object_name = exasol_schema.test.name
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("exasol_object_privilege.test", "privileges.#", "2"),
				),
			},
		},
	})
}

// --- Connection Grant ---

func TestAccConnectionGrant(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "exasol_connection" "test" {
  name = "ACC_TEST_CONNGRANT_CONN"
  to   = "localhost:8563"
}
resource "exasol_role" "test" {
  name = "ACC_TEST_CONNGRANT_ROLE"
}
resource "exasol_connection_grant" "test" {
  connection_name = exasol_connection.test.name
  grantee         = exasol_role.test.name
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("exasol_connection_grant.test", "id"),
				),
			},
		},
	})
}
