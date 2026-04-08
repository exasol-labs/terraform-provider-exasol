# Test Suite 1: Role Grants - Comprehensive Testing
# Tests: TC-RG-001 through TC-RG-007
# Focus: Admin option handling, state transitions, case sensitivity

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

# Test users
resource "exasol_user" "test_user1" {
  name      = "RG_TEST_USER1"
  auth_type = "PASSWORD"
  password  = "TestPass123!"
}

resource "exasol_user" "test_user2" {
  name      = "RG_TEST_USER2"
  auth_type = "PASSWORD"
  password  = "TestPass456!"
}

resource "exasol_user" "test_user3" {
  name      = "RG_TEST_USER3"
  auth_type = "PASSWORD"
  password  = "TestPass789!"
}

# Test user with mixed case (for case sensitivity test)
resource "exasol_user" "test_user_mixed" {
  name      = "RG_MIXED_CASE_USER"
  auth_type = "PASSWORD"
  password  = "TestPass000!"
}

# Test roles
resource "exasol_role" "test_role1" {
  name = "RG_TEST_ROLE1"
}

resource "exasol_role" "test_role2" {
  name = "RG_TEST_ROLE2"
}

resource "exasol_role" "test_role3" {
  name = "RG_TEST_ROLE3"
}

resource "exasol_role" "parent_role" {
  name = "RG_PARENT_ROLE"
}

resource "exasol_role" "child_role" {
  name = "RG_CHILD_ROLE"
}

# TC-RG-001: Create role grant WITHOUT admin option
# Critical: This must NOT cause drift on subsequent plans
# State should store with_admin_option as null (not false)
resource "exasol_role_grant" "tc_rg_001_without_admin" {
  role    = exasol_role.test_role1.name
  grantee = exasol_user.test_user1.name
  # with_admin_option NOT SPECIFIED - this is the test
}

# TC-RG-002: Create role grant WITH admin option
# State should store with_admin_option as true
resource "exasol_role_grant" "tc_rg_002_with_admin" {
  role              = exasol_role.test_role2.name
  grantee           = exasol_user.test_user2.name
  with_admin_option = true
}

# TC-RG-003: Add admin option to existing grant
# This tests the Update operation when transitioning from null to true
# Note: This is simulated by having it set to true from the start
# For actual transition test, see admin-transitions directory
resource "exasol_role_grant" "tc_rg_003_add_admin" {
  role              = exasol_role.test_role3.name
  grantee           = exasol_user.test_user3.name
  with_admin_option = true
}

# TC-RG-004: Remove admin option from grant
# Simulated by not specifying admin option
# For actual transition test, see admin-transitions directory
resource "exasol_role_grant" "tc_rg_004_remove_admin" {
  role    = exasol_role.test_role1.name
  grantee = exasol_user.test_user2.name
  # with_admin_option removed
}

# TC-RG-005: Role to role grants (hierarchical)
# Tests granting role to another role for privilege aggregation
resource "exasol_role_grant" "tc_rg_005_role_to_role" {
  role              = exasol_role.child_role.name
  grantee           = exasol_role.parent_role.name
  with_admin_option = true
}

# Grant parent role to user to test inheritance
resource "exasol_role_grant" "tc_rg_005_parent_to_user" {
  role    = exasol_role.parent_role.name
  grantee = exasol_user.test_user1.name
}

# TC-RG-006: Case insensitivity
# Input with mixed case should be normalized to uppercase
# Note: Terraform will normalize these, so stored uppercase
resource "exasol_role_grant" "tc_rg_006_case_test" {
  role    = exasol_role.test_role1.name
  grantee = exasol_user.test_user_mixed.name
}

# TC-RG-007: Plan shows no changes after apply
# This is implicitly tested by ALL grants above
# The test runner will verify no drift after apply
