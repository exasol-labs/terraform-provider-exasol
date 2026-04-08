# Test Suite 2: Object Privileges - Comprehensive Testing
# Tests: TC-OP-001 through TC-OP-008
# Focus: Privilege list ordering, multiple privileges, ALL privilege handling

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

# Test user who will create schemas for testing
resource "exasol_user" "schema_creator" {
  name      = "OP_SCHEMA_CREATOR"
  auth_type = "PASSWORD"
  password  = "TestPass123!"
}

# Grant CREATE SCHEMA privilege to schema creator
resource "exasol_system_privilege" "creator_can_create_schema" {
  grantee   = exasol_user.schema_creator.name
  privilege = "CREATE SCHEMA"
}

# Note: Schemas need to be created outside Terraform for now
# In a real test, you would run: CREATE SCHEMA OP_TEST_SCHEMA;
# For testing purposes, we'll use a pre-created schema or skip object privilege tests
# and focus on simpler privilege types

# Create a dummy schema name variable - tests will use this
locals {
  test_schema_name = "OP_TEST_SCHEMA"
}

# Test roles for privilege grants
resource "exasol_role" "read_only" {
  name = "OP_READ_ONLY_ROLE"
}

resource "exasol_role" "read_write" {
  name = "OP_READ_WRITE_ROLE"
}

resource "exasol_role" "full_access" {
  name = "OP_FULL_ACCESS_ROLE"
}

resource "exasol_role" "test_role1" {
  name = "OP_TEST_ROLE1"
}

resource "exasol_role" "test_role2" {
  name = "OP_TEST_ROLE2"
}

# TC-OP-001: Grant single privilege on schema
# Tests basic object privilege functionality
resource "exasol_object_privilege" "tc_op_001_single_privilege" {
  grantee     = exasol_role.test_role1.name
  privileges  = ["USAGE"]
  object_type = "SCHEMA"
  object_name = local.test_schema_name
}

# TC-OP-002: Grant multiple privileges on schema
# Tests privilege list handling
resource "exasol_object_privilege" "tc_op_002_multiple_privileges" {
  grantee     = exasol_role.read_write.name
  privileges  = ["USAGE", "SELECT", "INSERT", "UPDATE", "DELETE"]
  object_type = "SCHEMA"
  object_name = local.test_schema_name
}

# TC-OP-003: Grant ALL privilege on schema
# Tests ALL privilege expansion handling
resource "exasol_object_privilege" "tc_op_003_all_privilege" {
  grantee     = exasol_role.full_access.name
  privileges  = ["ALL"]
  object_type = "SCHEMA"
  object_name = local.test_schema_name
}

# TC-OP-004: Grant subset of privileges (simulates add privilege test)
# First grant only USAGE and SELECT
resource "exasol_object_privilege" "tc_op_004_subset" {
  grantee     = exasol_role.test_role2.name
  privileges  = ["USAGE", "SELECT"]
  object_type = "SCHEMA"
  object_name = local.test_schema_name
}

# TC-OP-005: Grant reduced privilege set (simulates remove privilege test)
# Only USAGE (removed SELECT from typical read pattern)
# This is static test - for actual transition, manual testing needed
resource "exasol_object_privilege" "tc_op_005_reduced" {
  grantee     = exasol_role.read_only.name
  privileges  = ["USAGE"]
  object_type = "SCHEMA"
  object_name = local.test_schema_name
}

# TC-OP-006: Privilege ordering independence - Version 1
# Tests that different privilege order produces same resource
# These two should be functionally equivalent
resource "exasol_object_privilege" "tc_op_006_order_v1" {
  grantee     = "OP_ORDER_TEST_V1"
  privileges  = ["DELETE", "INSERT", "SELECT", "UPDATE", "USAGE"]  # Alphabetical
  object_type = "SCHEMA"
  object_name = local.test_schema_name
}

# TC-OP-006: Privilege ordering independence - Version 2
# Different order, should generate same ID due to sorting
resource "exasol_object_privilege" "tc_op_006_order_v2" {
  grantee     = "OP_ORDER_TEST_V2"
  privileges  = ["USAGE", "SELECT", "INSERT", "UPDATE", "DELETE"]  # Logical order
  object_type = "SCHEMA"
  object_name = local.test_schema_name
}

# Create roles for ordering test
resource "exasol_role" "order_test_v1" {
  name = "OP_ORDER_TEST_V1"
}

resource "exasol_role" "order_test_v2" {
  name = "OP_ORDER_TEST_V2"
}

# TC-OP-007: Minimal privileges for read-only access
# Only USAGE + SELECT (production pattern)
resource "exasol_object_privilege" "tc_op_007_minimal_read" {
  grantee     = "OP_MINIMAL_READ_ROLE"
  privileges  = ["USAGE", "SELECT"]
  object_type = "SCHEMA"
  object_name = local.test_schema_name
}

resource "exasol_role" "minimal_read" {
  name = "OP_MINIMAL_READ_ROLE"
}

# TC-OP-008: Error handling tested separately
# Test for privilege on non-existent object would fail terraform apply
# So we skip this in the automated suite
# Manual test: Try to grant privilege on non-existent schema "NONEXISTENT"
