# Password authentication
resource "exasol_user" "analyst" {
  name      = "ANALYST_USER"
  auth_type = "PASSWORD"
  password  = var.analyst_password
}

# exasol_user grants no privileges, like Exasol's CREATE USER.
# Grant CREATE SESSION explicitly so the user can log in.
resource "exasol_system_privilege" "analyst_login" {
  grantee   = exasol_user.analyst.name
  privilege = "CREATE SESSION"
}

# LDAP authentication
resource "exasol_user" "ldap_user" {
  name      = "LDAP_USER"
  auth_type = "LDAP"
  ldap_dn   = "cn=ldap_user,dc=example,dc=com"
}

# OpenID authentication
resource "exasol_user" "openid_user" {
  name           = "OPENID_USER"
  auth_type      = "OPENID"
  openid_subject = "user@example.com"
}
