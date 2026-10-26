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
var _ resource.ResourceWithModifyPlan = &UserResource{}

// UserResource manages Exasol database users.
// It supports password, LDAP and OpenID authentication types.
type UserResource struct {
	db      *sql.DB
	reads   *exasolclient.ReadCache
	renames *exasolclient.RenameLog
}

func NewUserResource() resource.Resource { return &UserResource{} }

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates, updates (rename / change auth) and drops an Exasol user. " +
			"Like Exasol's CREATE USER, it grants no privileges: grant CREATE SESSION with " +
			"exasol_system_privilege so the user can log in. " +
			"Renaming keeps its grants: Exasol moves them to the new name. When a grant that references it changes in the same apply, " +
			"the provider revokes the previous grant under the new name. If the rename and the grant change land in different runs " +
			"(-target, a failed apply, another provider alias, or a rename outside Terraform), check the renamed object for leftover grants.",
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

// ModifyPlan warns when a user is about to be created. Up to v0.2.x the
// provider granted CREATE SESSION implicitly; configurations that relied on it
// need an exasol_system_privilege resource now, or the new user cannot log in.
// Remove this warning in v0.4.0, once the migration window has passed.
func (r *UserResource) ModifyPlan(_ context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if !req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	resp.Diagnostics.AddWarning(
		"exasol_user no longer grants CREATE SESSION",
		"Since v0.3.0, exasol_user only runs CREATE USER, like Exasol itself. "+
			"If this user needs to log in, add an exasol_system_privilege resource "+
			"with privilege = \"CREATE SESSION\". Existing users keep their grant.",
	)
}

func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
		r.reads = c.Reads
		r.renames = c.Renames
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
	if _, err := execWrite(ctx, r.db, r.reads, sqlStmt); err != nil {
		resp.Diagnostics.AddError("CREATE USER failed", err.Error())
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
	row, err := cachedRow(ctx, r.db, r.reads,
		`SELECT USER_NAME, DISTINGUISHED_NAME, OPENID_SUBJECT FROM EXA_DBA_USERS`, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read user failed", err.Error())
		return
	}
	if row == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	distinguishedName, openidSubject := row[1], row[2]

	// After import, name is empty - populate it from the ID
	if state.Name.IsNull() || state.Name.ValueString() == "" {
		state.Name = state.ID
	}
	state.ID = types.StringValue(strings.ToUpper(state.Name.ValueString()))

	// Infer auth type from DB and clear stale auth-specific fields.
	// This ensures drift is detected when auth is changed externally.
	if distinguishedName.Valid && distinguishedName.String != "" {
		state.AuthType = types.StringValue("LDAP")
		state.LDAPDN = types.StringValue(distinguishedName.String)
		state.OpenIDSubject = types.StringNull()
	} else if openidSubject.Valid && openidSubject.String != "" {
		state.AuthType = types.StringValue("OPENID")
		state.OpenIDSubject = types.StringValue(openidSubject.String)
		state.LDAPDN = types.StringNull()
	} else {
		state.AuthType = types.StringValue("PASSWORD")
		state.LDAPDN = types.StringNull()
		state.OpenIDSubject = types.StringNull()
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
		// Hold the grant lock across RENAME and Record, so a grant Update either
		// sees the rename in the log or revokes under the old name before it.
		lockDelete()
		stmt := fmt.Sprintf(`RENAME USER "%s" TO "%s"`, escapedOld, escapedNew)
		tflog.Info(ctx, "Renaming user", map[string]any{"sql": stmt})
		_, err := execWrite(ctx, r.db, r.reads, stmt)
		if err == nil {
			r.renames.Record(exasolclient.RenamePrincipal, upOld, upNew)
		}
		unlockDelete()
		if err != nil {
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
		if _, err := execWrite(ctx, r.db, r.reads, alter); err != nil {
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
	if _, err := execWrite(ctx, r.db, r.reads, stmt); err != nil {
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
