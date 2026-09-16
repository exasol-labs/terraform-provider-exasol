#!/bin/sh

# Format: GRANTEE|TARGET
terraform import exasol_user_impersonation_grant.mcp 'MCP_ROLE|ALICE@EXAMPLE.COM'
