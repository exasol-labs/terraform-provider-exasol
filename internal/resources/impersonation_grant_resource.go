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

var _ resource.Resource = &ImpersonationGrantResource{}
var _ resource.ResourceWithImportState = &ImpersonationGrantResource{}

// ImpersonationGrantResource manages a scoped impersonation grant from a user
// or role to one database user.
type ImpersonationGrantResource struct {
	db *sql.DB
}

func NewImpersonationGrantResource() resource.Resource {
	return &ImpersonationGrantResource{}
}

func (r *ImpersonationGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_impersonation_grant"
}

func (r *ImpersonationGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Grants a user or role permission to impersonate one regular database user.",
		Attributes: map[string]schema.Attribute{
			"grantee": schema.StringAttribute{
				Required:    true,
				Description: "User or role receiving the impersonation permission.",
			},
			"impersonated_user": schema.StringAttribute{
				Required:    true,
				Description: "Regular database user whose identity may be impersonated. Roles are not accepted.",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Terraform ID in format: GRANTEE|IMPERSONATED_USER",
			},
		},
	}
}

func (r *ImpersonationGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
	}
}

type impersonationGrantModel struct {
	ID               types.String `tfsdk:"id"`
	Grantee          types.String `tfsdk:"grantee"`
	ImpersonatedUser types.String `tfsdk:"impersonated_user"`
}

func (r *ImpersonationGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan impersonationGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	grantee := normalizeImpersonationName(plan.Grantee.ValueString())
	impersonatedUser := normalizeImpersonationName(plan.ImpersonatedUser.ValueString())
	if !isValidIdentifier(grantee) {
		resp.Diagnostics.AddError("Invalid grantee", "Grantee name must not be empty.")
		return
	}
	if !isValidIdentifier(impersonatedUser) {
		resp.Diagnostics.AddError("Invalid impersonated user", "Impersonated user name must not be empty.")
		return
	}
	if err := r.validateUser(ctx, impersonatedUser); err != nil {
		resp.Diagnostics.AddError("Invalid impersonated user", err.Error())
		return
	}

	exists, err := r.impersonationGrantExists(ctx, grantee, impersonatedUser)
	if err != nil {
		resp.Diagnostics.AddError("Check impersonation grant failed", err.Error())
		return
	}
	if exists {
		plan.ID = types.StringValue(impersonationGrantID(grantee, impersonatedUser))
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	grant, _ := impersonationGrantSQL(grantee, impersonatedUser)
	tflog.Info(ctx, "Granting impersonation", map[string]any{"sql": grant})
	if _, err := r.db.ExecContext(ctx, grant); err != nil {
		resp.Diagnostics.AddError("GRANT IMPERSONATION failed", err.Error())
		return
	}

	plan.ID = types.StringValue(impersonationGrantID(grantee, impersonatedUser))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ImpersonationGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	var state impersonationGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	grantee := normalizeImpersonationName(state.Grantee.ValueString())
	impersonatedUser := normalizeImpersonationName(state.ImpersonatedUser.ValueString())
	exists, err := r.impersonationGrantExists(ctx, grantee, impersonatedUser)
	if err != nil {
		resp.Diagnostics.AddError("Read impersonation grant failed", err.Error())
		return
	}
	if !exists {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(impersonationGrantID(grantee, impersonatedUser))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ImpersonationGrantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state impersonationGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	oldGrantee := normalizeImpersonationName(state.Grantee.ValueString())
	oldUser := normalizeImpersonationName(state.ImpersonatedUser.ValueString())
	newGrantee := normalizeImpersonationName(plan.Grantee.ValueString())
	newUser := normalizeImpersonationName(plan.ImpersonatedUser.ValueString())
	if oldGrantee == newGrantee && oldUser == newUser {
		plan.ID = types.StringValue(impersonationGrantID(newGrantee, newUser))
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}
	if !isValidIdentifier(newGrantee) || !isValidIdentifier(newUser) {
		resp.Diagnostics.AddError("Invalid impersonation grant", "Grantee and impersonated user names must not be empty.")
		return
	}
	if err := r.validateUser(ctx, newUser); err != nil {
		resp.Diagnostics.AddError("Invalid impersonated user", err.Error())
		return
	}

	newExists, err := r.impersonationGrantExists(ctx, newGrantee, newUser)
	if err != nil {
		resp.Diagnostics.AddError("Check new impersonation grant failed", err.Error())
		return
	}

	_, revoke := impersonationGrantSQL(oldGrantee, oldUser)
	tflog.Info(ctx, "Revoking old impersonation", map[string]any{"sql": revoke})
	if _, err := r.db.ExecContext(ctx, revoke); err != nil {
		resp.Diagnostics.AddError("REVOKE IMPERSONATION failed", err.Error())
		return
	}

	if !newExists {
		grant, _ := impersonationGrantSQL(newGrantee, newUser)
		tflog.Info(ctx, "Granting new impersonation", map[string]any{"sql": grant})
		if _, err := r.db.ExecContext(ctx, grant); err != nil {
			oldGrant, _ := impersonationGrantSQL(oldGrantee, oldUser)
			if _, rollbackErr := r.db.ExecContext(ctx, oldGrant); rollbackErr != nil {
				resp.Diagnostics.AddError("GRANT IMPERSONATION failed and rollback failed", fmt.Sprintf("grant error: %v; rollback error: %v", err, rollbackErr))
				return
			}
			resp.Diagnostics.AddError("GRANT IMPERSONATION failed", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(impersonationGrantID(newGrantee, newUser))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ImpersonationGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	lockDelete()
	defer unlockDelete()

	var state impersonationGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.db == nil {
		resp.Diagnostics.AddError("Database not configured", "Provider did not supply a database connection.")
		return
	}

	grantee := normalizeImpersonationName(state.Grantee.ValueString())
	impersonatedUser := normalizeImpersonationName(state.ImpersonatedUser.ValueString())
	exists, err := r.impersonationGrantExists(ctx, grantee, impersonatedUser)
	if err != nil {
		resp.Diagnostics.AddError("Check impersonation grant failed", err.Error())
		return
	}
	if !exists {
		return
	}

	_, revoke := impersonationGrantSQL(grantee, impersonatedUser)
	tflog.Info(ctx, "Revoking impersonation", map[string]any{"sql": revoke})
	if _, err := r.db.ExecContext(ctx, revoke); err != nil {
		resp.Diagnostics.AddError("REVOKE IMPERSONATION failed", err.Error())
	}
}

func (r *ImpersonationGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "|")
	if len(parts) != 2 || !isValidIdentifier(parts[0]) || !isValidIdentifier(parts[1]) {
		resp.Diagnostics.AddError("Invalid import ID", `Expected format: "GRANTEE|IMPERSONATED_USER"`)
		return
	}
	resp.State.SetAttribute(ctx, path.Root("grantee"), normalizeImpersonationName(parts[0]))
	resp.State.SetAttribute(ctx, path.Root("impersonated_user"), normalizeImpersonationName(parts[1]))
	resp.State.SetAttribute(ctx, path.Root("id"), impersonationGrantID(parts[0], parts[1]))
}

func (r *ImpersonationGrantResource) validateUser(ctx context.Context, user string) error {
	if strings.EqualFold(user, "SYS") {
		return fmt.Errorf("impersonating SYS is not allowed")
	}

	var found int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM EXA_DBA_USERS WHERE USER_NAME = ?`, user).Scan(&found)
	if err == sql.ErrNoRows {
		return fmt.Errorf("user %q does not exist", user)
	}
	return err
}

func (r *ImpersonationGrantResource) impersonationGrantExists(ctx context.Context, grantee, impersonatedUser string) (bool, error) {
	var found int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM EXA_DBA_IMPERSONATION_PRIVS WHERE GRANTEE = ? AND IMPERSONATION_ON = ?`,
		grantee, impersonatedUser).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func normalizeImpersonationName(name string) string {
	return strings.ToUpper(name)
}

func impersonationGrantSQL(grantee, impersonatedUser string) (string, string) {
	escapedGrantee := escapeIdentifierLiteral(normalizeImpersonationName(grantee))
	escapedUser := escapeIdentifierLiteral(normalizeImpersonationName(impersonatedUser))
	grant := fmt.Sprintf(`GRANT IMPERSONATION ON "%s" TO "%s"`, escapedUser, escapedGrantee)
	revoke := fmt.Sprintf(`REVOKE IMPERSONATION ON "%s" FROM "%s"`, escapedUser, escapedGrantee)
	return grant, revoke
}

func impersonationGrantID(grantee, impersonatedUser string) string {
	return fmt.Sprintf("%s|%s", normalizeImpersonationName(grantee), normalizeImpersonationName(impersonatedUser))
}
