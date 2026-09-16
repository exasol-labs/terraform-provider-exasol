locals {
  regular_users = [
    "ALICE@example.com",
    "BOB@example.com",
  ]
}

resource "exasol_user_impersonation_grant" "mcp" {
  for_each = toset(local.regular_users)

  grantee = "MCP_ROLE"
  target  = each.value
}
