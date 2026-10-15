package resources

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"terraform-provider-exasol/internal/exasolclient"
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

// existsFunc reports whether a database object with the given uppercase name
// exists.
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
func objectRenamedAway(renames *exasolclient.RenameLog, oldName, newName string) bool {
	oldSchema, _, _ := strings.Cut(strings.ToUpper(strings.ReplaceAll(oldName, `"`, "")), ".")
	newSchema, _, _ := strings.Cut(strings.ToUpper(strings.ReplaceAll(newName, `"`, "")), ".")
	return renamedAway(renames, exasolclient.RenameSchema, oldSchema, newSchema)
}

// resolveObject does the same for an object privilege target: a schema name, or
// SCHEMA.NAME whose schema may have been renamed. Names are uppercase; quotes
// are removed, as in qualify().
func resolveObject(ctx context.Context, db *sql.DB, renames *exasolclient.RenameLog, oldName, newName string) (string, bool, error) {
	oldName = strings.ReplaceAll(oldName, `"`, "")
	newName = strings.ReplaceAll(newName, `"`, "")
	schema, object, qualified := strings.Cut(oldName, ".")
	if current, ok := renames.Lookup(exasolclient.RenameSchema, schema); ok {
		if qualified {
			return current + "." + object, false, nil
		}
		return current, false, nil
	}
	if oldName == newName {
		return oldName, false, nil
	}
	found, err := objectExists(ctx, db, oldName)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", true, nil
	}
	return oldName, false, nil
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
	return rowExists(ctx, db, `SELECT 1 FROM EXA_SCHEMAS WHERE SCHEMA_NAME = ?`, unquoted)
}
