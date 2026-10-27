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

// RoleResource manages Exasol roles.
type RoleResource struct {
	db      *sql.DB
	reads   *exasolclient.ReadCache
	renames *exasolclient.RenameLog
}

var _ resource.Resource = &RoleResource{}
var _ resource.ResourceWithImportState = &RoleResource{}

func NewRoleResource() resource.Resource { return &RoleResource{} }

func (r *RoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *RoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates, renames and drops an Exasol role. " +
			"Roles are stored in UPPERCASE inside Exasol, but the 'name' attribute " +
			"preserves the exact spelling from the Terraform configuration. " +
			"Renaming keeps its grants: Exasol moves them to the new name. When a grant that references it changes in the same apply, " +
			"the provider revokes the previous grant under the new name. If the rename and the grant change land in different runs " +
			"(-target, a failed apply, another provider alias, or a rename outside Terraform), check the renamed object for leftover grants.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Desired role name (case preserved in Terraform).",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Role name as stored in Exasol (always UPPERCASE).",
			},
		},
	}
}

func (r *RoleResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
		r.reads = c.Reads
		r.renames = c.Renames
	}
}

type roleModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func upper(s string) string { return strings.ToUpper(s) }

func (r *RoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	upName := upper(plan.Name.ValueString())

	// Validate identifier to prevent SQL injection
	if !isValidIdentifier(upName) {
		resp.Diagnostics.AddError("Invalid role name",
			fmt.Sprintf("Role name %q contains invalid characters. Exasol identifiers must start with a letter and contain only letters, digits, and underscores.", plan.Name.ValueString()))
		return
	}

	stmt := fmt.Sprintf(`CREATE ROLE "%s"`, upName)
	tflog.Debug(ctx, "Creating role", map[string]any{"sql": stmt})
	if _, err := execWrite(ctx, r.db, r.reads, stmt); err != nil {
		resp.Diagnostics.AddError("Error creating role", err.Error())
		return
	}

	// id must always match Exasol's actual name (upper case)
	plan.ID = types.StringValue(upName)

	// name remains exactly as user wrote it
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	row, err := cachedRow(ctx, r.db, r.reads, `SELECT ROLE_NAME FROM EXA_DBA_ROLES`, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading role", err.Error())
		return
	}
	if row == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	current := row[0].String

	// keep the user's spelling of name; only update id (upper-case in DB)
	state.ID = types.StringValue(upper(current))
	// After import, name is empty - populate it from the database
	if state.Name.IsNull() || state.Name.ValueString() == "" {
		state.Name = types.StringValue(current)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior roleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	upNew := upper(plan.Name.ValueString())
	upOld := upper(prior.ID.ValueString()) // ID always upper-case

	// Validate identifiers to prevent SQL injection
	if !isValidIdentifier(upOld) {
		resp.Diagnostics.AddError("Invalid old role name",
			fmt.Sprintf("Role name %q contains invalid characters.", prior.ID.ValueString()))
		return
	}
	if !isValidIdentifier(upNew) {
		resp.Diagnostics.AddError("Invalid new role name",
			fmt.Sprintf("Role name %q contains invalid characters.", plan.Name.ValueString()))
		return
	}

	if upNew != upOld {
		// Hold the grant lock across RENAME and Record, so a grant Update either
		// sees the rename in the log or revokes under the old name before it.
		lockDelete()
		stmt := fmt.Sprintf(`RENAME ROLE "%s" TO "%s"`, upOld, upNew)
		tflog.Debug(ctx, "Renaming role", map[string]any{"sql": stmt})
		_, err := execWrite(ctx, r.db, r.reads, stmt)
		if err == nil {
			r.renames.Record(exasolclient.RenamePrincipal, upOld, upNew)
		}
		unlockDelete()
		if err != nil {
			resp.Diagnostics.AddError("Error renaming role", err.Error())
			return
		}
	}

	// Update id to match DB, keep name as in user config
	plan.ID = types.StringValue(upNew)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Serialize delete operations to prevent transaction collision errors
	lockDelete()
	defer unlockDelete()

	var state roleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	upName := upper(state.ID.ValueString())

	// Validate identifier to prevent SQL injection
	if !isValidIdentifier(upName) {
		resp.Diagnostics.AddError("Invalid role name",
			fmt.Sprintf("Role name %q contains invalid characters.", state.ID.ValueString()))
		return
	}

	stmt := fmt.Sprintf(`DROP ROLE "%s"`, upName)
	tflog.Debug(ctx, "Dropping role", map[string]any{"sql": stmt})
	if _, err := execWrite(ctx, r.db, r.reads, stmt); err != nil {
		resp.Diagnostics.AddError("Error dropping role", err.Error())
	}
}

func (r *RoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by upper-case role name
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
