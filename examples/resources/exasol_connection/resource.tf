# S3 connection
resource "exasol_connection" "s3" {
  name     = "MY_S3_BUCKET"
  to       = "https://my-bucket.s3.us-east-1.amazonaws.com"
  user     = var.aws_access_key
  password = var.aws_secret_key
}

# JDBC connection
resource "exasol_connection" "oracle" {
  name     = "ORACLE_DB"
  to       = "jdbc:oracle:thin:@//oracle-host:1521/ORCL"
  user     = "oracle_user"
  password = var.oracle_password
}

# Connection without credentials
resource "exasol_connection" "public_http" {
  name = "PUBLIC_API"
  to   = "https://api.example.com"
}
