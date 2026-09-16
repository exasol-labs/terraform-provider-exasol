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

type impersonationTargetKind string

const (
	impersonationUserTarget impersonationTargetKind = "USER"
	impersonationRoleTarget impersonationTargetKind = "ROLE"
)

// ImpersonationGrantResource manages a scoped impersonation grant for either
// one database user or one database role.
type ImpersonationGrantResource struct {
	db         *sql.DB
	targetKind impersonationTargetKind
}

func NewUserImpersonationGrantResource() resource.Resource {
	return &ImpersonationGrantResource{targetKind: impersonationUserTarget}
}

func NewRoleImpersonationGrantResource() resource.Resource {
	return &ImpersonationGrantResource{targetKind: impersonationRoleTarget}
}

func (r *ImpersonationGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.resourceSuffix()
}

func (r *ImpersonationGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	targetLabel := strings.ToLower(string(r.targetKind))
	resp.Schema = schema.Schema{
		Description: fmt.Sprintf("Grants a user or role permission to impersonate one database %s.", targetLabel),
		Attributes: map[string]schema.Attribute{
			"grantee": schema.StringAttribute{
				Required:    true,
				Description: "User or role receiving the impersonation permission.",
			},
			"target": schema.StringAttribute{
				Required:    true,
				Description: r.targetDescription(targetLabel),
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Terraform ID in format: GRANTEE|TARGET",
			},
		},
	}
}

func (r *ImpersonationGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*exasolclient.Client); ok {
		r.db = c.DB
	}
}

type impersonationGrantModel struct {
	ID      types.String `tfsdk:"id"`
	Grantee types.String `tfsdk:"grantee"`
	Target  types.String `tfsdk:"target"`
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

	grantee, target := r.names(plan.Grantee.ValueString(), plan.Target.ValueString())
	if err := r.validateTarget(ctx, grantee, target); err != nil {
		resp.Diagnostics.AddError("Invalid impersonation grant", err.Error())
		return
	}

	exists, err := r.grantExists(ctx, grantee, target)
	if err != nil {
		resp.Diagnostics.AddError("Check impersonation grant failed", err.Error())
		return
	}
	if !exists {
		grant, _ := impersonationGrantSQL(grantee, target)
		tflog.Info(ctx, "Granting impersonation", map[string]any{"sql": grant})
		if _, err := r.db.ExecContext(ctx, grant); err != nil {
			resp.Diagnostics.AddError("GRANT IMPERSONATION failed", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(impersonationGrantID(grantee, target))
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

	grantee, target := r.names(state.Grantee.ValueString(), state.Target.ValueString())
	exists, err := r.grantExists(ctx, grantee, target)
	if err != nil {
		resp.Diagnostics.AddError("Read impersonation grant failed", err.Error())
		return
	}
	if !exists {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(impersonationGrantID(grantee, target))
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

	oldGrantee, oldTarget := r.names(state.Grantee.ValueString(), state.Target.ValueString())
	newGrantee, newTarget := r.names(plan.Grantee.ValueString(), plan.Target.ValueString())
	if oldGrantee == newGrantee && oldTarget == newTarget {
		plan.ID = types.StringValue(impersonationGrantID(newGrantee, newTarget))
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}
	if err := r.validateTarget(ctx, newGrantee, newTarget); err != nil {
		resp.Diagnostics.AddError("Invalid impersonation grant", err.Error())
		return
	}

	newExists, err := r.grantExists(ctx, newGrantee, newTarget)
	if err != nil {
		resp.Diagnostics.AddError("Check new impersonation grant failed", err.Error())
		return
	}
	_, revoke := impersonationGrantSQL(oldGrantee, oldTarget)
	tflog.Info(ctx, "Revoking old impersonation", map[string]any{"sql": revoke})
	if _, err := r.db.ExecContext(ctx, revoke); err != nil {
		resp.Diagnostics.AddError("REVOKE IMPERSONATION failed", err.Error())
		return
	}
	if !newExists {
		grant, _ := impersonationGrantSQL(newGrantee, newTarget)
		tflog.Info(ctx, "Granting new impersonation", map[string]any{"sql": grant})
		if _, err := r.db.ExecContext(ctx, grant); err != nil {
			oldGrant, _ := impersonationGrantSQL(oldGrantee, oldTarget)
			tflog.Warn(ctx, "Restoring old impersonation after failed update", map[string]any{"sql": oldGrant})
			if _, rollbackErr := r.db.ExecContext(ctx, oldGrant); rollbackErr != nil {
				resp.Diagnostics.AddError("GRANT IMPERSONATION failed and rollback failed", fmt.Sprintf("grant error: %v; rollback error: %v", err, rollbackErr))
				return
			}
			resp.Diagnostics.AddError("GRANT IMPERSONATION failed", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(impersonationGrantID(newGrantee, newTarget))
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

	grantee, target := r.names(state.Grantee.ValueString(), state.Target.ValueString())
	exists, err := r.grantExists(ctx, grantee, target)
	if err != nil {
		resp.Diagnostics.AddError("Check impersonation grant failed", err.Error())
		return
	}
	if !exists {
		return
	}
	_, revoke := impersonationGrantSQL(grantee, target)
	tflog.Info(ctx, "Revoking impersonation", map[string]any{"sql": revoke})
	if _, err := r.db.ExecContext(ctx, revoke); err != nil {
		resp.Diagnostics.AddError("REVOKE IMPERSONATION failed", err.Error())
	}
}

func (r *ImpersonationGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "|")
	if len(parts) != 2 || !isValidIdentifier(parts[0]) || !isValidIdentifier(parts[1]) {
		resp.Diagnostics.AddError("Invalid import ID", `Expected format: "GRANTEE|TARGET"`)
		return
	}
	resp.State.SetAttribute(ctx, path.Root("grantee"), normalizeImpersonationName(parts[0]))
	resp.State.SetAttribute(ctx, path.Root("target"), normalizeImpersonationName(parts[1]))
	resp.State.SetAttribute(ctx, path.Root("id"), impersonationGrantID(parts[0], parts[1]))
}

func (r *ImpersonationGrantResource) validateTarget(ctx context.Context, grantee, target string) error {
	if !isValidIdentifier(grantee) || !isValidIdentifier(target) {
		return fmt.Errorf("grantee and target names must not be empty")
	}
	if r.targetKind == impersonationUserTarget && strings.EqualFold(target, "SYS") {
		return fmt.Errorf("impersonating SYS is not allowed")
	}
	if r.targetKind == impersonationRoleTarget && isProtectedImpersonationRole(target) {
		return fmt.Errorf("impersonating the %s role is not allowed", strings.ToUpper(target))
	}

	table := "EXA_DBA_ROLES"
	column := "ROLE_NAME"
	if r.targetKind == impersonationUserTarget {
		table = "EXA_DBA_USERS"
		column = "USER_NAME"
	}
	var found int
	err := r.db.QueryRowContext(ctx, fmt.Sprintf("SELECT 1 FROM %s WHERE %s = ?", table, column), target).Scan(&found)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%s %q does not exist", strings.ToLower(string(r.targetKind)), target)
	}
	return err
}

func (r *ImpersonationGrantResource) names(grantee, target string) (string, string) {
	return normalizeImpersonationName(grantee), normalizeImpersonationName(target)
}

func (r *ImpersonationGrantResource) resourceSuffix() string {
	if r.targetKind == impersonationRoleTarget {
		return "role_impersonation_grant"
	}
	return "user_impersonation_grant"
}

func (r *ImpersonationGrantResource) targetDescription(targetLabel string) string {
	if r.targetKind == impersonationRoleTarget {
		return "Role whose members may be impersonated. Every user holding this role becomes impersonable."
	}
	return fmt.Sprintf("Database %s whose identity may be impersonated.", targetLabel)
}

func (r *ImpersonationGrantResource) grantExists(ctx context.Context, grantee, target string) (bool, error) {
	var found int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM EXA_DBA_IMPERSONATION_PRIVS WHERE GRANTEE = ? AND IMPERSONATION_ON = ?`,
		grantee, target).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func normalizeImpersonationName(name string) string {
	return strings.ToUpper(name)
}

func isProtectedImpersonationRole(role string) bool {
	switch normalizeImpersonationName(role) {
	case "PUBLIC", "DBA":
		return true
	default:
		return false
	}
}

func impersonationGrantSQL(grantee, target string) (string, string) {
	escapedGrantee := escapeIdentifierLiteral(normalizeImpersonationName(grantee))
	escapedTarget := escapeIdentifierLiteral(normalizeImpersonationName(target))
	return fmt.Sprintf(`GRANT IMPERSONATION ON "%s" TO "%s"`, escapedTarget, escapedGrantee),
		fmt.Sprintf(`REVOKE IMPERSONATION ON "%s" FROM "%s"`, escapedTarget, escapedGrantee)
}

func impersonationGrantID(grantee, target string) string {
	return fmt.Sprintf("%s|%s", normalizeImpersonationName(grantee), normalizeImpersonationName(target))
}
