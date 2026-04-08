# Test Suite 4: Connection Grants - Comprehensive Testing
# Tests: TC-CG-001 through TC-CG-004
# Focus: Connection access grants to users and roles

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

# Test users for connection grants
resource "exasol_user" "data_engineer" {
  name      = "CG_DATA_ENGINEER"
  auth_type = "PASSWORD"
  password  = "TestPass123!"
}

resource "exasol_user" "etl_user" {
  name      = "CG_ETL_USER"
  auth_type = "PASSWORD"
  password  = "TestPass456!"
}

# Test roles for connection grants
resource "exasol_role" "connection_user_role" {
  name = "CG_CONNECTION_USER_ROLE"
}

resource "exasol_role" "etl_role" {
  name = "CG_ETL_ROLE"
}

# Test connections
resource "exasol_connection" "s3_test" {
  name       = "CG_S3_TEST_CONNECTION"
  connection_string = "https://s3.amazonaws.com/test-bucket"
  username   = "test-access-key"
  password   = "test-secret-key"
}

resource "exasol_connection" "jdbc_test" {
  name       = "CG_JDBC_TEST_CONNECTION"
  connection_string = "jdbc:postgresql://localhost:5432/testdb"
  username   = "testuser"
  password   = "testpass"
}

# TC-CG-001: Grant connection to user
# Direct user access to connection
resource "exasol_connection_grant" "tc_cg_001_user_grant" {
  connection_name = exasol_connection.s3_test.name
  grantee         = exasol_user.data_engineer.name
}

# TC-CG-002: Grant connection to role
# Preferred production pattern: grant to role, not user
resource "exasol_connection_grant" "tc_cg_002_role_grant" {
  connection_name = exasol_connection.s3_test.name
  grantee         = exasol_role.connection_user_role.name
}

# Grant role to user to test inheritance
resource "exasol_role_grant" "connection_role_to_user" {
  role    = exasol_role.connection_user_role.name
  grantee = exasol_user.etl_user.name
}

# TC-CG-003: Multiple connection grants to same user
# User has access to multiple connections
resource "exasol_connection_grant" "tc_cg_003_conn1" {
  connection_name = exasol_connection.s3_test.name
  grantee         = exasol_user.etl_user.name
}

resource "exasol_connection_grant" "tc_cg_003_conn2" {
  connection_name = exasol_connection.jdbc_test.name
  grantee         = exasol_user.etl_user.name
}

# TC-CG-004: Connection grant with case normalization
# Mixed case input should be normalized to uppercase
resource "exasol_connection_grant" "tc_cg_004_case_test" {
  connection_name = exasol_connection.jdbc_test.name
  grantee         = exasol_role.etl_role.name
}

# Additional test: Role-based connection workflow (production pattern)
# Connection -> Role -> User
resource "exasol_connection" "athena_test" {
  name       = "CG_ATHENA_TEST_CONNECTION"
  connection_string = "jdbc:awsathena://athena.us-east-1.amazonaws.com:443"
  username   = "test-key"
  password   = "test-secret"
}

resource "exasol_connection_grant" "athena_to_role" {
  connection_name = exasol_connection.athena_test.name
  grantee         = exasol_role.etl_role.name
}

resource "exasol_role_grant" "etl_role_to_data_engineer" {
  role    = exasol_role.etl_role.name
  grantee = exasol_user.data_engineer.name
}
