package resources

import (
	"slices"
	"strings"
	"sync"
)

// deleteMutex serializes all delete operations, and grant updates that revoke,
// to prevent transaction collision errors (40001)
// in Exasol when multiple REVOKE/DROP statements execute simultaneously.
//
// execWrite also retries a statement after error 40001. That covers creates,
// which run in parallel; deletes stay serialized as before.
var deleteMutex sync.Mutex

// lockDelete locks the global delete mutex to serialize delete operations.
// Call defer unlockDelete() immediately after calling this.
func lockDelete() {
	deleteMutex.Lock()
}

// unlockDelete unlocks the global delete mutex.
func unlockDelete() {
	deleteMutex.Unlock()
}

// granteeLocks holds one mutex per grantee name.
var granteeLocks sync.Map

// lockGrantees serializes grant statements per grantee and returns the unlock
// function: defer lockGrantees(a, b)(). Parallel GRANT and REVOKE statements on
// the same grantee collide in Exasol (40001); statements on different grantees
// do not, so grants to different grantees still run in parallel. Names are
// locked in sorted order, so two calls never wait for each other. Update and
// Delete take it after lockDelete, Create without it.
func lockGrantees(names ...string) func() {
	keys := make([]string, 0, len(names))
	for _, n := range names {
		keys = append(keys, strings.ToUpper(n))
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	locked := make([]*sync.Mutex, 0, len(keys))
	for _, k := range keys {
		m, _ := granteeLocks.LoadOrStore(k, &sync.Mutex{})
		mu := m.(*sync.Mutex)
		mu.Lock()
		locked = append(locked, mu)
	}
	return func() {
		for i := len(locked) - 1; i >= 0; i-- {
			locked[i].Unlock()
		}
	}
}
