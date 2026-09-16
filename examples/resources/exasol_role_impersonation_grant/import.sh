#!/bin/sh

# Format: GRANTEE|TARGET
terraform import exasol_role_impersonation_grant.mcp 'MCP_ROLE|TECHNICAL_DBA_ROLE'
