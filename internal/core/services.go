// Service ports the TUI calls. Implementations are wired by cmd/slk.
//
// Each port can be built from plain closures with its NewXxxService
// constructor; any nil closure makes that method a no-op returning the
// zero value, which is how tests fake a single method.
//
// Constructor shape:
//   - Services with ≤4 methods take positional func args
//     (NewReactionService(add, remove, loadFrecent, recordFrecent)).
//   - Services with ≥5 methods take a struct of named funcs
//     (NewThreadService(ThreadServiceFuncs{Fetch: fn, Mark: fn, ...})).
package core

import (
	"context"
	"image"
	"io/fs"

	"github.com/gammons/slk/internal/ids"
	imgpkg "github.com/gammons/slk/internal/image"
)

// ReactionService is the App's interface to the Slack reaction API
// and the user's recent-emoji-use history (frecency). Implementations
// are wired by cmd/slk/main.go.
//
// All methods are best-effort and nil-safe at the adapter level: an
// implementation built via NewReactionService with a nil component
// silently no-ops that operation.
type ReactionService interface {
	// Add adds emoji to messageTS in channelID. Returns an error if
	// the Slack API call fails; App turns that into a status-bar toast.
	Add(channelID ids.ChannelID, messageTS ids.MessageTS, emoji string) error

	// Remove removes the current user's emoji reaction from messageTS
	// in channelID.
	Remove(channelID ids.ChannelID, messageTS ids.MessageTS, emoji string) error

	// LoadFrecent returns up to limit emoji entries from the user's
	// recent-use history, ordered by frecency. May return nil; the
	// reaction picker handles an empty slice as "no recents yet".
	LoadFrecent(limit int) []EmojiEntry

	// RecordFrecent records emoji as recently used so future
	// LoadFrecent calls surface it. Called after every successful
	// reaction add.
	RecordFrecent(emoji string)
}

// NewReactionService builds a ReactionService from individual
// function closures. Any function may be nil; the resulting service
// no-ops that operation and returns the zero value for read paths.
// Used by both cmd/slk/main.go (production wiring) and tests (fake
// closures).
func NewReactionService(
	add ReactionAddFunc,
	remove ReactionRemoveFunc,
	loadFrecent FrecentLoadFunc,
	recordFrecent FrecentRecordFunc,
) ReactionService {
	return reactionAdapter{
		add:           add,
		remove:        remove,
		loadFrecent:   loadFrecent,
		recordFrecent: recordFrecent,
	}
}

type reactionAdapter struct {
	add           ReactionAddFunc
	remove        ReactionRemoveFunc
	loadFrecent   FrecentLoadFunc
	recordFrecent FrecentRecordFunc
}

func (r reactionAdapter) Add(channelID ids.ChannelID, messageTS ids.MessageTS, emoji string) error {
	if r.add == nil {
		return nil
	}
	return r.add(channelID, messageTS, emoji)
}

func (r reactionAdapter) Remove(channelID ids.ChannelID, messageTS ids.MessageTS, emoji string) error {
	if r.remove == nil {
		return nil
	}
	return r.remove(channelID, messageTS, emoji)
}

func (r reactionAdapter) LoadFrecent(limit int) []EmojiEntry {
	if r.loadFrecent == nil {
		return nil
	}
	return r.loadFrecent(limit)
}

func (r reactionAdapter) RecordFrecent(emoji string) {
	if r.recordFrecent == nil {
		return
	}
	r.recordFrecent(emoji)
}

// ThreadService is the App's interface to Slack's thread surfaces:
// fetching replies, marking threads read, posting replies, and loading
// the involved-threads list for the user's threads view. Includes
// ThreadLastRead because the thread panel needs the thread's own
// last-read cursor to render its unread boundary.
//
// Implementations are wired by cmd/slk/main.go. Build one via
// NewThreadService from a ThreadServiceFuncs struct so unused
// methods can be left nil without trailing positional nils.
type ThreadService interface {
	// Fetch retrieves replies for threadTS in channelID from Slack.
	// Returns a Msg (typically ThreadRepliesLoadedMsg).
	Fetch(channelID ids.ChannelID, threadTS ids.ThreadTS) Msg

	// CacheRead returns cached replies (or nil) so the thread panel
	// can populate without waiting for the network. A non-empty
	// return causes immediate render; the subsequent Fetch result
	// overwrites with authoritative data.
	CacheRead(channelID ids.ChannelID, threadTS ids.ThreadTS) []MessageItem

	// Mark marks the thread as read on Slack's servers
	// (subscriptions.thread.mark) and, on success, advances the local
	// thread_subscriptions cursor. channelID is the parent channel,
	// threadTS is the parent message ts, ts is the latest reply ts the
	// user has now seen. Returns a Cmd yielding ThreadMarkedLocalMsg.
	Mark(channelID ids.ChannelID, threadTS ids.ThreadTS, ts ids.MessageTS) Cmd

	// SendReply posts a reply to threadTS in channelID. When broadcast
	// is true the reply is also posted to the parent channel feed
	// (reply_broadcast=true). Returns a Msg (typically
	// ThreadReplySentMsg or ThreadReplySendFailedMsg).
	SendReply(channelID ids.ChannelID, threadTS ids.ThreadTS, text string, broadcast bool) Msg

	// ListFetch loads the involved-threads list for the workspace
	// (Slack subscriptions.list). Returns a Msg (typically
	// ThreadsListLoadedMsg).
	ListFetch(teamID ids.TeamID) Msg

	// EnsureSubscriptions kicks the workspace's throttled thread-
	// subscription sync (subscriptions.thread.getView) in the
	// background. Called on workspace-ready and on Threads-view
	// activation; the implementation collapses those to at most one
	// network sweep per throttle window, so callers fire
	// unconditionally.
	//
	// Separate from ListFetch because ListFetch is cache-only and runs
	// on workspace-ready (for the sidebar's Threads badge), whereas
	// this may issue subscriptions.thread.getView, which paginates to
	// a 1000-item hard cap — measured at ~62 requests per workspace.
	EnsureSubscriptions(teamID ids.TeamID)

	// ThreadLastRead returns the thread's own last-read cursor from
	// thread_subscriptions so the thread panel can render a "── new ──"
	// boundary. The parent channel's cursor is NOT a substitute: plain
	// thread replies never advance it, so it is systematically stale and
	// puts the divider too early. Optional; "" disables the boundary.
	ThreadLastRead(channelID ids.ChannelID, threadTS ids.ThreadTS) string
}

// ThreadServiceFuncs is the closure bundle accepted by
// NewThreadService. Any field may be nil; the resulting service
// no-ops that operation (and returns the zero value for read paths).
type ThreadServiceFuncs struct {
	Fetch               ThreadFetchFunc
	CacheRead           ThreadCacheReadFunc
	Mark                ThreadMarkFunc
	SendReply           ThreadReplySendFunc
	ListFetch           ThreadsListFetchFunc
	EnsureSubscriptions func(teamID ids.TeamID)
	ThreadLastRead      func(channelID ids.ChannelID, threadTS ids.ThreadTS) string
}

// NewThreadService builds a ThreadService from a ThreadServiceFuncs
// bundle. Used by both cmd/slk/main.go (production wiring) and tests
// (fake closures).
func NewThreadService(fns ThreadServiceFuncs) ThreadService {
	return threadAdapter{fns: fns}
}

type threadAdapter struct {
	fns ThreadServiceFuncs
}

func (t threadAdapter) Fetch(channelID ids.ChannelID, threadTS ids.ThreadTS) Msg {
	if t.fns.Fetch == nil {
		return nil
	}
	return t.fns.Fetch(channelID, threadTS)
}

func (t threadAdapter) CacheRead(channelID ids.ChannelID, threadTS ids.ThreadTS) []MessageItem {
	if t.fns.CacheRead == nil {
		return nil
	}
	return t.fns.CacheRead(channelID, threadTS)
}

func (t threadAdapter) Mark(channelID ids.ChannelID, threadTS ids.ThreadTS, ts ids.MessageTS) Cmd {
	if t.fns.Mark == nil {
		return nil
	}
	return t.fns.Mark(channelID, threadTS, ts)
}

func (t threadAdapter) SendReply(channelID ids.ChannelID, threadTS ids.ThreadTS, text string, broadcast bool) Msg {
	if t.fns.SendReply == nil {
		return nil
	}
	return t.fns.SendReply(channelID, threadTS, text, broadcast)
}

func (t threadAdapter) ListFetch(teamID ids.TeamID) Msg {
	if t.fns.ListFetch == nil {
		return nil
	}
	return t.fns.ListFetch(teamID)
}

func (t threadAdapter) EnsureSubscriptions(teamID ids.TeamID) {
	if t.fns.EnsureSubscriptions == nil {
		return
	}
	t.fns.EnsureSubscriptions(teamID)
}

func (t threadAdapter) ThreadLastRead(channelID ids.ChannelID, threadTS ids.ThreadTS) string {
	if t.fns.ThreadLastRead == nil {
		return ""
	}
	return t.fns.ThreadLastRead(channelID, threadTS)
}

// MessageService is the App's interface to Slack's per-message
// operations: send, edit, delete, mark-unread, and permalink lookup.
// Implementations are wired by cmd/slk/main.go.
//
// All methods are best-effort and nil-safe at the adapter level: an
// implementation built via NewMessageService with a nil component
// silently no-ops that operation (returning nil Msg or
// ("", nil) for Permalink).
type MessageService interface {
	// Send dispatches chat.postMessage for channelID with text.
	// Returns a Msg (typically MessageSentMsg or
	// MessageSendFailedMsg).
	Send(channelID ids.ChannelID, text string) Msg

	// Edit dispatches chat.update for the message identified by
	// (channelID, ts), replacing its text with newText.
	// Returns a Msg (typically MessageEditedMsg).
	Edit(channelID ids.ChannelID, ts ids.MessageTS, newText string) Msg

	// Delete dispatches chat.delete for the message identified by
	// (channelID, ts). Returns a Msg (typically MessageDeletedMsg).
	Delete(channelID ids.ChannelID, ts ids.MessageTS) Msg

	// MarkUnread dispatches conversations.mark (channel-level) or
	// subscriptions.thread.mark (when threadTS != "") with the
	// rolled-back boundaryTS. unreadCount is forwarded to the result
	// for the sidebar's badge update. Returns a Msg (typically
	// MessageMarkedUnreadMsg).
	MarkUnread(channelID ids.ChannelID, threadTS ids.ThreadTS, boundaryTS ids.MessageTS, unreadCount int) Msg

	// Permalink resolves the Slack permalink URL for the message
	// identified by (channelID, ts). Used by the copy-permalink
	// keybind. Synchronous (HTTP); callers wrap in a goroutine to
	// avoid blocking the Update loop.
	Permalink(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS) (string, error)
}

// MessageServiceFuncs is the closure bundle accepted by
// NewMessageService. Any field may be nil; the resulting service
// no-ops that operation.
type MessageServiceFuncs struct {
	Send       MessageSendFunc
	Edit       MessageEditFunc
	Delete     MessageDeleteFunc
	MarkUnread MarkUnreadFunc
	Permalink  PermalinkFetchFunc
}

// NewMessageService builds a MessageService from a MessageServiceFuncs
// bundle. Used by cmd/slk/main.go (production wiring) and tests.
func NewMessageService(fns MessageServiceFuncs) MessageService {
	return messageAdapter{fns: fns}
}

type messageAdapter struct {
	fns MessageServiceFuncs
}

func (m messageAdapter) Send(channelID ids.ChannelID, text string) Msg {
	if m.fns.Send == nil {
		return nil
	}
	return m.fns.Send(channelID, text)
}

func (m messageAdapter) Edit(channelID ids.ChannelID, ts ids.MessageTS, newText string) Msg {
	if m.fns.Edit == nil {
		return nil
	}
	return m.fns.Edit(channelID, ts, newText)
}

func (m messageAdapter) Delete(channelID ids.ChannelID, ts ids.MessageTS) Msg {
	if m.fns.Delete == nil {
		return nil
	}
	return m.fns.Delete(channelID, ts)
}

func (m messageAdapter) MarkUnread(channelID ids.ChannelID, threadTS ids.ThreadTS, boundaryTS ids.MessageTS, unreadCount int) Msg {
	if m.fns.MarkUnread == nil {
		return nil
	}
	return m.fns.MarkUnread(channelID, threadTS, boundaryTS, unreadCount)
}

func (m messageAdapter) Permalink(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS) (string, error) {
	if m.fns.Permalink == nil {
		return "", nil
	}
	return m.fns.Permalink(ctx, channelID, ts)
}

// ChannelService is the App's interface to the Slack channels API,
// the local SQLite channel cache, and per-channel session bookkeeping
// (visit timestamps, navigation-history lookups, membership fetches).
// Implementations are wired by cmd/slk/main.go.
//
// Largest service in the App. Mixes three concerns that happen to
// share the channel-as-domain-object boundary:
//   - Slack API: Fetch, FetchOlder, MarkRead, Join.
//   - Local cache: ReadCache, SyncedAt.
//   - Session bookkeeping: Lookup, RecordVisit, MembershipFetch.
//
// All methods are best-effort and nil-safe at the adapter level.
type ChannelService interface {
	// Fetch loads the most-recent messages for channelID from Slack.
	// channelName is for log context. Returns a Msg (typically
	// MessagesLoadedMsg).
	Fetch(channelID ids.ChannelID, channelName string) Msg

	// FetchOlder loads messages older than oldestTS for the
	// channel-history backfill triggered by scroll-past-top.
	// Returns a Msg (typically OlderMessagesLoadedMsg).
	FetchOlder(channelID ids.ChannelID, oldestTS ids.MessageTS) Msg

	// FetchAround loads a history window centered on ts for
	// jump-to-message navigation. Returns a Msg (typically
	// MessagesAroundLoadedMsg).
	FetchAround(channelID ids.ChannelID, ts ids.MessageTS) Msg

	// ReadCache returns the local-cache snapshot of channelID's
	// recent messages, or nil if no cache exists. Used by
	// ChannelSelectedMsg's tiered render policy.
	ReadCache(channelID ids.ChannelID) []MessageItem

	// SyncedAt returns the unix-seconds timestamp of the channel's
	// last authoritative cache-from-network sync, or 0 if never
	// synced. Used by ChannelSelectedMsg's tiered render policy to
	// decide between cache-only, cache-and-verify, and spinner-only
	// render.
	SyncedAt(channelID ids.ChannelID) int64

	// MarkRead dispatches conversations.mark + UpdateChannelReadState
	// to bring the channel's last_read_ts up to ts. Used by Tier 1
	// of ChannelSelectedMsg when cache is provably fresh. Returns
	// a Msg (typically ChannelMarkedReadMsg).
	MarkRead(channelID ids.ChannelID, ts ids.MessageTS) Msg

	// Lookup returns metadata (name, channelType) for channelID, or
	// ok=false if the channel is no longer available in the active
	// workspace. Used by navHistoryStore.Walk to skip stale entries.
	Lookup(channelID ids.ChannelID) (name, channelType string, ok bool)

	// Join sends conversations.join for channelID. channelName is
	// for log context. Returns a Msg (typically ChannelJoinedMsg
	// or ChannelJoinFailedMsg).
	Join(channelID ids.ChannelID, channelName string) Msg

	// RecordVisit persists a visit to channelID (SQLite write +
	// WorkspaceContext last-visited map update). Fired once per
	// ChannelSelectedMsg regardless of FromHistory.
	RecordVisit(channelID ids.ChannelID)

	// MembershipFetch asks membership.Manager to ensure-fresh the
	// member set for channelID. Fire-and-forget; results arrive
	// asynchronously via ChannelMembershipMsg.
	MembershipFetch(channelID ids.ChannelID)

	// OpenConversation dispatches conversations.open for userIDs (1
	// recipient = IM, 2-8 recipients = MPIM). Returns a Cmd whose
	// resolved Msg is NewMessageOpenedMsg on success or
	// NewMessageFailedMsg on error; both carry requestID so the
	// reducer can drop late results from cancelled submits.
	OpenConversation(userIDs []string, requestID uint64) Cmd

	// SearchRemote asks the server which channels match query,
	// including ones the user has not joined, and blocks until it
	// answers. Callers run it from a Cmd, debounced — see
	// App.scheduleChannelSearch.
	//
	// It replaced a background conversations.list walk that ran at
	// boot on every workspace, whether or not the finder was ever
	// opened. Returning nil (no client, or a failed request) leaves
	// the finder showing local matches only, which is what it showed
	// before this existed.
	SearchRemote(query string) []ChannelFinderItem
}

// ChannelServiceFuncs is the closure bundle accepted by
// NewChannelService. Any field may be nil; the resulting service
// no-ops that operation.
type ChannelServiceFuncs struct {
	Fetch            ChannelFetchFunc
	FetchOlder       OlderMessagesFetchFunc
	FetchAround      func(channelID ids.ChannelID, ts ids.MessageTS) Msg
	ReadCache        ChannelCacheReadFunc
	SyncedAt         func(channelID ids.ChannelID) int64
	MarkRead         func(channelID ids.ChannelID, ts ids.MessageTS) Msg
	Lookup           ChannelLookupFunc
	Join             JoinChannelFunc
	RecordVisit      ChannelVisitRecorder
	MembershipFetch  func(channelID ids.ChannelID)
	OpenConversation func(userIDs []string, requestID uint64) Cmd
	SearchRemote     func(query string) []ChannelFinderItem
}

// NewChannelService builds a ChannelService from a
// ChannelServiceFuncs bundle.
func NewChannelService(fns ChannelServiceFuncs) ChannelService {
	return channelAdapter{fns: fns}
}

type channelAdapter struct {
	fns ChannelServiceFuncs
}

func (c channelAdapter) Fetch(channelID ids.ChannelID, channelName string) Msg {
	if c.fns.Fetch == nil {
		return nil
	}
	return c.fns.Fetch(channelID, channelName)
}

func (c channelAdapter) FetchOlder(channelID ids.ChannelID, oldestTS ids.MessageTS) Msg {
	if c.fns.FetchOlder == nil {
		return nil
	}
	return c.fns.FetchOlder(channelID, oldestTS)
}

func (c channelAdapter) FetchAround(channelID ids.ChannelID, ts ids.MessageTS) Msg {
	if c.fns.FetchAround == nil {
		return nil
	}
	return c.fns.FetchAround(channelID, ts)
}

func (c channelAdapter) ReadCache(channelID ids.ChannelID) []MessageItem {
	if c.fns.ReadCache == nil {
		return nil
	}
	return c.fns.ReadCache(channelID)
}

func (c channelAdapter) SyncedAt(channelID ids.ChannelID) int64 {
	if c.fns.SyncedAt == nil {
		return 0
	}
	return c.fns.SyncedAt(channelID)
}

func (c channelAdapter) MarkRead(channelID ids.ChannelID, ts ids.MessageTS) Msg {
	if c.fns.MarkRead == nil {
		return nil
	}
	return c.fns.MarkRead(channelID, ts)
}

func (c channelAdapter) Lookup(channelID ids.ChannelID) (name, channelType string, ok bool) {
	if c.fns.Lookup == nil {
		return "", "", false
	}
	return c.fns.Lookup(channelID)
}

func (c channelAdapter) Join(channelID ids.ChannelID, channelName string) Msg {
	if c.fns.Join == nil {
		return nil
	}
	return c.fns.Join(channelID, channelName)
}

func (c channelAdapter) RecordVisit(channelID ids.ChannelID) {
	if c.fns.RecordVisit == nil {
		return
	}
	c.fns.RecordVisit(channelID)
}

func (c channelAdapter) MembershipFetch(channelID ids.ChannelID) {
	if c.fns.MembershipFetch == nil {
		return
	}
	c.fns.MembershipFetch(channelID)
}

func (c channelAdapter) SearchRemote(query string) []ChannelFinderItem {
	if c.fns.SearchRemote == nil {
		return nil
	}
	return c.fns.SearchRemote(query)
}

func (c channelAdapter) OpenConversation(userIDs []string, requestID uint64) Cmd {
	if c.fns.OpenConversation == nil {
		return nil
	}
	return c.fns.OpenConversation(userIDs, requestID)
}

// SearchService runs message searches. SearchChannel queries the local
// FTS cache for one channel; SearchWorkspace queries Slack's
// search.messages for the active workspace.
type SearchService interface {
	// SearchChannel returns a ChannelSearchResultsMsg for query in
	// channelID's cached history.
	SearchChannel(channelID ids.ChannelID, query string) Msg
	// SearchWorkspace returns a WorkspaceSearchResultsMsg for query
	// across the active workspace (server-side).
	SearchWorkspace(query string) Msg
}

// SearchServiceFuncs is the closure bundle accepted by
// NewSearchService. Any field may be nil; that operation no-ops.
type SearchServiceFuncs struct {
	SearchChannel   func(channelID ids.ChannelID, query string) Msg
	SearchWorkspace func(query string) Msg
}

// NewSearchService builds a SearchService from a SearchServiceFuncs
// bundle. Used by cmd/slk/main.go (production wiring) and tests.
func NewSearchService(fns SearchServiceFuncs) SearchService { return searchAdapter{fns: fns} }

type searchAdapter struct{ fns SearchServiceFuncs }

func (s searchAdapter) SearchChannel(channelID ids.ChannelID, query string) Msg {
	if s.fns.SearchChannel == nil {
		return nil
	}
	return s.fns.SearchChannel(channelID, query)
}

func (s searchAdapter) SearchWorkspace(query string) Msg {
	if s.fns.SearchWorkspace == nil {
		return nil
	}
	return s.fns.SearchWorkspace(query)
}

// FileService moves files between the user and Slack.
type FileService interface {
	// Upload sends attachments to channelID (threadTS for a thread
	// reply); caption goes on the last one. The returned Cmd yields
	// UploadResultMsg; progress arrives separately as UploadProgressMsg.
	Upload(channelID, threadTS, caption string, attachments []PendingAttachment) Cmd

	// Download saves the auth-gated file at url locally and returns its
	// path. name is the file's display name.
	Download(ctx context.Context, url, name string) (string, error)
}

// NewFileService builds a FileService from closures.
func NewFileService(
	upload func(channelID, threadTS, caption string, attachments []PendingAttachment) Cmd,
	download func(ctx context.Context, url, name string) (string, error),
) FileService {
	return fileAdapter{upload: upload, download: download}
}

type fileAdapter struct {
	upload   func(channelID, threadTS, caption string, attachments []PendingAttachment) Cmd
	download func(ctx context.Context, url, name string) (string, error)
}

func (f fileAdapter) Upload(channelID, threadTS, caption string, attachments []PendingAttachment) Cmd {
	if f.upload == nil {
		return nil
	}
	return f.upload(channelID, threadTS, caption, attachments)
}

func (f fileAdapter) Download(ctx context.Context, url, name string) (string, error) {
	if f.download == nil {
		return "", nil
	}
	return f.download(ctx, url, name)
}

// ClipboardFormat selects what DesktopService.ReadClipboard returns.
type ClipboardFormat int

const (
	ClipboardText ClipboardFormat = iota
	ClipboardImage
)

// DesktopService is the host OS: launching apps, the system clipboard,
// the filesystem, and the external status command.
type DesktopService interface {
	// Open hands target (a URL or file path) to the OS default handler.
	Open(target string) error

	// ReadClipboard returns the clipboard contents in format f, or nil.
	ReadClipboard(f ClipboardFormat) []byte

	// Stat describes the file at path.
	Stat(path string) (fs.FileInfo, error)

	// SaveThread writes the thread as Markdown to the export directory
	// and returns the file's path.
	SaveThread(parent MessageItem, replies []MessageItem, userNames, channelNames map[string]string, channelName string) (string, error)

	// ReportStatus mirrors the unread state onto the user's configured
	// status command, if any.
	ReportStatus(unread, otherUnread int, workspace, title string)
}

// DesktopServiceFuncs is the closure bundle accepted by
// NewDesktopService. Any field may be nil; that operation no-ops, and a
// nil Stat reports every path as missing.
type DesktopServiceFuncs struct {
	Open          func(target string) error
	ReadClipboard func(f ClipboardFormat) []byte
	Stat          func(path string) (fs.FileInfo, error)
	SaveThread    func(parent MessageItem, replies []MessageItem, userNames, channelNames map[string]string, channelName string) (string, error)
	ReportStatus  func(unread, otherUnread int, workspace, title string)
}

// NewDesktopService builds a DesktopService from a DesktopServiceFuncs bundle.
func NewDesktopService(fns DesktopServiceFuncs) DesktopService {
	return desktopAdapter{fns: fns}
}

type desktopAdapter struct{ fns DesktopServiceFuncs }

func (d desktopAdapter) Open(target string) error {
	if d.fns.Open == nil {
		return nil
	}
	return d.fns.Open(target)
}

func (d desktopAdapter) ReadClipboard(f ClipboardFormat) []byte {
	if d.fns.ReadClipboard == nil {
		return nil
	}
	return d.fns.ReadClipboard(f)
}

func (d desktopAdapter) Stat(path string) (fs.FileInfo, error) {
	// Not (nil, nil): callers read info whenever err is nil.
	if d.fns.Stat == nil {
		return nil, fs.ErrNotExist
	}
	return d.fns.Stat(path)
}

func (d desktopAdapter) SaveThread(parent MessageItem, replies []MessageItem, userNames, channelNames map[string]string, channelName string) (string, error) {
	if d.fns.SaveThread == nil {
		return "", nil
	}
	return d.fns.SaveThread(parent, replies, userNames, channelNames, channelName)
}

func (d desktopAdapter) ReportStatus(unread, otherUnread int, workspace, title string) {
	if d.fns.ReportStatus == nil {
		return
	}
	d.fns.ReportStatus(unread, otherUnread, workspace, title)
}

// PresenceService sets the user's own status and broadcasts typing.
type PresenceService interface {
	// SetStatus applies a presence-menu choice; snoozeMinutes is set
	// for PresenceSnooze.
	SetStatus(action PresenceAction, snoozeMinutes int)

	// SendTyping broadcasts a typing indicator for channelID. Called off
	// the Update goroutine.
	SendTyping(channelID string)
}

// NewPresenceService builds a PresenceService from closures.
func NewPresenceService(
	setStatus func(action PresenceAction, snoozeMinutes int),
	sendTyping func(channelID string),
) PresenceService {
	return presenceAdapter{setStatus: setStatus, sendTyping: sendTyping}
}

type presenceAdapter struct {
	setStatus  func(action PresenceAction, snoozeMinutes int)
	sendTyping func(channelID string)
}

func (p presenceAdapter) SetStatus(action PresenceAction, snoozeMinutes int) {
	if p.setStatus != nil {
		p.setStatus(action, snoozeMinutes)
	}
}

func (p presenceAdapter) SendTyping(channelID string) {
	if p.sendTyping != nil {
		p.sendTyping(channelID)
	}
}

// SettingsService persists the user's display preferences.
type SettingsService interface {
	SaveTheme(name string, scope ThemeScope)
	SaveSidebarWidth(width int)
}

// NewSettingsService builds a SettingsService from closures.
func NewSettingsService(
	saveTheme func(name string, scope ThemeScope),
	saveSidebarWidth func(width int),
) SettingsService {
	return settingsAdapter{saveTheme: saveTheme, saveSidebarWidth: saveSidebarWidth}
}

type settingsAdapter struct {
	saveTheme        func(name string, scope ThemeScope)
	saveSidebarWidth func(width int)
}

func (s settingsAdapter) SaveTheme(name string, scope ThemeScope) {
	if s.saveTheme != nil {
		s.saveTheme(name, scope)
	}
}

func (s settingsAdapter) SaveSidebarWidth(width int) {
	if s.saveSidebarWidth != nil {
		s.saveSidebarWidth(width)
	}
}

// UnreadService reads the unread state the sidebar and workspace rail
// render. Both methods are local reads, called at render time.
type UnreadService interface {
	// ChannelReadStates returns the active workspace's per-channel read
	// state, keyed by channel ID.
	ChannelReadStates() map[string]ReadState

	// UnreadWorkspaces returns the IDs of workspaces with at least one
	// channel their sidebar would show as unread.
	UnreadWorkspaces() []string
}

// NewUnreadService builds an UnreadService from closures.
func NewUnreadService(
	channelReadStates func() map[string]ReadState,
	unreadWorkspaces func() []string,
) UnreadService {
	return unreadAdapter{channelReadStates: channelReadStates, unreadWorkspaces: unreadWorkspaces}
}

type unreadAdapter struct {
	channelReadStates func() map[string]ReadState
	unreadWorkspaces  func() []string
}

func (u unreadAdapter) ChannelReadStates() map[string]ReadState {
	if u.channelReadStates == nil {
		return nil
	}
	return u.channelReadStates()
}

func (u unreadAdapter) UnreadWorkspaces() []string {
	if u.unreadWorkspaces == nil {
		return nil
	}
	return u.unreadWorkspaces()
}

// WorkspaceService switches the active workspace.
type WorkspaceService interface {
	// Switch makes teamID active and returns WorkspaceSwitchedMsg (or
	// nil if the workspace isn't connected).
	Switch(teamID string) Msg
}

// NewWorkspaceService builds a WorkspaceService from a closure.
func NewWorkspaceService(switchTo func(teamID string) Msg) WorkspaceService {
	return workspaceAdapter{switchTo: switchTo}
}

type workspaceAdapter struct{ switchTo func(teamID string) Msg }

func (w workspaceAdapter) Switch(teamID string) Msg {
	if w.switchTo == nil {
		return nil
	}
	return w.switchTo(teamID)
}

// AvatarService renders user avatars for the message panes.
type AvatarService interface {
	// Avatar returns the rendered half-block avatar for userID, or ""
	// while it isn't available yet.
	Avatar(userID string) string
}

// NewAvatarService builds an AvatarService from a closure.
func NewAvatarService(avatar func(userID string) string) AvatarService {
	return avatarAdapter{avatar: avatar}
}

type avatarAdapter struct{ avatar func(userID string) string }

func (a avatarAdapter) Avatar(userID string) string {
	if a.avatar == nil {
		return ""
	}
	return a.avatar(userID)
}

// ImageFetcher downloads and caches remote images for inline rendering
// and keeps pre-encoded renders of them. *image.Fetcher implements it.
type ImageFetcher interface {
	Fetch(ctx context.Context, req imgpkg.FetchRequest) (imgpkg.FetchResult, error)
	Cached(key string, target image.Point) (image.Image, bool)
	Prerendered(key string, cellTarget image.Point, proto imgpkg.Protocol) (imgpkg.Render, bool)
	ConfigurePrerender(proto imgpkg.Protocol)
	ConfigurePrerenderKitty(kr *imgpkg.KittyRenderer)
}

// Closure types accepted by the service constructors.

// ChannelFetchFunc is called when the user selects a channel.
type ChannelFetchFunc func(channelID ids.ChannelID, channelName string) Msg

// ChannelCacheReadFunc is called synchronously when the user selects a
// channel; it returns cached messages from local storage. Returning a
// non-empty slice causes the messagepane to render immediately without
// the loading spinner. Returning nil falls through to the network
// fetcher.
type ChannelCacheReadFunc func(channelID ids.ChannelID) []MessageItem

// OlderMessagesFetchFunc is called when the user scrolls to the top of a channel.
type OlderMessagesFetchFunc func(channelID ids.ChannelID, oldestTS ids.MessageTS) Msg

// MessageSendFunc is called when the user sends a message. Returns a Msg with the result.
type MessageSendFunc func(channelID ids.ChannelID, text string) Msg

// MessageEditFunc performs the chat.update API call. Returns a Msg
// (typically MessageEditedMsg) describing the result.
type MessageEditFunc func(channelID ids.ChannelID, ts ids.MessageTS, newText string) Msg

// MessageDeleteFunc performs the chat.delete API call. Returns a Msg
// (typically MessageDeletedMsg) describing the result.
type MessageDeleteFunc func(channelID ids.ChannelID, ts ids.MessageTS) Msg

// MarkUnreadFunc performs the conversations.mark or
// subscriptions.thread.mark HTTP call (with the rolled-back ts /
// read=0 form), updates SQLite + in-memory caches if the call
// succeeded, and returns a Msg (typically MessageMarkedUnreadMsg)
// describing the result. ThreadTS == "" means channel-level.
type MarkUnreadFunc func(channelID ids.ChannelID, threadTS ids.ThreadTS, boundaryTS ids.MessageTS, unreadCount int) Msg

// ThreadFetchFunc is called when the user opens a thread.
type ThreadFetchFunc func(channelID ids.ChannelID, threadTS ids.ThreadTS) Msg

// ThreadCacheReadFunc is called synchronously when a thread is opened;
// returns cached replies (or nil) so the thread panel can populate
// without waiting for the network. Returning a non-empty slice causes
// the thread panel to render immediately; the subsequent network
// response overwrites with authoritative data.
type ThreadCacheReadFunc func(channelID ids.ChannelID, threadTS ids.ThreadTS) []MessageItem

// ThreadMarkFunc is called to mark a thread as read on Slack's servers
// (subscriptions.thread.mark) and, on success, to advance the local
// thread_subscriptions cursor. Returns a Cmd yielding
// ThreadMarkedLocalMsg, or nil when no workspace is active.
type ThreadMarkFunc func(channelID ids.ChannelID, threadTS ids.ThreadTS, ts ids.MessageTS) Cmd

// ThreadReplySendFunc is called when the user sends a thread reply.
// broadcast is Slack's "Also send to #channel" (reply_broadcast=true).
type ThreadReplySendFunc func(channelID ids.ChannelID, threadTS ids.ThreadTS, text string, broadcast bool) Msg

// ThreadsListFetchFunc loads the involved-threads list for a workspace.
// Returns the resulting Msg (typically ThreadsListLoadedMsg).
type ThreadsListFetchFunc func(teamID ids.TeamID) Msg

type ReactionAddFunc func(channelID ids.ChannelID, messageTS ids.MessageTS, emoji string) error
type ReactionRemoveFunc func(channelID ids.ChannelID, messageTS ids.MessageTS, emoji string) error

// PermalinkFetchFunc is called to fetch the Slack permalink for a message.
// For thread replies, pass the reply's ts; Slack returns a thread-aware URL.
type PermalinkFetchFunc func(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS) (string, error)
type FrecentLoadFunc func(limit int) []EmojiEntry
type FrecentRecordFunc func(emoji string)

// JoinChannelFunc is called to join a public channel by ID. Returns a Msg
// describing the result (typically ChannelJoinedMsg or ChannelJoinFailedMsg).
type JoinChannelFunc func(channelID ids.ChannelID, channelName string) Msg

// ChannelVisitRecorder is invoked from case ChannelSelectedMsg to let
// main.go persist the visit (SQLite write + in-memory map update on
// the WorkspaceContext). Always called regardless of FromHistory.
type ChannelVisitRecorder func(channelID ids.ChannelID)

// ChannelLookupFunc returns metadata for a channel that the App has
// in its navigation history. Used by navigateBack / navigateForward
// to skip stale entries (channels the user has left, archived, or
// kicked from). Returns ok=false when the channel is no longer
// available in the active workspace.
type ChannelLookupFunc func(channelID ids.ChannelID) (name, channelType string, ok bool)
