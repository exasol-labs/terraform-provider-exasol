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

var _ resource.Resource = &SystemPrivilegeResource{}
var _ resource.ResourceWithImportState = &SystemPrivilegeResource{}

// SystemPrivilegeResource manages Exasol system privileges.
// System privileges include: CREATE SESSION, CREATE TABLE, CREATE SCHEMA, etc.
type SystemPrivilegeResource struct {
	db      *sql.DB
	reads   *exasolclient.ReadCache
	renames *exasolclient.RenameLog
}

func NewSystemPrivilegeResource() resource.Resource {
	return &SystemPrivilegeResource{}
}

func (r *SystemPrivilegeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_privilege"
}

func (r *SystemPrivilegeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Grants system-level privileges to users or roles. " +
			"System privileges include CREATE SESSION, CREATE TABLE, CREATE SCHEMA, USE ANY SCHEMA, etc.",
		Attributes: map[string]schema.Attribute{
			"grantee": schema.StringAttribute{
				Required:    true,
				Description: "User or role name receiving the privilege.",
			},
			"privilege": schema.StringAttribute{
				Required:    true,
				Description: "System privilege name (e.g., 'CREATE SESSION', 'CREATE TABLE', 'USE ANY SCHEMA').",
			},
			"with_admin_option": schema.BoolAttribute{
				Optional:    true,
				Description: "Grant the privilege with ADMIN OPTION, allowing the grantee to grant this privilege to others.",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Terraform ID in format: GRANTEE|PRIVILEGE|ADMIN_OPTION",
			},
		},
	}
}

func (r *SystemPrivilegeResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
		r.reads = c.Reads
		r.renames = c.Renames
	}
}

type systemPrivilegeModel struct {
	ID              types.String `tfsdk:"id"`
	Grantee         types.String `tfsdk:"grantee"`
	Privilege       types.String `tfsdk:"privilege"`
	WithAdminOption types.Bool   `tfsdk:"with_admin_option"`
}

func (r *SystemPrivilegeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemPrivilegeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer lockGrantees(plan.Grantee.ValueString())()
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	grantee := strings.ToUpper(plan.Grantee.ValueString())
	privilege := strings.ToUpper(plan.Privilege.ValueString())

	// Validate identifiers
	if !isValidIdentifier(grantee) {
		resp.Diagnostics.AddError("Invalid grantee", "Grantee name contains invalid characters")
		return
	}

	// Build GRANT statement
	stmt := fmt.Sprintf(`GRANT %s TO "%s"`, privilege, grantee)
	if !plan.WithAdminOption.IsNull() && plan.WithAdminOption.ValueBool() {
		stmt += " WITH ADMIN OPTION"
	}

	tflog.Info(ctx, "Granting system privilege", map[string]any{"sql": stmt})
	if _, err := execWrite(ctx, r.db, r.reads, stmt); err != nil {
		resp.Diagnostics.AddError("GRANT failed", err.Error())
		return
	}

	plan.ID = types.StringValue(systemPrivilegeID(plan))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SystemPrivilegeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	var state systemPrivilegeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	grantee := strings.ToUpper(state.Grantee.ValueString())
	privilege := strings.ToUpper(state.Privilege.ValueString())

	// Check if privilege exists in EXA_DBA_SYS_PRIVS
	row, err := cachedRow(ctx, r.db, r.reads,
		`SELECT GRANTEE, PRIVILEGE, ADMIN_OPTION FROM EXA_DBA_SYS_PRIVS`, grantee, privilege)
	if err != nil {
		resp.Diagnostics.AddError("Read system privilege failed", err.Error())
		return
	}
	if row == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	adminOption := row[2].String

	// Set with_admin_option based on database value
	// If database has TRUE, set to true. If database has FALSE, set to null.
	// This is because in Exasol, there's no distinction between "not specified" and "false"
	// Both result in no admin option. This prevents drift when upgrading from old provider versions.
	// Handle both uppercase (SaaS: "TRUE"/"1") and lowercase (Docker: "true") variants
	if adminOption == "TRUE" || adminOption == "1" || adminOption == "true" {
		state.WithAdminOption = types.BoolValue(true)
	} else {
		state.WithAdminOption = types.BoolNull()
	}
	state.ID = types.StringValue(systemPrivilegeID(state))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SystemPrivilegeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Update may REVOKE; serialize it with Delete to avoid transaction
	// collisions (40001), see delete_mutex.go.
	lockDelete()
	defer unlockDelete()

	var plan, state systemPrivilegeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer lockGrantees(plan.Grantee.ValueString(), state.Grantee.ValueString())()
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	// If grantee or privilege changed, need to revoke old and grant new. A
	// grantee renamed away in this run counts as changed even if the
	// configured name is the same.
	if plan.Grantee.ValueString() != state.Grantee.ValueString() ||
		plan.Privilege.ValueString() != state.Privilege.ValueString() ||
		renamedAway(r.renames, exasolclient.RenamePrincipal, state.Grantee.ValueString(), plan.Grantee.ValueString()) {

		oldGrantee := strings.ToUpper(state.Grantee.ValueString())
		oldPrivilege := strings.ToUpper(state.Privilege.ValueString())
		newGrantee := strings.ToUpper(plan.Grantee.ValueString())
		newPrivilege := strings.ToUpper(plan.Privilege.ValueString())
		if !requireExists(ctx, r.db, resp.Diagnostics.AddError, "Grantee", newGrantee, principalExists) {
			return
		}

		// Find where the previous grant lives now: Exasol moves privileges to
		// the new name when the grantee is renamed. Revoke it there unless it
		// is the grant this resource keeps. A dropped grantee took its
		// privileges with it.
		curGrantee, gone, err := resolveEndpoint(ctx, r.db, r.renames, exasolclient.RenamePrincipal, oldGrantee, newGrantee, principalExists)
		if err != nil {
			resp.Diagnostics.AddError("Check grantee failed", err.Error())
			return
		}
		if !gone && (curGrantee != newGrantee || oldPrivilege != newPrivilege) {
			oldExists, _, err := r.privilegeExists(ctx, curGrantee, oldPrivilege)
			if err != nil {
				resp.Diagnostics.AddError("Check system privilege failed", err.Error())
				return
			}
			if oldExists {
				revokeStmt := fmt.Sprintf(`REVOKE %s FROM "%s"`, oldPrivilege, curGrantee)
				tflog.Info(ctx, "Revoking old system privilege", map[string]any{"sql": revokeStmt})
				if _, err := execWrite(ctx, r.db, r.reads, revokeStmt); err != nil {
					resp.Diagnostics.AddError("REVOKE failed", err.Error())
					return
				}
			}
		}

		wantAdmin := !plan.WithAdminOption.IsNull() && plan.WithAdminOption.ValueBool()
		newExists, newAdmin, err := r.privilegeExists(ctx, newGrantee, newPrivilege)
		if err != nil {
			resp.Diagnostics.AddError("Check system privilege failed", err.Error())
			return
		}
		if newExists && newAdmin != wantAdmin {
			revokeStmt := fmt.Sprintf(`REVOKE %s FROM "%s"`, newPrivilege, newGrantee)
			tflog.Info(ctx, "Revoking system privilege to update admin option", map[string]any{"sql": revokeStmt})
			if _, err := execWrite(ctx, r.db, r.reads, revokeStmt); err != nil {
				resp.Diagnostics.AddError("REVOKE failed", err.Error())
				return
			}
			newExists = false
		}
		if !newExists {
			grantStmt := fmt.Sprintf(`GRANT %s TO "%s"`, newPrivilege, newGrantee)
			if wantAdmin {
				grantStmt += " WITH ADMIN OPTION"
			}
			tflog.Info(ctx, "Granting new system privilege", map[string]any{"sql": grantStmt})
			if _, err := execWrite(ctx, r.db, r.reads, grantStmt); err != nil {
				resp.Diagnostics.AddError("GRANT failed", err.Error())
				return
			}
		}
	} else if plan.WithAdminOption.ValueBool() != state.WithAdminOption.ValueBool() {
		// Only admin option changed - need to revoke and re-grant
		grantee := strings.ToUpper(plan.Grantee.ValueString())
		privilege := strings.ToUpper(plan.Privilege.ValueString())

		revokeStmt := fmt.Sprintf(`REVOKE %s FROM "%s"`, privilege, grantee)
		tflog.Info(ctx, "Revoking system privilege to update admin option", map[string]any{"sql": revokeStmt})
		if _, err := execWrite(ctx, r.db, r.reads, revokeStmt); err != nil {
			resp.Diagnostics.AddError("REVOKE failed", err.Error())
			return
		}

		grantStmt := fmt.Sprintf(`GRANT %s TO "%s"`, privilege, grantee)
		if !plan.WithAdminOption.IsNull() && plan.WithAdminOption.ValueBool() {
			grantStmt += " WITH ADMIN OPTION"
		}
		tflog.Info(ctx, "Re-granting system privilege with updated admin option", map[string]any{"sql": grantStmt})
		if _, err := execWrite(ctx, r.db, r.reads, grantStmt); err != nil {
			resp.Diagnostics.AddError("GRANT failed", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(systemPrivilegeID(plan))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SystemPrivilegeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Serialize delete operations to prevent transaction collision errors
	lockDelete()
	defer unlockDelete()

	var state systemPrivilegeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer lockGrantees(state.Grantee.ValueString())()
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	grantee := strings.ToUpper(state.Grantee.ValueString())
	privilege := strings.ToUpper(state.Privilege.ValueString())
	// A grantee renamed in this run carries the grant to its new name.
	if current, ok := r.renames.Lookup(exasolclient.RenamePrincipal, grantee); ok {
		grantee = current
	}
	// Already gone, e.g. the grantee was dropped: nothing to revoke.
	exists, _, err := r.privilegeExists(ctx, grantee, privilege)
	if err != nil {
		resp.Diagnostics.AddError("Check system privilege failed", err.Error())
		return
	}
	if !exists {
		return
	}
	stmt := fmt.Sprintf(`REVOKE %s FROM "%s"`, privilege, grantee)

	tflog.Info(ctx, "Revoking system privilege", map[string]any{"sql": stmt})
	if _, err := execWrite(ctx, r.db, r.reads, stmt); err != nil {
		resp.Diagnostics.AddError("REVOKE failed", err.Error())
	}
}

func (r *SystemPrivilegeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// ID format: GRANTEE|PRIVILEGE|ADMIN_OPTION
	parts := strings.Split(req.ID, "|")
	if len(parts) != 3 {
		resp.Diagnostics.AddError("Invalid import ID",
			`Expected format: "GRANTEE|PRIVILEGE|true|false"`)
		return
	}
	resp.State.SetAttribute(ctx, path.Root("grantee"), parts[0])
	resp.State.SetAttribute(ctx, path.Root("privilege"), parts[1])
	resp.State.SetAttribute(ctx, path.Root("with_admin_option"), strings.EqualFold(parts[2], "true"))
	resp.State.SetAttribute(ctx, path.Root("id"), req.ID)
}

// privilegeExists reports whether grantee holds privilege directly, and with
// which admin option.
func (r *SystemPrivilegeResource) privilegeExists(ctx context.Context, grantee, privilege string) (bool, bool, error) {
	var adminOption string
	err := r.db.QueryRowContext(ctx,
		`SELECT ADMIN_OPTION FROM EXA_DBA_SYS_PRIVS WHERE GRANTEE = ? AND PRIVILEGE = ?`,
		grantee, privilege).Scan(&adminOption)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, isTrueAdminOption(adminOption), nil
}

func systemPrivilegeID(m systemPrivilegeModel) string {
	grantee := strings.ToUpper(m.Grantee.ValueString())
	privilege := strings.ToUpper(m.Privilege.ValueString())
	adminOption := "false"
	if !m.WithAdminOption.IsNull() && m.WithAdminOption.ValueBool() {
		adminOption = "true"
	}
	return fmt.Sprintf("%s|%s|%s", grantee, privilege, adminOption)
}
