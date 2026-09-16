package resources

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBuildCreateUserSQL_Password(t *testing.T) {
	m := userModel{
		Name:     types.StringValue("myuser"),
		AuthType: types.StringValue("PASSWORD"),
		Password: types.StringValue("secret123"),
	}
	got, err := buildCreateUserSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `CREATE USER "MYUSER" IDENTIFIED BY "secret123"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCreateUserSQL_PasswordWithDoubleQuote(t *testing.T) {
	m := userModel{
		Name:     types.StringValue("myuser"),
		AuthType: types.StringValue("PASSWORD"),
		Password: types.StringValue(`pass"word`),
	}
	got, err := buildCreateUserSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `CREATE USER "MYUSER" IDENTIFIED BY "pass""word"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCreateUserSQL_LDAP(t *testing.T) {
	m := userModel{
		Name:     types.StringValue("ldapuser"),
		AuthType: types.StringValue("LDAP"),
		LDAPDN:   types.StringValue("cn=user,dc=example,dc=com"),
	}
	got, err := buildCreateUserSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `CREATE USER "LDAPUSER" IDENTIFIED AT LDAP AS 'cn=user,dc=example,dc=com'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCreateUserSQL_LDAPWithSingleQuote(t *testing.T) {
	m := userModel{
		Name:     types.StringValue("ldapuser"),
		AuthType: types.StringValue("LDAP"),
		LDAPDN:   types.StringValue("cn=user's,dc=example,dc=com"),
	}
	got, err := buildCreateUserSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `CREATE USER "LDAPUSER" IDENTIFIED AT LDAP AS 'cn=user''s,dc=example,dc=com'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCreateUserSQL_OpenID(t *testing.T) {
	m := userModel{
		Name:          types.StringValue("oidcuser"),
		AuthType:      types.StringValue("OPENID"),
		OpenIDSubject: types.StringValue("subject-123"),
	}
	got, err := buildCreateUserSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `CREATE USER "OIDCUSER" IDENTIFIED BY OPENID SUBJECT 'subject-123'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCreateUserSQL_MissingPassword(t *testing.T) {
	m := userModel{
		Name:     types.StringValue("myuser"),
		AuthType: types.StringValue("PASSWORD"),
		Password: types.StringNull(),
	}
	_, err := buildCreateUserSQL(m)
	if err == nil {
		t.Fatal("expected error for PASSWORD auth without password, got nil")
	}
}

func TestBuildCreateUserSQL_MissingLDAPDN(t *testing.T) {
	m := userModel{
		Name:     types.StringValue("myuser"),
		AuthType: types.StringValue("LDAP"),
		LDAPDN:   types.StringNull(),
	}
	_, err := buildCreateUserSQL(m)
	if err == nil {
		t.Fatal("expected error for LDAP auth without ldap_dn, got nil")
	}
}

func TestBuildCreateUserSQL_MissingOpenIDSubject(t *testing.T) {
	m := userModel{
		Name:          types.StringValue("myuser"),
		AuthType:      types.StringValue("OPENID"),
		OpenIDSubject: types.StringNull(),
	}
	_, err := buildCreateUserSQL(m)
	if err == nil {
		t.Fatal("expected error for OPENID auth without openid_subject, got nil")
	}
}

func TestBuildCreateUserSQL_UnsupportedAuthType(t *testing.T) {
	m := userModel{
		Name:     types.StringValue("myuser"),
		AuthType: types.StringValue("KERBEROS"),
	}
	_, err := buildCreateUserSQL(m)
	if err == nil {
		t.Fatal("expected error for unsupported auth type, got nil")
	}
}

func TestBuildCreateUserSQL_EmptyName(t *testing.T) {
	m := userModel{
		Name:     types.StringValue(""),
		AuthType: types.StringValue("PASSWORD"),
		Password: types.StringValue("pass"),
	}
	_, err := buildCreateUserSQL(m)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestBuildAlterUserSQL_Password(t *testing.T) {
	m := userModel{
		Name:     types.StringValue("myuser"),
		AuthType: types.StringValue("PASSWORD"),
		Password: types.StringValue("newpass"),
	}
	got, err := buildAlterUserSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `ALTER USER "MYUSER" IDENTIFIED BY "newpass"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCreateConnectionSQL_Basic(t *testing.T) {
	m := connectionModel{
		Name:     types.StringValue("myconn"),
		To:       types.StringValue("localhost:8563"),
		User:     types.StringNull(),
		Password: types.StringNull(),
	}
	got, err := buildCreateConnectionSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `CREATE CONNECTION "MYCONN" TO 'localhost:8563'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCreateConnectionSQL_WithCredentials(t *testing.T) {
	m := connectionModel{
		Name:     types.StringValue("s3conn"),
		To:       types.StringValue("https://bucket.s3.amazonaws.com"),
		User:     types.StringValue("AKIAIOSFODNN7EXAMPLE"),
		Password: types.StringValue("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
	}
	got, err := buildCreateConnectionSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `CREATE CONNECTION "S3CONN" TO 'https://bucket.s3.amazonaws.com' USER 'AKIAIOSFODNN7EXAMPLE' IDENTIFIED BY 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCreateConnectionSQL_WithSingleQuoteInPassword(t *testing.T) {
	m := connectionModel{
		Name:     types.StringValue("myconn"),
		To:       types.StringValue("host:1234"),
		User:     types.StringValue("admin"),
		Password: types.StringValue("pass'word"),
	}
	got, err := buildCreateConnectionSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `CREATE CONNECTION "MYCONN" TO 'host:1234' USER 'admin' IDENTIFIED BY 'pass''word'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCreateConnectionSQL_EmptyName(t *testing.T) {
	m := connectionModel{
		Name: types.StringValue(""),
		To:   types.StringValue("host:1234"),
	}
	_, err := buildCreateConnectionSQL(m)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestBuildAlterConnectionSQL_Basic(t *testing.T) {
	m := connectionModel{
		Name:     types.StringValue("myconn"),
		To:       types.StringValue("newhost:9999"),
		User:     types.StringNull(),
		Password: types.StringNull(),
	}
	got, err := buildAlterConnectionSQL(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `ALTER CONNECTION "MYCONN" TO 'newhost:9999'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- ID builder tests ---

func TestSystemPrivilegeID(t *testing.T) {
	m := systemPrivilegeModel{
		Grantee:         types.StringValue("myuser"),
		Privilege:       types.StringValue("create session"),
		WithAdminOption: types.BoolValue(true),
	}
	got := systemPrivilegeID(m)
	want := "MYUSER|CREATE SESSION|true"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSystemPrivilegeID_NoAdmin(t *testing.T) {
	m := systemPrivilegeModel{
		Grantee:         types.StringValue("myuser"),
		Privilege:       types.StringValue("CREATE TABLE"),
		WithAdminOption: types.BoolNull(),
	}
	got := systemPrivilegeID(m)
	want := "MYUSER|CREATE TABLE|false"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRoleGrantID(t *testing.T) {
	m := roleGrantModel{
		Role:            types.StringValue("analyst"),
		Grantee:         types.StringValue("myuser"),
		WithAdminOption: types.BoolValue(true),
	}
	got := roleGrantID(m)
	want := "ANALYST|MYUSER|true"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImpersonationGrantSQL(t *testing.T) {
	grant, revoke := impersonationGrantSQL("mcp_role", "alice@example.com")
	if grant != `GRANT IMPERSONATION ON "ALICE@EXAMPLE.COM" TO "MCP_ROLE"` {
		t.Fatalf("grant SQL = %q", grant)
	}
	if revoke != `REVOKE IMPERSONATION ON "ALICE@EXAMPLE.COM" FROM "MCP_ROLE"` {
		t.Fatalf("revoke SQL = %q", revoke)
	}
}

func TestImpersonationGrantSQL_EscapesIdentifiers(t *testing.T) {
	grant, revoke := impersonationGrantSQL(`mcp"role`, `alice"user`)
	if grant != `GRANT IMPERSONATION ON "ALICE""USER" TO "MCP""ROLE"` {
		t.Fatalf("grant SQL = %q", grant)
	}
	if revoke != `REVOKE IMPERSONATION ON "ALICE""USER" FROM "MCP""ROLE"` {
		t.Fatalf("revoke SQL = %q", revoke)
	}
}

func TestImpersonationGrantID(t *testing.T) {
	got := impersonationGrantID("mcp_role", "alice@example.com")
	if got != "MCP_ROLE|ALICE@EXAMPLE.COM" {
		t.Fatalf("ID = %q", got)
	}
}

func TestProtectedImpersonationRoles(t *testing.T) {
	for _, role := range []string{"PUBLIC", "public", "DBA", "dba"} {
		if !isProtectedImpersonationRole(role) {
			t.Errorf("isProtectedImpersonationRole(%q) = false, want true", role)
		}
	}
	if isProtectedImpersonationRole("REPORTING_ROLE") {
		t.Error("ordinary role was treated as protected")
	}
}
