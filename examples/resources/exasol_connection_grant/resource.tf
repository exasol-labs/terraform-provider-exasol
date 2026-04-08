resource "exasol_connection_grant" "s3_to_etl" {
  connection_name = exasol_connection.s3.name
  grantee         = exasol_role.etl_role.name
}
