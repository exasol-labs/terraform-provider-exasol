# Password authentication
resource "exasol_user" "analyst" {
  name      = "ANALYST_USER"
  auth_type = "PASSWORD"
  password  = var.analyst_password
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
