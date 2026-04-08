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

var _ resource.Resource = &ConnectionResource{}
var _ resource.ResourceWithImportState = &ConnectionResource{}

// ConnectionResource manages Exasol database connections.
// Connections are used for IMPORT/EXPORT and can connect to various external systems.
type ConnectionResource struct {
	db *sql.DB
}

func NewConnectionResource() resource.Resource {
	return &ConnectionResource{}
}

func (r *ConnectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_connection"
}

func (r *ConnectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates, updates and drops an Exasol connection. " +
			"Connections are used for IMPORT/EXPORT operations and can connect to " +
			"external databases (Exasol, Oracle, JDBC), file servers (FTP, S3), and other systems.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Connection name. Case-insensitive in Exasol.",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Terraform ID — set to the connection name in uppercase.",
			},
			"to": schema.StringAttribute{
				Required: true,
				Description: "Connection string (e.g., host:port for Exasol, URL for S3/FTP, " +
					"JDBC string, etc.). Multiple hosts can be separated by commas.",
			},
			"user": schema.StringAttribute{
				Optional:    true,
				Description: "Username for authentication.",
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Password for authentication.",
			},
		},
	}
}

func (r *ConnectionResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
	}
}

type connectionModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	To       types.String `tfsdk:"to"`
	User     types.String `tfsdk:"user"`
	Password types.String `tfsdk:"password"`
}

func (r *ConnectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan connectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	upName := strings.ToUpper(plan.Name.ValueString())

	// Validate connection name to prevent SQL injection
	if !isValidIdentifier(upName) {
		resp.Diagnostics.AddError("Invalid connection name",
			fmt.Sprintf("Connection name %q contains invalid characters. Exasol identifiers must start with a letter and contain only letters, digits, and underscores.", plan.Name.ValueString()))
		return
	}

	sqlStmt, err := buildCreateConnectionSQL(plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid connection configuration", err.Error())
		return
	}

	tflog.Info(ctx, "Creating connection", map[string]any{"sql": sanitizeLogSQL(sqlStmt)})
	if _, err := r.db.ExecContext(ctx, sqlStmt); err != nil {
		resp.Diagnostics.AddError("CREATE CONNECTION failed", err.Error())
		return
	}

	plan.ID = types.StringValue(upName)
	plan.Name = types.StringValue(upName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConnectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	var state connectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read back connection details from EXA_DBA_CONNECTIONS.
	// CONNECTION_STRING and USER_NAME are exposed; password is not.
	var connString sql.NullString
	var userName sql.NullString
	query := `SELECT CONNECTION_STRING, USER_NAME FROM EXA_DBA_CONNECTIONS WHERE CONNECTION_NAME = ?`
	err := r.db.QueryRowContext(ctx, query, state.ID.ValueString()).Scan(&connString, &userName)
	if err == sql.ErrNoRows {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read connection failed", err.Error())
		return
	}

	// After import, name is empty - populate it from the ID
	if state.Name.IsNull() || state.Name.ValueString() == "" {
		state.Name = state.ID
	}
	state.ID = types.StringValue(strings.ToUpper(state.Name.ValueString()))

	// Populate readable attributes from DB.
	// Always set from DB to detect external changes (drift).
	if connString.Valid {
		state.To = types.StringValue(connString.String)
	}
	if userName.Valid && userName.String != "" {
		state.User = types.StringValue(userName.String)
	} else {
		// DB has no user - clear it so drift is detected if user was removed externally
		state.User = types.StringNull()
	}
	// Password is not exposed by Exasol - keep whatever is in state

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ConnectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state connectionModel
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
	if !isValidIdentifier(upOld) || !isValidIdentifier(upNew) {
		resp.Diagnostics.AddError("Invalid connection name", "Connection name contains invalid characters")
		return
	}

	// If name changed, we need to rename first
	if upOld != upNew {
		stmt := fmt.Sprintf(`RENAME CONNECTION "%s" TO "%s"`, upOld, upNew)
		tflog.Info(ctx, "Renaming connection", map[string]any{"sql": stmt})
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			resp.Diagnostics.AddError("RENAME CONNECTION failed", err.Error())
			return
		}
	}

	// Check if connection properties changed
	if plan.To.ValueString() != state.To.ValueString() ||
		plan.User.ValueString() != state.User.ValueString() ||
		plan.Password.ValueString() != state.Password.ValueString() {

		alter, err := buildAlterConnectionSQL(plan)
		if err != nil {
			resp.Diagnostics.AddError("Invalid alter connection config", err.Error())
			return
		}
		tflog.Info(ctx, "Altering connection", map[string]any{"sql": sanitizeLogSQL(alter)})
		if _, err := r.db.ExecContext(ctx, alter); err != nil {
			resp.Diagnostics.AddError("ALTER CONNECTION failed", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(upNew)
	plan.Name = types.StringValue(upNew)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConnectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Serialize delete operations to prevent transaction collision errors
	lockDelete()
	defer unlockDelete()

	var state connectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	upName := strings.ToUpper(state.ID.ValueString())
	if !isValidIdentifier(upName) {
		resp.Diagnostics.AddError("Invalid connection name", "Connection name contains invalid characters")
		return
	}

	stmt := fmt.Sprintf(`DROP CONNECTION "%s"`, upName)
	tflog.Info(ctx, "Dropping connection", map[string]any{"sql": stmt})
	if _, err := r.db.ExecContext(ctx, stmt); err != nil {
		resp.Diagnostics.AddError("DROP CONNECTION failed", err.Error())
	}
}

func (r *ConnectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Allow import by connection name
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// --- helpers -------------------------------------------------------

func buildCreateConnectionSQL(m connectionModel) (string, error) {
	upName := strings.ToUpper(m.Name.ValueString())

	// Validate identifier
	if !isValidIdentifier(upName) {
		return "", fmt.Errorf("invalid connection name: contains illegal characters")
	}

	// Escape the connection string
	escapedTo := escapeStringLiteral(m.To.ValueString())

	var stmt strings.Builder
	fmt.Fprintf(&stmt, `CREATE CONNECTION "%s" TO '%s'`, upName, escapedTo)

	// Add credentials if provided
	if !m.User.IsNull() && !m.User.IsUnknown() && m.User.ValueString() != "" {
		escapedUser := escapeStringLiteral(m.User.ValueString())
		fmt.Fprintf(&stmt, ` USER '%s'`, escapedUser)
	}

	if !m.Password.IsNull() && !m.Password.IsUnknown() && m.Password.ValueString() != "" {
		escapedPwd := escapeStringLiteral(m.Password.ValueString())
		fmt.Fprintf(&stmt, ` IDENTIFIED BY '%s'`, escapedPwd)
	}

	return stmt.String(), nil
}

func buildAlterConnectionSQL(m connectionModel) (string, error) {
	upName := strings.ToUpper(m.Name.ValueString())

	// Validate identifier
	if !isValidIdentifier(upName) {
		return "", fmt.Errorf("invalid connection name: contains illegal characters")
	}

	// Escape the connection string
	escapedTo := escapeStringLiteral(m.To.ValueString())

	var stmt strings.Builder
	fmt.Fprintf(&stmt, `ALTER CONNECTION "%s" TO '%s'`, upName, escapedTo)

	// Add credentials if provided
	if !m.User.IsNull() && !m.User.IsUnknown() && m.User.ValueString() != "" {
		escapedUser := escapeStringLiteral(m.User.ValueString())
		fmt.Fprintf(&stmt, ` USER '%s'`, escapedUser)
	}

	if !m.Password.IsNull() && !m.Password.IsUnknown() && m.Password.ValueString() != "" {
		escapedPwd := escapeStringLiteral(m.Password.ValueString())
		fmt.Fprintf(&stmt, ` IDENTIFIED BY '%s'`, escapedPwd)
	}

	return stmt.String(), nil
}
