package resources

import "sync"

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
