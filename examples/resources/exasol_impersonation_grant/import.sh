#!/bin/sh

# Format: GRANTEE|IMPERSONATED_USER
terraform import exasol_impersonation_grant.mcp 'MCP_ROLE|ALICE@EXAMPLE.COM'
