package orchestrator

import (
	"context"
	"sync"
	"time"
)

// LockManager provides per-repository locking to allow concurrent operations
// on different repositories while serializing operations on the same repository.
type LockManager struct {
	mu    sync.Mutex
	locks map[string]*repoLock
}

// repoLock holds the lock and reference count for a single repository.
type repoLock struct {
	mu       sync.RWMutex
	refCount int
}

// NewLockManager creates a new lock manager.
func NewLockManager() *LockManager {
	return &LockManager{
		locks: make(map[string]*repoLock),
	}
}

// Lock acquires an exclusive lock for the given repository.
// It respects context cancellation and returns an error if the context is cancelled.
func (lm *LockManager) Lock(ctx context.Context, repoID string) error {
	lock := lm.getOrCreateLock(repoID)

	// Try to acquire the lock with context cancellation support
	acquired := make(chan struct{})
	go func() {
		lock.mu.Lock()
		close(acquired)
	}()

	select {
	case <-ctx.Done():
		// Context was cancelled - we need to clean up
		// Try to acquire anyway to avoid leaving goroutine hanging forever
		go func() {
			<-acquired // Wait for lock acquisition
			lock.mu.Unlock()
			lm.releaseLock(repoID)
		}()
		return ctx.Err()
	case <-acquired:
		return nil
	}
}

// Unlock releases the exclusive lock for the given repository.
func (lm *LockManager) Unlock(repoID string) {
	lm.mu.Lock()
	lock, ok := lm.locks[repoID]
	lm.mu.Unlock()

	if ok {
		lock.mu.Unlock()
		lm.releaseLock(repoID)
	}
}

// RLock acquires a read lock for the given repository.
// Multiple read locks can be held simultaneously.
func (lm *LockManager) RLock(ctx context.Context, repoID string) error {
	lock := lm.getOrCreateLock(repoID)

	acquired := make(chan struct{})
	go func() {
		lock.mu.RLock()
		close(acquired)
	}()

	select {
	case <-ctx.Done():
		go func() {
			<-acquired
			lock.mu.RUnlock()
			lm.releaseLock(repoID)
		}()
		return ctx.Err()
	case <-acquired:
		return nil
	}
}

// RUnlock releases a read lock for the given repository.
func (lm *LockManager) RUnlock(repoID string) {
	lm.mu.Lock()
	lock, ok := lm.locks[repoID]
	lm.mu.Unlock()

	if ok {
		lock.mu.RUnlock()
		lm.releaseLock(repoID)
	}
}

// TryLock attempts to acquire an exclusive lock without blocking.
// Returns true if the lock was acquired, false otherwise.
func (lm *LockManager) TryLock(repoID string) bool {
	lock := lm.getOrCreateLock(repoID)
	if lock.mu.TryLock() {
		return true
	}
	lm.releaseLock(repoID)
	return false
}

// LockWithTimeout acquires an exclusive lock with a timeout.
func (lm *LockManager) LockWithTimeout(repoID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return lm.Lock(ctx, repoID)
}

// getOrCreateLock returns the lock for a repository, creating it if necessary.
func (lm *LockManager) getOrCreateLock(repoID string) *repoLock {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lock, ok := lm.locks[repoID]
	if !ok {
		lock = &repoLock{}
		lm.locks[repoID] = lock
	}
	lock.refCount++
	return lock
}

// releaseLock decrements the reference count and removes the lock if no longer needed.
func (lm *LockManager) releaseLock(repoID string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lock, ok := lm.locks[repoID]
	if !ok {
		return
	}

	lock.refCount--
	if lock.refCount <= 0 {
		delete(lm.locks, repoID)
	}
}

// ActiveLocks returns the number of repositories currently being accessed.
// Useful for metrics and debugging.
func (lm *LockManager) ActiveLocks() int {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return len(lm.locks)
}
