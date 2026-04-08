# Terraform Provider for Exasol

A Terraform provider for managing Exasol database resources: users, roles, schemas, connections, and privileges.

> **Warning**: This is an open source project not officially supported by Exasol. We will try to help you as much as possible, but can't guarantee anything since this is not an official Exasol product.

## Installation

This provider is not published to the Terraform Registry. Install it from [GitHub Releases](https://github.com/exasol-labs/terraform-provider-exasol/releases):

```bash
VERSION=0.2.0

# Available builds: linux_amd64, darwin_arm64
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')

# Download and install
curl -fLO "https://github.com/exasol-labs/terraform-provider-exasol/releases/download/v${VERSION}/terraform-provider-exasol_${VERSION}_${OS}_${ARCH}.zip"
mkdir -p ~/.terraform.d/plugins/registry.terraform.io/exasol/exasol/${VERSION}/${OS}_${ARCH}
unzip terraform-provider-exasol_${VERSION}_${OS}_${ARCH}.zip \
  -d ~/.terraform.d/plugins/registry.terraform.io/exasol/exasol/${VERSION}/${OS}_${ARCH}/
rm terraform-provider-exasol_${VERSION}_${OS}_${ARCH}.zip
```

Then reference it in your Terraform configuration:

```hcl
terraform {
  required_providers {
    exasol = {
      source  = "exasol/exasol"
      version = "~> 0.2.0"
    }
  }
}
```

## Provider Configuration

```hcl
provider "exasol" {
  host     = "exasol.example.com"
  port     = 8563                        # default
  user     = "sys"
  password = "exasol"
  validate_server_certificate = true     # default; set false for self-signed certs
}
```

The provider also accepts Exasol Personal Access Tokens (PAT) in the `password` field. Tokens starting with `exa_pat_` are automatically detected.

## Resources

| Resource | Description |
|---|---|
| [`exasol_user`](docs/resources/user.md) | Database users (password, LDAP, or OpenID auth) |
| [`exasol_role`](docs/resources/role.md) | Database roles |
| [`exasol_schema`](docs/resources/schema.md) | Schemas with optional ownership control |
| [`exasol_connection`](docs/resources/connection.md) | External connections (S3, FTP, JDBC, etc.) |
| [`exasol_system_privilege`](docs/resources/system_privilege.md) | System-level privileges (CREATE SESSION, etc.) |
| [`exasol_object_privilege`](docs/resources/object_privilege.md) | Object-level privileges (SELECT, INSERT, etc.) |
| [`exasol_role_grant`](docs/resources/role_grant.md) | Role-to-user or role-to-role grants |
| [`exasol_connection_grant`](docs/resources/connection_grant.md) | Connection access grants |

All resources support in-place rename and `terraform import`. See individual resource docs for usage examples and import syntax.

## Quick Example

```hcl
resource "exasol_role" "analyst" {
  name = "ANALYST_ROLE"
}

resource "exasol_schema" "analytics" {
  name  = "ANALYTICS"
  owner = exasol_role.analyst.name
}

resource "exasol_object_privilege" "schema_access" {
  grantee     = exasol_role.analyst.name
  privileges  = ["USAGE", "SELECT"]
  object_type = "SCHEMA"
  object_name = exasol_schema.analytics.name
}
```

## Development

```bash
git clone https://github.com/exasol-labs/terraform-provider-exasol.git
cd terraform-provider-exasol
make build             # Build the provider
make test              # Unit tests
make test-integration  # Requires running Exasol database
make install-local     # Install for local Terraform testing
```

Requires Go 1.25+, Terraform 1.0+, and Make.

## License

See [LICENSE](LICENSE) file.
