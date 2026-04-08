resource "exasol_schema" "analytics" {
  name = "ANALYTICS"
}

# Schema with ownership transfer
resource "exasol_schema" "staging" {
  name  = "STAGING"
  owner = exasol_role.etl_role.name
}
