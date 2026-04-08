resource "exasol_system_privilege" "create_session" {
  grantee   = exasol_user.analyst.name
  privilege = "CREATE SESSION"
}

# With ADMIN OPTION (grantee can grant this privilege to others)
resource "exasol_system_privilege" "create_schema" {
  grantee           = exasol_role.etl_role.name
  privilege         = "CREATE SCHEMA"
  with_admin_option = true
}
