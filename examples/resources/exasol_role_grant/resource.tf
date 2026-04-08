# Grant role to user
resource "exasol_role_grant" "analyst_to_user" {
  role    = exasol_role.analyst.name
  grantee = exasol_user.analyst.name
}

# Grant role with ADMIN OPTION (grantee can grant this role to others)
resource "exasol_role_grant" "admin_grant" {
  role              = exasol_role.analyst.name
  grantee           = exasol_role.senior_analyst.name
  with_admin_option = true
}
