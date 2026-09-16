locals {
  impersonated_roles = [
    "TECHNICAL_DBA_ROLE",
    "REPORTING_ROLE",
  ]
}

resource "exasol_role_impersonation_grant" "mcp" {
  for_each = toset(local.impersonated_roles)

  grantee = "MCP_ROLE"
  target  = each.value
}
