package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ProviderConfig struct {
	Host                      string
	Port                      int64
	User                      string
	Password                  string
	ValidateServerCertificate bool
	ConnectTimeout            int64
}

func LoadConfig(ctx context.Context, req provider.ConfigureRequest) (*ProviderConfig, diag.Diagnostics) {
	var diags diag.Diagnostics

	var cfg struct {
		Host                      types.String `tfsdk:"host"`
		Port                      types.Int64  `tfsdk:"port"`
		User                      types.String `tfsdk:"user"`
		Password                  types.String `tfsdk:"password"`
		ValidateServerCertificate types.Bool   `tfsdk:"validate_server_certificate"`
		ConnectTimeout            types.Int64  `tfsdk:"connect_timeout"`
	}
	diags.Append(req.Config.Get(ctx, &cfg)...)

	out := &ProviderConfig{
		Host:                      cfg.Host.ValueString(),
		Port:                      8563,
		User:                      cfg.User.ValueString(),
		Password:                  cfg.Password.ValueString(),
		ValidateServerCertificate: true,
		ConnectTimeout:            60,
	}
	if !cfg.Port.IsNull() {
		out.Port = cfg.Port.ValueInt64()
	}
	if !cfg.ValidateServerCertificate.IsNull() {
		out.ValidateServerCertificate = cfg.ValidateServerCertificate.ValueBool()
	}
	if !cfg.ConnectTimeout.IsNull() {
		out.ConnectTimeout = cfg.ConnectTimeout.ValueInt64()
	}

	return out, diags
}
