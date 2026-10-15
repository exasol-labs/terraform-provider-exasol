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

var _ resource.Resource = &RoleGrantResource{}
var _ resource.ResourceWithImportState = &RoleGrantResource{}

// RoleGrantResource manages granting roles to users or other roles.
type RoleGrantResource struct {
	db *sql.DB
}

func NewRoleGrantResource() resource.Resource {
	return &RoleGrantResource{}
}

func (r *RoleGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role_grant"
}

func (r *RoleGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Grants a role to a user or another role. " +
			"This is distinct from object and system privileges - it assigns role membership.",
		Attributes: map[string]schema.Attribute{
			"role": schema.StringAttribute{
				Required:    true,
				Description: "Role name to grant.",
			},
			"grantee": schema.StringAttribute{
				Required:    true,
				Description: "User or role name receiving the role.",
			},
			"with_admin_option": schema.BoolAttribute{
				Optional:    true,
				Description: "Grant the role with ADMIN OPTION, allowing the grantee to grant this role to others.",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Terraform ID in format: ROLE|GRANTEE|ADMIN_OPTION",
			},
		},
	}
}

func (r *RoleGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
	}
}

type roleGrantModel struct {
	ID              types.String `tfsdk:"id"`
	Role            types.String `tfsdk:"role"`
	Grantee         types.String `tfsdk:"grantee"`
	WithAdminOption types.Bool   `tfsdk:"with_admin_option"`
}

func (r *RoleGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	role := strings.ToUpper(plan.Role.ValueString())
	grantee := strings.ToUpper(plan.Grantee.ValueString())

	// Validate identifiers
	if !isValidIdentifier(role) {
		resp.Diagnostics.AddError("Invalid role name", "Role name contains invalid characters")
		return
	}
	if !isValidIdentifier(grantee) {
		resp.Diagnostics.AddError("Invalid grantee", "Grantee name contains invalid characters")
		return
	}

	// Build GRANT statement
	stmt := fmt.Sprintf(`GRANT "%s" TO "%s"`, role, grantee)
	if !plan.WithAdminOption.IsNull() && plan.WithAdminOption.ValueBool() {
		stmt += " WITH ADMIN OPTION"
	}

	tflog.Info(ctx, "Granting role", map[string]any{"sql": stmt})
	if _, err := r.db.ExecContext(ctx, stmt); err != nil {
		resp.Diagnostics.AddError("GRANT failed", err.Error())
		return
	}

	plan.ID = types.StringValue(roleGrantID(plan))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RoleGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	var state roleGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	role := strings.ToUpper(state.Role.ValueString())
	grantee := strings.ToUpper(state.Grantee.ValueString())

	// Check if role grant exists in EXA_DBA_ROLE_PRIVS
	query := `SELECT ADMIN_OPTION FROM EXA_DBA_ROLE_PRIVS WHERE GRANTED_ROLE = ? AND GRANTEE = ?`
	var adminOption string
	err := r.db.QueryRowContext(ctx, query, role, grantee).Scan(&adminOption)
	if err == sql.ErrNoRows {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read role grant failed", err.Error())
		return
	}

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
	state.ID = types.StringValue(roleGrantID(state))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RoleGrantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state roleGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	// If role or grantee changed, need to revoke old and grant new
	if plan.Role.ValueString() != state.Role.ValueString() ||
		plan.Grantee.ValueString() != state.Grantee.ValueString() {

		// Renaming a role or grantee in Exasol moves its grants to the new
		// name, so the old grant may be gone and the new one may already exist.
		oldRole := strings.ToUpper(state.Role.ValueString())
		oldGrantee := strings.ToUpper(state.Grantee.ValueString())
		oldExists, _, err := r.grantExists(ctx, oldRole, oldGrantee)
		if err != nil {
			resp.Diagnostics.AddError("Check role grant failed", err.Error())
			return
		}
		if oldExists {
			revokeStmt := fmt.Sprintf(`REVOKE "%s" FROM "%s"`, oldRole, oldGrantee)
			tflog.Info(ctx, "Revoking old role grant", map[string]any{"sql": revokeStmt})
			if _, err := r.db.ExecContext(ctx, revokeStmt); err != nil {
				resp.Diagnostics.AddError("REVOKE failed", err.Error())
				return
			}
		}

		newRole := strings.ToUpper(plan.Role.ValueString())
		newGrantee := strings.ToUpper(plan.Grantee.ValueString())
		wantAdmin := !plan.WithAdminOption.IsNull() && plan.WithAdminOption.ValueBool()
		newExists, newAdmin, err := r.grantExists(ctx, newRole, newGrantee)
		if err != nil {
			resp.Diagnostics.AddError("Check role grant failed", err.Error())
			return
		}
		if newExists && newAdmin != wantAdmin {
			revokeStmt := fmt.Sprintf(`REVOKE "%s" FROM "%s"`, newRole, newGrantee)
			tflog.Info(ctx, "Revoking role to update admin option", map[string]any{"sql": revokeStmt})
			if _, err := r.db.ExecContext(ctx, revokeStmt); err != nil {
				resp.Diagnostics.AddError("REVOKE failed", err.Error())
				return
			}
			newExists = false
		}
		if !newExists {
			grantStmt := fmt.Sprintf(`GRANT "%s" TO "%s"`, newRole, newGrantee)
			if wantAdmin {
				grantStmt += " WITH ADMIN OPTION"
			}
			tflog.Info(ctx, "Granting new role", map[string]any{"sql": grantStmt})
			if _, err := r.db.ExecContext(ctx, grantStmt); err != nil {
				resp.Diagnostics.AddError("GRANT failed", err.Error())
				return
			}
		}
	} else if plan.WithAdminOption.ValueBool() != state.WithAdminOption.ValueBool() {
		// Only admin option changed - need to revoke and re-grant
		role := strings.ToUpper(plan.Role.ValueString())
		grantee := strings.ToUpper(plan.Grantee.ValueString())

		revokeStmt := fmt.Sprintf(`REVOKE "%s" FROM "%s"`, role, grantee)
		tflog.Info(ctx, "Revoking role to update admin option", map[string]any{"sql": revokeStmt})
		if _, err := r.db.ExecContext(ctx, revokeStmt); err != nil {
			resp.Diagnostics.AddError("REVOKE failed", err.Error())
			return
		}

		grantStmt := fmt.Sprintf(`GRANT "%s" TO "%s"`, role, grantee)
		if !plan.WithAdminOption.IsNull() && plan.WithAdminOption.ValueBool() {
			grantStmt += " WITH ADMIN OPTION"
		}
		tflog.Info(ctx, "Re-granting role with updated admin option", map[string]any{"sql": grantStmt})
		if _, err := r.db.ExecContext(ctx, grantStmt); err != nil {
			resp.Diagnostics.AddError("GRANT failed", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(roleGrantID(plan))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RoleGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Serialize delete operations to prevent transaction collision errors
	lockDelete()
	defer unlockDelete()

	var state roleGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	role := strings.ToUpper(state.Role.ValueString())
	grantee := strings.ToUpper(state.Grantee.ValueString())
	// Already gone, e.g. the grantee was renamed or dropped: nothing to revoke.
	exists, _, err := r.grantExists(ctx, role, grantee)
	if err != nil {
		resp.Diagnostics.AddError("Check role grant failed", err.Error())
		return
	}
	if !exists {
		return
	}
	stmt := fmt.Sprintf(`REVOKE "%s" FROM "%s"`, role, grantee)

	tflog.Info(ctx, "Revoking role grant", map[string]any{"sql": stmt})
	if _, err := r.db.ExecContext(ctx, stmt); err != nil {
		resp.Diagnostics.AddError("REVOKE failed", err.Error())
	}
}

func (r *RoleGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// ID format: ROLE|GRANTEE|ADMIN_OPTION
	parts := strings.Split(req.ID, "|")
	if len(parts) != 3 {
		resp.Diagnostics.AddError("Invalid import ID",
			`Expected format: "ROLE|GRANTEE|true|false"`)
		return
	}
	resp.State.SetAttribute(ctx, path.Root("role"), parts[0])
	resp.State.SetAttribute(ctx, path.Root("grantee"), parts[1])
	resp.State.SetAttribute(ctx, path.Root("with_admin_option"), strings.EqualFold(parts[2], "true"))
	resp.State.SetAttribute(ctx, path.Root("id"), req.ID)
}

func roleGrantID(m roleGrantModel) string {
	role := strings.ToUpper(m.Role.ValueString())
	grantee := strings.ToUpper(m.Grantee.ValueString())
	adminOption := "false"
	if !m.WithAdminOption.IsNull() && m.WithAdminOption.ValueBool() {
		adminOption = "true"
	}
	return fmt.Sprintf("%s|%s|%s", role, grantee, adminOption)
}

// grantExists reports whether role is granted to grantee, and with which admin
// option.
func (r *RoleGrantResource) grantExists(ctx context.Context, role, grantee string) (bool, bool, error) {
	var adminOption string
	err := r.db.QueryRowContext(ctx,
		`SELECT ADMIN_OPTION FROM EXA_DBA_ROLE_PRIVS WHERE GRANTED_ROLE = ? AND GRANTEE = ?`,
		role, grantee).Scan(&adminOption)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, isTrueAdminOption(adminOption), nil
}
