# Test Suite 5: Real-World Production Setup
# Test: TC-RW-001
# Replicates production module pattern from bi-aws-terraform/modules/exasol
# Focus: Multi-layer privilege hierarchy (RAW -> SNAP -> STG -> MART)

terraform {
  required_providers {
    exasol = {
      source  = "local/exasol/exasol"
    }
  }
}

provider "exasol" {
  host                        = "localhost"
  port                        = 8563
  user                        = "sys"
  password                    = "exasol"
  validate_server_certificate = false
}

# Schema names (created by setup script)
locals {
  raw_schema  = "RW_RAW_SCHEMA"
  snap_schema = "RW_SNAP_SCHEMA"
  stg_schema  = "RW_STG_SCHEMA"
  mart_schema = "RW_MART_SCHEMA"
}

# --- LAYER 1: RAW Layer Roles ---

# Individual schema roles for RAW layer
resource "exasol_role" "raw_schema1_sr" {
  name = "RW_RAW_SCHEMA1_SR"
}

resource "exasol_role" "raw_schema1_srw" {
  name = "RW_RAW_SCHEMA1_SRW"
}

# Aggregative RAW roles (production pattern)
resource "exasol_role" "all_raw_sr" {
  name = "RW_ALL_RAW_SR"
}

resource "exasol_role" "all_raw_srw" {
  name = "RW_ALL_RAW_SRW"
}

# Grant individual roles to aggregative roles with ADMIN OPTION
resource "exasol_role_grant" "raw_schema1_sr_to_all" {
  role              = exasol_role.raw_schema1_sr.name
  grantee           = exasol_role.all_raw_sr.name
  with_admin_option = true
}

resource "exasol_role_grant" "raw_schema1_srw_to_all" {
  role              = exasol_role.raw_schema1_srw.name
  grantee           = exasol_role.all_raw_srw.name
  with_admin_option = true
}

# Object privileges for RAW layer
resource "exasol_object_privilege" "raw_schema1_read" {
  grantee     = exasol_role.raw_schema1_sr.name
  privileges  = ["USAGE", "SELECT"]
  object_type = "SCHEMA"
  object_name = local.raw_schema
}

resource "exasol_object_privilege" "raw_schema1_write" {
  grantee     = exasol_role.raw_schema1_srw.name
  privileges  = ["USAGE", "SELECT", "INSERT", "UPDATE", "DELETE"]
  object_type = "SCHEMA"
  object_name = local.raw_schema
}

# --- LAYER 2: SNAP Layer Roles ---

resource "exasol_role" "snap_schema1_sr" {
  name = "RW_SNAP_SCHEMA1_SR"
}

resource "exasol_role" "snap_schema1_srw" {
  name = "RW_SNAP_SCHEMA1_SRW"
}

resource "exasol_role" "all_snap_sr" {
  name = "RW_ALL_SNAP_SR"
}

resource "exasol_role" "all_snap_srw" {
  name = "RW_ALL_SNAP_SRW"
}

resource "exasol_role_grant" "snap_schema1_sr_to_all" {
  role              = exasol_role.snap_schema1_sr.name
  grantee           = exasol_role.all_snap_sr.name
  with_admin_option = true
}

resource "exasol_role_grant" "snap_schema1_srw_to_all" {
  role              = exasol_role.snap_schema1_srw.name
  grantee           = exasol_role.all_snap_srw.name
  with_admin_option = true
}

resource "exasol_object_privilege" "snap_schema1_read" {
  grantee     = exasol_role.snap_schema1_sr.name
  privileges  = ["USAGE", "SELECT"]
  object_type = "SCHEMA"
  object_name = local.snap_schema
}

resource "exasol_object_privilege" "snap_schema1_write" {
  grantee     = exasol_role.snap_schema1_srw.name
  privileges  = ["USAGE", "SELECT", "INSERT", "UPDATE", "DELETE"]
  object_type = "SCHEMA"
  object_name = local.snap_schema
}

# --- LAYER 3: STG Layer Roles ---

resource "exasol_role" "stg_schema1_sr" {
  name = "RW_STG_SCHEMA1_SR"
}

resource "exasol_role" "stg_schema1_srw" {
  name = "RW_STG_SCHEMA1_SRW"
}

resource "exasol_role" "all_stg_sr" {
  name = "RW_ALL_STG_SR"
}

resource "exasol_role" "all_stg_srw" {
  name = "RW_ALL_STG_SRW"
}

resource "exasol_role_grant" "stg_schema1_sr_to_all" {
  role              = exasol_role.stg_schema1_sr.name
  grantee           = exasol_role.all_stg_sr.name
  with_admin_option = true
}

resource "exasol_role_grant" "stg_schema1_srw_to_all" {
  role              = exasol_role.stg_schema1_srw.name
  grantee           = exasol_role.all_stg_srw.name
  with_admin_option = true
}

resource "exasol_object_privilege" "stg_schema1_read" {
  grantee     = exasol_role.stg_schema1_sr.name
  privileges  = ["USAGE", "SELECT"]
  object_type = "SCHEMA"
  object_name = local.stg_schema
}

resource "exasol_object_privilege" "stg_schema1_write" {
  grantee     = exasol_role.stg_schema1_srw.name
  privileges  = ["USAGE", "SELECT", "INSERT", "UPDATE", "DELETE"]
  object_type = "SCHEMA"
  object_name = local.stg_schema
}

# --- LAYER 4: MART Layer Roles ---

resource "exasol_role" "mart_schema1_sr" {
  name = "RW_MART_SCHEMA1_SR"
}

resource "exasol_role" "mart_schema1_srw" {
  name = "RW_MART_SCHEMA1_SRW"
}

resource "exasol_role" "all_mart_sr" {
  name = "RW_ALL_MART_SR"
}

resource "exasol_role" "all_mart_srw" {
  name = "RW_ALL_MART_SRW"
}

resource "exasol_role_grant" "mart_schema1_sr_to_all" {
  role              = exasol_role.mart_schema1_sr.name
  grantee           = exasol_role.all_mart_sr.name
  with_admin_option = true
}

resource "exasol_role_grant" "mart_schema1_srw_to_all" {
  role              = exasol_role.mart_schema1_srw.name
  grantee           = exasol_role.all_mart_srw.name
  with_admin_option = true
}

resource "exasol_object_privilege" "mart_schema1_read" {
  grantee     = exasol_role.mart_schema1_sr.name
  privileges  = ["USAGE", "SELECT"]
  object_type = "SCHEMA"
  object_name = local.mart_schema
}

resource "exasol_object_privilege" "mart_schema1_write" {
  grantee     = exasol_role.mart_schema1_srw.name
  privileges  = ["USAGE", "SELECT", "INSERT", "UPDATE", "DELETE"]
  object_type = "SCHEMA"
  object_name = local.mart_schema
}

# --- CROSS-LAYER GRANTS (Critical Production Pattern) ---
# STG can read RAW
resource "exasol_role_grant" "stg_reads_raw" {
  role              = exasol_role.all_raw_sr.name
  grantee           = exasol_role.all_stg_srw.name
  with_admin_option = true
}

# MART can read STG
resource "exasol_role_grant" "mart_reads_stg" {
  role              = exasol_role.all_stg_sr.name
  grantee           = exasol_role.all_mart_srw.name
  with_admin_option = true
}

# MART can read SNAP
resource "exasol_role_grant" "mart_reads_snap" {
  role              = exasol_role.all_snap_sr.name
  grantee           = exasol_role.all_mart_srw.name
  with_admin_option = true
}

# --- TECHNICAL USERS (Production Pattern) ---

resource "exasol_user" "etl_user" {
  name      = "RW_ETL_USER"
  auth_type = "PASSWORD"
  password  = "EtlPass123!"
}

resource "exasol_user" "bi_user" {
  name      = "RW_BI_USER"
  auth_type = "PASSWORD"
  password  = "BiPass456!"
}

# ETL Pipeline Role with system privileges
resource "exasol_role" "etl_pipeline_role" {
  name = "RW_ETL_PIPELINE_ROLE"
}

resource "exasol_system_privilege" "etl_create_table" {
  grantee   = exasol_role.etl_pipeline_role.name
  privilege = "CREATE TABLE"
}

resource "exasol_system_privilege" "etl_create_view" {
  grantee   = exasol_role.etl_pipeline_role.name
  privilege = "CREATE VIEW"
}

resource "exasol_system_privilege" "etl_import" {
  grantee   = exasol_role.etl_pipeline_role.name
  privilege = "IMPORT"
}

# Grant aggregative roles to technical users
resource "exasol_role_grant" "etl_all_raw_write" {
  role    = exasol_role.all_raw_srw.name
  grantee = exasol_role.etl_pipeline_role.name
}

resource "exasol_role_grant" "etl_all_snap_write" {
  role    = exasol_role.all_snap_srw.name
  grantee = exasol_role.etl_pipeline_role.name
}

resource "exasol_role_grant" "etl_all_stg_write" {
  role    = exasol_role.all_stg_srw.name
  grantee = exasol_role.etl_pipeline_role.name
}

resource "exasol_role_grant" "etl_pipeline_to_user" {
  role    = exasol_role.etl_pipeline_role.name
  grantee = exasol_user.etl_user.name
}

# BI user only reads MART
resource "exasol_role_grant" "bi_reads_mart" {
  role    = exasol_role.all_mart_sr.name
  grantee = exasol_user.bi_user.name
}

# --- CONNECTION GRANTS (Production Pattern) ---

resource "exasol_connection" "s3_connection" {
  name       = "RW_S3_CONNECTION"
  connection_string = "https://s3.amazonaws.com/test-bucket"
  username   = "test-key"
  password   = "test-secret"
}

resource "exasol_connection_grant" "s3_to_etl_role" {
  connection_name = exasol_connection.s3_connection.name
  grantee         = exasol_role.etl_pipeline_role.name
}
