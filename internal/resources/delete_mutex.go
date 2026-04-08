package resources

import "sync"

// deleteMutex serializes all delete operations to prevent transaction collision errors (40001)
// in Exasol when multiple REVOKE/DROP statements execute simultaneously.
//
// A future improvement would be retry logic with exponential backoff for error 40001,
// which would allow parallel deletes while handling occasional collisions.
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
