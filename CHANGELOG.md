# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed
- Replace `WriteString(Sprintf)` with `Fprintf` to satisfy staticcheck QF1012

### Changed
- CI no longer triggers a release on every push to main; releases only when version is bumped
- Update exasol-driver-go 1.0.14 to 1.0.16
- Update terraform-plugin-framework 1.16.0 to 1.19.0
- Update terraform-plugin-log 0.9.0 to 0.10.0

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
