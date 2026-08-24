package provider

import (
	"context"
	"database/sql"
	"errors"
	"net"
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

	// Retry the initial ping until connect_timeout elapses, so a run rides out
	// IP-allowlist propagation delays instead of failing on the first attempt.
	// Each attempt is bounded at retryInterval (or the time remaining, if
	// shorter) so a hanging dial is re-tried with a fresh dial every interval,
	// and a new attempt starts each interval regardless of how fast the
	// previous one failed. connect_timeout = 0 means a single bounded attempt.
	// Only network errors are retried; anything else (failed login, invalid
	// certificate) surfaces immediately.
	const retryInterval = 10 * time.Second
	deadline := time.Now().Add(time.Duration(c.ConnectTimeout) * time.Second)
	for {
		attemptStart := time.Now()
		window := retryInterval
		if r := time.Until(deadline); r > 0 && r < window {
			window = r
		}
		attemptCtx, cancel := context.WithTimeout(ctx, window)
		err = db.PingContext(attemptCtx)
		cancel()
		if err == nil {
			return &Client{DB: db}, nil
		}
		if ctx.Err() != nil {
			_ = db.Close()
			return nil, ctx.Err()
		}
		if !isRetryableConnectError(err) || !time.Now().Before(deadline) {
			_ = db.Close()
			return nil, err
		}
		wait := retryInterval - time.Since(attemptStart)
		if r := time.Until(deadline); r < wait {
			wait = r
		}
		if wait > 0 {
			select {
			case <-ctx.Done():
				_ = db.Close()
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
	}
}

// isRetryableConnectError reports whether a connect failure is worth retrying.
// Network-level failures (dial timeout, connection refused, host unreachable)
// and our per-attempt deadline all satisfy net.Error; authentication and TLS
// certificate errors do not, and fail fast.
func isRetryableConnectError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}
