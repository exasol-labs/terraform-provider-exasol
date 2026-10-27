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

var _ resource.Resource = &ConnectionGrantResource{}
var _ resource.ResourceWithImportState = &ConnectionGrantResource{}

// ConnectionGrantResource manages GRANT CONNECTION ... TO ... statements.
type ConnectionGrantResource struct {
	db      *sql.DB
	reads   *exasolclient.ReadCache
	renames *exasolclient.RenameLog
}

func NewConnectionGrantResource() resource.Resource {
	return &ConnectionGrantResource{}
}

func (r *ConnectionGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_connection_grant"
}

func (r *ConnectionGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Grants access to an Exasol connection to a user or role.\n\n" +
			"Connections are used for IMPORT/EXPORT operations. By default, only the connection owner " +
			"can use it. Use this resource to grant access to other users or roles.",
		Attributes: map[string]schema.Attribute{
			"connection_name": schema.StringAttribute{
				Required:    true,
				Description: "Connection name to grant access to.",
			},
			"grantee": schema.StringAttribute{
				Required:    true,
				Description: "User or role name that receives connection access.",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Terraform ID in format: CONNECTION_NAME|GRANTEE",
			},
		},
	}
}

func (r *ConnectionGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
		r.reads = c.Reads
		r.renames = c.Renames
	}
}

type connectionGrantModel struct {
	ID             types.String `tfsdk:"id"`
	ConnectionName types.String `tfsdk:"connection_name"`
	Grantee        types.String `tfsdk:"grantee"`
}

func (r *ConnectionGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan connectionGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer lockGrantees(plan.Grantee.ValueString())()
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	connection := strings.ToUpper(plan.ConnectionName.ValueString())
	grantee := strings.ToUpper(plan.Grantee.ValueString())

	// Validate identifiers
	if !isValidIdentifier(connection) {
		resp.Diagnostics.AddError("Invalid connection name",
			fmt.Sprintf("Connection name %q contains invalid characters.", plan.ConnectionName.ValueString()))
		return
	}
	if !isValidIdentifier(grantee) {
		resp.Diagnostics.AddError("Invalid grantee name",
			fmt.Sprintf("Grantee name %q contains invalid characters.", plan.Grantee.ValueString()))
		return
	}

	// GRANT CONNECTION connection_name TO grantee
	sqlStmt := fmt.Sprintf(`GRANT CONNECTION "%s" TO "%s"`, connection, grantee)
	tflog.Info(ctx, "Granting connection access", map[string]any{"sql": sqlStmt})
	if _, err := execWrite(ctx, r.db, r.reads, sqlStmt); err != nil {
		resp.Diagnostics.AddError("GRANT CONNECTION failed", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s|%s", connection, grantee))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConnectionGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state connectionGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	connection := strings.ToUpper(state.ConnectionName.ValueString())
	grantee := strings.ToUpper(state.Grantee.ValueString())

	// Check if the grant exists in EXA_DBA_CONNECTION_PRIVS
	// Connection grants are tracked separately in the connection privileges view
	row, err := cachedRow(ctx, r.db, r.reads,
		`SELECT GRANTED_CONNECTION, GRANTEE FROM EXA_DBA_CONNECTION_PRIVS`, connection, grantee)
	if err != nil {
		resp.Diagnostics.AddError("Read connection grant failed", err.Error())
		return
	}
	if row == nil {
		// Grant doesn't exist, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with normalized names
	state.ConnectionName = types.StringValue(connection)
	state.Grantee = types.StringValue(grantee)
	state.ID = types.StringValue(fmt.Sprintf("%s|%s", connection, grantee))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ConnectionGrantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Update may REVOKE; serialize it with Delete to avoid transaction
	// collisions (40001), see delete_mutex.go.
	lockDelete()
	defer unlockDelete()

	var plan, state connectionGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer lockGranteesAndRenames(r.renames, plan.Grantee.ValueString(), state.Grantee.ValueString())()
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	oldConnection := strings.ToUpper(state.ConnectionName.ValueString())
	oldGrantee := strings.ToUpper(state.Grantee.ValueString())
	newConnection := strings.ToUpper(plan.ConnectionName.ValueString())
	newGrantee := strings.ToUpper(plan.Grantee.ValueString())

	// Validate identifiers
	if !isValidIdentifier(newConnection) || !isValidIdentifier(newGrantee) {
		resp.Diagnostics.AddError("Invalid identifier", "Connection or grantee name contains invalid characters")
		return
	}

	// If either changed, revoke old grant and create new one
	if oldConnection != newConnection || oldGrantee != newGrantee {
		if !requireExists(ctx, r.db, resp.Diagnostics.AddError, "Connection", newConnection, connectionExists) ||
			!requireExists(ctx, r.db, resp.Diagnostics.AddError, "Grantee", newGrantee, principalExists) {
			return
		}
		// Find where the previous grant lives now: Exasol moves grants to the
		// new name when the connection or grantee is renamed. Revoke it there
		// unless it is the grant this resource keeps. A dropped connection or
		// grantee took the grant with it.
		curConnection, connGone, err := resolveEndpoint(ctx, r.db, r.renames, exasolclient.RenameConnection, oldConnection, newConnection, connectionExists)
		if err != nil {
			resp.Diagnostics.AddError("Check connection failed", err.Error())
			return
		}
		curGrantee, granteeGone, err := resolveEndpoint(ctx, r.db, r.renames, exasolclient.RenamePrincipal, oldGrantee, newGrantee, principalExists)
		if err != nil {
			resp.Diagnostics.AddError("Check grantee failed", err.Error())
			return
		}
		if !connGone && !granteeGone && (curConnection != newConnection || curGrantee != newGrantee) {
			oldExists, err := r.grantExists(ctx, curConnection, curGrantee)
			if err != nil {
				resp.Diagnostics.AddError("Check connection grant failed", err.Error())
				return
			}
			if oldExists {
				revokeStmt := fmt.Sprintf(`REVOKE CONNECTION "%s" FROM "%s"`, curConnection, curGrantee)
				tflog.Info(ctx, "Revoking old connection grant", map[string]any{"sql": revokeStmt})
				if _, err := execWrite(ctx, r.db, r.reads, revokeStmt); err != nil {
					resp.Diagnostics.AddError("REVOKE CONNECTION failed", err.Error())
					return
				}
			}
		}

		newExists, err := r.grantExists(ctx, newConnection, newGrantee)
		if err != nil {
			resp.Diagnostics.AddError("Check connection grant failed", err.Error())
			return
		}
		if !newExists {
			grantStmt := fmt.Sprintf(`GRANT CONNECTION "%s" TO "%s"`, newConnection, newGrantee)
			tflog.Info(ctx, "Granting new connection access", map[string]any{"sql": grantStmt})
			if _, err := execWrite(ctx, r.db, r.reads, grantStmt); err != nil {
				resp.Diagnostics.AddError("GRANT CONNECTION failed", err.Error())
				return
			}
		}
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s|%s", newConnection, newGrantee))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConnectionGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Serialize delete operations to prevent transaction collision errors
	lockDelete()
	defer unlockDelete()

	var state connectionGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	connection := strings.ToUpper(state.ConnectionName.ValueString())
	grantee := strings.ToUpper(state.Grantee.ValueString())

	// Validate identifiers
	if !isValidIdentifier(connection) || !isValidIdentifier(grantee) {
		resp.Diagnostics.AddError("Invalid identifier", "Connection or grantee name contains invalid characters")
		return
	}

	// A connection or grantee renamed in this run carries the grant to its
	// new name.
	if current, ok := r.renames.Lookup(exasolclient.RenameConnection, connection); ok {
		connection = current
	}
	if current, ok := r.renames.Lookup(exasolclient.RenamePrincipal, grantee); ok {
		grantee = current
	}
	// Lock the name the grant lives under now, too: a Create on it must wait.
	defer lockGrantees(state.Grantee.ValueString(), grantee)()
	// Already gone, e.g. the grantee was dropped: nothing to revoke.
	exists, err := r.grantExists(ctx, connection, grantee)
	if err != nil {
		resp.Diagnostics.AddError("Check connection grant failed", err.Error())
		return
	}
	if !exists {
		return
	}

	// REVOKE CONNECTION connection_name FROM grantee
	sqlStmt := fmt.Sprintf(`REVOKE CONNECTION "%s" FROM "%s"`, connection, grantee)
	tflog.Info(ctx, "Revoking connection access", map[string]any{"sql": sqlStmt})
	if _, err := execWrite(ctx, r.db, r.reads, sqlStmt); err != nil {
		resp.Diagnostics.AddError("REVOKE CONNECTION failed", err.Error())
	}
}

func (r *ConnectionGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: CONNECTION_NAME|GRANTEE
	parts := strings.Split(req.ID, "|")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID",
			`Expected format: "CONNECTION_NAME|GRANTEE"`)
		return
	}

	connection := strings.ToUpper(parts[0])
	grantee := strings.ToUpper(parts[1])

	resp.State.SetAttribute(ctx, path.Root("connection_name"), connection)
	resp.State.SetAttribute(ctx, path.Root("grantee"), grantee)
	resp.State.SetAttribute(ctx, path.Root("id"), fmt.Sprintf("%s|%s", connection, grantee))
}

// grantExists reports whether connection is granted to grantee.
func (r *ConnectionGrantResource) grantExists(ctx context.Context, connection, grantee string) (bool, error) {
	var dummy int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM EXA_DBA_CONNECTION_PRIVS WHERE GRANTED_CONNECTION = ? AND GRANTEE = ?`,
		connection, grantee).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
