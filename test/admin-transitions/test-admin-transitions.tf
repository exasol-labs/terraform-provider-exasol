# Test file for admin_option state transitions
# This tests all scenarios of adding/removing with_admin_option

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

# Test resources
resource "exasol_user" "test_user" {
  name      = "TRANSITION_TEST_USER"
  auth_type = "PASSWORD"
  password  = "TestPassword123"
}

resource "exasol_user" "test_user2" {
  name      = "TRANSITION_TEST_USER2"
  auth_type = "PASSWORD"
  password  = "TestPassword456"
}

resource "exasol_role" "test_role" {
  name = "TRANSITION_TEST_ROLE"
}

# Scenario 1: Grant without admin_option -> NOW ADDING admin_option
# Testing adding admin_option to existing grant
resource "exasol_role_grant" "scenario1_never_defined" {
  role              = exasol_role.test_role.name
  grantee           = exasol_user.test_user.name
  with_admin_option = true
}

# Scenario 2: Grant WITH admin_option REMOVED
# Testing transition from true to not specified (null)
resource "exasol_role_grant" "scenario2_with_admin" {
  role    = exasol_role.test_role.name
  grantee = exasol_user.test_user2.name
  # with_admin_option removed - should detect drift and update
}

# Scenario 3: System privilege without admin_option
resource "exasol_system_privilege" "scenario3_never_defined" {
  grantee   = exasol_user.test_user.name
  privilege = "CREATE SCHEMA"
}

# Scenario 4: System privilege WITH admin_option
# Will test removing this later
resource "exasol_system_privilege" "scenario4_with_admin" {
  grantee           = exasol_user.test_user.name
  privilege         = "CREATE TABLE"
  with_admin_option = true
}
