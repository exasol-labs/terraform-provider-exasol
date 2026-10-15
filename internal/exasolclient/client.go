package exasolclient

import (
	"database/sql"
	"strings"
	"sync"
)

type Client struct {
	DB *sql.DB
	// Renames records the renames this provider instance executed. Terraform
	// starts one provider instance per provider configuration and command, so
	// it covers one apply of one provider block. A rename through another
	// provider alias, or in another run, is not in it.
	Renames *RenameLog
}

// Rename kinds. Users and roles share one namespace in Exasol.
const (
	RenamePrincipal  = "PRINCIPAL"
	RenameConnection = "CONNECTION"
	RenameSchema     = "SCHEMA"
)

// RenameLog maps old names to new names for renames executed in this run.
// Exasol moves grants to the new name on RENAME, and Terraform renames an
// object before it updates the grants that reference it, so grant resources
// use this log to find where a previous grant lives now.
type RenameLog struct {
	mu sync.Mutex
	m  map[string]string
}

func NewRenameLog() *RenameLog { return &RenameLog{m: map[string]string{}} }

func renameKey(kind, name string) string { return kind + "|" + strings.ToUpper(name) }

// Record notes that kind oldName was renamed to newName. A later rename of
// newName is followed, so old names always resolve to the latest name.
func (l *RenameLog) Record(kind, oldName, newName string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	newName = strings.ToUpper(newName)
	oldKey := renameKey(kind, oldName)
	// Follow chains: whatever pointed at oldName now points at newName. An
	// entry for an object that was renamed away from newName stays, because
	// grants recorded under that earlier name still belong to that object.
	for k, v := range l.m {
		if strings.HasPrefix(k, kind+"|") && renameKey(kind, v) == oldKey {
			l.m[k] = newName
		}
	}
	l.m[oldKey] = newName
}

// Lookup returns the current name of kind name if it was renamed in this run.
func (l *RenameLog) Lookup(kind, name string) (string, bool) {
	if l == nil {
		return "", false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.m[renameKey(kind, name)]
	return v, ok
}
