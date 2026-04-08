package resources

import (
	"context"
	"database/sql"
	"fmt"

	"terraform-provider-exasol/internal/exasolclient"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &SchemaResource{}
var _ resource.ResourceWithImportState = &SchemaResource{}

// SchemaResource manages Exasol schemas.
type SchemaResource struct {
	db *sql.DB
}

func NewSchemaResource() resource.Resource {
	return &SchemaResource{}
}

func (r *SchemaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schema"
}

func (r *SchemaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates, renames and drops an Exasol schema.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Schema name to create or rename to.",
			},
			"owner": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Schema owner (user or role). If specified, ownership will be transferred after creation.",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Current schema name (used as Terraform ID).",
			},
		},
	}
}

func (r *SchemaResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
	}
}

type schemaModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Owner types.String `tfsdk:"owner"`
}

func (r *SchemaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan schemaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	schemaName := plan.Name.ValueString()

	// Validate identifier to prevent SQL injection
	if !isValidIdentifier(schemaName) {
		resp.Diagnostics.AddError("Invalid schema name",
			fmt.Sprintf("Schema name %q contains invalid characters. Exasol identifiers must start with a letter and contain only letters, digits, and underscores.", schemaName))
		return
	}

	sqlStmt := fmt.Sprintf(`CREATE SCHEMA "%s"`, schemaName)
	tflog.Info(ctx, "Creating schema", map[string]any{"sql": sqlStmt})
	if _, err := r.db.ExecContext(ctx, sqlStmt); err != nil {
		resp.Diagnostics.AddError("CREATE SCHEMA failed", err.Error())
		return
	}

	// Transfer ownership if specified
	if !plan.Owner.IsNull() && !plan.Owner.IsUnknown() {
		owner := plan.Owner.ValueString()
		if !isValidIdentifier(owner) {
			resp.Diagnostics.AddError("Invalid owner name",
				fmt.Sprintf("Owner name %q contains invalid characters.", owner))
			return
		}
		alterStmt := fmt.Sprintf(`ALTER SCHEMA "%s" CHANGE OWNER "%s"`, schemaName, owner)
		tflog.Info(ctx, "Transferring schema ownership", map[string]any{"sql": alterStmt})
		if _, err := r.db.ExecContext(ctx, alterStmt); err != nil {
			resp.Diagnostics.AddError("ALTER SCHEMA CHANGE OWNER failed", err.Error())
			return
		}
	}

	plan.ID = plan.Name

	// Read back the actual owner so the Computed attribute is always known after apply
	var actualOwner sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT SCHEMA_OWNER FROM EXA_ALL_SCHEMAS WHERE SCHEMA_NAME = ?`, schemaName).Scan(&actualOwner)
	if err != nil {
		resp.Diagnostics.AddError("Read schema owner after create failed", err.Error())
		return
	}
	if actualOwner.Valid {
		plan.Owner = types.StringValue(actualOwner.String)
	} else {
		plan.Owner = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SchemaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state schemaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	var owner sql.NullString
	query := `SELECT SCHEMA_OWNER FROM EXA_ALL_SCHEMAS WHERE SCHEMA_NAME = ?`
	err := r.db.QueryRowContext(ctx, query, state.ID.ValueString()).Scan(&owner)
	if err == sql.ErrNoRows {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read schema failed", err.Error())
		return
	}

	// Update owner in state
	if owner.Valid {
		state.Owner = types.StringValue(owner.String)
	} else {
		state.Owner = types.StringNull()
	}

	// After import, name is empty - populate it from the ID
	if state.Name.IsNull() || state.Name.ValueString() == "" {
		state.Name = state.ID
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SchemaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state schemaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	oldName := state.ID.ValueString()
	newName := plan.Name.ValueString()

	// Validate identifiers to prevent SQL injection
	if !isValidIdentifier(oldName) {
		resp.Diagnostics.AddError("Invalid old schema name",
			fmt.Sprintf("Schema name %q contains invalid characters.", oldName))
		return
	}
	if !isValidIdentifier(newName) {
		resp.Diagnostics.AddError("Invalid new schema name",
			fmt.Sprintf("Schema name %q contains invalid characters.", newName))
		return
	}

	if oldName != newName {
		sqlStmt := fmt.Sprintf(`RENAME SCHEMA "%s" TO "%s"`, oldName, newName)
		tflog.Info(ctx, "Renaming schema", map[string]any{"sql": sqlStmt})
		if _, err := r.db.ExecContext(ctx, sqlStmt); err != nil {
			resp.Diagnostics.AddError("RENAME SCHEMA failed", err.Error())
			return
		}
	}

	// Handle ownership change
	currentName := newName // Use new name if renamed, otherwise same as old
	if !plan.Owner.IsNull() && !plan.Owner.IsUnknown() {
		newOwner := plan.Owner.ValueString()
		oldOwner := state.Owner.ValueString()

		if newOwner != oldOwner {
			if !isValidIdentifier(newOwner) {
				resp.Diagnostics.AddError("Invalid owner name",
					fmt.Sprintf("Owner name %q contains invalid characters.", newOwner))
				return
			}
			alterStmt := fmt.Sprintf(`ALTER SCHEMA "%s" CHANGE OWNER "%s"`, currentName, newOwner)
			tflog.Info(ctx, "Changing schema ownership", map[string]any{"sql": alterStmt})
			if _, err := r.db.ExecContext(ctx, alterStmt); err != nil {
				resp.Diagnostics.AddError("ALTER SCHEMA CHANGE OWNER failed", err.Error())
				return
			}
		}
	}

	// Update ID and Name to the new name
	plan.ID = plan.Name

	// Read back the actual owner so the Computed attribute is always known after apply
	var actualOwner sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT SCHEMA_OWNER FROM EXA_ALL_SCHEMAS WHERE SCHEMA_NAME = ?`, newName).Scan(&actualOwner)
	if err != nil {
		resp.Diagnostics.AddError("Read schema owner after update failed", err.Error())
		return
	}
	if actualOwner.Valid {
		plan.Owner = types.StringValue(actualOwner.String)
	} else {
		plan.Owner = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SchemaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Serialize delete operations to prevent transaction collision errors
	lockDelete()
	defer unlockDelete()

	var state schemaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	schemaName := state.ID.ValueString()

	// Validate identifier to prevent SQL injection
	if !isValidIdentifier(schemaName) {
		resp.Diagnostics.AddError("Invalid schema name",
			fmt.Sprintf("Schema name %q contains invalid characters.", schemaName))
		return
	}

	sqlStmt := fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schemaName)
	tflog.Info(ctx, "Dropping schema", map[string]any{"sql": sqlStmt})
	if _, err := r.db.ExecContext(ctx, sqlStmt); err != nil {
		resp.Diagnostics.AddError("DROP SCHEMA failed", err.Error())
	}
}

func (r *SchemaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// allow import by name
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
