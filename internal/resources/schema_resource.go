package resources

import (
	"context"
	"database/sql"
	"fmt"
	"unicode/utf8"

	"terraform-provider-exasol/internal/exasolclient"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &SchemaResource{}
var _ resource.ResourceWithImportState = &SchemaResource{}
var _ resource.ResourceWithValidateConfig = &SchemaResource{}

// SchemaResource manages Exasol schemas.
type SchemaResource struct {
	db      *sql.DB
	renames *exasolclient.RenameLog
}

func NewSchemaResource() resource.Resource {
	return &SchemaResource{}
}

func (r *SchemaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schema"
}

func (r *SchemaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates, renames and drops an Exasol schema. " +
			"Renaming keeps its grants: Exasol moves them to the new name. When a grant that references it changes in the same apply, " +
			"the provider revokes the previous grant under the new name. If the rename and the grant change land in different runs " +
			"(-target, a failed apply, another provider alias, or a rename outside Terraform), check the renamed object for leftover grants.",
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
			"comment": schema.StringAttribute{
				Optional: true,
				Description: "Schema comment. Removing the attribute clears the comment. " +
					"Comments set outside Terraform, or present on an imported schema, are not tracked until this attribute is set; " +
					"the next apply then sets the configured comment.",
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
		r.renames = c.Renames
	}
}

type schemaModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Owner   types.String `tfsdk:"owner"`
	Comment types.String `tfsdk:"comment"`
}

// maxCommentLength is the Exasol limit for COMMENT ON, in characters.
const maxCommentLength = 2000

func (r *SchemaResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var comment types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("comment"), &comment)...)
	if comment.IsNull() || comment.IsUnknown() {
		return
	}
	// Exasol stores an empty string as NULL, so "" would never match state
	if comment.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("comment"), "Empty schema comment",
			"Exasol stores an empty comment as NULL. Remove the attribute to clear the comment.")
	}
	if n := utf8.RuneCountInString(comment.ValueString()); n > maxCommentLength {
		resp.Diagnostics.AddAttributeError(path.Root("comment"), "Schema comment too long",
			fmt.Sprintf("Exasol allows at most %d characters in a comment, got %d.", maxCommentLength, n))
	}
}

// setSchemaComment runs COMMENT ON SCHEMA; a null comment clears it.
func (r *SchemaResource) setSchemaComment(ctx context.Context, schemaName string, comment types.String) error {
	value := "NULL"
	if !comment.IsNull() {
		value = "'" + escapeStringLiteral(comment.ValueString()) + "'"
	}
	sqlStmt := fmt.Sprintf(`COMMENT ON SCHEMA "%s" IS %s`, escapeIdentifierLiteral(schemaName), value)
	tflog.Info(ctx, "Setting schema comment", map[string]any{"sql": sqlStmt})
	_, err := r.db.ExecContext(ctx, sqlStmt)
	return err
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

	// Comment before the owner transfer: afterwards a non-DBA provider user
	// no longer owns the schema and may not comment on it
	if !plan.Comment.IsNull() {
		if err := r.setSchemaComment(ctx, schemaName, plan.Comment); err != nil {
			resp.Diagnostics.AddError("COMMENT ON SCHEMA failed", err.Error())
			return
		}
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

	var owner, comment sql.NullString
	query := `SELECT SCHEMA_OWNER, SCHEMA_COMMENT FROM EXA_ALL_SCHEMAS WHERE SCHEMA_NAME = ?`
	err := r.db.QueryRowContext(ctx, query, state.ID.ValueString()).Scan(&owner, &comment)
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

	// Only track the comment once Terraform manages it, so upgrading does not
	// plan to clear comments set outside Terraform
	if !state.Comment.IsNull() {
		if comment.Valid {
			state.Comment = types.StringValue(comment.String)
		} else {
			state.Comment = types.StringNull()
		}
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
		// Hold the grant lock across RENAME and Record, so a grant Update either
		// sees the rename in the log or revokes under the old name before it.
		lockDelete()
		sqlStmt := fmt.Sprintf(`RENAME SCHEMA "%s" TO "%s"`, oldName, newName)
		tflog.Info(ctx, "Renaming schema", map[string]any{"sql": sqlStmt})
		_, err := r.db.ExecContext(ctx, sqlStmt)
		if err == nil {
			r.renames.Record(exasolclient.RenameSchema, oldName, newName)
		}
		unlockDelete()
		if err != nil {
			resp.Diagnostics.AddError("RENAME SCHEMA failed", err.Error())
			return
		}
	}

	currentName := newName // Use new name if renamed, otherwise same as old

	// Comment before the owner change, see Create
	if !plan.Comment.Equal(state.Comment) {
		if err := r.setSchemaComment(ctx, currentName, plan.Comment); err != nil {
			resp.Diagnostics.AddError("COMMENT ON SCHEMA failed", err.Error())
			return
		}
	}

	// Handle ownership change
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
