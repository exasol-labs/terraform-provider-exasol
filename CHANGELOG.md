# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.3.2] - 2026-10-26

### Changed
- Refresh reads each system view once per plan, refresh or import (per provider configuration) instead of once per resource. `terraform plan` sent one query per resource to `EXA_DBA_ROLE_PRIVS`, `EXA_DBA_SYS_PRIVS`, `EXA_DBA_OBJ_PRIVS`, `EXA_DBA_IMPERSONATION_PRIVS`, `EXA_DBA_CONNECTION_PRIVS`, `EXA_DBA_USERS`, `EXA_DBA_ROLES`, `EXA_DBA_CONNECTIONS` and `EXA_ALL_SCHEMAS`, and each query costs about the same however few rows it returns. The provider now loads each view on the first read and answers the other resources from memory. Every statement that changes the database clears these copies, so a later read sees the database again. Create, update and delete still query the database directly. Measured on a local database with a copy of a production configuration (3,779 Exasol resources): 8,689 queries and 19 seconds per plan before, 10 queries and 4 seconds after, with the same plan, also after grants were revoked outside Terraform

### Fixed
- Statements that collide with a parallel transaction (Exasol SQL error 40001) are run again, up to 5 times within about 3 seconds. Exasol rolls a colliding statement back completely. Parallel creates, such as many `exasol_user_impersonation_grant` resources in one apply, failed with "Transaction collision" before
- `exasol_schema` drops a schema it just created when setting its comment or owner fails. Before, the schema stayed outside Terraform state and the next apply failed with "already exists". The schema is dropped without `CASCADE`, so only an empty schema is removed
- `exasol_schema`, `exasol_user` and `exasol_connection` keep the new name in state when a rename succeeds and a later step of the same update fails. Before, the next run did not find the object and planned to create it again

## [0.3.1] - 2026-10-26

### Added
- `exasol_schema` has a new optional `comment` attribute that sets the schema comment with `COMMENT ON SCHEMA`. Removing the attribute clears the comment. Comments set outside Terraform, or present on an imported schema, are not tracked until `comment` is set, so upgrading does not plan any change for existing schemas. The plan rejects an empty string, because Exasol stores it as NULL, and a comment longer than 2000 characters, the Exasol limit

## [0.3.0] - 2026-10-15

### Changed
- **BREAKING:** `exasol_user` no longer grants `CREATE SESSION` on create. It only runs `CREATE USER`, like Exasol itself, so login is managed in one place and can be revoked by Terraform. Users created with this version cannot log in until you add an `exasol_system_privilege` resource with `privilege = "CREATE SESSION"`. Existing users keep their grant and show no plan change. `terraform plan` shows a warning for each `exasol_user` that is about to be created
  - Migration: before or after upgrading, add one `exasol_system_privilege` with `privilege = "CREATE SESSION"` per user that should log in (`grantee = exasol_user.<name>.name`; if your users use `for_each`, use `for_each = exasol_user.<name>` and `grantee = each.value.name`). Applying it re-grants an existing privilege, which Exasol accepts without change; alternatively import it with the ID `USER|CREATE SESSION|false`
  - A user replaced with `-replace` or `taint` is created without `CREATE SESSION`. No warning is shown in this case, because Terraform passes the prior state to the provider

### Fixed
- Renaming a user, role, connection or schema that grants reference no longer fails in `exasol_system_privilege`, `exasol_role_grant`, `exasol_object_privilege`, `exasol_connection_grant`, `exasol_user_impersonation_grant` and `exasol_role_impersonation_grant`, and no longer leaves grants behind. Exasol moves grants to the new name itself. The provider now records the renames it executes in an apply, so the grant resources know where a previous grant lives and revoke it there when the grant changes in the same apply (another privilege, role, connection, target, object or privilege list, or another grantee). `Delete` no longer fails when the grant is already gone
  - Limit: the provider only knows renames it executed in the same apply and provider configuration. If a rename and a change of a grant that references it land in different runs (`-target` on the renamed resource, an apply that fails between the rename and the grant update, another provider alias against the same database, or a rename outside Terraform), the previous grant can stay under the new name outside Terraform state. Exasol has no stable ID for users, roles and connections, so this cannot be detected from the database. The same applies when another object takes over the old name in the same apply (rename A to C, then B to A) and the grant moves to it without any other change: Terraform plans no update for the grant. Check the renamed object for leftover grants in these cases
- Renames now hold the grant lock (see below), so a grant update cannot run between a rename and its record
- `exasol_object_privilege` on a table or view (`object_name = "SCHEMA.TABLE"`) is found again on refresh. Exasol stores the schema and the object name in separate columns, so the lookup never matched: every plan wanted to create the grant again. Schema grants were not affected
- `exasol_object_privilege`: moving an `ALL` grant to a grantee that already holds one of its privileges now grants all of them
- `exasol_object_privilege` remembers the exact object it granted on (in private state, not visible in plans), and refresh, update and delete stay on that object. Schemas that differ only in case (`s` and `S`, which `exasol_schema` can create) are kept apart, also when one of them is dropped outside Terraform or appears later next to a target that was resolved from another spelling. An unchanged `object_name` keeps its object, unless that object's schema is renamed in the same apply; then the configured spelling names the schema that now holds it. When the exact object is not known yet (an import, or state from 0.2.x), the exact spelling of `object_name` is tried first and its uppercase spelling second, so a lowercase spelling of an uppercase object still works. Refresh now also finds grants on lowercase schemas, which were created again on every plan before. A failed `REVOKE` during an update is now an error instead of a log warning
- Moving a grant to a grantee, role, connection, impersonation target or object that does not exist now fails before any `REVOKE`, with "... does not exist". Before, the previous grant was revoked first and the apply then failed, which removed access
- Grant updates are serialized with deletes, like deletes already were, to avoid Exasol transaction collisions (error 40001)

## [0.2.5] - 2026-09-16

### Added
- Scoped `exasol_user_impersonation_grant` and `exasol_role_impersonation_grant` resources for granting impersonation of individual users and roles

### Security
- Update `google.golang.org/grpc` from 1.83.1 to 1.83.2 for GHSA-2v4p-qf9q-27wj, plus refreshed indirect dependencies

## [0.2.4] - 2026-09-16

### Security
- Update `google.golang.org/grpc` from 1.82.1 to 1.83.1, including fixes for xDS RBAC authorization bypasses and HTTP/2 resource-exhaustion handling

## [0.2.3] - 2026-08-24

### Added
- New optional provider attribute `connect_timeout` (seconds, default 60): the initial connection is retried every 10 seconds until the timeout elapses, each attempt bounded by its own context. Only network errors are retried; failed logins or certificate errors surface immediately, and 0 disables retrying. Runs no longer fail while a freshly added IP-allowlist entry is still propagating on Exasol SaaS

### Security
- Update `google.golang.org/grpc` 1.79.3 to 1.82.1 (xDS RBAC and HTTP/2 vulnerabilities)
- Update `golang.org/x/crypto` 0.48.0 to 0.52.0 (multiple SSH advisories incl. auth bypass via unenforced @revoked status)
- Update `golang.org/x/net` 0.49.0 to 0.55.0

## [0.2.2] - 2026-06-17

### Added
- Provider docs Overview now includes a description, installation instructions, and the "not officially supported by Exasol" warning, via a new `templates/index.md.tmpl`

### Fixed
- GitHub release bodies were empty: `changelog.disable: true` in `.goreleaser.yaml` caused goreleaser to ignore `--release-notes`. Removed it so release notes are populated from the `CHANGELOG.md` section

## [0.2.1] - 2026-06-17

### Added
- GPG signing of the `SHA256SUMS` checksum file in goreleaser (`signs:` block) to produce the `SHA256SUMS.sig` required for Terraform Registry publishing
- GPG key import (`crazy-max/ghaction-import-gpg`) in the release job, feeding `GPG_FINGERPRINT` to goreleaser
- Release binaries now built for the full Terraform Registry platform matrix (linux, darwin, windows, freebsd across `386`/`amd64`/`arm`/`arm64`)

### Changed
- Update `github.com/cloudflare/circl` 1.6.1 to 1.6.3 (transitive dependency; ecc/p384 scalar-multiplication fix)
- README installation rewritten for the Terraform Registry flow (`source = "exasol-labs/exasol"`, `terraform init`); fix stale `~> 0.2.0` version pin
- Provider registry address in `main.go` corrected to `registry.terraform.io/exasol-labs/exasol` (was `exasol/exasol`) to match the `exasol-labs` publishing namespace
- Local dev install namespace aligned to `local/exasol-labs/exasol` (Makefile `PROVIDER_NAME`, `test/*.tf` source blocks, CLAUDE.md) so it mirrors the registry namespace

### Removed
- End-user local/manual install path and `install.sh` (the provider now installs from the Terraform Registry; `make install-local` is retained for development)

### Fixed
- Release workflow used the non-existent goreleaser flag `--release-header-file`; corrected to `--release-notes` so the GitHub release body is populated from the `CHANGELOG.md` section

## [0.2.0] - 2026-04-08

### Fixed
- Replace `WriteString(Sprintf)` with `Fprintf` to satisfy staticcheck QF1012
- Schema `owner` attribute left unknown after Create and Update (Computed attribute must be known after apply)
- Role and schema `name` not populated in Read after `terraform import`
- User and connection `ID` overwritten from empty `name` after `terraform import`
- Connection Read now reads `to` and `user` from `EXA_DBA_CONNECTIONS` (previously only checked existence)
- User Read switched from `EXA_ALL_USERS` to `EXA_DBA_USERS` to detect auth type, LDAP DN, and OpenID subject after import
- User Read now always infers `auth_type` from DB (detects external auth changes, clears stale `ldap_dn`/`openid_subject`)
- Connection Read clears `user` when removed externally (drift was invisible)
- Connection name no longer uppercased in state (caused perpetual drift when config used non-uppercase)
- Provider Address in `main.go` now matches actual source `exasol/exasol` (was `exasol/terraform-provider-exasol`)
- `make install-local` now installs to `local/exasol/exasol` matching test suite source blocks

### Added
- Unit tests for helpers, security functions, SQL builders, and ID builders (41 tests)
- Terraform acceptance tests for all resource types: create, rename (verified in-place), import, CheckDestroy (11 tests)
- `CHANGELOG.md` as single source of truth for release notes
- `docs/` directory with per-resource documentation generated by `tfplugindocs` (examples, schema reference, import syntax)

### Changed
- CI no longer triggers a release on every push to main; releases only when version is bumped
- Update exasol-driver-go 1.0.14 to 1.0.16
- Update terraform-plugin-framework 1.16.0 to 1.19.0
- Update terraform-plugin-log 0.9.0 to 0.10.0
- Connection pool tuning: `MaxIdleConns=10`, `MaxOpenConns=10` (reduces session churn from 40 to 10 on large workloads)
- Delete stale `user_resource.go.bak`
- README.md: fix wrong org URL, stale version pin, wrong Go version, remove redundant sections
- Delete `TODO.md` (retry logic deprioritized, mutex documented in CLAUDE.md)
- Trim `test/README.md`: remove references to non-existent docs, point to acceptance tests
- Include `terraform-registry-manifest.json` in goreleaser release artifacts (registry-ready)
- Update all CI GitHub Actions to latest major versions (checkout v6, cache v5, setup-terraform v4, codecov v6, golangci-lint v9, goreleaser v7)

### Removed
- `exasol_grant` resource (use `exasol_system_privilege`, `exasol_object_privilege`, `exasol_role_grant`, `exasol_connection_grant` instead)

### Security
- Bump google.golang.org/grpc 1.75.1 to 1.79.3 (authorization bypass fix)

## [0.1.8] - 2025-12-18

### Fixed
- Preserve original name case in user resource
- Allow quoted identifiers with special characters in user names

## [0.1.6] - 2025-11-11

### Fixed
- Handle lowercase 'true' for ADMIN_OPTION in role grants
- Move test_grants.go to cmd/test-grants to avoid main conflict

### Added
- Comprehensive test infrastructure with 6 test suites

## [0.1.5] - 2025-11-09

### Fixed
- Use EXA_DBA_CONNECTION_PRIVS for connection grant Read

## [0.1.4] - 2025-11-09

### Fixed
- Resolve role_grant with_admin_option drift issue

### Added
- with_admin_option examples in README

## [0.1.3] - 2025-11-09

### Fixed
- Use EXA_DBA_OBJ_PRIVS for connection grant read

## [0.1.1] - 2025-11-07

### Added
- Schema owner attribute
- Connection grant resource
- MIT License
- Fork PR protection in CI

## [0.1.0] - 2025-09-29

### Added
- Initial release: Terraform Provider for Exasol
- User, role, schema, connection resources
- System privilege, object privilege, role grant resources
- PAT token authentication support
