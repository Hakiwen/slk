package main

import (
	"sync"
	"testing"
)

// TestActiveTeam_ConcurrentSetAndGet is a race-detector test. It
// reproduces the pattern boot produces -- the Update goroutine's
// workspace switcher and every connect goroutine's initial-active
// claim calling Set, while every workspace's isActive closure calls
// Get from its own WebSocket goroutine -- and fails under -race
// against this commit's activeTeam, which has no synchronization. The
// next commit fixes it by deleting activeTeam in favor of
// workspaceRouter.Active(), which is already race-safe (see
// TestWorkspaceRouter_ConcurrentSetAndActive).
func TestActiveTeam_ConcurrentSetAndGet(t *testing.T) {
	a := &activeTeam{}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		id := string(rune('A' + i))
		wg.Add(2)
		go func() {
			defer wg.Done()
			a.Set(id)
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = a.Get()
			}
		}()
	}
	wg.Wait()
}
