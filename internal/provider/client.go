package provider

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"terraform-provider-exasol/internal/exasolclient"

	"github.com/exasol/exasol-driver-go"
	"github.com/exasol/exasol-driver-go/pkg/dsn"
)

// Re-export the concrete type so the rest of the provider can keep using provider.Client.
type Client = exasolclient.Client

// NewClient builds the correct Exasol DSN and opens the connection.
// It now always includes the `encryption` flag, and lets the caller
// control whether the server certificate is validated.
func NewClient(ctx context.Context, c *ProviderConfig) (*Client, error) {
	var config *dsn.DSNConfigBuilder

	// Detect if password is a PAT token
	if strings.HasPrefix(c.Password, "exa_pat_") {
		config = exasol.NewConfigWithRefreshToken(c.Password) // Use PAT as refresh token
	} else {
		config = exasol.NewConfig(c.User, c.Password) // Use regular password
	}

	dsnString := config.Host(c.Host).
		Port(int(c.Port)).
		ValidateServerCertificate(c.ValidateServerCertificate).
		String()

	db, err := sql.Open("exasol", dsnString)
	if err != nil {
		return nil, err
	}

	// Tune connection pool for Terraform's parallel operations (default parallelism=10).
	// Go's default MaxIdleConns=2 causes constant reconnection churn under parallel load.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Retry the initial ping until connect_timeout elapses: each ping dials a
	// fresh connection, so this rides out IP-allowlist propagation delays.
	deadline := time.Now().Add(time.Duration(c.ConnectTimeout) * time.Second)
	for {
		err = db.PingContext(ctx)
		if err == nil {
			return &Client{DB: db}, nil
		}
		if ctx.Err() != nil {
			_ = db.Close()
			return nil, ctx.Err()
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			_ = db.Close()
			return nil, err
		}
		wait := 10 * time.Second
		if remaining < wait {
			wait = remaining
		}
		select {
		case <-ctx.Done():
			_ = db.Close()
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}
