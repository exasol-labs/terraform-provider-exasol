# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Terraform provider for managing Exasol database resources (users, roles, schemas, connections, and privileges). It uses the Terraform Plugin Framework (not the legacy SDK) and connects to Exasol databases via the official exasol-driver-go.

**Important**: This is an open source project NOT officially supported by Exasol.

## Important Policies and Requirements

### GitHub Contribution Policy

**CRITICAL**: Do NOT add Claude as a contributor on GitHub or mention Claude in any GitHub-related content for this repository. This includes:
- Commit messages must NOT include "Co-Authored-By: Claude" or similar
- Pull request descriptions must NOT mention Claude Code
- Do NOT add any Claude Code attribution links or badges

### Testing Requirements

**ALWAYS test changes against a local Docker Exasol container**:

1. Check if the container exists and is running:
   ```bash
   docker ps -a | grep exasol
   ```

2. If the container exists but is stopped, start it:
   ```bash
   docker start <container_name>
   ```

3. If no container exists, create one:
   ```bash
   docker run -d --name exasoldb -p 8563:8563 exasol/docker-db:latest
   ```

4. **Default credentials**: `sys` / `exasol`

5. **Connection configuration for local Docker**: Set `validate_server_certificate = false` to bypass TLS certificate validation errors:
   ```hcl
   provider "exasol" {
     host                        = "localhost"
     port                        = 8563
     user                        = "sys"
     password                    = "exasol"
     validate_server_certificate = false
   }
   ```

6. **Quick test workflow**:
   ```bash
   make install-local
   cd test
   terraform init
   terraform apply -auto-approve
   terraform plan  # Should show "No changes" if no drift issues
   terraform destroy -auto-approve
   ```

### Changelog Policy

**Every PR MUST include an update to `CHANGELOG.md`**:
- Add entries under `## [Unreleased]` in the appropriate section (Added, Changed, Fixed, Security)
- Follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format
- Be specific: "Fix schema owner unknown after Create" not "Fix bug"
- When releasing: move Unreleased entries under a new version header, bump version in `main.go`

### Documentation

**ALWAYS check Exasol documentation at https://docs.exasol.com** when:
- Implementing new SQL features
- Working with Exasol-specific syntax
- Troubleshooting database behavior
- Understanding system views (EXA_DBA_* views)
- Verifying privilege semantics

### Python Usage Policy

**If Python scripts are required for any reason**:
- Use `uv venv` to create a virtual environment
- Do NOT commit Python scripts to the repository
- Do NOT commit the virtual environment (.venv, venv, etc.)
- Python is generally NOT part of this Go-based project and should only be used for temporary tasks
- Ensure `.gitignore` excludes any Python files and virtual environments

### Markdown File Policy

**Prevent "md-file-hell"**:
- Do NOT create unnecessary markdown files
- Before creating any .md file, verify it's absolutely necessary
- Re-check existing .md files before committing changes
- Do NOT create documentation files like TODO.md, NOTES.md, CHANGES.md unless explicitly requested
- The repository should only contain essential markdown files (README.md, CLAUDE.md, LICENSE, release notes)

## Build, Test, and Development Commands

```bash
# Build the provider
make build

# Run unit tests
make test

# Run integration tests (requires running Exasol database)
make test-integration

# Install provider locally for Terraform testing
make install-local

# Format code
make fmt

# Run linter
make lint

# Run all checks (format, vet, lint, test)
make check
```

### Working with Local Provider

After `make install-local`, configure Terraform to use the local build:

```hcl
terraform {
  required_providers {
    exasol = {
      source = "local/exasol/terraform-provider-exasol"
    }
  }
}
```

## Architecture

### Directory Structure

- `main.go` - Entry point, defines provider version and registry address
- `internal/provider/` - Provider implementation
  - `provider.go` - Main provider definition and resource registration
  - `client.go` - Database client creation (handles both password and PAT token auth)
  - `config.go` - Provider configuration schema and loading
- `internal/exasolclient/` - Thin wrapper around sql.DB
- `internal/resources/` - All Terraform resources
  - `user_resource.go` - User management (PASSWORD, LDAP, OPENID auth)
  - `role_resource.go` - Role management
  - `schema_resource.go` - Schema management with ownership transfer
  - `connection_resource.go` - External connections (S3, FTP, JDBC, etc.)
  - `system_privilege_resource.go` - System-level privileges (CREATE SESSION, etc.)
  - `object_privilege_resource.go` - Object-level privileges (SELECT, INSERT, etc.)
  - `role_grant_resource.go` - Role membership grants
  - `connection_grant_resource.go` - Connection access grants
  - `grant_resource.go` - Legacy grant resource (prefer specific grant resources)
  - `security.go` - Security helpers (identifier validation, SQL sanitization)
  - `helpers.go` - Utility functions (identifier quoting, escaping)

### Key Patterns

**Resource Lifecycle**: All resources follow standard Terraform Plugin Framework patterns:
- Implement `resource.Resource` interface
- Implement `resource.ResourceWithImportState` for import support
- Use `Configure()` to get database client from provider
- Store uppercase identifiers in state (Exasol normalizes to uppercase)

**SQL Execution**: Resources execute raw SQL statements using `db.ExecContext()` and `db.QueryContext()`. No ORM is used.

**State Verification**: Read operations query Exasol system views:
- `EXA_DBA_USERS` - User information
- `EXA_DBA_ROLES` - Role information
- `EXA_DBA_SCHEMAS` - Schema information
- `EXA_DBA_CONNECTIONS` - Connection information
- `EXA_DBA_SYS_PRIVS` - System privileges
- `EXA_DBA_OBJ_PRIVS` - Object privileges
- `EXA_DBA_ROLE_PRIVS` - Role grants
- `EXA_DBA_CONNECTION_PRIVS` - Connection grants

**Security**:
- All identifiers are validated using `isValidIdentifier()` to prevent SQL injection
- Identifiers are properly quoted using double quotes: `"IDENTIFIER"`
- String literals are escaped using `escapeStringLiteral()` (single quotes doubled)
- Passwords are redacted in logs using `sanitizeLogSQL()`
- The `qualify()` helper quotes and validates multi-part identifiers (SCHEMA.TABLE)

**Authentication**: The provider supports both traditional username/password and Exasol Personal Access Tokens (PAT). PAT tokens are detected by the `exa_pat_` prefix in `client.go`.

**Connection Strings**: The `host` parameter is passed directly to the exasol-driver-go library. For local Docker containers, use `validate_server_certificate = false` to bypass certificate validation errors. The driver also supports fingerprint validation - see exasol-driver-go documentation for advanced connection options.

**Version Management**: Version is defined in `main.go:14` and extracted by Makefile during build.

## Common Development Patterns

### Adding a New Resource

1. Create `internal/resources/{resource_name}_resource.go`
2. Implement the resource struct with `db *sql.DB` field
3. Add `New{Resource}Resource()` constructor
4. Implement required interfaces: `Resource`, `ResourceWithImportState`
5. Define schema in `Schema()` method
6. Implement CRUD operations using raw SQL
7. Query appropriate `EXA_DBA_*` views in `Read()`
8. Register in `internal/provider/provider.go` Resources() method

### Testing Resource Changes

**Use the `test/` directory for quick testing - it's pre-configured and ready to use**:

1. Ensure local Docker Exasol container is running (see "Testing Requirements" section above)
2. Build and install locally: `make install-local`
3. Run tests from the `test/` directory:
   ```bash
   cd test
   terraform init
   terraform apply -auto-approve
   terraform plan  # Check for drift - should show no changes
   terraform destroy -auto-approve
   ```

**For more comprehensive examples, use existing `examples/` directory - DO NOT create new test files**:

1. Use one of the existing example directories:
   - `examples/privileges/` - System privileges, object privileges, role grants (with and without ADMIN_OPTION)
   - `examples/grants/` - Legacy grant examples
   - `examples/connections/` - Connection examples
   - `examples/basic/` - Basic user/role/schema examples
2. Create `terraform.tfvars` in the example directory:
   ```hcl
   exa_host     = "localhost"
   exa_user     = "sys"
   exa_password = "exasol"
   ```
   Note: Make sure the provider configuration includes `validate_server_certificate = false` for local Docker containers
3. Navigate to the example directory and run:
   ```bash
   cd examples/privileges  # or other example directory
   terraform init
   terraform plan
   terraform apply
   terraform plan  # Check for drift - should show no changes
   terraform show
   terraform destroy
   ```

**Important**: Always check Exasol documentation at https://docs.exasol.com if any database-specific behavior is unclear

### Handling Drift

Resources detect drift by:
1. Reading current state from `EXA_DBA_*` views
2. Comparing with Terraform state
3. Returning proper diagnostics for missing resources

Common drift issues:
- **Case sensitivity**: Exasol stores identifiers in uppercase, ensure comparisons use uppercase
- **WITH ADMIN OPTION**: Check exact boolean values, handle `"TRUE"`/`"true"` variations (see `role_grant_resource.go:154-164`)
- **ALL privilege**: Some views expand `ALL` to individual privileges, check for both

### Working with Exasol SQL

Key SQL patterns used:
- `CREATE USER "name" IDENTIFIED BY 'password'`
- `ALTER USER "name" RENAME TO "new_name"`
- `CREATE ROLE "name"`
- `CREATE SCHEMA "name"`
- `CREATE CONNECTION "name" TO 'url' USER 'user' IDENTIFIED BY 'password'`
- `GRANT privilege TO "grantee" [WITH ADMIN OPTION]`
- `GRANT "role" TO "grantee" [WITH ADMIN OPTION]`
- `GRANT CONNECTION "conn" TO "grantee"`
- `ALTER SCHEMA "name" CHANGE OWNER "owner"`

## Important Gotchas

1. **Identifier Case**: Exasol normalizes unquoted identifiers to uppercase. Always use uppercase when comparing state or building queries.

2. **Password vs PAT**: Check for `exa_pat_` prefix to determine authentication method (`client.go:24-28`).

3. **Admin Option Drift**: Boolean comparison issues can occur due to database returning "TRUE" vs "true". See `role_grant_resource.go` for handling pattern.

4. **Connection Grants**: Use `EXA_DBA_CONNECTION_PRIVS` for reads, not `EXA_DBA_CONNECTIONS`.

5. **Legacy Grant Resource**: `grant_resource.go` exists for backward compatibility but new code should use specific grant resources (system_privilege, object_privilege, role_grant, connection_grant).

6. **Schema Ownership**: Ownership transfer happens after schema creation via `ALTER SCHEMA ... CHANGE OWNER`.

7. **No Test Files**: The repository has no automated tests. All testing must be done manually with actual Exasol database instances.

8. **Transaction Collision Prevention**: The provider uses a global mutex (`internal/resources/delete_mutex.go`) to serialize all delete operations. This prevents transaction collision errors (SQL error code 40001) that occur when multiple REVOKE/DROP statements execute simultaneously.

   **Current implementation**: All Delete methods call `lockDelete()` / `defer unlockDelete()` to serialize operations.

   **Future improvement**: Replace the global mutex with retry logic and exponential backoff. See `TODO.md` for implementation details. This would allow parallel deletes while gracefully handling occasional collisions.
