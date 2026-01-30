package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLockManager_BasicLocking(t *testing.T) {
	lm := NewLockManager()

	// Test basic lock/unlock
	ctx := context.Background()
	err := lm.Lock(ctx, "repo1")
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	lm.Unlock("repo1")

	// Verify we can lock again
	err = lm.Lock(ctx, "repo1")
	if err != nil {
		t.Fatalf("Second lock failed: %v", err)
	}
	lm.Unlock("repo1")
}

func TestLockManager_PerRepoLocking(t *testing.T) {
	lm := NewLockManager()
	ctx := context.Background()

	// Lock repo1
	if err := lm.Lock(ctx, "repo1"); err != nil {
		t.Fatalf("Lock repo1 failed: %v", err)
	}

	// Should be able to lock repo2 concurrently
	if err := lm.Lock(ctx, "repo2"); err != nil {
		t.Fatalf("Lock repo2 failed: %v", err)
	}

	lm.Unlock("repo1")
	lm.Unlock("repo2")
}

func TestLockManager_ConcurrentAccess(t *testing.T) {
	lm := NewLockManager()
	ctx := context.Background()

	var counter int32
	var wg sync.WaitGroup

	// Simulate concurrent access to same repo
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lm.Lock(ctx, "repo1"); err != nil {
				t.Errorf("Lock failed: %v", err)
				return
			}
			defer lm.Unlock("repo1")

			// Critical section
			current := atomic.LoadInt32(&counter)
			time.Sleep(time.Millisecond) // Simulate work
			atomic.StoreInt32(&counter, current+1)
		}()
	}

	wg.Wait()

	// All increments should have been serialized
	if atomic.LoadInt32(&counter) != 10 {
		t.Errorf("Expected counter=10, got %d", counter)
	}
}

func TestLockManager_DifferentReposConcurrent(t *testing.T) {
	lm := NewLockManager()
	ctx := context.Background()

	var repo1Counter, repo2Counter int32
	var wg sync.WaitGroup

	// Access repo1 and repo2 concurrently
	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := lm.Lock(ctx, "repo1"); err != nil {
				return
			}
			defer lm.Unlock("repo1")
			atomic.AddInt32(&repo1Counter, 1)
			time.Sleep(time.Millisecond)
		}()
		go func() {
			defer wg.Done()
			if err := lm.Lock(ctx, "repo2"); err != nil {
				return
			}
			defer lm.Unlock("repo2")
			atomic.AddInt32(&repo2Counter, 1)
			time.Sleep(time.Millisecond)
		}()
	}

	wg.Wait()

	if atomic.LoadInt32(&repo1Counter) != 5 || atomic.LoadInt32(&repo2Counter) != 5 {
		t.Errorf("Expected both counters=5, got repo1=%d repo2=%d", repo1Counter, repo2Counter)
	}
}

func TestLockManager_ContextCancellation(t *testing.T) {
	lm := NewLockManager()
	bgCtx := context.Background()

	// Lock repo1
	if err := lm.Lock(bgCtx, "repo1"); err != nil {
		t.Fatalf("Lock failed: %v", err)
	}

	// Try to lock with cancelled context
	ctx, cancel := context.WithCancel(bgCtx)
	cancel() // Cancel immediately

	err := lm.Lock(ctx, "repo1")
	if err == nil {
		t.Error("Expected error for cancelled context")
		lm.Unlock("repo1")
	}

	// Release original lock
	lm.Unlock("repo1")
}

func TestLockManager_TryLock(t *testing.T) {
	lm := NewLockManager()
	ctx := context.Background()

	// TryLock should succeed when unlocked
	if !lm.TryLock("repo1") {
		t.Error("TryLock should succeed on unlocked repo")
	}

	// TryLock should fail when locked
	if lm.TryLock("repo1") {
		t.Error("TryLock should fail on locked repo")
	}

	lm.Unlock("repo1")

	// TryLock on different repo should succeed
	if err := lm.Lock(ctx, "repo1"); err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	if !lm.TryLock("repo2") {
		t.Error("TryLock should succeed on different repo")
	}
	lm.Unlock("repo1")
	lm.Unlock("repo2")
}

func TestLockManager_RLock(t *testing.T) {
	lm := NewLockManager()
	ctx := context.Background()

	// Multiple read locks should succeed
	if err := lm.RLock(ctx, "repo1"); err != nil {
		t.Fatalf("First RLock failed: %v", err)
	}
	if err := lm.RLock(ctx, "repo1"); err != nil {
		t.Fatalf("Second RLock failed: %v", err)
	}

	lm.RUnlock("repo1")
	lm.RUnlock("repo1")
}

func TestLockManager_ActiveLocks(t *testing.T) {
	lm := NewLockManager()
	ctx := context.Background()

	if lm.ActiveLocks() != 0 {
		t.Error("Expected 0 active locks initially")
	}

	lm.Lock(ctx, "repo1")
	if lm.ActiveLocks() != 1 {
		t.Error("Expected 1 active lock")
	}

	lm.Lock(ctx, "repo2")
	if lm.ActiveLocks() != 2 {
		t.Error("Expected 2 active locks")
	}

	lm.Unlock("repo1")
	if lm.ActiveLocks() != 1 {
		t.Error("Expected 1 active lock after unlock")
	}

	lm.Unlock("repo2")
	if lm.ActiveLocks() != 0 {
		t.Error("Expected 0 active locks after all unlocks")
	}
}

func TestLockManager_LockWithTimeout(t *testing.T) {
	lm := NewLockManager()
	ctx := context.Background()

	// Lock repo1
	if err := lm.Lock(ctx, "repo1"); err != nil {
		t.Fatalf("Lock failed: %v", err)
	}

	// Try to lock with short timeout - should fail
	err := lm.LockWithTimeout("repo1", 10*time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error")
		lm.Unlock("repo1")
	}

	lm.Unlock("repo1")

	// Now should succeed
	err = lm.LockWithTimeout("repo1", 100*time.Millisecond)
	if err != nil {
		t.Errorf("Expected success after unlock: %v", err)
	}
	lm.Unlock("repo1")
}
