# Test Suite 3: System Privileges - Comprehensive Testing
# Tests: TC-SP-001 through TC-SP-006
# Focus: Admin option handling for system privileges, various privilege types

terraform {
  required_providers {
    exasol = {
      source  = "local/exasol/exasol"
    }
  }
}

provider "exasol" {
  host                        = "localhost"
  port                        = 8563
  user                        = "sys"
  password                    = "exasol"
  validate_server_certificate = false
}

# Test users for system privileges
resource "exasol_user" "etl_user" {
  name      = "SP_ETL_USER"
  auth_type = "PASSWORD"
  password  = "TestPass123!"
}

resource "exasol_user" "admin_user" {
  name      = "SP_ADMIN_USER"
  auth_type = "PASSWORD"
  password  = "TestPass456!"
}

# Test roles for system privileges
resource "exasol_role" "etl_pipeline_role" {
  name = "SP_ETL_PIPELINE_ROLE"
}

resource "exasol_role" "schema_manager_role" {
  name = "SP_SCHEMA_MANAGER_ROLE"
}

resource "exasol_role" "developer_role" {
  name = "SP_DEVELOPER_ROLE"
}

# TC-SP-001: Grant single system privilege without admin option
# Basic DDL privilege
resource "exasol_system_privilege" "tc_sp_001_create_table" {
  grantee   = exasol_user.etl_user.name
  privilege = "CREATE TABLE"
  # with_admin_option NOT SPECIFIED - should not cause drift
}

# TC-SP-002: Grant multiple system privileges independently
# Tests multiple privilege resources for same grantee
resource "exasol_system_privilege" "tc_sp_002_create_schema" {
  grantee   = exasol_role.schema_manager_role.name
  privilege = "CREATE SCHEMA"
}

resource "exasol_system_privilege" "tc_sp_002_drop_schema" {
  grantee   = exasol_role.schema_manager_role.name
  privilege = "DROP ANY SCHEMA"
}

resource "exasol_system_privilege" "tc_sp_002_alter_schema" {
  grantee   = exasol_role.schema_manager_role.name
  privilege = "ALTER ANY SCHEMA"
}

# TC-SP-003: System privilege WITH admin option
# Critical for delegation capability
resource "exasol_system_privilege" "tc_sp_003_with_admin" {
  grantee           = exasol_user.admin_user.name
  privilege         = "CREATE ROLE"
  with_admin_option = true
}

# TC-SP-004: Admin option transition test (simulated)
# This shows current state with admin option
# For actual transition test, see admin-transitions directory
resource "exasol_system_privilege" "tc_sp_004_transition" {
  grantee           = exasol_role.etl_pipeline_role.name
  privilege         = "CREATE VIEW"
  with_admin_option = true
}

# TC-SP-005: Permission escalation privileges
# These are high-privilege system grants
resource "exasol_system_privilege" "tc_sp_005_grant_any_privilege" {
  grantee           = exasol_user.admin_user.name
  privilege         = "GRANT ANY PRIVILEGE"
  with_admin_option = true
}

resource "exasol_system_privilege" "tc_sp_005_grant_any_role" {
  grantee           = exasol_user.admin_user.name
  privilege         = "GRANT ANY ROLE"
  with_admin_option = true
}

# TC-SP-006: No drift on plan after apply
# Production-like ETL pipeline privileges
resource "exasol_system_privilege" "tc_sp_006_import" {
  grantee   = exasol_role.etl_pipeline_role.name
  privilege = "IMPORT"
}

resource "exasol_system_privilege" "tc_sp_006_export" {
  grantee   = exasol_role.etl_pipeline_role.name
  privilege = "EXPORT"
}

# Additional common system privileges for coverage
resource "exasol_system_privilege" "create_session" {
  grantee   = exasol_role.developer_role.name
  privilege = "CREATE SESSION"
}

resource "exasol_system_privilege" "use_any_schema" {
  grantee   = exasol_role.developer_role.name
  privilege = "USE ANY SCHEMA"
}

resource "exasol_system_privilege" "select_any_dictionary" {
  grantee   = exasol_role.developer_role.name
  privilege = "SELECT ANY DICTIONARY"
}

# Grant roles to users for testing
resource "exasol_role_grant" "etl_role_to_user" {
  role    = exasol_role.etl_pipeline_role.name
  grantee = exasol_user.etl_user.name
}

resource "exasol_role_grant" "schema_manager_to_admin" {
  role              = exasol_role.schema_manager_role.name
  grantee           = exasol_user.admin_user.name
  with_admin_option = true
}

resource "exasol_role_grant" "developer_to_etl" {
  role    = exasol_role.developer_role.name
  grantee = exasol_user.etl_user.name
}
