# Single privilege on a schema
resource "exasol_object_privilege" "schema_usage" {
  grantee     = exasol_role.analyst.name
  privileges  = ["USAGE"]
  object_type = "SCHEMA"
  object_name = "ANALYTICS"
}

# Multiple privileges on a schema
resource "exasol_object_privilege" "schema_read_write" {
  grantee     = exasol_role.etl_role.name
  privileges  = ["USAGE", "SELECT", "INSERT", "UPDATE", "DELETE"]
  object_type = "SCHEMA"
  object_name = "STAGING"
}

# Privilege on a specific table
resource "exasol_object_privilege" "table_select" {
  grantee     = exasol_role.analyst.name
  privileges  = ["SELECT"]
  object_type = "TABLE"
  object_name = "ANALYTICS.SALES"
}
