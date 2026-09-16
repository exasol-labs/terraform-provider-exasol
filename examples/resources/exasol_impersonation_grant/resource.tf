locals {
  regular_users = [
    "ALICE@example.com",
    "BOB@example.com",
  ]
}

# One Terraform resource is created for each regular user.
resource "exasol_impersonation_grant" "mcp" {
  for_each = toset(local.regular_users)

  grantee           = "MCP_ROLE"
  impersonated_user = each.value
}
