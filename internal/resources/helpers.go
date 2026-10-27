package resources

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"terraform-provider-exasol/internal/exasolclient"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func qualify(obj string) string {
	// Allow user to pass SCHEMA.TABLE or just SCHEMA.
	// We quote identifiers but keep dots as separators.
	// Also validate each part to prevent SQL injection.
	parts := strings.Split(obj, ".")
	for i, p := range parts {
		// Remove existing quotes if any
		cleaned := strings.Trim(p, `"`)

		// Validate the identifier
		if !isValidIdentifier(cleaned) {
			// If validation fails, still escape it to prevent SQL injection
			// but don't panic - let the database return an error
			cleaned = escapeIdentifierLiteral(cleaned)
		}

		parts[i] = fmt.Sprintf(`"%s"`, cleaned)
	}
	return strings.Join(parts, ".")
}

// isTrueAdminOption interprets the ADMIN_OPTION column of EXA_DBA_SYS_PRIVS and
// EXA_DBA_ROLE_PRIVS. SaaS returns "TRUE"/"1", Docker returns "true".
func isTrueAdminOption(v string) bool {
	return v == "TRUE" || v == "1" || v == "true"
}

// existsFunc reports whether a database object with the given name exists.
// Names are compared as Exasol stores them: users, roles and connections in
// uppercase, schema objects with their case.
type existsFunc func(ctx context.Context, db *sql.DB, name string) (bool, error)

// resolveEndpoint returns where a grant endpoint (grantee, role, connection or
// target) that was oldName lives now, or gone=true if it no longer exists.
//
// Renames executed by this provider run come from the rename log, so they are
// exact: Exasol moved the grant to the new name. Otherwise the old name is
// checked in the database; if it is missing, it was dropped (and its grants
// with it). Users, roles and connections have no stable ID in Exasol, so a
// rename done outside this run cannot be detected here; Read drops such grants
// from state before an apply.
func resolveEndpoint(ctx context.Context, db *sql.DB, renames *exasolclient.RenameLog, kind, oldName, newName string, exists existsFunc) (string, bool, error) {
	if current, ok := renames.Lookup(kind, oldName); ok {
		return current, false, nil
	}
	if oldName == newName {
		return oldName, false, nil
	}
	found, err := exists(ctx, db, oldName)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", true, nil
	}
	return oldName, false, nil
}

// renamedAway reports whether the object that oldName named in state was renamed
// in this run to a name other than newName. That happens when another object
// takes over the old name in the same apply (rename A to C, then B to A): the
// configured name is unchanged, but the grant has to be reconciled as moved.
func renamedAway(renames *exasolclient.RenameLog, kind, oldName, newName string) bool {
	current, ok := renames.Lookup(kind, oldName)
	if !ok {
		return false
	}
	if kind == exasolclient.RenameSchema {
		return current != newName
	}
	return current != strings.ToUpper(newName)
}

// objectRenamedAway does the same for the schema of an object privilege target.
// Schema names are compared with their case, like in the rename log.
func objectRenamedAway(renames *exasolclient.RenameLog, oldName, newName string) bool {
	oldSchema, _, _ := strings.Cut(strings.ReplaceAll(oldName, `"`, ""), ".")
	newSchema, _, _ := strings.Cut(strings.ReplaceAll(newName, `"`, ""), ".")
	return renamedAway(renames, exasolclient.RenameSchema, oldSchema, newSchema)
}

// objectTarget resolves the target of an object privilege whose exact target is
// not known yet (an import, or state written by provider 0.2.x), and reports
// whether that object exists. Quoted names are case-sensitive in Exasol and
// exasol_schema keeps the configured case, so "s" and "S" can both exist: the
// exact spelling wins. Only when no object has the exact spelling is the
// uppercase one used, e.g. for a lowercase spelling of an uppercase object in
// an import. Quotes are ignored, as in qualify(). The result is stored in
// private state, so later operations use the known target, see
// knownObjectTarget.
func objectTarget(ctx context.Context, db *sql.DB, name string) (string, bool, error) {
	exact := strings.ReplaceAll(name, `"`, "")
	found, err := objectExists(ctx, db, exact)
	if err != nil || found {
		return exact, found, err
	}
	upper := strings.ToUpper(exact)
	if upper == exact {
		return exact, false, nil
	}
	found, err = objectExists(ctx, db, upper)
	return upper, found, err
}

// resolveKnownObject returns where an object privilege target that was known
// exactly as oldTarget (SCHEMA or SCHEMA.NAME, case as stored, no quotes) lives
// now, or gone=true if it no longer exists. A schema renamed in this run comes
// from the rename log. There is no case fallback: a known target that
// disappeared was dropped.
func resolveKnownObject(ctx context.Context, db *sql.DB, renames *exasolclient.RenameLog, oldTarget string) (string, bool, error) {
	schema, object, qualified := strings.Cut(oldTarget, ".")
	if current, ok := renames.Lookup(exasolclient.RenameSchema, schema); ok {
		if qualified {
			current += "." + object
		}
		return current, false, nil
	}
	found, err := objectExists(ctx, db, oldTarget)
	if err != nil {
		return "", false, err
	}
	return oldTarget, !found, nil
}

// requireExists adds an error and returns false when a new grant endpoint does
// not exist. Update calls it before any REVOKE, so a grant moved to a missing
// grantee, role, connection, target or object fails without removing access.
func requireExists(ctx context.Context, db *sql.DB, add func(string, string), what, name string, exists existsFunc) bool {
	found, err := exists(ctx, db, name)
	if err != nil {
		add(fmt.Sprintf("Check %s failed", strings.ToLower(what)), err.Error())
		return false
	}
	if !found {
		add(fmt.Sprintf("%s does not exist", what),
			fmt.Sprintf("%s %q does not exist, so the grant cannot move to it. Nothing was revoked.", what, name))
		return false
	}
	return true
}

func rowExists(ctx context.Context, db *sql.DB, query string, args ...any) (bool, error) {
	var dummy int
	err := db.QueryRowContext(ctx, query, args...).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// principalExists reports whether a user or role with this name exists.
func principalExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	return rowExists(ctx, db,
		`SELECT 1 FROM EXA_DBA_USERS WHERE USER_NAME = ? UNION ALL SELECT 1 FROM EXA_DBA_ROLES WHERE ROLE_NAME = ?`,
		name, name)
}

func roleExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	return rowExists(ctx, db, `SELECT 1 FROM EXA_DBA_ROLES WHERE ROLE_NAME = ?`, name)
}

func connectionExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	return rowExists(ctx, db, `SELECT 1 FROM EXA_DBA_CONNECTIONS WHERE CONNECTION_NAME = ?`, name)
}

// objectExists reports whether a schema, or a schema object given as
// SCHEMA.NAME, exists. Quotes are ignored, as in qualify().
func objectExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	unquoted := strings.ReplaceAll(name, `"`, "")
	if schema, object, qualified := strings.Cut(unquoted, "."); qualified {
		return rowExists(ctx, db, `SELECT 1 FROM EXA_DBA_OBJECTS WHERE ROOT_NAME = ? AND OBJECT_NAME = ?`, schema, object)
	}
	// EXA_DBA_SCHEMAS lists every schema, independent of the provider user's
	// access to it, like the other EXA_DBA_* views the provider uses.
	return rowExists(ctx, db, `SELECT 1 FROM EXA_DBA_SCHEMAS WHERE SCHEMA_NAME = ?`, unquoted)
}

// cachedRow returns the first row of query whose leading columns equal key,
// from the read cache, or nil when there is none. Only Read uses it: Create
// and Update query directly, so an apply does not reload whole views.
func cachedRow(ctx context.Context, db *sql.DB, reads *exasolclient.ReadCache, query string, key ...string) ([]sql.NullString, error) {
	rows, err := reads.Rows(ctx, db, query, key...)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

// collisionRetries is how often execWrite runs a statement again after a
// transaction collision. The waits add up to at most about 9 seconds.
const collisionRetries = 8

// execWrite runs a statement that changes the database and then clears the
// read cache, so a later Read does not see rows from before the change.
//
// Parallel GRANT, REVOKE and DROP statements on the same user or role can
// collide in Exasol (SQL error 40001). Exasol rolls the colliding statement
// back completely, so execWrite runs it again after a random wait ("full
// jitter": anywhere up to an exponential limit), which spreads the retries of
// statements that collided with each other.
func execWrite(ctx context.Context, db *sql.DB, reads *exasolclient.ReadCache, query string, args ...any) (sql.Result, error) {
	defer reads.Clear()
	for attempt := 0; ; attempt++ {
		res, err := db.ExecContext(ctx, query, args...)
		if err == nil || attempt == collisionRetries || !isTransactionCollision(err) {
			return res, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w (retry stopped: %v)", err, ctx.Err())
		case <-time.After(collisionWait(attempt)):
		}
	}
}

// collisionWait is the wait before retry attempt+1. Tests replace it.
var collisionWait = func(attempt int) time.Duration {
	return 10*time.Millisecond + rand.N(min(2*time.Second, 100*time.Millisecond<<attempt))
}

// isTransactionCollision reports whether err is Exasol's SQL error 40001. The
// driver only returns it as text: "... SQL error code '40001' and message ...".
func isTransactionCollision(err error) bool {
	return strings.Contains(err.Error(), "SQL error code '40001'")
}

// keepRenameInState stores the new name in state right after a rename. When a
// later step of the same Update fails, Terraform keeps this state, and the next
// run finds the object under its new name instead of planning to create it.
func keepRenameInState(ctx context.Context, state *tfsdk.State, id, name types.String) diag.Diagnostics {
	diags := state.SetAttribute(ctx, path.Root("id"), id)
	diags.Append(state.SetAttribute(ctx, path.Root("name"), name)...)
	return diags
}
