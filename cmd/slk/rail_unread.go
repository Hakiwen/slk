package main

import (
	"log"

	"github.com/gammons/slk/internal/cache"
)

// railUnreadWorkspaces returns the workspace IDs whose rail dot should
// be lit. It is the reader wireCallbacks installs through
// App.SetWorkspaceUnreadReader; OtherUnreadCount (the title's "+N" and
// $SLK_OTHER_UNREAD) reads through the same installed reader, so the
// three surfaces cannot disagree.
//
// A workspace is lit when its own sidebar would show something unread,
// and the sidebar has two such signals, so the rail asks both:
//
//   - channels: at least one channel in wctx.Channels is
//     ChannelItem.IsVisiblyUnread (internal/ui/sidebar/model.go), the
//     one predicate the sidebar dot, UnreadChannelCount and $SLK_UNREAD
//     already share. That is what keeps a muted channel from lighting
//     the rail while the sidebar shows nothing unread.
//   - threads: threadsUnread(teamID, selfUserID) reports whether
//     cache.ListSubscribedThreads returns any summary with Unread set,
//     which is exactly what the sidebar's Threads-row badge counts
//     (threadsview.Model.UnreadCount). A thread reply never sets the
//     channel's has_unread (OnMessage, channelEligible), so without
//     this half a workspace could carry a "•N" badge in its sidebar
//     and a dark dot in the rail -- the field case, two unread DM
//     thread replies and no dot. Threads are not subject to channel
//     mute, and the boot-time counts.threads.has_unreads flag is not
//     used here: it is stale from the first thread_marked onward,
//     which is why per-thread last_read replaced it.
//
// unread is db.UnreadChannels; teamIDs is the configured workspace
// list (the rail's rows); byID resolves a workspace to its live
// context and is router.ByID in production; threadsUnread is
// railThreadsUnread(db). All are parameters so the predicate is pure
// and testable with neither a router nor a DB, the same reason
// sidebar.IsStale takes its read state as arguments.
//
// Two edge cases go opposite ways:
//
//   - byID returns nil (the workspace is still connecting, or its
//     connect failed): any unread channel row lights it, and the thread
//     query runs with an empty self ID, so its self-authored suppression
//     cannot fire and any unread thread lights it too. There is no
//     channel list or user ID to check against, so MuteStore.Ready's
//     conservative default applies -- a dot we might have suppressed
//     beats one the user wanted and lost. This also keeps last
//     session's cached dots visible during boot.
//   - the row's channel is not in wctx.Channels: it never lights. The
//     sidebar and the local channel finder are built from that list,
//     so such a channel has no row on screen to explain a rail dot and
//     no keystroke to clear it. (The field case was an archived
//     channel: userBoot lists it, bootConversations drops it,
//     hydrateFirstSight still caches it, client.counts still reports
//     it unread.)
//
// wctx.Channels is read here on the UI goroutine without
// synchronization, against writes from the WebSocket handler
// (refreshMutedForActive, OnConversationOpened). That is how the
// Lookup callback in wireCallbacks already reads it; this adds a
// reader, not a convention.
func railUnreadWorkspaces(unread []cache.UnreadChannel, teamIDs []string, byID func(teamID string) *WorkspaceContext, threadsUnread func(teamID, selfUserID string) bool) []string {
	var out []string
	lit := map[string]bool{}
	for _, u := range unread {
		if lit[u.WorkspaceID] {
			continue
		}
		if railRowLights(u, byID(u.WorkspaceID)) {
			lit[u.WorkspaceID] = true
			out = append(out, u.WorkspaceID)
		}
	}
	for _, teamID := range teamIDs {
		if lit[teamID] {
			continue
		}
		selfUserID := ""
		if wctx := byID(teamID); wctx != nil {
			selfUserID = wctx.UserID
		}
		if threadsUnread(teamID, selfUserID) {
			lit[teamID] = true
			out = append(out, teamID)
		}
	}
	return out
}

// railRowLights reports whether one unread row lights its workspace's
// dot. The nil-wctx branch is the "unknown, so light it" case
// railUnreadWorkspaces documents; a channel absent from wctx.Channels
// is the "cannot be shown, so never light it" case.
func railRowLights(u cache.UnreadChannel, wctx *WorkspaceContext) bool {
	if wctx == nil {
		return true
	}
	for _, item := range wctx.Channels {
		if item.ID == u.ChannelID {
			return item.IsVisiblyUnread(u.State)
		}
	}
	return false
}

// railThreadsUnread is the production threadsUnread input for
// railUnreadWorkspaces: it runs the same cache.ListSubscribedThreads
// query the Threads badge is counted from and reports whether any row
// is Unread. Reusing the query rather than writing an EXISTS twin of
// it is deliberate: a second predicate is a second place for the rail
// and the badge to drift apart. The cost was measured before choosing
// this: five workspaces with at most ten active subscriptions each
// answer in well under a millisecond total on the field cache, and
// the reader only runs on read-state events.
func railThreadsUnread(db *cache.DB) func(teamID, selfUserID string) bool {
	return func(teamID, selfUserID string) bool {
		summaries, err := db.ListSubscribedThreads(teamID, selfUserID)
		if err != nil {
			log.Printf("Warning: ListSubscribedThreads(%s): %v", teamID, err)
			return false
		}
		for _, s := range summaries {
			if s.Unread {
				return true
			}
		}
		return false
	}
}
