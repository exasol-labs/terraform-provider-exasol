package resources

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"terraform-provider-exasol/internal/exasolclient"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}

// UserResource manages Exasol database users.
// It supports password, LDAP and OpenID authentication types.
type UserResource struct {
	db *sql.DB
}

func NewUserResource() resource.Resource { return &UserResource{} }

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates, updates (rename / change auth) and drops an Exasol user.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "User name. Exasol user names are case-insensitive.",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Terraform ID — always set to the user name in uppercase.",
			},
			"auth_type": schema.StringAttribute{
				Required:    true,
				Description: `Authentication type: "PASSWORD", "LDAP" or "OPENID".`,
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Password for PASSWORD authentication.",
			},
			"ldap_dn": schema.StringAttribute{
				Optional:    true,
				Description: "LDAP distinguished name if auth_type is LDAP.",
			},
			"openid_subject": schema.StringAttribute{
				Optional:    true,
				Description: "OpenID subject if auth_type is OPENID.",
			},
		},
	}
}

func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
	}
}

type userModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	AuthType      types.String `tfsdk:"auth_type"`
	Password      types.String `tfsdk:"password"`
	LDAPDN        types.String `tfsdk:"ldap_dn"`
	OpenIDSubject types.String `tfsdk:"openid_subject"`
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	upName := strings.ToUpper(plan.Name.ValueString())

	// Validate identifier
	if !isValidIdentifier(upName) {
		resp.Diagnostics.AddError("Invalid user name", "User name must not be empty.")
		return
	}

	sqlStmt, err := buildCreateUserSQL(plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid user configuration", err.Error())
		return
	}
	tflog.Info(ctx, "Creating user", map[string]any{"sql": sanitizeLogSQL(sqlStmt)})
	if _, err := r.db.ExecContext(ctx, sqlStmt); err != nil {
		resp.Diagnostics.AddError("CREATE USER failed", err.Error())
		return
	}

	// also grant CREATE SESSION so user can log in
	escapedName := escapeIdentifierLiteral(upName)
	grant := fmt.Sprintf(`GRANT CREATE SESSION TO "%s"`, escapedName)
	if _, err := r.db.ExecContext(ctx, grant); err != nil {
		resp.Diagnostics.AddError("Grant CREATE SESSION failed", err.Error())
		return
	}

	plan.ID = types.StringValue(upName)
	// Keep original name - don't uppercase it (Terraform expects consistency)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read back user details from EXA_DBA_USERS.
	// DISTINGUISHED_NAME and OPENID_SUBJECT are readable; password is a hash (not usable).
	var distinguishedName sql.NullString
	var openidSubject sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT DISTINGUISHED_NAME, OPENID_SUBJECT FROM EXA_DBA_USERS WHERE USER_NAME = ?`,
		state.ID.ValueString()).Scan(&distinguishedName, &openidSubject)
	if err == sql.ErrNoRows {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read user failed", err.Error())
		return
	}

	// After import, name is empty - populate it from the ID
	if state.Name.IsNull() || state.Name.ValueString() == "" {
		state.Name = state.ID
	}
	state.ID = types.StringValue(strings.ToUpper(state.Name.ValueString()))

	// Infer auth type and populate readable attributes from DB
	if distinguishedName.Valid && distinguishedName.String != "" {
		state.AuthType = types.StringValue("LDAP")
		state.LDAPDN = types.StringValue(distinguishedName.String)
	} else if openidSubject.Valid && openidSubject.String != "" {
		state.AuthType = types.StringValue("OPENID")
		state.OpenIDSubject = types.StringValue(openidSubject.String)
	} else if state.AuthType.IsNull() || state.AuthType.ValueString() == "" {
		// Only set PASSWORD if auth_type is not already known (import case)
		state.AuthType = types.StringValue("PASSWORD")
	}
	// Password is a hash in EXA_DBA_USERS - keep whatever is in state

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	upOld := strings.ToUpper(state.Name.ValueString())
	upNew := strings.ToUpper(plan.Name.ValueString())

	// Validate identifiers
	if !isValidIdentifier(upOld) {
		resp.Diagnostics.AddError("Invalid old user name", "User name must not be empty.")
		return
	}
	if !isValidIdentifier(upNew) {
		resp.Diagnostics.AddError("Invalid new user name", "User name must not be empty.")
		return
	}

	// Escape usernames for use in quoted identifiers
	escapedOld := escapeIdentifierLiteral(upOld)
	escapedNew := escapeIdentifierLiteral(upNew)

	if upOld != upNew {
		stmt := fmt.Sprintf(`RENAME USER "%s" TO "%s"`, escapedOld, escapedNew)
		tflog.Info(ctx, "Renaming user", map[string]any{"sql": stmt})
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			resp.Diagnostics.AddError("RENAME USER failed", err.Error())
			return
		}
	}

	// Change authentication if type/params changed
	if plan.AuthType.ValueString() != state.AuthType.ValueString() ||
		plan.Password.ValueString() != state.Password.ValueString() ||
		plan.LDAPDN.ValueString() != state.LDAPDN.ValueString() ||
		plan.OpenIDSubject.ValueString() != state.OpenIDSubject.ValueString() {
		alter, err := buildAlterUserSQL(plan)
		if err != nil {
			resp.Diagnostics.AddError("Invalid alter user config", err.Error())
			return
		}
		tflog.Info(ctx, "Altering user", map[string]any{"sql": sanitizeLogSQL(alter)})
		if _, err := r.db.ExecContext(ctx, alter); err != nil {
			resp.Diagnostics.AddError("ALTER USER failed", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(upNew)
	// Keep original name - don't uppercase it (Terraform expects consistency)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Serialize delete operations to prevent transaction collision errors
	lockDelete()
	defer unlockDelete()

	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	upName := strings.ToUpper(state.ID.ValueString())

	// Validate identifier
	if !isValidIdentifier(upName) {
		resp.Diagnostics.AddError("Invalid user name", "User name must not be empty.")
		return
	}

	// Escape username for use in quoted identifier
	escapedName := escapeIdentifierLiteral(upName)
	stmt := fmt.Sprintf(`DROP USER "%s"`, escapedName)
	tflog.Info(ctx, "Dropping user", map[string]any{"sql": stmt})
	if _, err := r.db.ExecContext(ctx, stmt); err != nil {
		resp.Diagnostics.AddError("DROP USER failed", err.Error())
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// allow import by username
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// --- helpers -------------------------------------------------------

func buildCreateUserSQL(m userModel) (string, error) {
	upName := strings.ToUpper(m.Name.ValueString())

	// Validate identifier
	if !isValidIdentifier(upName) {
		return "", fmt.Errorf("invalid user name: must not be empty")
	}

	// Escape username for use in quoted identifier
	escapedName := escapeIdentifierLiteral(upName)

	switch strings.ToUpper(m.AuthType.ValueString()) {
	case "PASSWORD":
		if m.Password.IsNull() {
			return "", fmt.Errorf("password must be set when auth_type is PASSWORD")
		}
		// Escape the password (which is used as an identifier literal in Exasol)
		escapedPassword := escapeIdentifierLiteral(m.Password.ValueString())
		return fmt.Sprintf(`CREATE USER "%s" IDENTIFIED BY "%s"`, escapedName, escapedPassword), nil
	case "LDAP":
		if m.LDAPDN.IsNull() {
			return "", fmt.Errorf("ldap_dn must be set when auth_type is LDAP")
		}
		// Escape the LDAP DN (string literal)
		escapedDN := escapeStringLiteral(m.LDAPDN.ValueString())
		return fmt.Sprintf(`CREATE USER "%s" IDENTIFIED AT LDAP AS '%s'`, escapedName, escapedDN), nil
	case "OPENID":
		if m.OpenIDSubject.IsNull() {
			return "", fmt.Errorf("openid_subject must be set when auth_type is OPENID")
		}
		// Escape the OpenID subject (string literal)
		escapedSubject := escapeStringLiteral(m.OpenIDSubject.ValueString())
		return fmt.Sprintf(`CREATE USER "%s" IDENTIFIED BY OPENID SUBJECT '%s'`, escapedName, escapedSubject), nil
	default:
		return "", fmt.Errorf("unsupported auth_type %q", m.AuthType.ValueString())
	}
}

func buildAlterUserSQL(m userModel) (string, error) {
	upName := strings.ToUpper(m.Name.ValueString())

	// Validate identifier
	if !isValidIdentifier(upName) {
		return "", fmt.Errorf("invalid user name: must not be empty")
	}

	// Escape username for use in quoted identifier
	escapedName := escapeIdentifierLiteral(upName)

	switch strings.ToUpper(m.AuthType.ValueString()) {
	case "PASSWORD":
		if m.Password.IsNull() {
			return "", fmt.Errorf("password must be set when auth_type is PASSWORD")
		}
		// Escape the password (which is used as an identifier literal in Exasol)
		escapedPassword := escapeIdentifierLiteral(m.Password.ValueString())
		return fmt.Sprintf(`ALTER USER "%s" IDENTIFIED BY "%s"`, escapedName, escapedPassword), nil
	case "LDAP":
		if m.LDAPDN.IsNull() {
			return "", fmt.Errorf("ldap_dn must be set when auth_type is LDAP")
		}
		// Escape the LDAP DN (string literal)
		escapedDN := escapeStringLiteral(m.LDAPDN.ValueString())
		return fmt.Sprintf(`ALTER USER "%s" IDENTIFIED AT LDAP AS '%s'`, escapedName, escapedDN), nil
	case "OPENID":
		if m.OpenIDSubject.IsNull() {
			return "", fmt.Errorf("openid_subject must be set when auth_type is OPENID")
		}
		// Escape the OpenID subject (string literal)
		escapedSubject := escapeStringLiteral(m.OpenIDSubject.ValueString())
		return fmt.Sprintf(`ALTER USER "%s" IDENTIFIED BY OPENID SUBJECT '%s'`, escapedName, escapedSubject), nil
	default:
		return "", fmt.Errorf("unsupported auth_type %q", m.AuthType.ValueString())
	}
}
