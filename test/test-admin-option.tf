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

# Create test resources
resource "exasol_user" "test_user" {
  name      = "ADMIN_FIX_TEST_USER"
  auth_type = "PASSWORD"
  password  = "TestPassword123"
}

resource "exasol_user" "test_user2" {
  name      = "ADMIN_FIX_TEST_USER2"
  auth_type = "PASSWORD"
  password  = "TestPassword456"
}

resource "exasol_role" "test_role" {
  name = "ADMIN_FIX_TEST_ROLE"
}

# Test Case 1: Role grant WITH admin_option explicitly set to true
# After apply, plan should show no changes
resource "exasol_role_grant" "with_admin_true" {
  role              = exasol_role.test_role.name
  grantee           = exasol_user.test_user.name
  with_admin_option = true
}

# Test Case 2: Role grant WITHOUT admin_option (not specified)
# This should NOT cause drift - should stay null in state
resource "exasol_role_grant" "without_admin" {
  role    = exasol_role.test_role.name
  grantee = exasol_user.test_user2.name
}

# Test Case 3: System privilege WITH admin_option explicitly set to true
resource "exasol_system_privilege" "with_admin_true" {
  grantee           = exasol_user.test_user.name
  privilege         = "CREATE TABLE"
  with_admin_option = true
}

# Test Case 4: System privilege WITHOUT admin_option (not specified)
# This should NOT cause drift - should stay null in state
resource "exasol_system_privilege" "without_admin" {
  grantee   = exasol_user.test_user.name
  privilege = "CREATE SCHEMA"
}
