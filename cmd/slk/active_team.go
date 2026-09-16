package main

// activeTeam holds the currently active workspace's team ID: set by
// the workspace switcher (Update goroutine) and by whichever connect
// goroutine claims the initial-active slot, and read by every
// workspace's isActive closure on its own WebSocket goroutine, and by
// the theme/sidebar-width savers on the Update goroutine.
//
// This version has no synchronization -- see
// TestActiveTeam_ConcurrentSetAndGet, which shows why that's a live
// bug rather than a simplification.
type activeTeam struct {
	id string
}

func (a *activeTeam) Set(id string) { a.id = id }
func (a *activeTeam) Get() string   { return a.id }
