package main

import "github.com/gammons/slk/internal/config"

// workspaceConfigStore holds cfg.Workspaces. SaveTheme and
// SaveSidebarWidth are the in-memory half of the theme/sidebar-width
// savers wired in run(), mutating the map in place on the Update
// goroutine. Snapshot is what every connect goroutine does with cfg
// before reading it further (WorkspaceByTeamID, MatchSectionAndOrder,
// SectionOrder, ResolveTheme, ResolveWidth).
//
// This version has no synchronization: a value copy of config.Config
// does not copy Workspaces independently -- Workspaces is a map, so
// Snapshot's copy aliases the same underlying map SaveTheme and
// SaveSidebarWidth mutate concurrently from another goroutine. See
// TestWorkspaceConfigStore_ConcurrentSaveAndSnapshot, which shows why
// that's a live bug rather than a simplification.
type workspaceConfigStore struct {
	cfg config.Config
}

func newWorkspaceConfigStore(cfg config.Config) *workspaceConfigStore {
	return &workspaceConfigStore{cfg: cfg}
}

// SaveTheme finds or creates teamID's TOML block and sets its Theme,
// returning the TOML key for the caller to persist to disk.
func (s *workspaceConfigStore) SaveTheme(teamID, name string) (tomlKey string) {
	tomlKey = s.tomlKeyFor(teamID)
	ws := s.cfg.Workspaces[tomlKey]
	ws.TeamID = teamID
	ws.Theme = name
	s.cfg.Workspaces[tomlKey] = ws
	return tomlKey
}

// SaveSidebarWidth is SaveTheme's sibling for sidebar width.
func (s *workspaceConfigStore) SaveSidebarWidth(teamID string, width int) (tomlKey string) {
	tomlKey = s.tomlKeyFor(teamID)
	ws := s.cfg.Workspaces[tomlKey]
	ws.TeamID = teamID
	ws.SidebarWidth = width
	s.cfg.Workspaces[tomlKey] = ws
	return tomlKey
}

// tomlKeyFor finds the existing TOML key for teamID, if any -- if no
// block exists yet it falls back to the team ID itself (legacy
// default; a future --add-workspace may have already written a
// slug-keyed block) -- and ensures Workspaces is non-nil.
func (s *workspaceConfigStore) tomlKeyFor(teamID string) string {
	if s.cfg.Workspaces == nil {
		s.cfg.Workspaces = make(map[string]config.Workspace)
	}
	for k, w := range s.cfg.Workspaces {
		if w.TeamID == teamID {
			return k
		}
	}
	return teamID
}

// Snapshot returns cfg as every connect goroutine currently receives
// it: a plain value copy.
func (s *workspaceConfigStore) Snapshot() config.Config {
	return s.cfg
}
