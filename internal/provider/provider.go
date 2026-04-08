package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"terraform-provider-exasol/internal/resources"
)

var _ provider.Provider = &ExasolProvider{}

type ExasolProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ExasolProvider{version: version}
	}
}

func (p *ExasolProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "exasol"
	resp.Version = p.version
}

func (p *ExasolProvider) Schema(
	_ context.Context,
	_ provider.SchemaRequest,
	resp *provider.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Manage Exasol database resources: users, roles, schemas, connections, and privileges.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Required:    true,
				Description: "Exasol host (DNS or IP).",
			},
			"port": schema.Int64Attribute{
				Optional:    true,
				Description: "Exasol port. Default 8563.",
			},
			"user": schema.StringAttribute{
				Required:    true,
				Description: "Exasol username.",
			},
			"password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Exasol password or Personal Access Token (PAT). Tokens starting with exa_pat_ are detected automatically.",
			},
			"validate_server_certificate": schema.BoolAttribute{
				Optional:    true,
				Description: "Validate server TLS certificate. Default true.",
			},
		},
	}
}

func (p *ExasolProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	cfg, diags := LoadConfig(ctx, req)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := NewClient(ctx, cfg)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create client", err.Error())
		return
	}
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *ExasolProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewConnectionResource,
		resources.NewConnectionGrantResource,
		resources.NewObjectPrivilegeResource,
		resources.NewRoleGrantResource,
		resources.NewRoleResource,
		resources.NewSchemaResource,
		resources.NewSystemPrivilegeResource,
		resources.NewUserResource,
	}
}

func (p *ExasolProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
