package ui

import (
	tea "charm.land/bubbletea/v2"
	"golang.design/x/clipboard"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/compose"
	"github.com/gammons/slk/internal/ui/presencemenu"
	"github.com/gammons/slk/internal/ui/themeswitcher"
)

// How seams_test.go installs each collaborator on the App. The scenarios
// there must not change when the wiring does; this file is the only one
// that should.

func wireStatusSetter(a *App, fn func(action presencemenu.Action, mins int)) {
	a.SetStatusSetter(fn)
}

func wireTypingSender(a *App, fn func(channelID string)) {
	a.SetTypingSender(fn)
}

func wireThemeSaver(a *App, fn func(name string, scope themeswitcher.ThemeScope)) {
	a.SetThemeSaver(fn)
}

func wireWidthSaver(a *App, fn func(width int)) {
	a.SetWidthSaver(fn)
}

func wireUploader(a *App, fn func(channelID, threadTS, caption string, atts []compose.PendingAttachment) tea.Cmd) {
	a.SetUploader(fn)
}

func wireWorkspaceSwitcher(a *App, fn func(teamID string) tea.Msg) {
	a.SetWorkspaceSwitcher(fn)
}

func wireUnreadReaders(a *App, channels func() map[string]cache.ReadState, workspaces func() []string) {
	a.SetReadStateReader(channels)
	a.SetWorkspaceUnreadReader(workspaces)
}

func wireStatusReporter(a *App, fn func(unread, otherUnread int, workspace, title string)) {
	a.SetStatusReporter(fn)
}

func wireAvatars(a *App, fn func(userID string) string) {
	a.SetAvatarFunc(fn)
}

// wireClipboard makes the clipboard hold text and/or PNG bytes.
func wireClipboard(a *App, text string, png []byte) {
	a.SetClipboardAvailable(true)
	a.SetClipboardReader(func(f clipboard.Format) []byte {
		if f == clipboard.FmtImage {
			return png
		}
		return []byte(text)
	})
}

// wireFilesystem gives the App the real filesystem for paste-a-path.
func wireFilesystem(a *App) {}

// wireThreadExport gives the App the real thread exporter.
func wireThreadExport(a *App) {}

// wireEditor gives the App the real external-editor plumbing, launching
// argv. nil leaves the editor unconfigured.
func wireEditor(a *App, argv []string) {
	a.SetComposeEditor(argv)
}

func wireChannelFetch(a *App, fn func(channelID ids.ChannelID, channelName string) tea.Msg) {
	a.SetChannelService(core.NewChannelService(core.ChannelServiceFuncs{
		Fetch: func(ch ids.ChannelID, name string) core.Msg { return fn(ch, name) },
	}))
}

func wireOpenConversation(a *App, fn func(userIDs []string, requestID uint64) tea.Cmd) {
	a.SetChannelService(core.NewChannelService(core.ChannelServiceFuncs{
		OpenConversation: func(userIDs []string, requestID uint64) core.Cmd { return coreCmd(fn(userIDs, requestID)) },
	}))
}

func wireMessageSend(a *App, fn func(channelID ids.ChannelID, text string) tea.Msg) {
	a.SetMessageService(core.NewMessageService(core.MessageServiceFuncs{
		Send: func(ch ids.ChannelID, text string) core.Msg { return fn(ch, text) },
	}))
}

func wireThreadMark(a *App, fn func(channelID ids.ChannelID, threadTS ids.ThreadTS, ts ids.MessageTS) tea.Cmd) {
	a.SetThreadService(core.NewThreadService(core.ThreadServiceFuncs{
		Mark: func(ch ids.ChannelID, thread ids.ThreadTS, ts ids.MessageTS) core.Cmd {
			return coreCmd(fn(ch, thread, ts))
		},
	}))
}

// coreCmd is teaCmd in reverse, for scenarios written against tea.Cmd.
func coreCmd(c tea.Cmd) core.Cmd {
	if c == nil {
		return nil
	}
	return func() core.Msg { return c() }
}
